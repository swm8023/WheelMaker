package com.wheelmaker.android

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayInputStream
import java.util.zip.GZIPInputStream

class DoubaoSpeechProtocolTest {
    @Test
    fun buildsFullClientRequestFrame() {
        val frame = buildDoubaoFullClientRequest(SpeechAudioConfig())

        assertArrayEquals(byteArrayOf(0x11, 0x10, 0x11, 0x00), frame.copyOfRange(0, 4))
        val size = readUint32(frame, 4)
        assertEquals(frame.size - 8, size)
        val payload = gunzip(frame.copyOfRange(8, frame.size)).decodeToString()

        assertTrue(payload.contains("\"uid\":\"wheelmaker\""))
        assertTrue(payload.contains("\"format\":\"pcm\""))
        assertTrue(payload.contains("\"codec\":\"raw\""))
        assertTrue(payload.contains("\"rate\":16000"))
        assertTrue(payload.contains("\"bits\":16"))
        assertTrue(payload.contains("\"channel\":1"))
        assertTrue(payload.contains("\"model_name\":\"bigmodel\""))
        assertTrue(payload.contains("\"enable_itn\":true"))
        assertTrue(payload.contains("\"enable_punc\":true"))
        assertTrue(payload.contains("\"enable_ddc\":true"))
        assertTrue(payload.contains("\"show_utterances\":false"))
        assertTrue(payload.contains("\"result_type\":\"full\""))
        assertTrue(payload.contains("\"enable_nonstream\":true"))
    }

    @Test
    fun buildsAudioAndFinalFrames() {
        val frame = buildDoubaoAudioRequest(byteArrayOf(1, 2, 3), final = false)

        assertArrayEquals(byteArrayOf(0x11, 0x20, 0x01, 0x00), frame.copyOfRange(0, 4))
        assertEquals(frame.size - 8, readUint32(frame, 4))
        assertArrayEquals(byteArrayOf(1, 2, 3), gunzip(frame.copyOfRange(8, frame.size)))

        val finalFrame = buildDoubaoAudioRequest(byteArrayOf(), final = true)

        assertArrayEquals(byteArrayOf(0x11, 0x22, 0x01, 0x00), finalFrame.copyOfRange(0, 4))
        assertEquals(finalFrame.size - 8, readUint32(finalFrame, 4))
    }

    @Test
    fun parsesTranscriptAndFinalFrames() {
        val interimBody = gzip("""{"result":{"text":"你好世界","utterances":[{"text":"你好"},{"text":"世界"}]}}""".encodeToByteArray())
        val interimFrame = byteArrayOf(0x11, 0x91.toByte(), 0x11, 0x00) +
            int32Bytes(7) +
            uint32Bytes(interimBody.size) +
            interimBody

        val interim = parseDoubaoSpeechFrame(interimFrame)

        assertEquals("你好世界", interim.text)
        assertTrue(interim.hasTranscript)
        assertFalse(interim.final)

        val finalBody = gzip("""{"result":{"text":"最终"}}""".encodeToByteArray())
        val finalFrame = byteArrayOf(0x11, 0x93.toByte(), 0x11, 0x00) +
            int32Bytes(-3) +
            uint32Bytes(finalBody.size) +
            finalBody

        val final = parseDoubaoSpeechFrame(finalFrame)

        assertEquals("最终", final.text)
        assertTrue(final.final)
    }

    @Test
    fun combinesFullResultListAndReportsServerErrors() {
        val body = gzip("""{"result":[{"text":"前面"},{"text":"后面"}]}""".encodeToByteArray())
        val frame = byteArrayOf(0x11, 0x91.toByte(), 0x11, 0x00) +
            int32Bytes(8) +
            uint32Bytes(body.size) +
            body

        val parsed = parseDoubaoSpeechFrame(frame)

        assertEquals("前面后面", parsed.text)

        val errorPayload = """{"message":"bad auth"}""".encodeToByteArray()
        val errorFrame = byteArrayOf(0x11, 0xf0.toByte(), 0x10, 0x00) +
            uint32Bytes(45000001) +
            uint32Bytes(errorPayload.size) +
            errorPayload

        try {
            parseDoubaoSpeechFrame(errorFrame)
            throw AssertionError("expected server error")
        } catch (error: DoubaoSpeechException) {
            assertEquals(45000001, error.code)
            assertEquals("bad auth", error.message)
        }
    }

    private fun readUint32(bytes: ByteArray, offset: Int): Int {
        return ((bytes[offset].toInt() and 0xff) shl 24) or
            ((bytes[offset + 1].toInt() and 0xff) shl 16) or
            ((bytes[offset + 2].toInt() and 0xff) shl 8) or
            (bytes[offset + 3].toInt() and 0xff)
    }

    private fun gunzip(bytes: ByteArray): ByteArray {
        return GZIPInputStream(ByteArrayInputStream(bytes)).use { it.readBytes() }
    }
}
