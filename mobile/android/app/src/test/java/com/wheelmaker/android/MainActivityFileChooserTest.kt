package com.wheelmaker.android

import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files
import java.nio.file.Paths

class MainActivityFileChooserTest {
    private val source: String
        get() = String(Files.readAllBytes(
            Paths.get("src/main/java/com/wheelmaker/android/MainActivity.kt")
        ))

    @Test
    fun fileChooserUsesOpenDocumentIntentWithReadableContentUris() {
        val mainActivity = source

        assertTrue(mainActivity.contains("target.settings.allowContentAccess = true"))
        assertTrue(mainActivity.contains("target.settings.allowFileAccess = true"))
        assertTrue(mainActivity.contains("createAndroidFileChooserIntent(fileChooserParams)"))
        assertTrue(mainActivity.contains("Intent.ACTION_OPEN_DOCUMENT"))
        assertTrue(mainActivity.contains("Intent.CATEGORY_OPENABLE"))
        assertTrue(mainActivity.contains("Intent.FLAG_GRANT_READ_URI_PERMISSION"))
        assertTrue(mainActivity.contains("Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION"))
        assertTrue(mainActivity.contains("Intent.EXTRA_ALLOW_MULTIPLE"))
    }

    @Test
    fun fileChooserResultDeliversSelectedClipDataUris() {
        val mainActivity = source

        assertTrue(mainActivity.contains("deliverFileChooserResult(resultCode, data)"))
        assertTrue(mainActivity.contains("data?.clipData"))
        assertTrue(mainActivity.contains("data?.data"))
        assertTrue(mainActivity.contains("takePersistableUriPermission"))
    }
}
