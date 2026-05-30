package com.wheelmaker.android

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.util.UUID
import java.util.concurrent.TimeUnit

interface DoubaoSpeechClientListener {
    fun onConnected()
    fun onTranscript(text: String, final: Boolean)
    fun onError(code: String, message: String, retryable: Boolean)
    fun onClosed(reason: String)
}

class DoubaoSpeechClient(
    private val okHttpClient: OkHttpClient,
    private val apiKey: String,
    private val listener: DoubaoSpeechClientListener,
    private val requestId: String = UUID.randomUUID().toString()
) {
    @Volatile
    private var webSocket: WebSocket? = null

    @Volatile
    private var closedByClient = false

    @Volatile
    private var finalFrameReceived = false

    fun connect() {
        val request = Request.Builder()
            .url(DOUBAO_SPEECH_ENDPOINT)
            .addHeader("X-Api-Key", apiKey)
            .addHeader("X-Api-Resource-Id", DOUBAO_SPEECH_RESOURCE_ID)
            .addHeader("X-Api-Request-Id", requestId)
            .addHeader("X-Api-Sequence", "-1")
            .build()
        webSocket = okHttpClient.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                val accepted = webSocket.send(buildDoubaoFullClientRequest(SpeechAudioConfig()).toByteString())
                if (!accepted) {
                    listener.onError("SEND_FAILED", "Failed to send Doubao speech start frame.", true)
                    return
                }
                listener.onConnected()
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                handleFrame(webSocket, bytes.toByteArray())
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!closedByClient && !finalFrameReceived) {
                    listener.onError("UNAVAILABLE", t.message ?: "Doubao speech connection failed.", true)
                }
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                listener.onClosed(reason.ifBlank { "closed" })
            }
        })
    }

    fun sendAudio(pcm: ByteArray) {
        val socket = webSocket ?: throw IllegalStateException("Doubao speech socket is not connected.")
        if (!socket.send(buildDoubaoAudioRequest(pcm, final = false).toByteString())) {
            throw IllegalStateException("Failed to send Doubao speech audio frame.")
        }
    }

    fun finish() {
        val socket = webSocket ?: throw IllegalStateException("Doubao speech socket is not connected.")
        if (!socket.send(buildDoubaoAudioRequest(ByteArray(0), final = true).toByteString())) {
            throw IllegalStateException("Failed to send Doubao speech final frame.")
        }
    }

    fun cancel(reason: String) {
        closedByClient = true
        webSocket?.close(1000, reason)
        webSocket = null
    }

    private fun handleFrame(socket: WebSocket, frame: ByteArray) {
        try {
            val parsed = parseDoubaoSpeechFrame(frame)
            if (parsed.hasTranscript || parsed.final) {
                listener.onTranscript(parsed.text, parsed.final)
            }
            if (parsed.final) {
                finalFrameReceived = true
                socket.close(1000, "final")
            }
        } catch (error: DoubaoSpeechException) {
            listener.onError("DOUBAO_${error.code}", error.message, false)
            socket.close(1000, "server_error")
        } catch (error: Exception) {
            listener.onError("PROTOCOL_ERROR", error.message ?: "Invalid Doubao speech frame.", false)
            socket.close(1000, "protocol_error")
        }
    }

    companion object {
        fun defaultOkHttpClient(): OkHttpClient = OkHttpClient.Builder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .pingInterval(30, TimeUnit.SECONDS)
            .build()
    }
}
