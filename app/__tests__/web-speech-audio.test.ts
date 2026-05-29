import {
  base64FromBytes,
  chunkPCMBytes,
  createPCMTranscoder,
  floatTo16BitPCM,
  resampleLinear,
  startMicrophonePCMStream,
} from '../web/src/features/speech/audioCapture';
import {
  VOICE_AUDIO_CHUNK_BYTES,
  VOICE_AUDIO_CHUNK_MS,
  VOICE_LONG_TIMEOUT_MS,
  VOICE_SHORT_TIMEOUT_MS,
} from '../web/src/features/speech/voiceInputConstants';

describe('speech audio helpers', () => {
  test('uses the agreed voice timing constants', () => {
    expect(VOICE_SHORT_TIMEOUT_MS).toBe(1000);
    expect(VOICE_LONG_TIMEOUT_MS).toBe(15000);
    expect(VOICE_AUDIO_CHUNK_MS).toBe(200);
    expect(VOICE_AUDIO_CHUNK_BYTES).toBe(6400);
  });

  test('converts float samples into little-endian signed 16-bit PCM', () => {
    const pcm = new Uint8Array(floatTo16BitPCM(new Float32Array([-1, 0, 1])));

    expect(Array.from(pcm)).toEqual([0x00, 0x80, 0x00, 0x00, 0xff, 0x7f]);
  });

  test('resamples mono audio linearly', () => {
    const out = resampleLinear(new Float32Array([0, 1, 0]), 48000, 16000);

    expect(Array.from(out)).toEqual([0]);
  });

  test('encodes bytes to base64 and chunks PCM by size', () => {
    expect(base64FromBytes(new Uint8Array([1, 2, 3]))).toBe('AQID');
    expect(
      chunkPCMBytes(new Uint8Array([1, 2, 3, 4, 5]), 2).map(item =>
        Array.from(item),
      ),
    ).toEqual([[1, 2], [3, 4], [5]]);
  });

  test('stateful transcoder keeps resampler phase across input buffers', () => {
    const transcoder = createPCMTranscoder({
      targetRate: 16000,
      chunkBytes: 1000,
    });

    expect(transcoder.push(new Float32Array([0, 0, 0, 0, 0]), 44100)).toEqual(
      [],
    );
    expect(transcoder.push(new Float32Array([0, 0, 0, 0, 0]), 44100)).toEqual(
      [],
    );

    const tail = transcoder.flush();
    expect(tail).toHaveLength(1);
    expect(tail[0].bytes).toHaveLength(6);
  });

  test('notifies when the first microphone audio frame is processed', async () => {
    const originalAudioContext = window.AudioContext;
    const originalMediaDevices = navigator.mediaDevices;
    const onReady = jest.fn();
    const source = { connect: jest.fn(), disconnect: jest.fn() };
    const processor = {
      connect: jest.fn(),
      disconnect: jest.fn(),
      onaudioprocess: null as
        | null
        | ((event: {
            inputBuffer: { getChannelData: () => Float32Array };
          }) => void),
    };
    const audioContext = {
      state: 'running',
      sampleRate: 16000,
      resume: jest.fn(() => Promise.resolve()),
      close: jest.fn(() => Promise.resolve()),
      createMediaStreamSource: jest.fn(() => source),
      createScriptProcessor: jest.fn(() => processor),
      destination: {},
    };
    const track = { stop: jest.fn(), addEventListener: jest.fn() };

    Object.defineProperty(window, 'AudioContext', {
      configurable: true,
      value: jest.fn(() => audioContext),
    });
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: jest.fn(() =>
          Promise.resolve({ getTracks: () => [track] }),
        ),
      },
    });

    try {
      const capture = await startMicrophonePCMStream({
        chunkBytes: 10,
        onChunk: jest.fn(),
        onReady,
      });
      processor.onaudioprocess?.({
        inputBuffer: {
          getChannelData: () => new Float32Array([0, 0]),
        },
      });
      processor.onaudioprocess?.({
        inputBuffer: {
          getChannelData: () => new Float32Array([0, 0]),
        },
      });

      expect(onReady).toHaveBeenCalledTimes(1);
      capture.stop();
    } finally {
      Object.defineProperty(window, 'AudioContext', {
        configurable: true,
        value: originalAudioContext,
      });
      Object.defineProperty(navigator, 'mediaDevices', {
        configurable: true,
        value: originalMediaDevices,
      });
    }
  });

  test('resumes audio context before waiting for microphone permission', async () => {
    const originalAudioContext = window.AudioContext;
    const originalMediaDevices = navigator.mediaDevices;
    let resolveStream!: (stream: MediaStream) => void;
    const resume = jest.fn(() => Promise.resolve());
    const close = jest.fn(() => Promise.resolve());
    const source = { connect: jest.fn(), disconnect: jest.fn() };
    const processor = {
      connect: jest.fn(),
      disconnect: jest.fn(),
      onaudioprocess: null,
    };
    const audioContext = {
      state: 'suspended',
      sampleRate: 48000,
      resume,
      close,
      createMediaStreamSource: jest.fn(() => source),
      createScriptProcessor: jest.fn(() => processor),
      destination: {},
    };
    const AudioContextMock = jest.fn(() => audioContext);
    const getUserMedia = jest.fn(
      () =>
        new Promise<MediaStream>(resolve => {
          resolveStream = resolve;
        }),
    );
    const track = { stop: jest.fn(), addEventListener: jest.fn() };

    Object.defineProperty(window, 'AudioContext', {
      configurable: true,
      value: AudioContextMock,
    });
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: { getUserMedia },
    });

    try {
      const capturePromise = startMicrophonePCMStream({ onChunk: jest.fn() });
      expect(AudioContextMock).toHaveBeenCalledTimes(1);
      expect(resume).toHaveBeenCalledTimes(1);
      expect(getUserMedia).toHaveBeenCalledTimes(1);

      audioContext.state = 'running';
      resolveStream({ getTracks: () => [track] } as unknown as MediaStream);
      const capture = await capturePromise;
      capture.stop();
    } finally {
      Object.defineProperty(window, 'AudioContext', {
        configurable: true,
        value: originalAudioContext,
      });
      Object.defineProperty(navigator, 'mediaDevices', {
        configurable: true,
        value: originalMediaDevices,
      });
    }
  });

  test('flushes a partial PCM chunk when microphone capture stops', async () => {
    const originalAudioContext = window.AudioContext;
    const originalMediaDevices = navigator.mediaDevices;
    const onChunk = jest.fn();
    const close = jest.fn(() => Promise.resolve());
    const source = { connect: jest.fn(), disconnect: jest.fn() };
    const processor = {
      connect: jest.fn(),
      disconnect: jest.fn(),
      onaudioprocess: null as
        | null
        | ((event: {
            inputBuffer: { getChannelData: () => Float32Array };
          }) => void),
    };
    const audioContext = {
      state: 'running',
      sampleRate: 16000,
      resume: jest.fn(() => Promise.resolve()),
      close,
      createMediaStreamSource: jest.fn(() => source),
      createScriptProcessor: jest.fn(() => processor),
      destination: {},
    };
    const track = { stop: jest.fn(), addEventListener: jest.fn() };

    Object.defineProperty(window, 'AudioContext', {
      configurable: true,
      value: jest.fn(() => audioContext),
    });
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {
        getUserMedia: jest.fn(() =>
          Promise.resolve({ getTracks: () => [track] }),
        ),
      },
    });

    try {
      const capture = await startMicrophonePCMStream({
        chunkBytes: 10,
        onChunk,
      });
      processor.onaudioprocess?.({
        inputBuffer: {
          getChannelData: () => new Float32Array([0, 0.5]),
        },
      });

      expect(onChunk).not.toHaveBeenCalled();

      capture.stop({ flush: true });

      expect(onChunk).toHaveBeenCalledTimes(1);
      expect(Array.from(onChunk.mock.calls[0][0].bytes)).toEqual([
        0, 0, 0xff, 0x3f,
      ]);
    } finally {
      Object.defineProperty(window, 'AudioContext', {
        configurable: true,
        value: originalAudioContext,
      });
      Object.defineProperty(navigator, 'mediaDevices', {
        configurable: true,
        value: originalMediaDevices,
      });
    }
  });
});
