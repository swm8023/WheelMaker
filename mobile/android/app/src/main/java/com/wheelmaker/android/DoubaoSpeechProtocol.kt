package com.wheelmaker.android

import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.zip.GZIPInputStream
import java.util.zip.GZIPOutputStream

private const val VOLC_PROTOCOL_VERSION = 0x1
private const val VOLC_HEADER_SIZE = 0x1
private const val VOLC_MESSAGE_FULL_CLIENT_REQUEST = 0x1
private const val VOLC_MESSAGE_AUDIO_ONLY_REQUEST = 0x2
private const val VOLC_MESSAGE_FULL_SERVER_RESPONSE = 0x9
private const val VOLC_MESSAGE_SERVER_ACK = 0xb
private const val VOLC_MESSAGE_SERVER_ERROR = 0xf
private const val VOLC_FLAG_NO_SEQUENCE = 0x0
private const val VOLC_FLAG_POSITIVE_SEQ = 0x1
private const val VOLC_FLAG_LAST_NO_SEQ = 0x2
private const val VOLC_FLAG_NEGATIVE_WITH_SEQ = 0x3
private const val VOLC_SERIALIZATION_NONE = 0x0
private const val VOLC_SERIALIZATION_JSON = 0x1
private const val VOLC_COMPRESSION_NONE = 0x0
private const val VOLC_COMPRESSION_GZIP = 0x1

const val DOUBAO_SPEECH_ENDPOINT = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
const val DOUBAO_SPEECH_RESOURCE_ID = "volc.seedasr.sauc.duration"
const val DOUBAO_SPEECH_MODEL_NAME = "bigmodel"

data class SpeechAudioConfig(
    val format: String = "pcm",
    val codec: String = "raw",
    val rate: Int = 16000,
    val bits: Int = 16,
    val channel: Int = 1
)

data class DoubaoSpeechFrame(
    val text: String = "",
    val final: Boolean = false,
    val hasTranscript: Boolean = false
)

class DoubaoSpeechException(
    val code: Int,
    override val message: String
) : Exception("doubao speech error code $code: $message")

fun buildDoubaoFullClientRequest(audio: SpeechAudioConfig): ByteArray {
    val payload = JSONObject()
        .put("user", JSONObject().put("uid", "wheelmaker"))
        .put(
            "audio",
            JSONObject()
                .put("format", audio.format)
                .put("codec", audio.codec)
                .put("rate", audio.rate)
                .put("bits", audio.bits)
                .put("channel", audio.channel)
        )
        .put(
            "request",
            JSONObject()
                .put("model_name", DOUBAO_SPEECH_MODEL_NAME)
                .put("enable_itn", true)
                .put("enable_punc", true)
                .put("enable_ddc", true)
                .put("show_utterances", false)
                .put("result_type", "full")
                .put("enable_nonstream", true)
        )
        .toString()
        .encodeToByteArray()
    val compressed = gzip(payload)
    return doubaoHeader(
        VOLC_MESSAGE_FULL_CLIENT_REQUEST,
        VOLC_FLAG_NO_SEQUENCE,
        VOLC_SERIALIZATION_JSON,
        VOLC_COMPRESSION_GZIP
    ) + uint32Bytes(compressed.size) + compressed
}

fun buildDoubaoAudioRequest(pcm: ByteArray, final: Boolean): ByteArray {
    val compressed = gzip(pcm)
    val flags = if (final) VOLC_FLAG_LAST_NO_SEQ else VOLC_FLAG_NO_SEQUENCE
    return doubaoHeader(
        VOLC_MESSAGE_AUDIO_ONLY_REQUEST,
        flags,
        VOLC_SERIALIZATION_NONE,
        VOLC_COMPRESSION_GZIP
    ) + uint32Bytes(compressed.size) + compressed
}

fun parseDoubaoSpeechFrame(frame: ByteArray): DoubaoSpeechFrame {
    if (frame.size < 8) {
        throw IllegalArgumentException("doubao frame too short")
    }
    val headerSize = (frame[0].toInt() and 0x0f) * 4
    if (headerSize < 4 || frame.size < headerSize + 4) {
        throw IllegalArgumentException("doubao invalid header size")
    }
    val messageType = (frame[1].toInt() and 0xff) ushr 4
    val flags = frame[1].toInt() and 0x0f
    val serialization = (frame[2].toInt() and 0xff) ushr 4
    val compression = frame[2].toInt() and 0x0f
    var payload = frame.copyOfRange(headerSize, frame.size)

    return when (messageType) {
        VOLC_MESSAGE_FULL_SERVER_RESPONSE, VOLC_MESSAGE_SERVER_ACK -> {
            var sequence = 0
            if (flags == VOLC_FLAG_POSITIVE_SEQ || flags == VOLC_FLAG_NEGATIVE_WITH_SEQ) {
                if (payload.size < 8) {
                    throw IllegalArgumentException("doubao response missing sequence or payload size")
                }
                sequence = ByteBuffer.wrap(payload, 0, 4).order(ByteOrder.BIG_ENDIAN).int
                payload = payload.copyOfRange(4, payload.size)
            }
            val body = readDoubaoPayload(payload, compression)
            val isFinal = flags == VOLC_FLAG_NEGATIVE_WITH_SEQ || sequence < 0
            if (serialization != VOLC_SERIALIZATION_JSON || body.isEmpty()) {
                return DoubaoSpeechFrame(final = isFinal)
            }
            val text = extractDoubaoText(body)
            DoubaoSpeechFrame(
                text = text,
                final = isFinal,
                hasTranscript = text.isNotEmpty()
            )
        }

        VOLC_MESSAGE_SERVER_ERROR -> {
            if (payload.size < 8) {
                throw IllegalArgumentException("doubao error frame too short")
            }
            val code = readUint32(payload, 0)
            val body = readDoubaoPayload(payload.copyOfRange(4, payload.size), compression)
            throw DoubaoSpeechException(code, extractDoubaoErrorMessage(body))
        }

        else -> DoubaoSpeechFrame()
    }
}

private fun doubaoHeader(
    messageType: Int,
    flags: Int,
    serialization: Int,
    compression: Int
): ByteArray {
    return byteArrayOf(
        ((VOLC_PROTOCOL_VERSION shl 4) or VOLC_HEADER_SIZE).toByte(),
        ((messageType shl 4) or flags).toByte(),
        ((serialization shl 4) or compression).toByte(),
        0x00
    )
}

private fun readDoubaoPayload(payload: ByteArray, compression: Int): ByteArray {
    if (payload.size < 4) {
        throw IllegalArgumentException("doubao payload missing size")
    }
    val size = readUint32(payload, 0)
    if (size < 0 || size > payload.size - 4) {
        throw IllegalArgumentException("doubao payload size exceeds frame")
    }
    val body = payload.copyOfRange(4, 4 + size)
    return if (compression == VOLC_COMPRESSION_GZIP) gunzip(body) else body
}

private fun extractDoubaoText(body: ByteArray): String {
    return try {
        val root = JSONObject(body.decodeToString())
        extractDoubaoResultText(root.opt("result"))
    } catch (_: Exception) {
        ""
    }
}

private fun extractDoubaoResultText(value: Any?): String {
    return when (value) {
        null, JSONObject.NULL -> ""
        is String -> value
        is JSONArray -> buildString {
            for (index in 0 until value.length()) {
                append(extractDoubaoResultText(value.opt(index)))
            }
        }
        is JSONObject -> {
            val text = value.optString("text", "")
            if (text.isNotEmpty()) {
                text
            } else {
                val utterances = value.optJSONArray("utterances")
                if (utterances == null) {
                    ""
                } else {
                    buildString {
                        for (index in 0 until utterances.length()) {
                            append(extractDoubaoResultText(utterances.opt(index)))
                        }
                    }
                }
            }
        }
        else -> ""
    }
}

private fun extractDoubaoErrorMessage(body: ByteArray): String {
    return try {
        val root = JSONObject(body.decodeToString())
        listOf("message", "msg", "error")
            .firstNotNullOfOrNull { key -> root.optString(key, "").takeIf { it.isNotEmpty() } }
            ?: body.decodeToString()
    } catch (_: Exception) {
        body.decodeToString()
    }
}

fun gzip(raw: ByteArray): ByteArray {
    val out = ByteArrayOutputStream()
    GZIPOutputStream(out).use { it.write(raw) }
    return out.toByteArray()
}

private fun gunzip(raw: ByteArray): ByteArray {
    return GZIPInputStream(ByteArrayInputStream(raw)).use { it.readBytes() }
}

fun int32Bytes(value: Int): ByteArray {
    return ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN).putInt(value).array()
}

fun uint32Bytes(value: Int): ByteArray {
    return int32Bytes(value)
}

private fun readUint32(bytes: ByteArray, offset: Int): Int {
    return ByteBuffer.wrap(bytes, offset, 4).order(ByteOrder.BIG_ENDIAN).int
}
