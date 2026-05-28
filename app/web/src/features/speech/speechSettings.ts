export type SpeechProviderId = 'volcengine';
export type SpeechModelId = 'doubao-streaming-asr-2.0';

export type SpeechSettings = {
  enabled: boolean;
  provider: SpeechProviderId;
  model: SpeechModelId;
  volcengineApiKey: string;
};

export type SpeechModelOption = {
  id: SpeechModelId;
  label: string;
  resourceId: string;
};

export const SPEECH_MODEL_OPTIONS: SpeechModelOption[] = [
  {
    id: 'doubao-streaming-asr-2.0',
    label: 'Doubao Streaming ASR 2.0',
    resourceId: 'volc.seedasr.sauc.duration',
  },
];

export const DEFAULT_SPEECH_SETTINGS: SpeechSettings = {
  enabled: false,
  provider: 'volcengine',
  model: 'doubao-streaming-asr-2.0',
  volcengineApiKey: '',
};

export function normalizeSpeechSettings(input: unknown): SpeechSettings {
  if (!input || typeof input !== 'object' || Array.isArray(input)) {
    return DEFAULT_SPEECH_SETTINGS;
  }
  const record = input as Partial<SpeechSettings>;
  return {
    enabled: record.enabled === true,
    provider: 'volcengine',
    model: 'doubao-streaming-asr-2.0',
    volcengineApiKey: typeof record.volcengineApiKey === 'string'
      ? record.volcengineApiKey
      : '',
  };
}

export function maskSpeechSettingsForExport(input: unknown): SpeechSettings {
  const settings = normalizeSpeechSettings(input);
  return {
    ...settings,
    volcengineApiKey: settings.volcengineApiKey ? '[redacted]' : '',
  };
}
