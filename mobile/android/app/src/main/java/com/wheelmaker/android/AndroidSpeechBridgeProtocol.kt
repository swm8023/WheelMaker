package com.wheelmaker.android

import org.json.JSONObject

data class AndroidSpeechStartRequest(
    val provider: String,
    val model: String,
    val apiKey: String,
    val audio: SpeechAudioConfig
)

sealed class AndroidSpeechEvent {
    abstract val streamId: String
    abstract fun toJson(): String

    data class Status(
        override val streamId: String,
        val status: String
    ) : AndroidSpeechEvent() {
        override fun toJson(): String = JSONObject()
            .put("type", "status")
            .put("streamId", streamId)
            .put("status", status)
            .toString()
    }

    data class Level(
        override val streamId: String,
        val level: Double
    ) : AndroidSpeechEvent() {
        override fun toJson(): String = JSONObject()
            .put("type", "level")
            .put("streamId", streamId)
            .put("level", level)
            .toString()
    }

    data class Transcript(
        override val streamId: String,
        val text: String,
        val final: Boolean
    ) : AndroidSpeechEvent() {
        override fun toJson(): String = JSONObject()
            .put("type", "transcript")
            .put("streamId", streamId)
            .put("text", text)
            .put("final", final)
            .toString()
    }

    data class Error(
        override val streamId: String = "",
        val code: String,
        val message: String,
        val retryable: Boolean
    ) : AndroidSpeechEvent() {
        override fun toJson(): String {
            val payload = JSONObject()
                .put("type", "error")
                .put("code", code)
                .put("message", message)
                .put("retryable", retryable)
            if (streamId.isNotBlank()) {
                payload.put("streamId", streamId)
            }
            return payload.toString()
        }
    }

    data class Closed(
        override val streamId: String,
        val reason: String
    ) : AndroidSpeechEvent() {
        override fun toJson(): String = JSONObject()
            .put("type", "closed")
            .put("streamId", streamId)
            .put("reason", reason)
            .toString()
    }
}

fun parseAndroidSpeechStartRequest(rawJson: String): AndroidSpeechStartRequest {
    val root = JSONObject(rawJson)
    val provider = root.optString("provider", "").trim()
    val model = root.optString("model", "").trim()
    val apiKey = root.optString("apiKey", "").trim()
    if (apiKey.isBlank()) {
        throw IllegalArgumentException("Volcengine API Key is required.")
    }
    if (provider != "volcengine") {
        throw IllegalArgumentException("Only Volcengine speech is supported on Android.")
    }
    if (model != "doubao-streaming-asr-2.0") {
        throw IllegalArgumentException("Unsupported Android speech model.")
    }
    val audio = root.optJSONObject("audio")
    return AndroidSpeechStartRequest(
        provider = provider,
        model = model,
        apiKey = apiKey,
        audio = SpeechAudioConfig(
            format = audio?.optString("format", "pcm") ?: "pcm",
            codec = audio?.optString("codec", "raw") ?: "raw",
            rate = audio?.optInt("rate", 16000) ?: 16000,
            bits = audio?.optInt("bits", 16) ?: 16,
            channel = audio?.optInt("channel", 1) ?: 1
        )
    )
}

fun androidSpeechCommandAccepted(streamId: String): String = JSONObject()
    .put("accepted", true)
    .put("streamId", streamId)
    .toString()

fun androidSpeechCommandRejected(code: String, message: String): String = JSONObject()
    .put("accepted", false)
    .put("code", code)
    .put("message", message)
    .toString()
