export type ChatOptionReply = {
  label: string;
  text: string;
};

export type ChatOptionReplyTextPart =
  | {type: 'markdown'; text: string}
  | {type: 'option'; reply: ChatOptionReply};

export type ChatConfirmationReply = {
  sentence: string;
  replyText: '确认' | '接受' | '同意';
};

export type ChatConfirmationReplyTextPart =
  | {type: 'markdown'; text: string}
  | {type: 'confirmation'; reply: ChatConfirmationReply};

const OPTION_LINE_PATTERN = /^\s*([A-H])\.\s+(.+?)\s*$/;
const OPTION_LABELS = 'ABCDEFGH';
const CONFIRMATION_TAIL_WINDOW_CHARS = 700;
const QUESTION_SENTENCE_PATTERN = /[^。！？?？\r\n]*[？?]/g;

type ChatOptionReplyBlock = {
  entries: ChatOptionReplyEntry[];
};

type ChatOptionReplyEntry = {
  reply: ChatOptionReply;
  line: number;
};

type ChatConfirmationReplyMatch = {
  reply: ChatConfirmationReply;
  start: number;
  end: number;
};

function optionLabelIndex(label: string): number {
  return OPTION_LABELS.indexOf(label.toUpperCase());
}

function isFenceLine(line: string): boolean {
  return line.trimStart().startsWith('```');
}

function validOptionBlock(block: ChatOptionReplyEntry[]): boolean {
  if (block.length < 2) {
    return false;
  }
  return block.every((item, index) => optionLabelIndex(item.reply.label) === index);
}

function findLatestOptionReplyBlock(text: string): ChatOptionReplyBlock | null {
  const lines = text.split(/\r?\n/);
  let inCodeFence = false;
  let currentBlock: ChatOptionReplyEntry[] = [];
  let latestValidBlock: ChatOptionReplyBlock | null = null;

  const finishBlock = () => {
    if (validOptionBlock(currentBlock)) {
      latestValidBlock = {
        entries: currentBlock,
      };
    }
    currentBlock = [];
  };

  for (const [index, line] of lines.entries()) {
    if (isFenceLine(line)) {
      finishBlock();
      inCodeFence = !inCodeFence;
      continue;
    }
    if (inCodeFence) {
      continue;
    }
    const match = OPTION_LINE_PATTERN.exec(line);
    if (!match) {
      continue;
    }
    const label = match[1].toUpperCase();
    const labelIndex = optionLabelIndex(label);
    const textValue = match[2].trim();
    if (labelIndex < 0 || !textValue) {
      finishBlock();
      continue;
    }
    if (labelIndex === 0) {
      finishBlock();
      currentBlock = [{reply: {label, text: textValue}, line: index}];
      continue;
    }
    if (currentBlock.length === 0) {
      continue;
    }
    if (labelIndex !== currentBlock.length) {
      finishBlock();
      continue;
    }
    currentBlock = [...currentBlock, {reply: {label, text: textValue}, line: index}];
  }
  finishBlock();

  return latestValidBlock;
}

export function extractChatOptionReplies(text: string): ChatOptionReply[] {
  return findLatestOptionReplyBlock(text)?.entries.map(entry => entry.reply) ?? [];
}

export function splitChatOptionReplyText(text: string): ChatOptionReplyTextPart[] {
  const block = findLatestOptionReplyBlock(text);
  if (!block) {
    return text ? [{type: 'markdown', text}] : [];
  }

  const lines = text.split(/\r?\n/);
  const parts: ChatOptionReplyTextPart[] = [];
  let cursor = 0;
  for (const entry of block.entries) {
    const beforeLines = lines.slice(cursor, entry.line);
    if (beforeLines.length > 0) {
      const leadingBreak = cursor > 0 ? '\n' : '';
      parts.push({type: 'markdown', text: `${leadingBreak}${beforeLines.join('\n')}\n`});
    }
    parts.push({type: 'option', reply: entry.reply});
    cursor = entry.line + 1;
  }
  const afterLines = lines.slice(cursor);
  if (afterLines.length > 0) {
    const leadingBreak = cursor > 0 ? '\n' : '';
    parts.push({type: 'markdown', text: `${leadingBreak}${afterLines.join('\n')}`});
  }
  return parts;
}

function confirmationReplyText(sentence: string): ChatConfirmationReply['replyText'] | null {
  const compact = sentence.replace(/\s+/g, '');
  if (!/[？?]$/.test(compact)) {
    return null;
  }
  if (/^你[^？?]{0,80}接受/.test(compact)) {
    return '接受';
  }
  if (/^你[^？?]{0,80}同意/.test(compact)) {
    return '同意';
  }
  if (/^确认/.test(compact) || /^你[^？?]{0,80}确认/.test(compact)) {
    return '确认';
  }
  if (/^你(?!认为)[^？?]{0,80}认/.test(compact)) {
    return '确认';
  }
  return null;
}

function findLatestConfirmationReplyMatch(text: string): ChatConfirmationReplyMatch | null {
  if (!text || extractChatOptionReplies(text).length > 0) {
    return null;
  }

  const tailStart = Math.max(0, text.length - CONFIRMATION_TAIL_WINDOW_CHARS);
  const linePattern = /([^\r\n]*)(\r?\n|$)/g;
  let inCodeFence = false;
  let latest: ChatConfirmationReplyMatch | null = null;
  let lineMatch: RegExpExecArray | null;

  while ((lineMatch = linePattern.exec(text)) !== null) {
    if (lineMatch[0] === '' && lineMatch.index >= text.length) {
      break;
    }
    const line = lineMatch[1];
    const lineStart = lineMatch.index;
    if (isFenceLine(line)) {
      inCodeFence = !inCodeFence;
      continue;
    }
    if (inCodeFence) {
      continue;
    }

    let sentenceMatch: RegExpExecArray | null;
    QUESTION_SENTENCE_PATTERN.lastIndex = 0;
    while ((sentenceMatch = QUESTION_SENTENCE_PATTERN.exec(line)) !== null) {
      const rawSentence = sentenceMatch[0];
      const leadingWhitespace = rawSentence.match(/^\s*/)?.[0].length ?? 0;
      const sentence = rawSentence.trim();
      if (!sentence) {
        continue;
      }
      const start = lineStart + sentenceMatch.index + leadingWhitespace;
      if (start < tailStart) {
        continue;
      }
      const replyText = confirmationReplyText(sentence);
      if (!replyText) {
        continue;
      }
      latest = {
        reply: {sentence, replyText},
        start,
        end: start + sentence.length,
      };
    }
  }

  return latest;
}

export function extractChatConfirmationReply(text: string): ChatConfirmationReply | null {
  return findLatestConfirmationReplyMatch(text)?.reply ?? null;
}

export function splitChatConfirmationReplyText(text: string): ChatConfirmationReplyTextPart[] {
  const match = findLatestConfirmationReplyMatch(text);
  if (!match) {
    return text ? [{type: 'markdown', text}] : [];
  }

  const parts: ChatConfirmationReplyTextPart[] = [];
  const before = text.slice(0, match.start);
  if (before) {
    parts.push({type: 'markdown', text: before});
  }
  parts.push({type: 'confirmation', reply: match.reply});
  const after = text.slice(match.end);
  if (after) {
    parts.push({type: 'markdown', text: after});
  }
  return parts;
}
