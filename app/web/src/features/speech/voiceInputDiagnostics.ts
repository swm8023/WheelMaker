import {appDiagnosticStore, sanitizeAppDiagnosticDetails} from '../../debug/appDiagnostics';

export type VoiceInputDiagnosticLevel = 'debug' | 'warn' | 'error';

export type VoiceInputDiagnosticEntry = {
  level: VoiceInputDiagnosticLevel;
  event: string;
  details?: Record<string, unknown>;
};

export function formatVoiceInputDiagnosticError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error);
  if (!error || typeof error !== 'object') {
    return message;
  }
  const details = (error as {details?: unknown}).details;
  if (!details || typeof details !== 'object' || Array.isArray(details)) {
    return message;
  }
  const providerError = (details as Record<string, unknown>).error;
  if (typeof providerError !== 'string' || providerError.length === 0) {
    return message;
  }
  return `${message}: ${providerError}`;
}

export function logVoiceInputDiagnostic(
  level: VoiceInputDiagnosticLevel,
  event: string,
  details: Record<string, unknown> = {},
): void {
  if (level === 'debug') {
    return;
  }
  const safeDetails = sanitizeAppDiagnosticDetails(details);
  const payload = {
    at: new Date().toISOString(),
    ...safeDetails,
  };
  appDiagnosticStore.record({
    category: 'voice',
    level,
    event,
    details: payload,
  });
  const message = `[VoiceInput] ${event}`;
  if (level === 'error') {
    console.error(message, payload);
    return;
  }
  if (level === 'warn') {
    console.warn(message, payload);
    return;
  }
}
