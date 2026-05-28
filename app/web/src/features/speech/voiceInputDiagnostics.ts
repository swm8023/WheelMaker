export type VoiceInputDiagnosticLevel = 'debug' | 'warn' | 'error';

export type VoiceInputDiagnosticEntry = {
  level: VoiceInputDiagnosticLevel;
  event: string;
  details?: Record<string, unknown>;
};

export function formatVoiceInputDiagnosticError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function logVoiceInputDiagnostic(
  level: VoiceInputDiagnosticLevel,
  event: string,
  details: Record<string, unknown> = {},
): void {
  const payload = {
    at: new Date().toISOString(),
    ...details,
  };
  const message = `[VoiceInput] ${event}`;
  if (level === 'error') {
    console.error(message, payload);
    return;
  }
  if (level === 'warn') {
    console.warn(message, payload);
    return;
  }
  console.debug(message, payload);
}
