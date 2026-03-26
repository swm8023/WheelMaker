import {nextThemeMode} from '../src/theme';

describe('nextThemeMode', () => {
  test('toggles dark and light', () => {
    expect(nextThemeMode('dark')).toBe('light');
    expect(nextThemeMode('light')).toBe('dark');
  });
});
