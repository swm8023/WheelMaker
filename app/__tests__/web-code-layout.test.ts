import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses shiki renderer with transformer-based line metadata and custom diff rendering', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const shikiRenderer = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'shikiRenderer.ts'), 'utf8');

    expect(mainTsx).toContain("from './services/shikiRenderer'");
    expect(mainTsx).toContain('renderShikiHtml');
    expect(mainTsx).toContain('renderShikiDiffHtml');
    expect(mainTsx).toContain("mode: 'block'");
    expect(mainTsx).toContain('themeMode={themeMode}');
    expect(mainTsx).toContain('codeTheme={codeTheme}');
    expect(mainTsx).toContain('type UnifiedDiffRow = {');
    expect(mainTsx).toContain('parseUnifiedDiffRows(content)');
    expect(mainTsx).toContain('Promise.all([');
    expect(mainTsx).toContain("className={`diff-grid ${wrap ? 'wrap' : 'nowrap'}`}");
    expect(mainTsx).toContain("className={`diff-side diff-old ${wrap ? 'wrap' : 'nowrap'}`}");
    expect(mainTsx).toContain("className={`diff-side diff-new ${wrap ? 'wrap' : 'nowrap'}`}");
    expect(mainTsx).toContain('codeFont={codeFont}');
    expect(mainTsx).toContain('codeFontSize={codeFontSize}');
    expect(mainTsx).toContain('codeLineHeight={codeLineHeight}');
    expect(mainTsx).toContain('codeTabSize={codeTabSize}');
    expect(mainTsx).toContain("dangerouslySetInnerHTML={{__html: html || '<pre><code> </code></pre>'}}");
    expect(shikiRenderer).toContain('transformers: [buildLineTransformer(');
    expect(shikiRenderer).toContain('export type DiffRenderLine = {');
    expect(shikiRenderer).toContain('export async function renderShikiDiffHtml');
    expect(shikiRenderer).toContain('data-line-kind');
    expect(shikiRenderer).toContain('wm-shiki-diff-line');
    expect(shikiRenderer).toContain("hast.properties['data-line-number'] = String(line);");
    expect(shikiRenderer).toContain('bundledThemesInfo');
    expect(shikiRenderer).toContain('CODE_FONT_OPTIONS');
    expect(shikiRenderer).toContain('resolveCodeFontFamily');
    expect(mainTsx).toContain("const VS_CODE_EDITOR_FONT_FAMILY = \"Consolas, 'Courier New', monospace\";");
    expect(mainTsx).toContain('codeFontFamily || VS_CODE_EDITOR_FONT_FAMILY');
    expect(mainTsx).not.toContain("from 'react-diff-viewer-continued'");
    expect(mainTsx).not.toContain('ReactDiffViewer');
    expect(mainTsx).not.toContain('<PrismInlineCode');
  });
});
