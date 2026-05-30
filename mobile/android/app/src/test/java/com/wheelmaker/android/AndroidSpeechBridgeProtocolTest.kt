package com.wheelmaker.android

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AndroidSpeechBridgeProtocolTest {
    @Test
    fun parsesStartRequestAndRequiresApiKey() {
        val request = parseAndroidSpeechStartRequest(
            """
            {
              "provider": "volcengine",
              "model": "doubao-streaming-asr-2.0",
              "apiKey": "secret-key",
              "audio": {"format": "pcm", "codec": "raw", "rate": 16000, "bits": 16, "channel": 1}
            }
            """.trimIndent()
        )

        assertEquals("volcengine", request.provider)
        assertEquals("doubao-streaming-asr-2.0", request.model)
        assertEquals("secret-key", request.apiKey)
        assertEquals(16000, request.audio.rate)

        try {
            parseAndroidSpeechStartRequest("""{"provider":"volcengine","apiKey":""}""")
            throw AssertionError("expected missing api key error")
        } catch (error: IllegalArgumentException) {
            assertEquals("Volcengine API Key is required.", error.message)
        }
    }

    @Test
    fun serializesCommandResponses() {
        val accepted = JSONObject(androidSpeechCommandAccepted("speech-1"))

        assertTrue(accepted.getBoolean("accepted"))
        assertEquals("speech-1", accepted.getString("streamId"))

        val rejected = JSONObject(androidSpeechCommandRejected("BUSY", "Native speech is already active."))

        assertFalse(rejected.getBoolean("accepted"))
        assertEquals("BUSY", rejected.getString("code"))
        assertEquals("Native speech is already active.", rejected.getString("message"))
    }

    @Test
    fun serializesEventsForWebCallback() {
        val transcript = JSONObject(AndroidSpeechEvent.Transcript(
            streamId = "speech-1",
            text = "hello",
            final = false
        ).toJson())
        val error = JSONObject(AndroidSpeechEvent.Error(
            streamId = "speech-1",
            code = "NETWORK",
            message = "network down",
            retryable = false
        ).toJson())

        assertEquals("transcript", transcript.getString("type"))
        assertEquals("speech-1", transcript.getString("streamId"))
        assertEquals("hello", transcript.getString("text"))
        assertFalse(transcript.getBoolean("final"))
        assertEquals("error", error.getString("type"))
        assertEquals("NETWORK", error.getString("code"))
        assertFalse(error.getBoolean("retryable"))
    }
}
