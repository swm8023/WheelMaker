import fs from 'fs';
import path from 'path';

describe('web voice input local buffering wiring', () => {
  test('starts local voice capture before registry stream is ready and flushes through a serial queue', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('import {createVoiceInputBuffer');
    expect(mainTsx).toContain("from './features/speech/voiceInputBuffer';");
    expect(mainTsx).toContain('import {createVoiceInputSendQueue');
    expect(mainTsx).toContain("from './features/speech/voiceInputSendQueue';");
    expect(mainTsx).toContain("from './features/speech/voiceInputConstants';");
    expect(mainTsx).toContain('VOICE_LONG_TIMEOUT_MS');
    expect(mainTsx).toContain('VOICE_SHORT_TIMEOUT_MS');
    expect(mainTsx).toContain('VOICE_AUDIO_CHUNK_BYTES');
    expect(mainTsx).toContain('const voiceInputReconnectAvailable = () => (');
    expect(mainTsx).toContain(
      "logVoiceInputState('warn', 'start_buffering_disconnected'",
    );
    expect(mainTsx).toContain(
      "message.includes('speech stream already active')",
    );
    expect(mainTsx).toContain(
      'voiceInputBufferRef.current = createVoiceInputBuffer({',
    );
    expect(mainTsx).toContain(
      'voiceSendQueueRef.current = createVoiceInputSendQueue({',
    );
    expect(mainTsx).toContain('voiceSendQueueRef.current.enqueue(chunk.bytes)');
    expect(mainTsx).toContain('await queue.drain();');
    expect(mainTsx).toContain('status={voiceRecordingStatus}');
  });

  test('keeps voice input alive across registry reconnects and preserves recognized text', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('handleVoiceRegistryClosedDuringInput');
    expect(mainTsx).toContain(
      "logVoiceInputDiagnostic('warn', 'registry_closed_during_voice'",
    );
    expect(mainTsx).toContain(
      "logVoiceInputDiagnostic('warn', 'voice_reconnect_buffering'",
    );
    expect(mainTsx).toContain(
      "logVoiceInputDiagnostic('warn', 'voice_reconnect_stream_started'",
    );
    expect(mainTsx).toContain(
      "logVoiceInputDiagnostic('warn', 'voice_reconnect_buffer_overflow'",
    );
    expect(mainTsx).toContain('commitLiveTranscript()');
    expect(mainTsx).toContain('clearVoiceStreamStateForReconnect');
    expect(mainTsx).toContain('finishVoiceInputPreservingTranscript');
  });

  test('handles speech errors and finalizing without rolling back recognized text', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      "logVoiceInputDiagnostic('error', 'speech_error_event'",
    );
    expect(mainTsx).toContain('handleVoiceSpeechErrorEvent');
    expect(mainTsx).toContain('VOICE_LONG_TIMEOUT_MS');
    expect(mainTsx).toContain('voiceAwaitingFinalRef');
    expect(mainTsx).toContain('scheduleVoiceFinalTimeout');
    expect(mainTsx).toContain('completeVoiceInputFinalizing');
    expect(mainTsx).toContain("setVoiceRecordingStatus('recognizing')");
  });

  test('stops microphone capture that resolves after recording was already stopped', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      'voiceCaptureGenerationRef.current !== generation ||',
    );
    expect(mainTsx).toMatch(
      /if \(\s*voiceCaptureGenerationRef\.current !== generation \|\|\s*!isVoiceGenerationActive\(generation\)\s*\) \{\s*logVoiceInputState\('warn', 'microphone_started_after_cancel'\);\s*capture\.stop\(\);\s*return;\s*\}/,
    );
  });

  test('wires composer action modes without eager microphone prewarm on send paths', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('type VoiceInputInteractionMode');
    expect(mainTsx).toContain(
      'const [voiceInteractionMode, setVoiceInteractionMode] = useState<VoiceInputInteractionMode | null>(null);',
    );
    expect(mainTsx).toContain(
      'const voiceInteractionModeRef = useRef<VoiceInputInteractionMode | null>(null);',
    );
    expect(mainTsx).toContain(
      "const startVoiceInput = async (interactionMode: VoiceInputInteractionMode = 'locked') => {",
    );
    expect(mainTsx).toContain(
      'voiceInteractionModeRef.current = interactionMode;',
    );
    expect(mainTsx).toContain('setVoiceInteractionMode(interactionMode);');
    expect(mainTsx).toContain('recordingMode={voiceInteractionMode}');
    expect(mainTsx).toContain(
      'hasSendableContent={chatComposerHasSendableContent}',
    );
    expect(mainTsx).toContain(
      'onSend={() => sendChatMessage().catch(() => undefined)}',
    );
    expect(mainTsx).toContain('onCancel={cancelVoiceInputByGesture}');
    expect(mainTsx).toContain('onModeChange={setVoiceInputInteractionMode}');
    expect(mainTsx).toContain('onCancelIntentChange={setVoiceCancelIntent}');
    expect(mainTsx).not.toContain('onPrewarmStart={prewarmVoiceCapture}');
    expect(mainTsx).not.toContain(
      'onPrewarmCancel={cancelVoicePrewarmCapture}',
    );
  });
});
