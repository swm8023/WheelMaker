import {
  createAndroidNativeSpeechRuntime,
  getAndroidNativeSpeechBridge,
  isAndroidNativeSpeechHost,
  type AndroidNativeSpeechEvent,
} from '../web/src/features/speech/androidNativeSpeechRuntime';

describe('android native speech runtime', () => {
  afterEach(() => {
    delete (globalThis as {window?: unknown}).window;
  });

  test('detects only Android native hosts with speech bridge methods', () => {
    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroidNative: {
        getWebSourceState: jest.fn(),
      },
    };

    expect(isAndroidNativeSpeechHost()).toBe(true);
    expect(getAndroidNativeSpeechBridge()).toBeNull();

    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroidNative: {
        startSpeech: jest.fn(),
        finishSpeech: jest.fn(),
        cancelSpeech: jest.fn(),
      },
    };

    expect(getAndroidNativeSpeechBridge()).not.toBeNull();
  });

  test('starts native speech and routes async native events', async () => {
    const native = {
      startSpeech: jest.fn(() => JSON.stringify({accepted: true, streamId: 'android-speech-1'})),
      finishSpeech: jest.fn(() => JSON.stringify({accepted: true, streamId: 'android-speech-1'})),
      cancelSpeech: jest.fn(() => JSON.stringify({accepted: true, streamId: 'android-speech-1'})),
    };
    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroidNative: native,
    };
    const runtime = createAndroidNativeSpeechRuntime();
    const events: AndroidNativeSpeechEvent[] = [];
    const unsubscribe = runtime?.onEvent(event => events.push(event));

    await expect(runtime?.start({
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      apiKey: 'secret-key',
      audio: {format: 'pcm', codec: 'raw', rate: 16000, bits: 16, channel: 1},
    })).resolves.toEqual({streamId: 'android-speech-1'});
    await runtime?.finish('android-speech-1');
    await runtime?.cancel('android-speech-1', 'gesture');

    expect(native.startSpeech).toHaveBeenCalledWith(JSON.stringify({
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      apiKey: 'secret-key',
      audio: {format: 'pcm', codec: 'raw', rate: 16000, bits: 16, channel: 1},
    }));
    expect(native.finishSpeech).toHaveBeenCalledWith('android-speech-1');
    expect(native.cancelSpeech).toHaveBeenCalledWith('android-speech-1', 'gesture');

    const callback = (window as Window & {
      __wheelmakerAndroidSpeechEvent?: (event: unknown) => void;
    }).__wheelmakerAndroidSpeechEvent;
    callback?.({
      type: 'transcript',
      streamId: 'android-speech-1',
      text: 'hello',
      final: false,
    });
    callback?.(JSON.stringify({
      type: 'status',
      streamId: 'android-speech-1',
      status: 'recording',
    }));

    expect(events).toEqual([
      {type: 'transcript', streamId: 'android-speech-1', text: 'hello', final: false},
      {type: 'status', streamId: 'android-speech-1', status: 'recording'},
    ]);
    unsubscribe?.();
  });

  test('rejects native command responses that were not accepted', async () => {
    (globalThis as {window?: unknown}).window = {
      WheelMakerAndroidNative: {
        startSpeech: jest.fn(() => JSON.stringify({
          accepted: false,
          code: 'BUSY',
          message: 'Native speech is already active.',
        })),
        finishSpeech: jest.fn(),
        cancelSpeech: jest.fn(),
      },
    };

    const runtime = createAndroidNativeSpeechRuntime();

    await expect(runtime?.start({
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      apiKey: 'secret-key',
      audio: {format: 'pcm', codec: 'raw', rate: 16000, bits: 16, channel: 1},
    })).rejects.toThrow('Native speech is already active.');
  });
});
