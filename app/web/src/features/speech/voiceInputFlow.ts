import type { VoiceRecordingStatus } from './voiceInputConstants';
import { formatVoiceInputDiagnosticError } from './voiceInputDiagnostics';

export type VoiceInputRuntimeSnapshot = {
  generation: number;
  expectedRuntimeKey: string;
  currentRuntimeKey: string;
  recording: boolean;
  streamId: string;
  awaitingFinal: boolean;
};

export type VoiceCaptureReadyState = {
  hasStream: boolean;
  pendingFinish: boolean;
};

export function isVoiceInputContextCurrent(
  snapshot: VoiceInputRuntimeSnapshot,
): boolean {
  return (
    !snapshot.expectedRuntimeKey ||
    snapshot.expectedRuntimeKey === snapshot.currentRuntimeKey
  );
}

export function isVoiceInputActive(
  snapshot: Pick<
    VoiceInputRuntimeSnapshot,
    'recording' | 'streamId' | 'awaitingFinal'
  >,
): boolean {
  return snapshot.recording || !!snapshot.streamId || snapshot.awaitingFinal;
}

export function isVoiceGenerationActive(
  snapshot: VoiceInputRuntimeSnapshot,
  generation: number,
): boolean {
  return (
    snapshot.generation === generation &&
    (snapshot.recording || !!snapshot.streamId) &&
    isVoiceInputContextCurrent(snapshot)
  );
}

export function isVoiceInputStartRetryableError(error: unknown): boolean {
  const message = formatVoiceInputDiagnosticError(error).toLowerCase();
  return (
    message.includes('websocket is not connected') ||
    message.includes('connect timeout') ||
    message.includes('closed during connect') ||
    message.includes('connection closed before response') ||
    message.includes('request timed out') ||
    message.includes('session is not ready') ||
    message.includes('speech stream already active')
  );
}

export function isVoiceInputStreamRetryableError(error: unknown): boolean {
  const message = formatVoiceInputDiagnosticError(error).toLowerCase();
  return (
    isVoiceInputStartRetryableError(error) ||
    message.includes('provider disconnected') ||
    message.includes('speech provider disconnected') ||
    message.includes('speech stream idle timeout')
  );
}

export function resolveVoiceCaptureReadyStatus(
  state: VoiceCaptureReadyState,
): VoiceRecordingStatus {
  if (state.pendingFinish) {
    return 'finishing';
  }
  return state.hasStream ? 'recording' : 'buffering';
}
