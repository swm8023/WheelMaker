import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses shiki renderer with transformer-based line metadata and keeps diff viewer integration', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const shikiRenderer = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'shikiRenderer.ts'), 'utf8');

    expect(mainTsx).toContain("from './services/shikiRenderer'");
    expect(mainTsx).toContain('renderShikiHtml');
    expect(mainTsx).toContain("from 'react-diff-viewer-continued'");
    expect(mainTsx).toContain("mode: 'block'");
    expect(mainTsx).toContain("mode: 'inline'");
    expect(mainTsx).toContain('themeMode={themeMode}');
    expect(mainTsx).toContain('codeTheme={codeTheme}');
    expect(mainTsx).toContain('styles={getDiffViewerStyles(wrapLines, codeFontFamily, codeFontSize, codeLineHeight, codeTabSize)}');
    expect(mainTsx).toContain('disableWordDiff={true}');
    expect(mainTsx).toContain('compareMethod={DiffMethod.LINES}');
    expect(mainTsx).toContain('linesOffset={linesOffset}');
    expect(mainTsx).toContain('renderContent={line => (');
    expect(mainTsx).toContain('<PrismInlineCode');
    expect(mainTsx).toContain('codeFont={codeFont}');
    expect(mainTsx).toContain('codeFontSize={codeFontSize}');
    expect(mainTsx).toContain('codeLineHeight={codeLineHeight}');
    expect(mainTsx).toContain('codeTabSize={codeTabSize}');
    expect(mainTsx).toContain('showDiffOnly={false}');
    expect(mainTsx).toContain("dangerouslySetInnerHTML={{__html: html || '<pre><code> </code></pre>'}}");
    expect(mainTsx).toContain("style={wrap");
    expect(mainTsx).toContain("fontFamily: resolveCodeFontFamily(codeFont)");
    expect(shikiRenderer).toContain('transformers: [buildLineTransformer(');
    expect(shikiRenderer).toContain("hast.properties['data-line-number'] = String(line);");
    expect(shikiRenderer).toContain('bundledThemesInfo');
    expect(shikiRenderer).toContain('CODE_FONT_OPTIONS');
    expect(shikiRenderer).toContain('resolveCodeFontFamily');
    expect(mainTsx).toContain("const VS_CODE_EDITOR_FONT_FAMILY = \"Consolas, 'Courier New', monospace\";");
    expect(mainTsx).toContain('codeFontFamily || VS_CODE_EDITOR_FONT_FAMILY');
    expect(mainTsx).not.toContain("from 'react-syntax-highlighter'");
    expect(mainTsx).not.toContain("from 'prismjs'");
    expect(mainTsx).not.toContain("import 'prismjs/");
  });
});
