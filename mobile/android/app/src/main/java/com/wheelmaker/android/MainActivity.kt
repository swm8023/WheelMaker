package com.wheelmaker.android

import android.app.Activity
import android.os.Bundle
import android.webkit.WebView

class MainActivity : Activity() {
    private lateinit var webView: WebView
    private lateinit var webSourceRuntime: WebSourceRuntime

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webSourceRuntime = WebSourceRuntime(SharedPreferencesWebSourceStore(this))
        webSourceRuntime.refreshActualSource()
        webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.webViewClient = StableOriginWebViewClient(this, webSourceRuntime)
        setContentView(webView)
        webView.loadUrl(ANDROID_APP_ORIGIN)
    }
}
