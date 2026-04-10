import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses shiki renderer with transformer-based line metadata and keeps diff viewer integration', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const shikiRenderer = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'shikiRenderer.ts'), 'utf8');

    expect(mainTsx).toContain("import {renderShikiHtml} from './services/shikiRenderer'");
    expect(mainTsx).toContain("from 'react-diff-viewer-continued'");
    expect(mainTsx).toContain("mode: 'block'");
    expect(mainTsx).toContain("mode: 'inline'");
    expect(mainTsx).toContain('themeMode={themeMode}');
    expect(mainTsx).toContain('styles={getDiffViewerStyles(wrapLines)}');
    expect(mainTsx).toContain('disableWordDiff={true}');
    expect(mainTsx).toContain('compareMethod={DiffMethod.LINES}');
    expect(mainTsx).toContain('linesOffset={linesOffset}');
    expect(mainTsx).toContain('renderContent={line => <PrismInlineCode content={line} language={language} wrap={wrapLines} themeMode={themeMode} />}');
    expect(mainTsx).toContain('showDiffOnly={false}');
    expect(mainTsx).toContain("dangerouslySetInnerHTML={{__html: html || '<pre><code> </code></pre>'}}");
    expect(mainTsx).toContain("style={wrap ? {whiteSpace: 'pre-wrap', wordBreak: 'break-word', overflowWrap: 'anywhere'} : {whiteSpace: 'pre'}}");
    expect(shikiRenderer).toContain('buildLineTransformer(wrap, lineNumbers)');
    expect(shikiRenderer).toContain("hast.properties['data-line-number'] = String(line);");
    expect(shikiRenderer).toContain("transformerRenderWhitespace({position: 'boundary'})");
    expect(mainTsx).toContain("const VS_CODE_EDITOR_FONT_FAMILY = \"Consolas, 'Courier New', monospace\";");
    expect(mainTsx).toContain('fontFamily: VS_CODE_EDITOR_FONT_FAMILY');
    expect(mainTsx).not.toContain("@fontsource/jetbrains-mono");
    expect(mainTsx).not.toContain("from 'react-syntax-highlighter'");
    expect(mainTsx).not.toContain("from 'prismjs'");
    expect(mainTsx).not.toContain("import 'prismjs/");
  });
});
