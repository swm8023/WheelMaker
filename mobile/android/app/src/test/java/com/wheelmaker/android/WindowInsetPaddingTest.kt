package com.wheelmaker.android

import org.junit.Assert.assertEquals
import org.junit.Test

class WindowInsetPaddingTest {
    @Test
    fun mergesSystemBarsAndDisplayCutoutByTakingLargestEdge() {
        val safeArea = mergedSafeAreaInsets(
            systemBars = EdgeInsets(left = 0, top = 44, right = 0, bottom = 24),
            displayCutout = EdgeInsets(left = 8, top = 88, right = 12, bottom = 0)
        )

        assertEquals(8, safeArea.left)
        assertEquals(88, safeArea.top)
        assertEquals(12, safeArea.right)
        assertEquals(24, safeArea.bottom)
    }

    @Test
    fun keepsZeroInsetsWhenNoSystemOverlapExists() {
        val safeArea = mergedSafeAreaInsets(
            systemBars = EdgeInsets(left = 0, top = 0, right = 0, bottom = 0),
            displayCutout = EdgeInsets(left = 0, top = 0, right = 0, bottom = 0)
        )

        assertEquals(EdgeInsets(left = 0, top = 0, right = 0, bottom = 0), safeArea)
    }
}
