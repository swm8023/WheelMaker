package com.wheelmaker.android

import android.content.Context
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import java.util.Locale

interface WebSourceStore {
    fun load(): WebSourceConfig
    fun save(config: WebSourceConfig)
}

class SharedPreferencesWebSourceStore(context: Context) : WebSourceStore {
    private val prefs = context.getSharedPreferences("wheelmaker_web_source", Context.MODE_PRIVATE)

    override fun load(): WebSourceConfig {
        return WebSourceConfig(
            webSourcePreference = prefs.getString("webSourcePreference", WEB_SOURCE_AUTO) ?: WEB_SOURCE_AUTO,
            remoteWebUrl = prefs.getString("remoteWebUrl", "") ?: "",
            remoteWebRegistryOrigin = prefs.getString("remoteWebRegistryOrigin", "") ?: ""
        ).sanitize()
    }

    override fun save(config: WebSourceConfig) {
        val clean = config.sanitize()
        prefs.edit()
            .putString("webSourcePreference", clean.webSourcePreference)
            .putString("remoteWebUrl", clean.remoteWebUrl)
            .putString("remoteWebRegistryOrigin", clean.remoteWebRegistryOrigin)
            .apply()
    }
}

class InMemoryWebSourceStore(private var config: WebSourceConfig) : WebSourceStore {
    override fun load(): WebSourceConfig = config

    override fun save(config: WebSourceConfig) {
        this.config = config
    }
}

class WebSourceRuntime(private val store: WebSourceStore) {
    private var config: WebSourceConfig = store.load().sanitize()
    private var actualSource: String = actualSourceForConfig(config)

    @Synchronized
    fun state(): WebSourceState = stateForConfig(config, actualSource)

    @Synchronized
    fun setPreference(preference: String): WebSourceState {
        val cleanPreference = if (preference == WEB_SOURCE_EMBEDDED) WEB_SOURCE_EMBEDDED else WEB_SOURCE_AUTO
        config = config.copy(webSourcePreference = cleanPreference).sanitize()
        actualSource = actualSourceForConfig(config)
        store.save(config)
        return state()
    }

    @Synchronized
    fun setRemoteCandidate(candidate: RemoteWebCandidate): WebSourceState {
        val normalized = normalizeRemoteWebCandidate(candidate)
        config = if (normalized.accepted) {
            config.copy(
                remoteWebUrl = normalized.remoteWebUrl,
                remoteWebRegistryOrigin = normalized.registryOrigin
            )
        } else {
            config.copy(remoteWebUrl = "", remoteWebRegistryOrigin = "")
        }.sanitize()
        actualSource = actualSourceForConfig(config)
        store.save(config)
        return state()
    }

    @Synchronized
    fun refreshActualSource(): WebSourceState {
        actualSource = if (
            config.webSourcePreference == WEB_SOURCE_AUTO &&
            config.remoteWebUrl.isNotBlank() &&
            probeRemote(config.remoteWebUrl)
        ) {
            WEB_ACTUAL_REMOTE
        } else {
            WEB_ACTUAL_EMBEDDED
        }
        return state()
    }

    @Synchronized
    fun remoteBaseForRequest(): String {
        return if (
            actualSource == WEB_ACTUAL_REMOTE &&
            config.webSourcePreference == WEB_SOURCE_AUTO
        ) {
            config.remoteWebUrl
        } else {
            ""
        }
    }

    private fun probeRemote(remoteWebUrl: String): Boolean {
        var connection: HttpURLConnection? = null
        return try {
            connection = URL(remoteWebUrl).openConnection() as HttpURLConnection
            connection.connectTimeout = 3000
            connection.readTimeout = 3000
            connection.requestMethod = "GET"
            connection.instanceFollowRedirects = true
            val ok = connection.responseCode in 200..299
            connection.inputStream.close()
            ok
        } catch (_: Exception) {
            false
        } finally {
            connection?.disconnect()
        }
    }
}

fun WebSourceConfig.sanitize(): WebSourceConfig {
    val cleanPreference = if (webSourcePreference == WEB_SOURCE_EMBEDDED) WEB_SOURCE_EMBEDDED else WEB_SOURCE_AUTO
    val cleanRemote = normalizeRemoteWebUrl(remoteWebUrl)
    return copy(
        webSourcePreference = cleanPreference,
        remoteWebUrl = cleanRemote,
        remoteWebRegistryOrigin = if (cleanRemote.isBlank()) "" else remoteWebRegistryOrigin
    )
}

fun actualSourceForConfig(config: WebSourceConfig): String {
    return if (
        config.webSourcePreference == WEB_SOURCE_AUTO &&
        config.remoteWebUrl.isNotBlank()
    ) {
        WEB_ACTUAL_REMOTE
    } else {
        WEB_ACTUAL_EMBEDDED
    }
}

fun stateForConfig(config: WebSourceConfig, actualSource: String): WebSourceState {
    val remoteHost = parseUri(config.remoteWebUrl)?.host ?: ""
    val cleanActual = if (actualSource == WEB_ACTUAL_REMOTE && config.remoteWebUrl.isNotBlank()) {
        WEB_ACTUAL_REMOTE
    } else {
        WEB_ACTUAL_EMBEDDED
    }
    val displaySource = if (cleanActual == WEB_ACTUAL_REMOTE && remoteHost.isNotBlank()) {
        remoteHost
    } else {
        "Embedded"
    }
    return WebSourceState(
        preference = config.webSourcePreference,
        actualSource = cleanActual,
        displayTitle = "WheelMaker - $displaySource",
        displaySource = displaySource,
        remoteUrl = config.remoteWebUrl,
        remoteHost = remoteHost
    )
}

fun normalizeRemoteWebCandidate(candidate: RemoteWebCandidate): NormalizedRemoteCandidate {
    if (candidate.source != "registry") return NormalizedRemoteCandidate(false)
    val registryUri = parseUri(candidate.registryAddress.trim()) ?: return NormalizedRemoteCandidate(false)
    val remoteUri = parseUri(candidate.remoteWebUrl.trim()) ?: return NormalizedRemoteCandidate(false)
    if (!isAllowedRegistryScheme(registryUri.scheme) || !isAllowedRemoteScheme(remoteUri.scheme)) {
        return NormalizedRemoteCandidate(false)
    }
    if (registryUri.host.isNullOrBlank() || remoteUri.host.isNullOrBlank()) {
        return NormalizedRemoteCandidate(false)
    }
    if (registryUri.userInfo != null || remoteUri.userInfo != null) {
        return NormalizedRemoteCandidate(false)
    }
    if (isLoopbackHost(registryUri.host ?: "") || isLoopbackHost(remoteUri.host ?: "")) {
        return NormalizedRemoteCandidate(false)
    }
    if (!registryUri.rawAuthority.equals(remoteUri.rawAuthority, ignoreCase = true)) {
        return NormalizedRemoteCandidate(false)
    }
    if (!remoteUri.rawQuery.isNullOrBlank() || !remoteUri.rawFragment.isNullOrBlank()) {
        return NormalizedRemoteCandidate(false)
    }
    val path = remoteUri.rawPath ?: ""
    if (path.isNotBlank() && path != "/") {
        return NormalizedRemoteCandidate(false)
    }
    return NormalizedRemoteCandidate(
        accepted = true,
        remoteWebUrl = "${remoteUri.scheme.lowercase(Locale.US)}://${remoteUri.rawAuthority}/",
        registryOrigin = registryOriginForUri(registryUri)
    )
}

fun normalizeRemoteWebUrl(raw: String): String {
    if (raw.isBlank()) return ""
    val uri = parseUri(raw.trim()) ?: return ""
    if (!isAllowedRemoteScheme(uri.scheme) || uri.host.isNullOrBlank()) return ""
    if (uri.userInfo != null || isLoopbackHost(uri.host ?: "")) return ""
    if (!uri.rawQuery.isNullOrBlank() || !uri.rawFragment.isNullOrBlank()) return ""
    val path = uri.rawPath ?: ""
    if (path.isNotBlank() && path != "/") return ""
    return "${uri.scheme.lowercase(Locale.US)}://${uri.rawAuthority}/"
}

fun webSourceStateToJson(state: WebSourceState): String {
    return JSONObject()
        .put("preference", state.preference)
        .put("actualSource", state.actualSource)
        .put("displayTitle", state.displayTitle)
        .put("displaySource", state.displaySource)
        .put("remoteUrl", state.remoteUrl)
        .put("remoteHost", state.remoteHost)
        .toString()
}

private fun parseUri(raw: String): URI? {
    return try {
        URI(raw)
    } catch (_: Exception) {
        null
    }
}

private fun registryOriginForUri(uri: URI): String {
    val scheme = when (uri.scheme?.lowercase(Locale.US)) {
        "http" -> "ws"
        "https" -> "wss"
        else -> uri.scheme?.lowercase(Locale.US) ?: "ws"
    }
    return "$scheme://${uri.rawAuthority}"
}

private fun isAllowedRegistryScheme(scheme: String?): Boolean {
    val clean = scheme?.lowercase(Locale.US)
    return clean == "ws" || clean == "wss" || clean == "http" || clean == "https"
}

private fun isAllowedRemoteScheme(scheme: String?): Boolean {
    val clean = scheme?.lowercase(Locale.US)
    return clean == "http" || clean == "https"
}

private fun isLoopbackHost(host: String): Boolean {
    val value = host.lowercase(Locale.US).trim('[', ']')
    return value == "localhost" || value == "127.0.0.1" || value == "::1"
}
