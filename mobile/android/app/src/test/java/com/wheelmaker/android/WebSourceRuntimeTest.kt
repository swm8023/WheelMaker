package com.wheelmaker.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class WebSourceRuntimeTest {
    @Test
    fun rejectsLoopbackRemoteCandidates() {
        val candidate = RemoteWebCandidate(
            source = "registry",
            registryAddress = "ws://127.0.0.1:9630/ws",
            remoteWebUrl = "http://127.0.0.1:9630/"
        )

        assertFalse(normalizeRemoteWebCandidate(candidate).accepted)
    }

    @Test
    fun acceptsRemoteCandidateWhenRegistryAndRemoteHostsMatch() {
        val candidate = RemoteWebCandidate(
            source = "registry",
            registryAddress = "wss://workspace.example.com/ws",
            remoteWebUrl = "https://workspace.example.com/"
        )

        val normalized = normalizeRemoteWebCandidate(candidate)

        assertTrue(normalized.accepted)
        assertEquals("https://workspace.example.com/", normalized.remoteWebUrl)
        assertEquals("wss://workspace.example.com", normalized.registryOrigin)
    }

    @Test
    fun rejectsRemoteCandidateWhenRegistryAndRemoteHostsDiffer() {
        val candidate = RemoteWebCandidate(
            source = "registry",
            registryAddress = "wss://workspace.example.com/ws",
            remoteWebUrl = "https://other.example.com/"
        )

        assertFalse(normalizeRemoteWebCandidate(candidate).accepted)
    }

    @Test
    fun stateFallsBackToEmbeddedWhenRemoteUrlIsEmpty() {
        val runtime = WebSourceRuntime(InMemoryWebSourceStore(WebSourceConfig()))

        val state = runtime.state()

        assertEquals("auto", state.preference)
        assertEquals("embedded", state.actualSource)
        assertEquals("", state.remoteUrl)
    }
}
