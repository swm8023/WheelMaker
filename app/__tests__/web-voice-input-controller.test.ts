import {
  createVoiceInputSession,
  replaceVoiceSegment,
  resolveVoiceGestureState,
} from '../web/src/features/speech/useVoiceInputController';

describe('voice input controller helpers', () => {
  test('replaces the active voice segment instead of appending text', () => {
    expect(replaceVoiceSegment('hi world', 3, 8, 'there')).toBe('hi there');
    expect(replaceVoiceSegment('prefix suffix', 7, 7, 'voice ')).toBe('prefix voice suffix');
  });

  test('tracks transcript replacement and restores base text on cancel', () => {
    const session = createVoiceInputSession('hello world', 6, 11);

    expect(session.applyTranscript('WheelMaker')).toBe('hello WheelMaker');
    expect(session.applyTranscript('WheelMaker speech')).toBe('hello WheelMaker speech');
    expect(session.cancel()).toBe('hello world');
  });

  test('commits current stream transcript before applying a new stream transcript', () => {
    const session = createVoiceInputSession('prefix suffix', 7, 7);

    expect(session.applyTranscript('你好')).toBe('prefix 你好suffix');
    session.commitLiveTranscript();
    expect(session.applyTranscript('世界')).toBe('prefix 你好世界suffix');
    expect(session.currentTranscriptText()).toBe('你好世界');
  });

  test('keeps earlier live speech when a provider emits a shorter segment-only transcript', () => {
    const session = createVoiceInputSession('prefix suffix', 7, 7);

    expect(session.applyTranscript('我想打开')).toBe('prefix 我想打开suffix');
    expect(session.applyTranscript('语音输入')).toBe('prefix 我想打开语音输入suffix');
    expect(session.currentTranscriptText()).toBe('我想打开语音输入');
  });

  test('detects swipe-up cancellation threshold', () => {
    expect(resolveVoiceGestureState(200, 170)).toBe('recording');
    expect(resolveVoiceGestureState(200, 144)).toBe('cancel');
  });
});
