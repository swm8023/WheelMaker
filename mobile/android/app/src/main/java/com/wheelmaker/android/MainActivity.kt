package com.wheelmaker.android

import android.Manifest
import android.app.Activity
import android.app.DownloadManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Bundle
import android.os.Environment
import android.webkit.PermissionRequest
import android.webkit.URLUtil
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebSettings
import android.webkit.WebView
import android.widget.Toast
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat

class MainActivity : Activity() {
    private lateinit var webView: WebView
    private lateinit var webSourceRuntime: WebSourceRuntime
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null
    private var pendingAudioPermissionRequest: PermissionRequest? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webSourceRuntime = WebSourceRuntime(SharedPreferencesWebSourceStore(this))
        webSourceRuntime.refreshActualSource()

        webView = WebView(this)
        configureWebView(webView)
        setContentView(webView)
        webView.loadUrl(ANDROID_APP_ORIGIN)
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
            return
        }
        super.onBackPressed()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: android.content.Intent?) {
        if (requestCode == FILE_CHOOSER_REQUEST_CODE) {
            val result = WebChromeClient.FileChooserParams.parseResult(resultCode, data)
            fileChooserCallback?.onReceiveValue(result)
            fileChooserCallback = null
            return
        }
        super.onActivityResult(requestCode, resultCode, data)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != AUDIO_PERMISSION_REQUEST_CODE) return
        val request = pendingAudioPermissionRequest ?: return
        pendingAudioPermissionRequest = null
        if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
            request.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
        } else {
            request.deny()
        }
    }

    private fun configureWebView(target: WebView) {
        target.settings.javaScriptEnabled = true
        target.settings.domStorageEnabled = true
        target.settings.databaseEnabled = true
        target.settings.cacheMode = WebSettings.LOAD_DEFAULT
        target.settings.mediaPlaybackRequiresUserGesture = false
        target.settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        target.webViewClient = StableOriginWebViewClient(this, webSourceRuntime)
        target.webChromeClient = object : WebChromeClient() {
            override fun onPermissionRequest(request: PermissionRequest) {
                if (request.resources.contains(PermissionRequest.RESOURCE_AUDIO_CAPTURE)) {
                    handleAudioPermissionRequest(request)
                    return
                }
                request.deny()
            }

            override fun onShowFileChooser(
                webView: WebView,
                filePathCallback: ValueCallback<Array<Uri>>,
                fileChooserParams: FileChooserParams
            ): Boolean {
                fileChooserCallback?.onReceiveValue(null)
                fileChooserCallback = filePathCallback
                return try {
                    startActivityForResult(fileChooserParams.createIntent(), FILE_CHOOSER_REQUEST_CODE)
                    true
                } catch (_: ActivityNotFoundException) {
                    fileChooserCallback = null
                    filePathCallback.onReceiveValue(null)
                    false
                }
            }
        }
        target.setDownloadListener { url, userAgent, contentDisposition, mimeType, _ ->
            enqueueDownload(url, userAgent, contentDisposition, mimeType)
        }
        target.addJavascriptInterface(WheelMakerBridge(webSourceRuntime), "WheelMakerAndroidNative")
    }

    private fun handleAudioPermissionRequest(request: PermissionRequest) {
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
            request.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
            return
        }
        pendingAudioPermissionRequest?.deny()
        pendingAudioPermissionRequest = request
        ActivityCompat.requestPermissions(this, arrayOf(Manifest.permission.RECORD_AUDIO), AUDIO_PERMISSION_REQUEST_CODE)
    }

    private fun enqueueDownload(url: String, userAgent: String, contentDisposition: String, mimeType: String) {
        if (!url.startsWith("http://") && !url.startsWith("https://")) {
            Toast.makeText(this, "Download is not available for this file.", Toast.LENGTH_SHORT).show()
            return
        }
        val fileName = URLUtil.guessFileName(url, contentDisposition, mimeType)
        val request = DownloadManager.Request(Uri.parse(url))
            .setMimeType(mimeType)
            .addRequestHeader("User-Agent", userAgent)
            .setTitle(fileName)
            .setDescription("WheelMaker")
            .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
            .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, fileName)
        val manager = getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager
        manager.enqueue(request)
        Toast.makeText(this, "Download started.", Toast.LENGTH_SHORT).show()
    }

    companion object {
        private const val AUDIO_PERMISSION_REQUEST_CODE = 1001
        private const val FILE_CHOOSER_REQUEST_CODE = 1002
    }
}
