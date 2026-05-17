export type ChatSlashInsertionResult = {
  text: string;
  selectionStart: number;
  selectionEnd: number;
};

function normalizeChatSlashCommandName(name: string): string {
  const normalized = name.trim();
  if (!normalized) {
    return '';
  }
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

function clampSelection(value: number | null | undefined, max: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return max;
  }
  return Math.max(0, Math.min(Math.trunc(value), max));
}

function replaceActiveSlashQuery(text: string, commandName: string): ChatSlashInsertionResult | null {
  const leadingWhitespace = text.match(/^\s*/)?.[0] ?? '';
  const leadingTrimmed = text.slice(leadingWhitespace.length);
  if (!leadingTrimmed.startsWith('/')) {
    return null;
  }
  const firstToken = leadingTrimmed.split(/\s+/, 1)[0] || '';
  if (leadingTrimmed.length > firstToken.length) {
    return null;
  }
  const nextText = `${leadingWhitespace}${commandName} `;
  return {
    text: nextText,
    selectionStart: nextText.length,
    selectionEnd: nextText.length,
  };
}

export function insertChatSlashCommandText(
  currentText: string,
  commandName: string,
  selectionStart?: number | null,
  selectionEnd?: number | null,
): ChatSlashInsertionResult {
  const normalizedCommand = normalizeChatSlashCommandName(commandName);
  if (!normalizedCommand) {
    const fallbackSelection = clampSelection(selectionStart, currentText.length);
    return {
      text: currentText,
      selectionStart: fallbackSelection,
      selectionEnd: fallbackSelection,
    };
  }

  const activeQueryReplacement = replaceActiveSlashQuery(currentText, normalizedCommand);
  if (activeQueryReplacement) {
    return activeQueryReplacement;
  }

  const start = clampSelection(selectionStart, currentText.length);
  const end = Math.max(start, clampSelection(selectionEnd, currentText.length));
  const before = currentText.slice(0, start);
  const after = currentText.slice(end);
  const leadingSpace = before && !/\s$/.test(before) ? ' ' : '';
  const trailingSpace = ' ';
  const nextAfter = after.replace(/^\s+/, '');
  const insertion = `${leadingSpace}${normalizedCommand}${trailingSpace}`;
  const nextText = `${before}${insertion}${nextAfter}`;
  const nextSelection = before.length + insertion.length;
  return {
    text: nextText,
    selectionStart: nextSelection,
    selectionEnd: nextSelection,
  };
}
