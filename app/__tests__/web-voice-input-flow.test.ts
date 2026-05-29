import {
  isVoiceGenerationActive,
  isVoiceInputActive,
  isVoiceInputContextCurrent,
  isVoiceInputStartRetryableError,
  isVoiceInputStreamRetryableError,
  resolveVoiceCaptureReadyStatus,
  type VoiceInputRuntimeSnapshot,
} from '../web/src/features/speech/voiceInputFlow';

function snapshot(
  overrides: Partial<VoiceInputRuntimeSnapshot> = {},
): VoiceInputRuntimeSnapshot {
  return {
    generation: 2,
    expectedRuntimeKey: 'project/session',
    currentRuntimeKey: 'project/session',
    recording: true,
    streamId: '',
    awaitingFinal: false,
    ...overrides,
  };
}

describe('voice input flow helpers', () => {
  test('derives context, active, and generation guards from one snapshot', () => {
    expect(isVoiceInputContextCurrent(snapshot())).toBe(true);
    expect(
      isVoiceInputContextCurrent(snapshot({ currentRuntimeKey: 'other' })),
    ).toBe(false);
    expect(
      isVoiceInputActive(snapshot({ recording: false, streamId: 's' })),
    ).toBe(true);
    expect(isVoiceGenerationActive(snapshot({ streamId: 's' }), 2)).toBe(true);
    expect(
      isVoiceGenerationActive(snapshot({ currentRuntimeKey: 'other' }), 2),
    ).toBe(false);
  });

  test('classifies retryable speech start and stream errors', () => {
    expect(
      isVoiceInputStartRetryableError(
        new Error('Registry WebSocket is not connected'),
      ),
    ).toBe(true);
    expect(
      isVoiceInputStartRetryableError(
        new Error('speech stream already active'),
      ),
    ).toBe(true);
    expect(isVoiceInputStartRetryableError(new Error('bad api key'))).toBe(
      false,
    );
    expect(
      isVoiceInputStreamRetryableError(
        new Error('speech provider disconnected'),
      ),
    ).toBe(true);
    expect(
      isVoiceInputStreamRetryableError(new Error('pcm must be base64')),
    ).toBe(false);
  });

  test('resolves the visible status after microphone capture is ready', () => {
    expect(
      resolveVoiceCaptureReadyStatus({
        hasStream: false,
        pendingFinish: false,
      }),
    ).toBe('buffering');
    expect(
      resolveVoiceCaptureReadyStatus({ hasStream: true, pendingFinish: false }),
    ).toBe('recording');
    expect(
      resolveVoiceCaptureReadyStatus({ hasStream: true, pendingFinish: true }),
    ).toBe('finishing');
  });
});
