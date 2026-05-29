export const VOICE_SHORT_TIMEOUT_MS = 1000;
export const VOICE_LONG_TIMEOUT_MS = 15000;
export const VOICE_AUDIO_CHUNK_MS = 200;
export const VOICE_PCM_SAMPLE_RATE = 16000;
export const VOICE_PCM_BYTES_PER_SAMPLE = 2;
export const VOICE_PCM_CHANNELS = 1;
export const VOICE_PCM_BYTES_PER_SECOND =
  VOICE_PCM_SAMPLE_RATE * VOICE_PCM_BYTES_PER_SAMPLE * VOICE_PCM_CHANNELS;
export const VOICE_AUDIO_CHUNK_BYTES = Math.floor(
  (VOICE_PCM_BYTES_PER_SECOND * VOICE_AUDIO_CHUNK_MS) / 1000,
);

export type VoiceRecordingStatus =
  | 'permission'
  | 'starting'
  | 'buffering'
  | 'recording'
  | 'finishing'
  | 'recognizing';
