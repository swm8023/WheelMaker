export function getLineNumberDigits(lineCount: number): number {
  const safeCount = Math.max(1, Math.floor(lineCount || 0));
  return String(safeCount).length;
}
