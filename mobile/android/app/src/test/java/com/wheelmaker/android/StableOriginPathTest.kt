package com.wheelmaker.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class StableOriginPathTest {
    @Test
    fun mapsRootToIndexHtml() {
        assertEquals("index.html", assetNameForStablePath("/"))
    }

    @Test
    fun stripsLeadingSlashForAssets() {
        assertEquals("bundle.abc.js", assetNameForStablePath("/bundle.abc.js"))
    }

    @Test
    fun removesParentTraversalSegments() {
        assertEquals("secret.js", assetNameForStablePath("/../secret.js"))
    }

    @Test
    fun treatsWorkspaceRoutesAsIndexFallbackCandidates() {
        assertTrue(isWorkspaceRoute("settings/update"))
        assertFalse(isWorkspaceRoute("bundle.abc.js"))
    }
}
