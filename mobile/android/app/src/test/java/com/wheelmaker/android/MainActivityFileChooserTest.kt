package com.wheelmaker.android

import android.os.Build
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
        assertTrue(mainActivity.contains("createAndroidDocumentFileChooserIntent("))
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

    @Test
    fun imageOnlyChooserUsesAndroidPhotoPickerWhenAvailable() {
        val mainActivity = source

        assertTrue(mainActivity.contains("import android.provider.MediaStore"))
        assertTrue(mainActivity.contains("createAndroidPhotoPickerIntent("))
        assertTrue(mainActivity.contains("shouldUseAndroidPhotoPicker(acceptTypes, Build.VERSION.SDK_INT)"))
        assertTrue(mainActivity.contains("Build.VERSION_CODES.TIRAMISU"))
        assertTrue(mainActivity.contains("MediaStore.ACTION_PICK_IMAGES"))
        assertTrue(mainActivity.contains("MediaStore.EXTRA_PICK_IMAGES_MAX"))
        assertTrue(mainActivity.contains("MediaStore.getPickImagesMaxLimit()"))
    }

    @Test
    fun chooserPolicyUsesPhotoPickerOnlyForVisualMediaOnAndroid13Plus() {
        assertTrue(shouldUseAndroidPhotoPicker(listOf("image/*"), Build.VERSION_CODES.TIRAMISU))
        assertTrue(shouldUseAndroidPhotoPicker(listOf("image/png", "image/jpeg"), Build.VERSION_CODES.TIRAMISU))
        assertTrue(shouldUseAndroidPhotoPicker(listOf("video/*"), Build.VERSION_CODES.TIRAMISU))

        assertFalse(shouldUseAndroidPhotoPicker(emptyList(), Build.VERSION_CODES.TIRAMISU))
        assertFalse(shouldUseAndroidPhotoPicker(listOf("application/pdf"), Build.VERSION_CODES.TIRAMISU))
        assertFalse(shouldUseAndroidPhotoPicker(listOf("image/*", "application/pdf"), Build.VERSION_CODES.TIRAMISU))
        assertFalse(shouldUseAndroidPhotoPicker(listOf("image/*"), Build.VERSION_CODES.S))
    }

    @Test
    fun photoPickerTypeNarrowsImageOrVideoFamilies() {
        assertEquals("image/*", photoPickerTypeForAcceptTypes(listOf("image/png", "image/jpeg")))
        assertEquals("video/*", photoPickerTypeForAcceptTypes(listOf("video/mp4")))
        assertEquals(null, photoPickerTypeForAcceptTypes(listOf("image/*", "video/*")))
    }

    @Test
    fun acceptTypeNormalizerSplitsCommaSeparatedValues() {
        assertEquals(
            listOf("image/png", "image/jpeg"),
            normalizeAndroidFileChooserAcceptTypes(arrayOf(" image/png, image/jpeg ", "", ".png"))
        )
    }
}
