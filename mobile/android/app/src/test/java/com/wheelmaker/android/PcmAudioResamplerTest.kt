package com.wheelmaker.android

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class PcmAudioResamplerTest {
    @Test
    fun leavesSixteenKilohertzPcmUnchanged() {
        val input = shortArrayOf(0, 1000, -1000, Short.MAX_VALUE)

        val output = resamplePcm16Mono(input, sourceRate = 16000, targetRate = 16000)

        assertArrayEquals(input, output)
    }

    @Test
    fun resamplesFortyEightKilohertzPcmToSixteenKilohertz() {
        val input = shortArrayOf(0, 3000, 6000, 9000, 12000, 15000)

        val output = resamplePcm16Mono(input, sourceRate = 48000, targetRate = 16000)

        assertArrayEquals(shortArrayOf(0, 9000), output)
    }

    @Test
    fun convertsLittleEndianBytesToShortsAndBack() {
        val bytes = byteArrayOf(0x01, 0x00, 0xff.toByte(), 0x7f, 0x00, 0x80.toByte())

        val shorts = pcm16LittleEndianToShorts(bytes, 6)

        assertArrayEquals(shortArrayOf(1, 32767, -32768), shorts)
        assertArrayEquals(bytes, shortsToPcm16LittleEndian(shorts))
    }

    @Test
    fun ignoresTrailingOddByteWhenConvertingPcm() {
        val bytes = byteArrayOf(0x01, 0x00, 0x55)

        val shorts = pcm16LittleEndianToShorts(bytes, 3)

        assertEquals(1, shorts.size)
        assertEquals(1, shorts[0].toInt())
    }
}
