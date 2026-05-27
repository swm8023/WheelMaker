export const MOBILE_HAPTIC_LIGHT_MS = 12;

export function triggerMobileHaptic(durationMs = MOBILE_HAPTIC_LIGHT_MS): void {
  try {
    navigator.vibrate?.(durationMs);
  } catch {
    // Unsupported browsers, including iOS Safari, simply skip web haptics.
  }
}
