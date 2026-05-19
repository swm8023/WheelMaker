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

const OPTION_LINE_PATTERN = /^\s*([A-H1-9])\.\s+(.+?)\s*$/;
const LETTER_OPTION_LABELS = 'ABCDEFGH';
const NUMBER_OPTION_LABELS = '123456789';
const NUMERIC_CHOICE_CONTEXT_PATTERN =
  /可选|选项|候选|请选择|请选|选哪个|选哪一个|要哪个|要哪一个|回复数字|回复序号|选一个|二选一|三选一/;
const CONFIRMATION_TAIL_WINDOW_CHARS = 700;
const QUESTION_SENTENCE_PATTERN = /[^。！？?？\r\n]*[？?]/g;

type ChatOptionReplyBlock = {
  entries: ChatOptionReplyEntry[];
};

type ChatOptionReplyEntry = {
  reply: ChatOptionReply;
  line: number;
  kind: ChatOptionReplyKind;
};

type ChatOptionReplyKind = 'letter' | 'number';

type ChatConfirmationReplyMatch = {
  reply: ChatConfirmationReply;
  start: number;
  end: number;
};

type ChatConfirmationSentenceCandidate = {
  sentence: string;
  start: number;
  end: number;
};

function optionLabelKind(label: string): ChatOptionReplyKind | null {
  if (LETTER_OPTION_LABELS.includes(label.toUpperCase())) {
    return 'letter';
  }
  if (NUMBER_OPTION_LABELS.includes(label)) {
    return 'number';
  }
  return null;
}

function optionLabelIndex(label: string, kind: ChatOptionReplyKind): number {
  if (kind === 'letter') {
    return LETTER_OPTION_LABELS.indexOf(label.toUpperCase());
  }
  return NUMBER_OPTION_LABELS.indexOf(label);
}

function isFenceLine(line: string): boolean {
  return line.trimStart().startsWith('```');
}

function hasNumericChoiceContext(lines: string[], firstOptionLine: number): boolean {
  const contextLines = lines.slice(Math.max(0, firstOptionLine - 4), firstOptionLine);
  return NUMERIC_CHOICE_CONTEXT_PATTERN.test(contextLines.join('\n'));
}

function validOptionBlock(block: ChatOptionReplyEntry[], lines: string[]): boolean {
  if (block.length < 2) {
    return false;
  }
  const kind = block[0].kind;
  if (kind === 'number' && !hasNumericChoiceContext(lines, block[0].line)) {
    return false;
  }
  return block.every(
    (item, index) => item.kind === kind && optionLabelIndex(item.reply.label, kind) === index,
  );
}

function findLatestOptionReplyBlock(text: string): ChatOptionReplyBlock | null {
  const lines = text.split(/\r?\n/);
  let inCodeFence = false;
  let currentBlock: ChatOptionReplyEntry[] = [];
  let latestValidBlock: ChatOptionReplyBlock | null = null;

  const finishBlock = () => {
    if (validOptionBlock(currentBlock, lines)) {
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
    const kind = optionLabelKind(label);
    const textValue = match[2].trim();
    if (!kind || !textValue) {
      finishBlock();
      continue;
    }
    const labelIndex = optionLabelIndex(label, kind);
    if (labelIndex === 0) {
      finishBlock();
      currentBlock = [{reply: {label, text: textValue}, line: index, kind}];
      continue;
    }
    if (currentBlock.length === 0) {
      continue;
    }
    if (kind !== currentBlock[0].kind || labelIndex !== currentBlock.length) {
      finishBlock();
      continue;
    }
    currentBlock = [...currentBlock, {reply: {label, text: textValue}, line: index, kind}];
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
  if (
    /是否[^？?]{0,120}[？?]$/.test(compact) ||
    /要不要[^？?]{0,120}[？?]$/.test(compact) ||
    /需不需要[^？?]{0,120}[？?]$/.test(compact) ||
    /能不能[^？?]{0,120}[？?]$/.test(compact) ||
    /可不可以[^？?]{0,120}[？?]$/.test(compact) ||
    /行不行[^？?]{0,120}[？?]$/.test(compact) ||
    /可以吗[？?]$/.test(compact) ||
    /要[^？?]{1,80}吗[？?]$/.test(compact) ||
    /需要[^？?]{1,80}吗[？?]$/.test(compact)
  ) {
    return '确认';
  }
  return null;
}

function resolveMarkdownBoldQuestionCandidate(
  line: string,
  sentenceStart: number,
  questionEnd: number,
): ChatConfirmationSentenceCandidate | null {
  if (line.slice(questionEnd, questionEnd + 2) !== '**') {
    return null;
  }
  const beforeQuestion = line.slice(sentenceStart, questionEnd);
  const markerIndex = beforeQuestion.lastIndexOf('**');
  if (markerIndex < 0) {
    return null;
  }
  const markerStart = sentenceStart + markerIndex;
  const textStart = markerStart + 2;
  const sentence = line.slice(textStart, questionEnd).trim();
  if (!sentence) {
    return null;
  }
  return {
    sentence,
    start: markerStart,
    end: questionEnd + 2,
  };
}

function resolveConfirmationSentenceCandidate(
  line: string,
  lineStart: number,
  sentenceMatch: RegExpExecArray,
): ChatConfirmationSentenceCandidate | null {
  const rawSentence = sentenceMatch[0];
  const leadingWhitespace = rawSentence.match(/^\s*/)?.[0].length ?? 0;
  const sentenceStart = sentenceMatch.index + leadingWhitespace;
  const questionEnd = sentenceMatch.index + rawSentence.length;
  const markdownWrappedCandidate = resolveMarkdownBoldQuestionCandidate(
    line,
    sentenceStart,
    questionEnd,
  );
  if (markdownWrappedCandidate) {
    return {
      sentence: markdownWrappedCandidate.sentence,
      start: lineStart + markdownWrappedCandidate.start,
      end: lineStart + markdownWrappedCandidate.end,
    };
  }

  const sentence = rawSentence.trim();
  if (!sentence) {
    return null;
  }
  return {
    sentence,
    start: lineStart + sentenceStart,
    end: lineStart + sentenceStart + sentence.length,
  };
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
      const candidate = resolveConfirmationSentenceCandidate(line, lineStart, sentenceMatch);
      if (!candidate) {
        continue;
      }
      if (candidate.start < tailStart) {
        continue;
      }
      const replyText = confirmationReplyText(candidate.sentence);
      if (!replyText) {
        continue;
      }
      latest = {
        reply: {sentence: candidate.sentence, replyText},
        start: candidate.start,
        end: candidate.end,
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
