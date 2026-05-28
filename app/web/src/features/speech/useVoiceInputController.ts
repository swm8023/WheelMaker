export type VoiceGestureState = 'recording' | 'cancel';

export type VoiceInputSession = {
  applyTranscript: (text: string) => string;
  commitLiveTranscript: () => string;
  cancel: () => string;
  currentText: () => string;
  currentTranscriptText: () => string;
};

function suffixPrefixOverlap(left: string, right: string): number {
  const max = Math.min(left.length, right.length);
  for (let size = max; size > 0; size -= 1) {
    if (left.endsWith(right.slice(0, size))) {
      return size;
    }
  }
  return 0;
}

function commonPrefixLength(left: string, right: string): number {
  const max = Math.min(left.length, right.length);
  let index = 0;
  while (index < max && left[index] === right[index]) {
    index += 1;
  }
  return index;
}

export function mergeVoiceTranscriptUpdate(previous: string, next: string): string {
  if (!previous || next.startsWith(previous)) {
    return next;
  }
  if (!next || previous === next || previous.startsWith(next) || previous.endsWith(next)) {
    return previous;
  }
  const sharedPrefix = commonPrefixLength(previous, next);
  if (sharedPrefix >= Math.max(2, Math.floor(Math.min(previous.length, next.length) / 2))) {
    return next;
  }
  const overlap = suffixPrefixOverlap(previous, next);
  return `${previous}${next.slice(overlap)}`;
}

export function replaceVoiceSegment(
  baseText: string,
  insertStart: number,
  insertEnd: number,
  voiceText: string,
): string {
  const safeStart = Math.max(0, Math.min(baseText.length, Math.floor(insertStart)));
  const safeEnd = Math.max(safeStart, Math.min(baseText.length, Math.floor(insertEnd)));
  return `${baseText.slice(0, safeStart)}${voiceText}${baseText.slice(safeEnd)}`;
}

export function createVoiceInputSession(
  baseText: string,
  insertStart: number,
  insertEnd: number,
): VoiceInputSession {
  let current = baseText;
  let committedTranscript = '';
  let liveTranscript = '';
  const render = () => {
    current = replaceVoiceSegment(baseText, insertStart, insertEnd, `${committedTranscript}${liveTranscript}`);
    return current;
  };
  return {
    applyTranscript: text => {
      liveTranscript = mergeVoiceTranscriptUpdate(liveTranscript, text);
      return render();
    },
    commitLiveTranscript: () => {
      committedTranscript += liveTranscript;
      liveTranscript = '';
      return render();
    },
    cancel: () => {
      current = baseText;
      return current;
    },
    currentText: () => current,
    currentTranscriptText: () => `${committedTranscript}${liveTranscript}`,
  };
}

export function resolveVoiceGestureState(
  startY: number,
  currentY: number,
  threshold = 52,
): VoiceGestureState {
  return startY - currentY >= threshold ? 'cancel' : 'recording';
}
