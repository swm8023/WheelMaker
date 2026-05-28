export type VoiceGestureState = 'recording' | 'cancel';

export type VoiceInputSession = {
  applyTranscript: (text: string) => string;
  cancel: () => string;
  currentText: () => string;
};

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
  return {
    applyTranscript: text => {
      current = replaceVoiceSegment(baseText, insertStart, insertEnd, text);
      return current;
    },
    cancel: () => {
      current = baseText;
      return current;
    },
    currentText: () => current,
  };
}

export function resolveVoiceGestureState(
  startY: number,
  currentY: number,
  threshold = 52,
): VoiceGestureState {
  return startY - currentY >= threshold ? 'cancel' : 'recording';
}
