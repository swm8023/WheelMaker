package com.wheelmaker.android

import android.Manifest
import android.app.Activity
import android.app.DownloadManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.view.ViewGroup
import android.view.WindowManager
import android.webkit.PermissionRequest
import android.webkit.URLUtil
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebSettings
import android.webkit.WebView
import android.widget.FrameLayout
import android.widget.Toast
import androidx.core.graphics.Insets
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import java.util.Locale

class MainActivity : Activity() {
    private lateinit var rootView: FrameLayout
    private lateinit var webView: WebView
    private lateinit var webSourceRuntime: WebSourceRuntime
    private lateinit var androidSpeechRuntime: AndroidSpeechRuntime
    private var fileChooserCallback: ValueCallback<Array<Uri>>? = null
    private var pendingAudioPermissionRequest: PermissionRequest? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        webSourceRuntime = WebSourceRuntime(SharedPreferencesWebSourceStore(this))
        webSourceRuntime.refreshActualSource()

        rootView = FrameLayout(this)
        rootView.setBackgroundColor(APP_BACKGROUND_COLOR)
        webView = WebView(this)
        androidSpeechRuntime = AndroidSpeechRuntime(this, webView, NATIVE_SPEECH_PERMISSION_REQUEST_CODE)
        webView.setBackgroundColor(APP_BACKGROUND_COLOR)
        configureWindowInsets(rootView)
        configureWebView(webView)
        rootView.addView(
            webView,
            FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
        )
        setContentView(rootView)
        webView.loadUrl(ANDROID_APP_ORIGIN)
    }

    override fun onPause() {
        if (::androidSpeechRuntime.isInitialized) {
            androidSpeechRuntime.stopForAppBackground()
        }
        super.onPause()
    }

    override fun onDestroy() {
        if (::androidSpeechRuntime.isInitialized) {
            androidSpeechRuntime.stopForAppBackground()
        }
        super.onDestroy()
    }

    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
            return
        }
        super.onBackPressed()
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        if (requestCode == FILE_CHOOSER_REQUEST_CODE) {
            deliverFileChooserResult(resultCode, data)
            return
        }
        super.onActivityResult(requestCode, resultCode, data)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (::androidSpeechRuntime.isInitialized && androidSpeechRuntime.onRequestPermissionsResult(requestCode, grantResults)) {
            return
        }
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
        target.settings.allowContentAccess = true
        target.settings.allowFileAccess = true
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
                    startActivityForResult(createAndroidFileChooserIntent(fileChooserParams), FILE_CHOOSER_REQUEST_CODE)
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
        target.addJavascriptInterface(WheelMakerBridge(webSourceRuntime, androidSpeechRuntime), "WheelMakerAndroidNative")
    }

    private fun createAndroidFileChooserIntent(fileChooserParams: WebChromeClient.FileChooserParams): Intent {
        val acceptTypes = normalizeFileChooserAcceptTypes(fileChooserParams.acceptTypes)
        return Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            type = acceptTypes.singleOrNull() ?: "*/*"
            if (acceptTypes.size > 1) {
                putExtra(Intent.EXTRA_MIME_TYPES, acceptTypes.toTypedArray())
            }
            putExtra(
                Intent.EXTRA_ALLOW_MULTIPLE,
                fileChooserParams.mode == WebChromeClient.FileChooserParams.MODE_OPEN_MULTIPLE
            )
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            addFlags(Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION)
        }
    }

    private fun normalizeFileChooserAcceptTypes(acceptTypes: Array<String>?): List<String> {
        return acceptTypes
            .orEmpty()
            .flatMap { it.split(',') }
            .map { it.trim().lowercase(Locale.US) }
            .filter { it.isNotBlank() && (it == "*/*" || it.contains('/')) }
            .distinct()
    }

    private fun deliverFileChooserResult(resultCode: Int, data: Intent?) {
        val result = collectFileChooserResultUris(resultCode, data)
        fileChooserCallback?.onReceiveValue(result)
        fileChooserCallback = null
    }

    private fun collectFileChooserResultUris(resultCode: Int, data: Intent?): Array<Uri>? {
        if (resultCode != RESULT_OK) {
            return null
        }
        val uris = linkedSetOf<Uri>()
        val clipData = data?.clipData
        if (clipData != null) {
            for (index in 0 until clipData.itemCount) {
                clipData.getItemAt(index).uri?.let { uris.add(it) }
            }
        }
        data?.data?.let { uris.add(it) }
        WebChromeClient.FileChooserParams.parseResult(resultCode, data)?.forEach { uris.add(it) }
        if (uris.isEmpty()) {
            return null
        }
        persistFileChooserReadPermissions(uris, data)
        return uris.toTypedArray()
    }

    private fun persistFileChooserReadPermissions(uris: Set<Uri>, data: Intent?) {
        val flags = data?.flags ?: 0
        if ((flags and Intent.FLAG_GRANT_READ_URI_PERMISSION) == 0) {
            return
        }
        if ((flags and Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION) == 0) {
            return
        }
        for (uri in uris) {
            try {
                contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION)
            } catch (_: SecurityException) {
                // Some providers grant temporary read access only; WebView can still consume those URIs immediately.
            }
        }
    }

    private fun configureWindowInsets(target: FrameLayout) {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        window.statusBarColor = Color.TRANSPARENT
        window.navigationBarColor = Color.TRANSPARENT
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val attributes = window.attributes
            attributes.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
            window.attributes = attributes
        }
        WindowInsetsControllerCompat(window, target).isAppearanceLightStatusBars = false
        WindowInsetsControllerCompat(window, target).isAppearanceLightNavigationBars = false
        ViewCompat.setOnApplyWindowInsetsListener(target) { view, insets ->
            val safeArea = mergedSafeAreaInsets(
                systemBars = insets.getInsets(WindowInsetsCompat.Type.systemBars()).toEdgeInsets(),
                displayCutout = insets.getInsets(WindowInsetsCompat.Type.displayCutout()).toEdgeInsets()
            )
            view.setPadding(safeArea.left, safeArea.top, safeArea.right, safeArea.bottom)
            WindowInsetsCompat.Builder(insets)
                .setInsets(WindowInsetsCompat.Type.systemBars(), Insets.NONE)
                .setInsets(WindowInsetsCompat.Type.displayCutout(), Insets.NONE)
                .build()
        }
        ViewCompat.requestApplyInsets(target)
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
        private const val NATIVE_SPEECH_PERMISSION_REQUEST_CODE = 1003
        private val APP_BACKGROUND_COLOR = Color.rgb(11, 18, 32)
    }
}
