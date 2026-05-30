package com.wheelmaker.android

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.webkit.WebView
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import okhttp3.OkHttpClient
import java.util.UUID

class AndroidSpeechRuntime(
    private val activity: Activity,
    private val webView: WebView,
    private val permissionRequestCode: Int,
    private val okHttpClient: OkHttpClient = DoubaoSpeechClient.defaultOkHttpClient()
) {
    private val lock = Any()
    private var activeSession: ActiveSpeechSession? = null

    fun start(rawJson: String): String {
        val request = try {
            parseAndroidSpeechStartRequest(rawJson)
        } catch (error: Exception) {
            return androidSpeechCommandRejected("INVALID_REQUEST", error.message ?: "Invalid Android speech request.")
        }
        val session = ActiveSpeechSession(
            streamId = "android-speech-${UUID.randomUUID()}",
            request = request
        )
        synchronized(lock) {
            if (activeSession != null) {
                return androidSpeechCommandRejected("BUSY", "Native speech is already active.")
            }
            activeSession = session
        }
        if (ContextCompat.checkSelfPermission(activity, Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            sendEvent(AndroidSpeechEvent.Status(session.streamId, "permission"))
            ActivityCompat.requestPermissions(
                activity,
                arrayOf(Manifest.permission.RECORD_AUDIO),
                permissionRequestCode
            )
            return androidSpeechCommandAccepted(session.streamId)
        }
        startSession(session)
        return androidSpeechCommandAccepted(session.streamId)
    }

    fun finish(streamId: String): String {
        val session = activeSessionFor(streamId)
            ?: return androidSpeechCommandRejected("NOT_FOUND", "Native speech stream is not active.")
        session.finishRequested = true
        sendEvent(AndroidSpeechEvent.Status(streamId, "finishing"))
        val recorder = session.recorder
        if (recorder == null) {
            sendFinalFrame(session)
        } else {
            recorder.stop(flush = true)
        }
        return androidSpeechCommandAccepted(streamId)
    }

    fun cancel(streamId: String, reason: String): String {
        val session = takeActiveSession(streamId)
            ?: return androidSpeechCommandRejected("NOT_FOUND", "Native speech stream is not active.")
        session.recorder?.stop(flush = false)
        session.client?.cancel(reason)
        sendEvent(AndroidSpeechEvent.Closed(streamId, reason))
        return androidSpeechCommandAccepted(streamId)
    }

    fun stopForAppBackground() {
        val session = takeAnyActiveSession() ?: return
        session.recorder?.stop(flush = false)
        session.client?.cancel("app_background")
        sendEvent(AndroidSpeechEvent.Closed(session.streamId, "app_background"))
    }

    fun onRequestPermissionsResult(requestCode: Int, grantResults: IntArray): Boolean {
        if (requestCode != permissionRequestCode) {
            return false
        }
        val session = synchronized(lock) { activeSession } ?: return true
        if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
            startSession(session)
            return true
        }
        clearActiveSession(session.streamId)
        sendEvent(AndroidSpeechEvent.Error(
            streamId = session.streamId,
            code = "PERMISSION_DENIED",
            message = "Microphone permission was denied.",
            retryable = false
        ))
        return true
    }

    private fun startSession(session: ActiveSpeechSession) {
        if (!isActive(session.streamId)) {
            return
        }
        sendEvent(AndroidSpeechEvent.Status(session.streamId, "connecting"))
        val client = DoubaoSpeechClient(
            okHttpClient = okHttpClient,
            apiKey = session.request.apiKey,
            listener = object : DoubaoSpeechClientListener {
                override fun onConnected() {
                    if (!isActive(session.streamId)) {
                        clientFor(session)?.cancel("stale")
                        return
                    }
                    if (session.finishRequested) {
                        sendFinalFrame(session)
                        return
                    }
                    startRecorder(session)
                }

                override fun onTranscript(text: String, final: Boolean) {
                    if (!isActive(session.streamId)) {
                        return
                    }
                    sendEvent(AndroidSpeechEvent.Transcript(session.streamId, text, final))
                    if (final) {
                        takeActiveSession(session.streamId)
                    }
                }

                override fun onError(code: String, message: String, retryable: Boolean) {
                    handleSessionError(session, code, message, retryable)
                }

                override fun onClosed(reason: String) {
                    if (isActive(session.streamId) && reason != "final") {
                        handleSessionError(session, "UNAVAILABLE", "Doubao speech connection closed.", true)
                    }
                }
            }
        )
        session.client = client
        try {
            client.connect()
        } catch (error: Exception) {
            handleSessionError(session, "UNAVAILABLE", error.message ?: "Failed to connect Doubao speech.", true)
        }
    }

    private fun startRecorder(session: ActiveSpeechSession) {
        val recorder = PcmAudioRecorder(
            context = activity,
            listener = object : PcmAudioRecorderListener {
                override fun onAudioChunk(pcm: ByteArray) {
                    if (!isActive(session.streamId) || session.finalSent) {
                        return
                    }
                    try {
                        session.client?.sendAudio(pcm)
                    } catch (error: Exception) {
                        handleSessionError(session, "SEND_FAILED", error.message ?: "Failed to send microphone audio.", true)
                    }
                }

                override fun onLevel(level: Double) {
                    if (isActive(session.streamId)) {
                        sendEvent(AndroidSpeechEvent.Level(session.streamId, level))
                    }
                }

                override fun onError(message: String) {
                    handleSessionError(session, "MICROPHONE", message, false)
                }

                override fun onStopped() {
                    if (!isActive(session.streamId)) {
                        return
                    }
                    if (session.finishRequested) {
                        sendFinalFrame(session)
                    } else {
                        handleSessionError(session, "MICROPHONE", "Microphone capture stopped.", false)
                    }
                }
            }
        )
        session.recorder = recorder
        try {
            recorder.start()
            sendEvent(AndroidSpeechEvent.Status(session.streamId, "recording"))
        } catch (error: Exception) {
            handleSessionError(session, "MICROPHONE", error.message ?: "Failed to start microphone capture.", false)
        }
    }

    private fun sendFinalFrame(session: ActiveSpeechSession) {
        if (!isActive(session.streamId) || session.finalSent) {
            return
        }
        session.finalSent = true
        try {
            session.client?.finish()
                ?: throw IllegalStateException("Doubao speech socket is not connected.")
            sendEvent(AndroidSpeechEvent.Status(session.streamId, "recognizing"))
        } catch (error: Exception) {
            handleSessionError(session, "FINISH_FAILED", error.message ?: "Failed to finish speech recognition.", true)
        }
    }

    private fun handleSessionError(
        session: ActiveSpeechSession,
        code: String,
        message: String,
        retryable: Boolean
    ) {
        val removed = takeActiveSession(session.streamId) ?: return
        removed.recorder?.stop(flush = false)
        removed.client?.cancel("error")
        sendEvent(AndroidSpeechEvent.Error(
            streamId = session.streamId,
            code = code,
            message = message,
            retryable = retryable
        ))
    }

    private fun sendEvent(event: AndroidSpeechEvent) {
        val script = "window.__wheelmakerAndroidSpeechEvent && window.__wheelmakerAndroidSpeechEvent(${event.toJson()});"
        activity.runOnUiThread {
            webView.evaluateJavascript(script, null)
        }
    }

    private fun isActive(streamId: String): Boolean = synchronized(lock) {
        activeSession?.streamId == streamId
    }

    private fun activeSessionFor(streamId: String): ActiveSpeechSession? = synchronized(lock) {
        activeSession?.takeIf { it.streamId == streamId }
    }

    private fun clientFor(session: ActiveSpeechSession): DoubaoSpeechClient? = synchronized(lock) {
        activeSession?.takeIf { it.streamId == session.streamId }?.client
    }

    private fun takeActiveSession(streamId: String): ActiveSpeechSession? = synchronized(lock) {
        val current = activeSession ?: return@synchronized null
        if (current.streamId != streamId) {
            return@synchronized null
        }
        activeSession = null
        current
    }

    private fun takeAnyActiveSession(): ActiveSpeechSession? = synchronized(lock) {
        val current = activeSession
        activeSession = null
        current
    }

    private fun clearActiveSession(streamId: String) {
        synchronized(lock) {
            if (activeSession?.streamId == streamId) {
                activeSession = null
            }
        }
    }

    private class ActiveSpeechSession(
        val streamId: String,
        val request: AndroidSpeechStartRequest
    ) {
        @Volatile
        var client: DoubaoSpeechClient? = null

        @Volatile
        var recorder: PcmAudioRecorder? = null

        @Volatile
        var finishRequested: Boolean = false

        @Volatile
        var finalSent: Boolean = false
    }
}
