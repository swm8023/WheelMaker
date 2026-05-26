export type FloatingBackdropTone = 'dark' | 'light' | 'mixed';

type RgbaColor = {
  r: number;
  g: number;
  b: number;
  a: number;
};

type SamplePoint = {
  x: number;
  y: number;
};

type SampleRect = {
  left: number;
  top: number;
  width: number;
  height: number;
};

export const FLOATING_BACKDROP_TONE_THROTTLE_MS = 5000;
export const FLOATING_BACKDROP_CONTROL_SELECTOR =
  '.floating-nav-group, .drawer-toggle-bubble, .port-relay-floating-bubble';

const LIGHT_LUMINANCE_THRESHOLD = 0.72;
const DARK_LUMINANCE_THRESHOLD = 0.42;
const MAJORITY_RATIO = 0.6;
const MIN_VISIBLE_ALPHA = 0.05;

function clampColorChannel(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(255, Math.max(0, value));
}

function parseCssNumber(value: string): number {
  const trimmed = value.trim();
  if (trimmed.endsWith('%')) {
    return (Number.parseFloat(trimmed) / 100) * 255;
  }
  return Number.parseFloat(trimmed);
}

function parseCssAlpha(value: string | undefined): number {
  if (!value) return 1;
  const trimmed = value.trim();
  if (trimmed.endsWith('%')) {
    return Math.min(1, Math.max(0, Number.parseFloat(trimmed) / 100));
  }
  return Math.min(1, Math.max(0, Number.parseFloat(trimmed)));
}

function parseHexColor(input: string): RgbaColor | null {
  const value = input.trim();
  if (!value.startsWith('#')) return null;
  const hex = value.slice(1);
  if (![3, 4, 6, 8].includes(hex.length)) return null;

  const expand = (part: string) => part.length === 1 ? `${part}${part}` : part;
  const r = Number.parseInt(expand(hex.length <= 4 ? hex[0] : hex.slice(0, 2)), 16);
  const g = Number.parseInt(expand(hex.length <= 4 ? hex[1] : hex.slice(2, 4)), 16);
  const b = Number.parseInt(expand(hex.length <= 4 ? hex[2] : hex.slice(4, 6)), 16);
  const alphaHex = hex.length === 4 ? hex[3] : hex.length === 8 ? hex.slice(6, 8) : '';
  const a = alphaHex ? Number.parseInt(expand(alphaHex), 16) / 255 : 1;

  if ([r, g, b, a].some(channel => Number.isNaN(channel))) return null;
  return { r, g, b, a };
}

function parseRgbColor(input: string): RgbaColor | null {
  const match = input.trim().match(/^rgba?\((.*)\)$/i);
  if (!match) return null;

  const normalized = match[1].replace(/\s+\/\s+/, ', ');
  const parts = normalized.includes(',')
    ? normalized.split(',').map(part => part.trim())
    : normalized.split(/\s+/).map(part => part.trim()).filter(Boolean);
  if (parts.length < 3) return null;

  const r = clampColorChannel(parseCssNumber(parts[0]));
  const g = clampColorChannel(parseCssNumber(parts[1]));
  const b = clampColorChannel(parseCssNumber(parts[2]));
  const a = parseCssAlpha(parts[3]);
  if ([r, g, b, a].some(channel => Number.isNaN(channel))) return null;
  return { r, g, b, a };
}

function parseSrgbColor(input: string): RgbaColor | null {
  const match = input.trim().match(/^color\(\s*srgb\s+(.+)\)$/i);
  if (!match) return null;

  const normalized = match[1].replace(/\s+\/\s+/, ' ');
  const parts = normalized.split(/\s+/).map(part => part.trim()).filter(Boolean);
  if (parts.length < 3) return null;

  const channel = (value: string) => {
    if (value.endsWith('%')) return clampColorChannel((Number.parseFloat(value) / 100) * 255);
    return clampColorChannel(Number.parseFloat(value) * 255);
  };
  const r = channel(parts[0]);
  const g = channel(parts[1]);
  const b = channel(parts[2]);
  const a = parseCssAlpha(parts[3]);
  if ([r, g, b, a].some(value => Number.isNaN(value))) return null;
  return { r, g, b, a };
}

export function parseCssColor(input: string): RgbaColor | null {
  const value = input.trim();
  if (!value || value === 'transparent') return null;
  return parseRgbColor(value) ?? parseHexColor(value) ?? parseSrgbColor(value);
}

function linearizeSrgbChannel(channel: number): number {
  const normalized = channel / 255;
  if (normalized <= 0.03928) {
    return normalized / 12.92;
  }
  return ((normalized + 0.055) / 1.055) ** 2.4;
}

function relativeLuminance(color: RgbaColor): number {
  return (
    0.2126 * linearizeSrgbChannel(color.r) +
    0.7152 * linearizeSrgbChannel(color.g) +
    0.0722 * linearizeSrgbChannel(color.b)
  );
}

export function resolveFloatingBackdropTone(colors: string[]): FloatingBackdropTone {
  const luminances = colors
    .map(parseCssColor)
    .filter((color): color is RgbaColor => !!color && color.a > MIN_VISIBLE_ALPHA)
    .map(relativeLuminance);

  if (luminances.length === 0) {
    return 'mixed';
  }

  const lightCount = luminances.filter(value => value >= LIGHT_LUMINANCE_THRESHOLD).length;
  const darkCount = luminances.filter(value => value <= DARK_LUMINANCE_THRESHOLD).length;
  if (lightCount / luminances.length >= MAJORITY_RATIO) {
    return 'light';
  }
  if (darkCount / luminances.length >= MAJORITY_RATIO) {
    return 'dark';
  }
  return 'mixed';
}

export function shouldMeasureFloatingBackdropTone(now: number, lastMeasuredAt: number): boolean {
  return lastMeasuredAt <= 0 || now - lastMeasuredAt >= FLOATING_BACKDROP_TONE_THROTTLE_MS;
}

function buildFloatingBackdropSamplePoints(rects: SampleRect[]): SamplePoint[] {
  const points: SamplePoint[] = [];
  const seen = new Set<string>();

  rects.forEach(rect => {
    if (rect.width <= 0 || rect.height <= 0) {
      return;
    }

    const yFractions = rect.height >= 96 ? [0.18, 0.5, 0.82] : [0.5];
    yFractions.forEach(yFraction => {
      const point = {
        x: rect.left + rect.width * 0.5,
        y: rect.top + rect.height * yFraction,
      };
      const key = `${Math.round(point.x)}:${Math.round(point.y)}`;
      if (!seen.has(key)) {
        seen.add(key);
        points.push(point);
      }
    });
  });

  return points;
}

function resolveElementBackgroundColor(element: Element): string | null {
  let current: Element | null = element;
  while (current) {
    const color = window.getComputedStyle(current).backgroundColor;
    const parsed = parseCssColor(color);
    if (parsed && parsed.a > MIN_VISIBLE_ALPHA) {
      return color;
    }
    current = current.parentElement;
  }
  return null;
}

export function measureFloatingBackdropTone(stack: HTMLElement): FloatingBackdropTone | null {
  const ownerDocument = stack.ownerDocument;
  if (typeof ownerDocument.elementsFromPoint !== 'function') {
    return null;
  }

  const controls = Array.from(
    stack.querySelectorAll<HTMLElement>(FLOATING_BACKDROP_CONTROL_SELECTOR),
  );
  const rects = controls.map(control => control.getBoundingClientRect());
  const colors: string[] = [];

  buildFloatingBackdropSamplePoints(rects).forEach(point => {
    const backdropElement = ownerDocument
      .elementsFromPoint(point.x, point.y)
      .find(element => !stack.contains(element));
    if (!backdropElement) {
      return;
    }

    const color = resolveElementBackgroundColor(backdropElement);
    if (color) {
      colors.push(color);
    }
  });

  return colors.length > 0 ? resolveFloatingBackdropTone(colors) : null;
}