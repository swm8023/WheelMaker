package com.wheelmaker.android

import android.webkit.JavascriptInterface
import org.json.JSONObject

class WheelMakerBridge(
    private val webSourceRuntime: WebSourceRuntime,
    private val androidSpeechRuntime: AndroidSpeechRuntime
) {
    @JavascriptInterface
    fun getWebSourceState(): String = webSourceStateToJson(webSourceRuntime.state())

    @JavascriptInterface
    fun setWebSourcePreference(preference: String): String {
        return webSourceStateToJson(webSourceRuntime.setPreference(preference))
    }

    @JavascriptInterface
    fun setRemoteWebCandidate(rawJson: String): String {
        val input = JSONObject(rawJson)
        val candidate = RemoteWebCandidate(
            source = input.optString("source"),
            registryAddress = input.optString("registryAddress"),
            remoteWebUrl = input.optString("remoteWebUrl")
        )
        return webSourceStateToJson(webSourceRuntime.setRemoteCandidate(candidate))
    }

    @JavascriptInterface
    fun startSpeech(rawJson: String): String = androidSpeechRuntime.start(rawJson)

    @JavascriptInterface
    fun finishSpeech(streamId: String): String = androidSpeechRuntime.finish(streamId)

    @JavascriptInterface
    fun cancelSpeech(streamId: String, reason: String): String = androidSpeechRuntime.cancel(streamId, reason)
}
