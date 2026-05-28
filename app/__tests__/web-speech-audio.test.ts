import {
  base64FromBytes,
  chunkPCMBytes,
  floatTo16BitPCM,
  resampleLinear,
  startMicrophonePCMStream,
} from '../web/src/features/speech/audioCapture';

describe('speech audio helpers', () => {
  test('converts float samples into little-endian signed 16-bit PCM', () => {
    const pcm = new Uint8Array(floatTo16BitPCM(new Float32Array([-1, 0, 1])));

    expect(Array.from(pcm)).toEqual([
      0x00, 0x80,
      0x00, 0x00,
      0xff, 0x7f,
    ]);
  });

  test('resamples mono audio linearly', () => {
    const out = resampleLinear(new Float32Array([0, 1, 0]), 48000, 16000);

    expect(Array.from(out)).toEqual([0]);
  });

  test('encodes bytes to base64 and chunks PCM by size', () => {
    expect(base64FromBytes(new Uint8Array([1, 2, 3]))).toBe('AQID');
    expect(chunkPCMBytes(new Uint8Array([1, 2, 3, 4, 5]), 2).map(item => Array.from(item))).toEqual([
      [1, 2],
      [3, 4],
      [5],
    ]);
  });

  test('resumes audio context before waiting for microphone permission', async () => {
    const originalAudioContext = window.AudioContext;
    const originalMediaDevices = navigator.mediaDevices;
    let resolveStream!: (stream: MediaStream) => void;
    const resume = jest.fn(() => Promise.resolve());
    const close = jest.fn(() => Promise.resolve());
    const source = {connect: jest.fn(), disconnect: jest.fn()};
    const processor = {connect: jest.fn(), disconnect: jest.fn(), onaudioprocess: null};
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
    const getUserMedia = jest.fn(() => new Promise<MediaStream>(resolve => {
      resolveStream = resolve;
    }));
    const track = {stop: jest.fn(), addEventListener: jest.fn()};

    Object.defineProperty(window, 'AudioContext', {
      configurable: true,
      value: AudioContextMock,
    });
    Object.defineProperty(navigator, 'mediaDevices', {
      configurable: true,
      value: {getUserMedia},
    });

    try {
      const capturePromise = startMicrophonePCMStream({onChunk: jest.fn()});
      expect(AudioContextMock).toHaveBeenCalledTimes(1);
      expect(resume).toHaveBeenCalledTimes(1);
      expect(getUserMedia).toHaveBeenCalledTimes(1);

      audioContext.state = 'running';
      resolveStream({getTracks: () => [track]} as unknown as MediaStream);
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
});
