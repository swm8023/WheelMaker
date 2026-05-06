export function formatPromptDurationMs(ms: number): string {
  if (ms < 15000) return `${ms}ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return secs > 0 ? `${minutes}m ${secs}s` : `${minutes}m`;
}

export function parseUpdatedAtMs(value: string): number {
  if (!value) return 0;
  const parsed = new Date(value);
  const time = parsed.getTime();
  return Number.isNaN(time) ? 0 : time;
}

export function compareUpdatedAtDesc(left: string, right: string): number {
  const delta = parseUpdatedAtMs(right) - parseUpdatedAtMs(left);
  if (delta !== 0) return delta;
  if (!left) return 1;
  if (!right) return -1;
  return right.localeCompare(left);
}
