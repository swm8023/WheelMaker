import {
  appDiagnosticStore,
  createAppDiagnosticStore,
  filterAppDiagnosticRecords,
} from '../web/src/debug/appDiagnostics';
import {logVoiceInputDiagnostic} from '../web/src/features/speech/voiceInputDiagnostics';

describe('app diagnostics', () => {
  afterEach(() => {
    appDiagnosticStore.clear();
    jest.restoreAllMocks();
  });

  test('stores recent warning and error diagnostics by category', () => {
    const store = createAppDiagnosticStore(() => 1000);
    store.record({category: 'voice', level: 'debug', event: 'start_requested'});
    store.record({category: 'voice', level: 'warn', event: 'finish_without_stream'});
    store.record({category: 'voice', level: 'error', event: 'start_failed'});

    const records = filterAppDiagnosticRecords(store.getRecords(), {
      category: 'voice',
      levels: ['warn', 'error'],
    });

    expect(records.map(record => record.event)).toEqual([
      'finish_without_stream',
      'start_failed',
    ]);
  });

  test('voice diagnostics redact sensitive fields before storing and logging', () => {
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => undefined);

    logVoiceInputDiagnostic('warn', 'start_ignored', {
      apiKey: 'secret-key',
      streamId: 'stream-1',
      pcm: 'audio-data',
      connected: true,
    });

    const loggedPayload = warnSpy.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(loggedPayload.apiKey).toBe('[redacted]');
    expect(loggedPayload.streamId).toBe('[redacted]');
    expect(loggedPayload.pcm).toBe('[omitted]');
    expect(loggedPayload.connected).toBe(true);
  });
});
