import type {CSSProperties} from 'react';

type BuildPrismCodeBlockConfigOptions = {
  tabSize: number;
  highlightLine?: number | null;
};

const VS_CODE_EDITOR_FONT_FAMILY = "Consolas, 'Courier New', monospace";

export function buildPrismCodeBlockConfig({tabSize, highlightLine = null}: BuildPrismCodeBlockConfigOptions) {
  const tabSizeStyle = String(tabSize);
  const codeStyle: CSSProperties = {
    background: 'transparent',
    fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
    fontWeight: 400,
    fontVariantLigatures: 'none',
    fontFeatureSettings: '"liga" 0, "calt" 0',
    tabSize: tabSizeStyle,
  };
  const customStyle: CSSProperties = {
    margin: 0,
    minWidth: '100%',
    background: 'transparent',
    padding: '0 10px',
    fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
    fontWeight: 400,
    fontVariantLigatures: 'none',
    fontFeatureSettings: '"liga" 0, "calt" 0',
  };
  const lineNumberStyle: CSSProperties = {
    fontFamily: VS_CODE_EDITOR_FONT_FAMILY,
    fontWeight: 400,
    color: 'var(--muted)',
    paddingRight: '10px',
    borderRight: '1px solid rgba(127, 127, 127, 0.18)',
    marginRight: '10px',
    textAlign: 'right',
    userSelect: 'none',
    fontVariantNumeric: 'tabular-nums',
    fontFeatureSettings: '"tnum" 1',
  };

  return {
    codeTagProps: {
      style: codeStyle,
    },
    lineProps: (lineNumber: number) => ({
      'data-line-number': String(lineNumber),
      style: {
        background: highlightLine === lineNumber ? 'rgba(0, 122, 204, 0.24)' : 'transparent',
      } as CSSProperties,
    }),
    customStyle,
    lineNumberStyle,
  };
}
