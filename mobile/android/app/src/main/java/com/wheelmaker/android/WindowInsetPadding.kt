package com.wheelmaker.android

import androidx.core.graphics.Insets

data class EdgeInsets(
    val left: Int,
    val top: Int,
    val right: Int,
    val bottom: Int
)

fun mergedSafeAreaInsets(systemBars: EdgeInsets, displayCutout: EdgeInsets): EdgeInsets = EdgeInsets(
    left = maxOf(systemBars.left, displayCutout.left),
    top = maxOf(systemBars.top, displayCutout.top),
    right = maxOf(systemBars.right, displayCutout.right),
    bottom = maxOf(systemBars.bottom, displayCutout.bottom)
)

fun Insets.toEdgeInsets(): EdgeInsets = EdgeInsets(
    left = left,
    top = top,
    right = right,
    bottom = bottom
)
