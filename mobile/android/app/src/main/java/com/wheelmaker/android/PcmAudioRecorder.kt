package com.wheelmaker.android

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import androidx.core.content.ContextCompat
import java.util.concurrent.Executors
import kotlin.math.max
import kotlin.math.min
import kotlin.math.sqrt

interface PcmAudioRecorderListener {
    fun onAudioChunk(pcm: ByteArray)
    fun onLevel(level: Double)
    fun onError(message: String)
    fun onStopped()
}

class PcmAudioRecorder(
    private val context: Context,
    private val listener: PcmAudioRecorderListener,
    private val targetSampleRate: Int = 16000,
    private val chunkBytes: Int = 6400,
    private val sampleRates: IntArray = intArrayOf(16000, 48000, 44100)
) {
    @Volatile
    private var running = false

    @Volatile
    private var flushOnStop = false

    @Volatile
    private var audioRecord: AudioRecord? = null

    private val executor = Executors.newSingleThreadExecutor()
    private val pendingLock = Any()
    private var pending = ByteArray(0)

    fun start() {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            throw SecurityException("Microphone permission is required.")
        }
        val opened = openAudioRecord()
        audioRecord = opened.record
        running = true
        executor.execute {
            runCaptureLoop(opened.record, opened.sampleRate, opened.readBufferBytes)
        }
    }

    fun stop(flush: Boolean = false) {
        flushOnStop = flush
        running = false
        try {
            audioRecord?.stop()
        } catch (_: Exception) {
        }
    }

    @SuppressLint("MissingPermission")
    private fun openAudioRecord(): OpenedAudioRecord {
        for (sampleRate in sampleRates) {
            val minBuffer = AudioRecord.getMinBufferSize(
                sampleRate,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT
            )
            if (minBuffer <= 0) {
                continue
            }
            val bufferSize = max(minBuffer * 2, chunkBytes * max(1, sampleRate / targetSampleRate))
            val record = AudioRecord(
                MediaRecorder.AudioSource.MIC,
                sampleRate,
                AudioFormat.CHANNEL_IN_MONO,
                AudioFormat.ENCODING_PCM_16BIT,
                bufferSize
            )
            if (record.state == AudioRecord.STATE_INITIALIZED) {
                return OpenedAudioRecord(record, sampleRate, minBuffer)
            }
            record.release()
        }
        throw IllegalStateException("No supported microphone sample rate is available.")
    }

    private fun runCaptureLoop(record: AudioRecord, sampleRate: Int, readBufferBytes: Int) {
        try {
            record.startRecording()
            val buffer = ByteArray(readBufferBytes)
            while (running) {
                val read = record.read(buffer, 0, buffer.size)
                if (read <= 0) {
                    continue
                }
                val pcm = if (sampleRate == targetSampleRate) {
                    buffer.copyOf(read)
                } else {
                    shortsToPcm16LittleEndian(
                        resamplePcm16Mono(
                            pcm16LittleEndianToShorts(buffer, read),
                            sampleRate,
                            targetSampleRate
                        )
                    )
                }
                enqueuePcm(pcm)
                listener.onLevel(calculatePcmLevel(pcm))
            }
            if (flushOnStop) {
                flushPending()
            }
        } catch (error: Exception) {
            if (running) {
                listener.onError(error.message ?: "Microphone capture failed.")
            }
        } finally {
            running = false
            try {
                record.release()
            } catch (_: Exception) {
            }
            audioRecord = null
            listener.onStopped()
            executor.shutdown()
        }
    }

    private fun enqueuePcm(pcm: ByteArray) {
        if (pcm.isEmpty()) {
            return
        }
        val chunks = mutableListOf<ByteArray>()
        synchronized(pendingLock) {
            pending += pcm
            while (pending.size >= chunkBytes) {
                chunks += pending.copyOfRange(0, chunkBytes)
                pending = pending.copyOfRange(chunkBytes, pending.size)
            }
        }
        for (chunk in chunks) {
            listener.onAudioChunk(chunk)
        }
    }

    private fun flushPending() {
        val chunk = synchronized(pendingLock) {
            val value = pending
            pending = ByteArray(0)
            value
        }
        if (chunk.isNotEmpty()) {
            listener.onAudioChunk(chunk)
        }
    }

    private fun calculatePcmLevel(pcm: ByteArray): Double {
        if (pcm.size < 2) {
            return 0.0
        }
        var sum = 0.0
        var count = 0
        var index = 0
        while (index + 1 < pcm.size) {
            val sample = ((pcm[index + 1].toInt() shl 8) or (pcm[index].toInt() and 0xff)).toShort().toInt()
            sum += sample.toDouble() * sample.toDouble()
            count += 1
            index += 2
        }
        if (count == 0) {
            return 0.0
        }
        return min(1.0, sqrt(sum / count) / 32768.0 * 4.0)
    }

    private data class OpenedAudioRecord(
        val record: AudioRecord,
        val sampleRate: Int,
        val readBufferBytes: Int
    )
}
