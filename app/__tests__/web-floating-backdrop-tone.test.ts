import {
  FLOATING_BACKDROP_CONTROL_SELECTOR,
  FLOATING_BACKDROP_TONE_THROTTLE_MS,
  resolveFloatingBackdropTone,
  shouldMeasureFloatingBackdropTone,
} from '../web/src/services/floatingBackdropTone';

describe('floating backdrop tone detection', () => {
  test('classifies mostly light backgrounds so mobile floating controls can become more solid', () => {
    expect(resolveFloatingBackdropTone([
      'rgb(255, 255, 255)',
      'rgb(246, 247, 249)',
      'rgba(238, 241, 245, 0.96)',
      'rgb(24, 24, 27)',
    ])).toBe('light');
  });

  test('keeps mostly dark backgrounds in the translucent mode', () => {
    expect(resolveFloatingBackdropTone([
      'rgb(12, 12, 14)',
      'rgb(28, 30, 35)',
      'rgba(0, 0, 0, 0.88)',
      'rgb(245, 245, 245)',
    ])).toBe('dark');
  });

  test('does not remeasure more than once per throttle window', () => {
    expect(FLOATING_BACKDROP_TONE_THROTTLE_MS).toBe(5000);
    expect(shouldMeasureFloatingBackdropTone(10_000, 0)).toBe(true);
    expect(shouldMeasureFloatingBackdropTone(12_000, 10_000)).toBe(false);
    expect(shouldMeasureFloatingBackdropTone(15_000, 10_000)).toBe(true);
  });

  test('samples the primary nav, drawer, and relay floating button surfaces', () => {
    expect(FLOATING_BACKDROP_CONTROL_SELECTOR).toContain('.floating-nav-group');
    expect(FLOATING_BACKDROP_CONTROL_SELECTOR).toContain('.drawer-toggle-bubble');
    expect(FLOATING_BACKDROP_CONTROL_SELECTOR).toContain('.port-relay-floating-bubble');
  });
});