import {formatPromptDurationMs} from '../web/src/sessionTime';

describe('web session time formatting', () => {
  test('formats prompt durations from milliseconds into readable labels', () => {
    expect(formatPromptDurationMs(800)).toBe('800ms');
    expect(formatPromptDurationMs(20000)).toBe('20.0s');
    expect(formatPromptDurationMs(125000)).toBe('2m 5s');
    expect(formatPromptDurationMs(119999)).toBe('1m 59s');
  });
});
