package com.wheelmaker.android

import android.content.Context
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.ByteArrayInputStream
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URI

class StableOriginWebViewClient(
    private val context: Context,
    private val webSourceRuntime: WebSourceRuntime
) : WebViewClient() {
    override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
        val uri = request.url ?: return null
        if (uri.scheme != "https" || uri.host != "appassets.androidplatform.net") {
            return null
        }
        val assetName = assetNameForStablePath(uri.encodedPath ?: "/")
        if (assetName == "ws") {
            return notFoundResponse()
        }
        remoteResponse(assetName)?.let { return it }
        embeddedResponse(assetName)?.let { return it }
        if (isWorkspaceRoute(assetName)) {
            return embeddedResponse("index.html") ?: notFoundResponse()
        }
        return notFoundResponse()
    }

    private fun remoteResponse(assetName: String): WebResourceResponse? {
        val remoteBase = webSourceRuntime.remoteBaseForRequest()
        if (remoteBase.isBlank()) return null
        var connection: HttpURLConnection? = null
        return try {
            val suffix = if (assetName == "index.html") "" else assetName
            val url = URI(remoteBase).resolve(suffix).toURL()
            connection = url.openConnection() as HttpURLConnection
            connection.connectTimeout = 5000
            connection.readTimeout = 15000
            connection.instanceFollowRedirects = true
            val status = connection.responseCode
            if (status !in 200..299) {
                connection.disconnect()
                return null
            }
            val headers = responseHeadersForAsset(assetName).toMutableMap()
            connection.getHeaderField("Cache-Control")?.let { headers["Cache-Control"] = it }
            WebResourceResponse(
                mimeTypeFromHeader(connection.contentType, assetName),
                null,
                status,
                connection.responseMessage ?: "OK",
                headers,
                connection.inputStream
            )
        } catch (_: Exception) {
            connection?.disconnect()
            null
        }
    }

    private fun embeddedResponse(assetName: String): WebResourceResponse? {
        val stream = openEmbeddedAsset(assetName) ?: return null
        return WebResourceResponse(
            contentTypeForAsset(assetName),
            null,
            200,
            "OK",
            responseHeadersForAsset(assetName),
            stream
        )
    }

    private fun openEmbeddedAsset(assetName: String): InputStream? {
        return try {
            context.assets.open(assetName)
        } catch (_: Exception) {
            null
        }
    }

    private fun notFoundResponse(): WebResourceResponse {
        return WebResourceResponse(
            "text/plain",
            "utf-8",
            404,
            "Not Found",
            mapOf("Cache-Control" to "no-store"),
            ByteArrayInputStream("not found".toByteArray())
        )
    }
}

fun assetNameForStablePath(path: String): String {
    val clean = path
        .replace('\\', '/')
        .split('/')
        .filter { it.isNotBlank() && it != "." && it != ".." }
        .joinToString("/")
    return clean.ifBlank { "index.html" }
}

fun isWorkspaceRoute(assetName: String): Boolean {
    val baseName = assetName.substringAfterLast('/')
    return baseName.isNotBlank() && !baseName.contains('.')
}

fun contentTypeForAsset(assetName: String): String {
    return when {
        assetName.endsWith(".html") -> "text/html"
        assetName.endsWith(".js") -> "application/javascript"
        assetName.endsWith(".css") -> "text/css"
        assetName.endsWith(".json") -> "application/json"
        assetName.endsWith(".webmanifest") -> "application/manifest+json"
        assetName.endsWith(".svg") -> "image/svg+xml"
        assetName.endsWith(".png") -> "image/png"
        assetName.endsWith(".jpg") || assetName.endsWith(".jpeg") -> "image/jpeg"
        assetName.endsWith(".ico") -> "image/x-icon"
        assetName.endsWith(".woff2") -> "font/woff2"
        assetName.endsWith(".woff") -> "font/woff"
        else -> "application/octet-stream"
    }
}

fun responseHeadersForAsset(assetName: String): Map<String, String> {
    val baseName = assetName.substringAfterLast('/')
    val cacheControl = when {
        baseName == "index.html" || baseName == "service-worker.js" -> "no-cache, must-revalidate"
        baseName == "runtime-config.js" || baseName == "web-build.json" -> "no-store"
        baseName.startsWith("bundle.") && (baseName.endsWith(".js") || baseName.endsWith(".css")) ->
            "public, max-age=31536000, immutable"
        baseName.endsWith(".svg") || baseName.endsWith(".woff") || baseName.endsWith(".woff2") ->
            "public, max-age=31536000, immutable"
        else -> "no-cache"
    }
    return mapOf("Cache-Control" to cacheControl)
}

private fun mimeTypeFromHeader(contentType: String?, assetName: String): String {
    return contentType
        ?.substringBefore(';')
        ?.trim()
        ?.takeIf { it.isNotBlank() }
        ?: contentTypeForAsset(assetName)
}
