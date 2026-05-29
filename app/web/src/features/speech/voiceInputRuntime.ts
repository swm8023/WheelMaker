import {
  startMicrophonePCMStream,
  type MicrophonePCMStream,
  type MicrophonePCMStreamOptions,
} from './audioCapture';
import {
  createVoiceInputBuffer,
  type VoiceInputBuffer,
} from './voiceInputBuffer';
import {
  createVoiceInputSendQueue,
  type VoiceInputSendQueue,
  type VoiceInputSendQueueOptions,
} from './voiceInputSendQueue';
import {
  VOICE_AUDIO_CHUNK_BYTES,
  VOICE_LONG_TIMEOUT_MS,
  VOICE_PCM_SAMPLE_RATE,
} from './voiceInputConstants';

export type { MicrophonePCMStream, VoiceInputBuffer, VoiceInputSendQueue };

export function createDefaultVoiceInputBuffer(): VoiceInputBuffer {
  return createVoiceInputBuffer({ maxDurationMs: VOICE_LONG_TIMEOUT_MS });
}

export function createDefaultVoiceInputSendQueue(
  options: VoiceInputSendQueueOptions,
): VoiceInputSendQueue {
  return createVoiceInputSendQueue(options);
}

export function startVoiceInputMicrophoneStream(
  options: Omit<MicrophonePCMStreamOptions, 'targetRate' | 'chunkBytes'>,
): Promise<MicrophonePCMStream> {
  return startMicrophonePCMStream({
    ...options,
    targetRate: VOICE_PCM_SAMPLE_RATE,
    chunkBytes: VOICE_AUDIO_CHUNK_BYTES,
  });
}
