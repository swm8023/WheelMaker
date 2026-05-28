import fs from 'fs';
import path from 'path';
import {
  DEFAULT_SPEECH_SETTINGS,
  SPEECH_MODEL_OPTIONS,
  maskSpeechSettingsForExport,
  normalizeSpeechSettings,
} from '../web/src/features/speech/speechSettings';

describe('web speech settings', () => {
  test('defaults voice input off and targets Doubao streaming ASR 2.0 postpaid', () => {
    expect(DEFAULT_SPEECH_SETTINGS).toEqual({
      enabled: false,
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      volcengineApiKey: '',
    });
    expect(SPEECH_MODEL_OPTIONS).toEqual([
      {
        id: 'doubao-streaming-asr-2.0',
        label: 'Doubao Streaming ASR 2.0',
        resourceId: 'volc.seedasr.sauc.duration',
      },
    ]);
  });

  test('normalizes and masks user supplied speech settings', () => {
    expect(normalizeSpeechSettings({
      enabled: true,
      provider: 'bad',
      model: 'bad-model',
      volcengineApiKey: '  key-with-spaces  ',
    })).toEqual({
      enabled: true,
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      volcengineApiKey: '  key-with-spaces  ',
    });

    expect(maskSpeechSettingsForExport({
      enabled: true,
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      volcengineApiKey: 'secret-key',
    })).toEqual({
      enabled: true,
      provider: 'volcengine',
      model: 'doubao-streaming-asr-2.0',
      volcengineApiKey: '[redacted]',
    });
  });

  test('persists speech settings and redacts API key from database dumps', () => {
    const projectRoot = path.join(__dirname, '..');
    const persistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(persistence).toContain("from '../features/speech/speechSettings';");
    expect(persistence).toContain('DEFAULT_SPEECH_SETTINGS');
    expect(persistence).toContain('maskSpeechSettingsForExport');
    expect(persistence).toContain('normalizeSpeechSettings');
    expect(persistence).toContain('speechSettings: SpeechSettings;');
    expect(persistence).toContain("speechSettings: 'speechSettings',");
    expect(persistence).toContain('speechSettings: DEFAULT_SPEECH_SETTINGS,');
    expect(persistence).toContain('speechSettings: normalizeSpeechSettings(input.speechSettings),');
    expect(persistence).toContain('{k: GLOBAL_KEYS.speechSettings, v: serialize(next.speechSettings), updatedAt: now}');
    expect(persistence).toContain('maskSpeechSettingsForExport');
    expect(persistence).toContain('redactGlobalDumpRows');
  });
});
