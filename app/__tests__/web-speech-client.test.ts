import {
  createRegistryDebugStore,
  redactRegistryDebugEnvelope,
} from '../web/src/debug/registryDebug';
import {
  createRegistrySpeechClient,
  isSpeechErrorEvent,
  isSpeechTranscriptEvent,
} from '../web/src/features/speech/registrySpeechClient';
import type {RegistryEnvelope} from '../web/src/types/registry';

describe('web speech registry debug redaction', () => {
  test('redacts speech start API key and speech chunk PCM before debug records are stored', () => {
    const startEnvelope = redactRegistryDebugEnvelope({
      requestId: 1,
      type: 'request',
      method: 'speech.start',
      payload: {
        provider: 'volcengine',
        model: 'doubao-streaming-asr-2.0',
        apiKey: 'secret-key',
      },
    });
    expect(JSON.stringify(startEnvelope)).not.toContain('secret-key');
    expect(startEnvelope.payload).toMatchObject({apiKey: '[redacted]'});

    const chunkEnvelope = redactRegistryDebugEnvelope({
      requestId: 2,
      type: 'request',
      method: 'speech.chunk',
      payload: {
        streamId: 'speech-1',
        seq: 3,
        pcm: 'AQIDBA==',
      },
    });
    expect(JSON.stringify(chunkEnvelope)).not.toContain('AQIDBA==');
    expect(chunkEnvelope.payload).toMatchObject({
      streamId: 'speech-1',
      seq: 3,
      pcm: '[base64 omitted]',
      byteCount: 4,
    });
  });

  test('debug store records only redacted speech payloads', () => {
    const store = createRegistryDebugStore(() => 1000);
    store.setEnabled(true);

    store.recordOutbound({
      envelope: {
        requestId: 7,
        type: 'request',
        method: 'speech.start',
        payload: {
          provider: 'volcengine',
          model: 'doubao-streaming-asr-2.0',
          apiKey: 'secret-key',
        },
      },
      raw: '{"apiKey":"secret-key"}',
    });

    const [record] = store.getRecords();
    expect(JSON.stringify(record.envelope)).not.toContain('secret-key');
    expect(record.raw).not.toContain('secret-key');
    expect(record.envelope?.payload).toMatchObject({apiKey: '[redacted]'});
  });
});

describe('registry speech client', () => {
  test('sends speech requests and routes speech events', async () => {
    const events = new Set<(event: RegistryEnvelope) => void>();
    const requests: Array<{method: string; payload: unknown; timeoutMs?: number}> = [];
    const client = createRegistrySpeechClient({
      request: async args => {
        requests.push(args);
        if (args.method === 'speech.start') {
          return {
            type: 'response',
            method: 'speech.start',
            payload: {streamId: 'speech-1'},
          };
        }
        return {
          type: 'response',
          method: args.method,
          payload: {ok: true, streamId: 'speech-1'},
        };
      },
      onEvent: listener => {
        events.add(listener);
        return () => events.delete(listener);
      },
      onClose: () => () => undefined,
    });

    const transcripts: string[] = [];
    const unsubscribe = client.onTranscript(event => {
      transcripts.push(event.text);
    });

    const start = await client.start({
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      apiKey: 'secret-key',
      audio: {format: 'pcm', codec: 'raw', rate: 16000, bits: 16, channel: 1},
    });
    await client.chunk({streamId: start.streamId, seq: 1, pcm: 'AQID'});
    await client.finish({streamId: start.streamId});
    await client.cancel({streamId: start.streamId, reason: 'user'});

    expect(requests.map(item => item.method)).toEqual([
      'speech.start',
      'speech.chunk',
      'speech.finish',
      'speech.cancel',
    ]);

    for (const listener of events) {
      listener({
        type: 'event',
        method: 'speech.transcript',
        payload: {streamId: 'speech-1', text: 'hello', final: false},
      });
    }
    expect(transcripts).toEqual(['hello']);
    unsubscribe();
  });

  test('narrows speech event payloads', () => {
    expect(isSpeechTranscriptEvent({
      type: 'event',
      method: 'speech.transcript',
      payload: {streamId: 'speech-1', text: 'hello', final: true},
    })).toBe(true);
    expect(isSpeechErrorEvent({
      type: 'event',
      method: 'speech.error',
      payload: {streamId: 'speech-1', code: 'UNAVAILABLE', message: 'down', retryable: true},
    })).toBe(true);
  });
});
