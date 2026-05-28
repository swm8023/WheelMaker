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

  test('detects swipe-up cancellation threshold', () => {
    expect(resolveVoiceGestureState(200, 170)).toBe('recording');
    expect(resolveVoiceGestureState(200, 144)).toBe('cancel');
  });
});
