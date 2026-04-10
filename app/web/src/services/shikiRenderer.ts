import {createHighlighter, createJavaScriptRegexEngine, type Highlighter, type ShikiTransformer} from 'shiki';
import {transformerRenderWhitespace} from '@shikijs/transformers';

type ThemeMode = 'dark' | 'light';
type RenderMode = 'block' | 'inline';

type RenderShikiOptions = {
  code: string;
  language: string;
  themeMode: ThemeMode;
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

let highlighterPromise: Promise<Highlighter> | null = null;

function resolveTheme(themeMode: ThemeMode): string {
  return themeMode === 'light' ? SHIKI_THEME_LIGHT : SHIKI_THEME_DARK;
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

function buildLineTransformer(wrap: boolean, lineNumbers: boolean): ShikiTransformer {
  const lineContentStyle = wrap
    ? 'display:block;min-width:0;white-space:pre-wrap;word-break:break-word;overflow-wrap:anywhere;'
    : 'display:block;min-width:0;white-space:pre;';

  return {
    name: 'wm-line-layout',
    pre(hast) {
      this.addClassToHast(hast, ['wm-shiki-pre', wrap ? 'wm-shiki-wrap' : 'wm-shiki-nowrap']);
      appendStyle(hast, `margin:0;padding:0;border-radius:0;overflow-x:${wrap ? 'hidden' : 'auto'};`);
    },
    code(hast) {
      this.addClassToHast(hast, 'wm-shiki-code');
      appendStyle(hast, wrap ? 'display:block;min-width:100%;' : 'display:block;min-width:100%;width:max-content;');
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

function getInlineCacheKey(options: RenderShikiOptions, lang: string, theme: string): string {
  return `${theme}|${lang}|${options.wrap ? 1 : 0}|${options.code}`;
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
  mode: RenderMode,
): string {
  const normalizedCode = code || ' ';
  const whitespaceTransformer = transformerRenderWhitespace({position: 'boundary'});
  if (mode === 'inline') {
    return highlighter.codeToHtml(normalizedCode, {
      lang,
      theme,
      structure: 'inline',
      transformers: [whitespaceTransformer],
    });
  }
  return highlighter.codeToHtml(normalizedCode, {
    lang,
    theme,
    structure: 'classic',
    transformers: [whitespaceTransformer, buildLineTransformer(wrap, lineNumbers)],
  });
}

export async function renderShikiHtml(options: RenderShikiOptions): Promise<string> {
  const language = resolveLanguage(options.language);
  const theme = resolveTheme(options.themeMode);
  const highlighter = await getHighlighter();
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
