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
  replies: ChatOptionReply[];
  startLine: number;
  endLine: number;
};

function optionLabelIndex(label: string): number {
  return OPTION_LABELS.indexOf(label.toUpperCase());
}

function isFenceLine(line: string): boolean {
  return line.trimStart().startsWith('```');
}

function validOptionBlock(block: ChatOptionReply[]): boolean {
  if (block.length < 2) {
    return false;
  }
  return block.every((item, index) => optionLabelIndex(item.label) === index);
}

function findLatestOptionReplyBlock(text: string): ChatOptionReplyBlock | null {
  const lines = text.split(/\r?\n/);
  let inCodeFence = false;
  let currentBlock: ChatOptionReply[] = [];
  let currentBlockStartLine = -1;
  let currentBlockEndLine = -1;
  let latestValidBlock: ChatOptionReplyBlock | null = null;

  const finishBlock = () => {
    if (validOptionBlock(currentBlock) && currentBlockStartLine >= 0) {
      latestValidBlock = {
        replies: currentBlock,
        startLine: currentBlockStartLine,
        endLine: currentBlockEndLine,
      };
    }
    currentBlock = [];
    currentBlockStartLine = -1;
    currentBlockEndLine = -1;
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
      finishBlock();
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
      currentBlock = [{label, text: textValue}];
      currentBlockStartLine = index;
      currentBlockEndLine = index;
      continue;
    }
    if (labelIndex !== currentBlock.length) {
      finishBlock();
      continue;
    }
    currentBlock = [...currentBlock, {label, text: textValue}];
    currentBlockEndLine = index;
  }
  finishBlock();

  return latestValidBlock;
}

export function extractChatOptionReplies(text: string): ChatOptionReply[] {
  return findLatestOptionReplyBlock(text)?.replies ?? [];
}

export function splitChatOptionReplyText(text: string): ChatOptionReplyTextPart[] {
  const block = findLatestOptionReplyBlock(text);
  if (!block) {
    return text ? [{type: 'markdown', text}] : [];
  }

  const lines = text.split(/\r?\n/);
  const parts: ChatOptionReplyTextPart[] = [];
  const beforeLines = lines.slice(0, block.startLine);
  const afterLines = lines.slice(block.endLine + 1);
  if (beforeLines.length > 0) {
    parts.push({type: 'markdown', text: `${beforeLines.join('\n')}\n`});
  }
  for (const reply of block.replies) {
    parts.push({type: 'option', reply});
  }
  if (afterLines.length > 0) {
    parts.push({type: 'markdown', text: `\n${afterLines.join('\n')}`});
  }
  return parts;
}
