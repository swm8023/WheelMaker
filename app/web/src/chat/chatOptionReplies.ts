export type ChatOptionReply = {
  label: string;
  text: string;
};

export type ChatOptionReplyTextPart =
  | {type: 'markdown'; text: string}
  | {type: 'option'; reply: ChatOptionReply};

const OPTION_LINE_PATTERN = /^\s*([A-H])\.\s+(.+?)\s*$/;
const OPTION_LABELS = 'ABCDEFGH';

type ChatOptionReplyBlock = {
  entries: ChatOptionReplyEntry[];
};

type ChatOptionReplyEntry = {
  reply: ChatOptionReply;
  line: number;
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
