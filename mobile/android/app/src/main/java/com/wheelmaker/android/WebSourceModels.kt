package com.wheelmaker.android

data class WebSourceConfig(
    val webSourcePreference: String = WEB_SOURCE_AUTO,
    val remoteWebUrl: String = "",
    val remoteWebRegistryOrigin: String = ""
)

data class WebSourceState(
    val preference: String,
    val actualSource: String,
    val displayTitle: String,
    val displaySource: String,
    val remoteUrl: String,
    val remoteHost: String
)

data class RemoteWebCandidate(
    val source: String,
    val registryAddress: String,
    val remoteWebUrl: String
)

data class NormalizedRemoteCandidate(
    val accepted: Boolean,
    val remoteWebUrl: String = "",
    val registryOrigin: String = ""
)

const val WEB_SOURCE_AUTO = "auto"
const val WEB_SOURCE_EMBEDDED = "embedded"
const val WEB_ACTUAL_EMBEDDED = "embedded"
const val WEB_ACTUAL_REMOTE = "remote"
const val ANDROID_APP_ORIGIN = "https://appassets.androidplatform.net/"
