package com.wheelmaker.android

import kotlin.math.floor
import kotlin.math.max
import kotlin.math.min

fun pcm16LittleEndianToShorts(bytes: ByteArray, byteCount: Int): ShortArray {
    val safeCount = min(max(0, byteCount), bytes.size)
    val sampleCount = safeCount / 2
    val out = ShortArray(sampleCount)
    for (index in 0 until sampleCount) {
        val lo = bytes[index * 2].toInt() and 0xff
        val hi = bytes[index * 2 + 1].toInt()
        out[index] = ((hi shl 8) or lo).toShort()
    }
    return out
}

fun shortsToPcm16LittleEndian(samples: ShortArray): ByteArray {
    val out = ByteArray(samples.size * 2)
    for (index in samples.indices) {
        val value = samples[index].toInt()
        out[index * 2] = (value and 0xff).toByte()
        out[index * 2 + 1] = ((value ushr 8) and 0xff).toByte()
    }
    return out
}

fun resamplePcm16Mono(
    input: ShortArray,
    sourceRate: Int,
    targetRate: Int = 16000
): ShortArray {
    if (input.isEmpty() || sourceRate <= 0 || targetRate <= 0) {
        return ShortArray(0)
    }
    if (sourceRate == targetRate) {
        return input.copyOf()
    }
    val outputLength = max(1, floor(input.size.toDouble() * targetRate.toDouble() / sourceRate.toDouble()).toInt())
    val output = ShortArray(outputLength)
    val ratio = sourceRate.toDouble() / targetRate.toDouble()
    for (index in 0 until outputLength) {
        val position = index.toDouble() * ratio
        val left = floor(position).toInt()
        val right = min(input.size - 1, left + 1)
        val fraction = position - left.toDouble()
        val value = input[left].toDouble() + (input[right].toDouble() - input[left].toDouble()) * fraction
        output[index] = value.toInt().coerceIn(Short.MIN_VALUE.toInt(), Short.MAX_VALUE.toInt()).toShort()
    }
    return output
}
