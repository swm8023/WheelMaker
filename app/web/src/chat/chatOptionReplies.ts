export type ChatOptionReply = {
  label: string;
  text: string;
};

const OPTION_LINE_PATTERN = /^\s*([A-H])\.\s+(.+?)\s*$/;
const OPTION_LABELS = 'ABCDEFGH';

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

export function extractChatOptionReplies(text: string): ChatOptionReply[] {
  const lines = text.split(/\r?\n/);
  let inCodeFence = false;
  let currentBlock: ChatOptionReply[] = [];
  let latestValidBlock: ChatOptionReply[] = [];

  const finishBlock = () => {
    if (validOptionBlock(currentBlock)) {
      latestValidBlock = currentBlock;
    }
    currentBlock = [];
  };

  for (const line of lines) {
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
      continue;
    }
    if (labelIndex !== currentBlock.length) {
      finishBlock();
      continue;
    }
    currentBlock = [...currentBlock, {label, text: textValue}];
  }
  finishBlock();

  return latestValidBlock;
}
