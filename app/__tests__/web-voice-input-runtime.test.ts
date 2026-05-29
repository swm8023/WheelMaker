import {
  createDefaultVoiceInputBuffer,
  createDefaultVoiceInputSendQueue,
  startVoiceInputMicrophoneStream,
} from '../web/src/features/speech/voiceInputRuntime';
import {
  VOICE_AUDIO_CHUNK_BYTES,
  VOICE_PCM_SAMPLE_RATE,
} from '../web/src/features/speech/voiceInputConstants';

const mockStartMicrophonePCMStream = jest.fn();

jest.mock('../web/src/features/speech/audioCapture', () => {
  const actual = jest.requireActual('../web/src/features/speech/audioCapture');
  return {
    ...actual,
    startMicrophonePCMStream: (...args: unknown[]) =>
      mockStartMicrophonePCMStream(...args),
  };
});

describe('voice input runtime defaults', () => {
  beforeEach(() => {
    mockStartMicrophonePCMStream.mockReset();
  });

  test('creates a buffer using the shared 15 second voice limit', () => {
    const buffer = createDefaultVoiceInputBuffer();
    expect(buffer.stats().maxBytes).toBe(480000);
  });

  test('creates a serial send queue with the existing queue behavior', async () => {
    const sent: Array<{ seq: number; pcm: string }> = [];
    const queue = createDefaultVoiceInputSendQueue({
      streamId: 'speech-1',
      sendChunk: async payload => {
        sent.push({ seq: payload.seq, pcm: payload.pcm });
      },
    });

    await queue.enqueue(new Uint8Array([1, 2, 3]));
    await queue.drain();

    expect(sent).toEqual([{ seq: 1, pcm: 'AQID' }]);
  });

  test('starts microphone capture with the shared voice PCM defaults', async () => {
    const capture = { stop: jest.fn() };
    const onChunk = jest.fn();
    mockStartMicrophonePCMStream.mockResolvedValue(capture);

    await expect(startVoiceInputMicrophoneStream({ onChunk })).resolves.toBe(
      capture,
    );

    expect(mockStartMicrophonePCMStream).toHaveBeenCalledWith({
      onChunk,
      targetRate: VOICE_PCM_SAMPLE_RATE,
      chunkBytes: VOICE_AUDIO_CHUNK_BYTES,
    });
  });
});
