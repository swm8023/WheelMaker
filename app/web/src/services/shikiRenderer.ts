import {createHighlighter, createJavaScriptRegexEngine, type Highlighter, type ShikiTransformer} from 'shiki';

type ThemeMode = 'dark' | 'light';
type RenderMode = 'block' | 'inline';
export type CodeThemeId =
  | 'auto-plus'
  | 'dark-plus'
  | 'light-plus'
  | 'material-theme-darker'
  | 'github-dark'
  | 'github-light'
  | 'nord'
  | 'vitesse-dark'
  | 'vitesse-light';

export type CodeFontId =
  | 'consolas'
  | 'jetbrains-mono'
  | 'cascadia'
  | 'menlo';

export type CodeThemeOption = {
  id: CodeThemeId;
  label: string;
};

export type CodeFontOption = {
  id: CodeFontId;
  label: string;
  fontFamily: string;
};

export const CODE_THEME_OPTIONS: CodeThemeOption[] = [
  {id: 'auto-plus', label: 'Auto (Dark+/Light+)'},
  {id: 'dark-plus', label: 'Dark Plus'},
  {id: 'light-plus', label: 'Light Plus'},
  {id: 'material-theme-darker', label: 'Material Theme Darker'},
  {id: 'github-dark', label: 'GitHub Dark'},
  {id: 'github-light', label: 'GitHub Light'},
  {id: 'nord', label: 'Nord'},
  {id: 'vitesse-dark', label: 'Vitesse Dark'},
  {id: 'vitesse-light', label: 'Vitesse Light'},
];
export const DEFAULT_CODE_THEME: CodeThemeId = 'auto-plus';
export const DEFAULT_CODE_FONT: CodeFontId = 'consolas';
export const DEFAULT_CODE_FONT_SIZE = 13;
export const DEFAULT_CODE_LINE_HEIGHT = 1.5;
export const DEFAULT_CODE_TAB_SIZE = 2;
export const CODE_FONT_OPTIONS: CodeFontOption[] = [
  {id: 'consolas', label: 'Consolas', fontFamily: "Consolas, 'Courier New', monospace"},
  {id: 'jetbrains-mono', label: 'JetBrains Mono', fontFamily: "'JetBrains Mono', Consolas, 'Courier New', monospace"},
  {id: 'cascadia', label: 'Cascadia Mono', fontFamily: "'Cascadia Mono', Consolas, 'Courier New', monospace"},
  {id: 'menlo', label: 'Menlo / Monaco', fontFamily: "Menlo, Monaco, Consolas, 'Courier New', monospace"},
];

type RenderShikiOptions = {
  code: string;
  language: string;
  themeMode: ThemeMode;
  codeTheme: CodeThemeId;
  codeFont: CodeFontId;
  codeFontSize: number;
  codeLineHeight: number;
  codeTabSize: number;
  wrap: boolean;
  lineNumbers: boolean;
  mode: RenderMode;
};

const SHIKI_THEME_DARK = 'dark-plus';
const SHIKI_THEME_LIGHT = 'light-plus';
const SHIKI_LANGS = [
  'text',
  'plaintext',
  'typescript',
  'tsx',
  'javascript',
  'jsx',
  'json',
  'go',
  'c',
  'cpp',
  'rust',
  'bash',
  'shellscript',
  'yaml',
  'markdown',
  'diff',
  'html',
];
const INLINE_CACHE_LIMIT = 4000;
const inlineCache = new Map<string, string>();
const loadedThemes = new Set<string>([SHIKI_THEME_DARK, SHIKI_THEME_LIGHT]);

let highlighterPromise: Promise<Highlighter> | null = null;

function resolveTheme(themeMode: ThemeMode, codeTheme: CodeThemeId): string {
  if (codeTheme === 'auto-plus') {
    return themeMode === 'light' ? SHIKI_THEME_LIGHT : SHIKI_THEME_DARK;
  }
  return codeTheme;
}

export function isCodeThemeId(value: string): value is CodeThemeId {
  return CODE_THEME_OPTIONS.some(item => item.id === value);
}

export function isCodeFontId(value: string): value is CodeFontId {
  return CODE_FONT_OPTIONS.some(item => item.id === value);
}

export function resolveCodeFontFamily(codeFont: CodeFontId): string {
  return CODE_FONT_OPTIONS.find(item => item.id === codeFont)?.fontFamily ?? CODE_FONT_OPTIONS[0].fontFamily;
}

function resolveLanguage(language: string): string {
  const normalized = (language || '').trim().toLowerCase();
  switch (normalized) {
    case 'clike':
      return 'c';
    case 'markup':
      return 'html';
    default:
      return normalized || 'text';
  }
}

function appendStyle(node: any, styleText: string): void {
  if (!styleText) return;
  const current = typeof node?.properties?.style === 'string' ? node.properties.style.trim() : '';
  node.properties = node.properties || {};
  node.properties.style = current ? `${current};${styleText}` : styleText;
}

function buildLineTransformer(
  wrap: boolean,
  lineNumbers: boolean,
  codeFont: CodeFontId,
  codeFontSize: number,
  codeLineHeight: number,
  codeTabSize: number,
): ShikiTransformer {
  const fontFamily = resolveCodeFontFamily(codeFont);
  const fontSize = `${codeFontSize}px`;
  const lineContentStyle = wrap
    ? `display:block;min-width:0;white-space:pre-wrap;word-break:break-word;overflow-wrap:anywhere;tab-size:${codeTabSize};font-family:${fontFamily};font-size:${fontSize};line-height:${codeLineHeight};`
    : `display:block;min-width:0;white-space:pre;tab-size:${codeTabSize};font-family:${fontFamily};font-size:${fontSize};line-height:${codeLineHeight};`;

  return {
    name: 'wm-line-layout',
    pre(hast) {
      this.addClassToHast(hast, ['wm-shiki-pre', wrap ? 'wm-shiki-wrap' : 'wm-shiki-nowrap']);
      appendStyle(hast, `margin:0;padding:0;border-radius:0;white-space:normal;overflow-x:${wrap ? 'hidden' : 'auto'};font-family:${fontFamily};font-size:${fontSize};line-height:${codeLineHeight};`);
    },
    code(hast) {
      this.addClassToHast(hast, 'wm-shiki-code');
      appendStyle(hast, wrap ? `display:block;min-width:100%;tab-size:${codeTabSize};` : `display:block;min-width:100%;width:max-content;tab-size:${codeTabSize};`);
    },
    line(hast, line) {
      const originalChildren = Array.isArray(hast.children) ? hast.children : [];
      const contentNode = {
        type: 'element' as const,
        tagName: 'span',
        properties: {
          className: ['wm-shiki-line-content'],
          style: lineContentStyle,
        },
        children: originalChildren as any[],
      };

      hast.properties = hast.properties || {};
      hast.properties['data-line'] = String(line);
      hast.properties['data-line-number'] = String(line);

      if (lineNumbers) {
        const lineNumberNode = {
          type: 'element' as const,
          tagName: 'span',
          properties: {
            className: ['wm-shiki-line-number'],
            'aria-hidden': 'true',
            style: 'display:inline-block;min-width:3.5em;padding-right:1em;text-align:right;user-select:none;color:var(--muted);opacity:0.75;',
          },
          children: [{type: 'text' as const, value: String(line)}],
        };
        hast.children = [lineNumberNode as any, contentNode as any];
        appendStyle(hast, 'display:grid;grid-template-columns:auto minmax(0,1fr);align-items:start;');
      } else {
        hast.children = [contentNode as any];
      }

      return hast;
    },
  };
}

function escapeHtml(raw: string): string {
  return raw
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}

async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      engine: createJavaScriptRegexEngine(),
      themes: [SHIKI_THEME_DARK, SHIKI_THEME_LIGHT],
      langs: SHIKI_LANGS,
    });
  }
  return highlighterPromise;
}

async function ensureThemeLoaded(highlighter: Highlighter, theme: string): Promise<void> {
  if (loadedThemes.has(theme)) return;
  await highlighter.loadTheme(theme as any);
  loadedThemes.add(theme);
}

function getInlineCacheKey(options: RenderShikiOptions, lang: string, theme: string): string {
  return `${theme}|${lang}|${options.wrap ? 1 : 0}|${options.codeFont}|${options.codeFontSize}|${options.codeLineHeight}|${options.codeTabSize}|${options.code}`;
}

function setInlineCache(key: string, value: string): void {
  inlineCache.set(key, value);
  if (inlineCache.size <= INLINE_CACHE_LIMIT) return;
  const oldest = inlineCache.keys().next().value as string | undefined;
  if (oldest) inlineCache.delete(oldest);
}

function renderWithHighlighter(
  highlighter: Highlighter,
  code: string,
  lang: string,
  theme: string,
  wrap: boolean,
  lineNumbers: boolean,
  codeFont: CodeFontId,
  codeFontSize: number,
  codeLineHeight: number,
  codeTabSize: number,
  mode: RenderMode,
): string {
  const normalizedCode = code || ' ';
  if (mode === 'inline') {
    return highlighter.codeToHtml(normalizedCode, {
      lang,
      theme,
      structure: 'inline',
    });
  }
  return highlighter.codeToHtml(normalizedCode, {
    lang,
    theme,
    structure: 'classic',
    transformers: [buildLineTransformer(
      wrap,
      lineNumbers,
      codeFont,
      codeFontSize,
      codeLineHeight,
      codeTabSize,
    )],
  });
}

export async function renderShikiHtml(options: RenderShikiOptions): Promise<string> {
  const language = resolveLanguage(options.language);
  const theme = resolveTheme(options.themeMode, options.codeTheme);
  const highlighter = await getHighlighter();
  try {
    await ensureThemeLoaded(highlighter, theme);
  } catch {
    // fall through with already loaded default themes
  }
  const langCandidates = language === 'text' ? ['text'] : [language, 'text'];

  let inlineCacheKey = '';
  if (options.mode === 'inline') {
    inlineCacheKey = getInlineCacheKey(options, language, theme);
    const cached = inlineCache.get(inlineCacheKey);
    if (cached) return cached;
  }

  for (const lang of langCandidates) {
    try {
      const html = renderWithHighlighter(
        highlighter,
        options.code,
        lang,
        theme,
        options.wrap,
        options.lineNumbers,
        options.codeFont,
        options.codeFontSize,
        options.codeLineHeight,
        options.codeTabSize,
        options.mode,
      );
      if (options.mode === 'inline') {
        setInlineCache(inlineCacheKey, html);
      }
      return html;
    } catch {
      // Try fallback language.
    }
  }

  return `<span>${escapeHtml(options.code || ' ')}</span>`;
}
