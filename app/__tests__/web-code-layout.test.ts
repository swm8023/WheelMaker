import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses react-syntax-highlighter and react-diff-viewer without direct prismjs imports', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("from 'react-syntax-highlighter'");
    expect(mainTsx).toContain("from 'react-diff-viewer-continued'");
    expect(mainTsx).toContain('showLineNumbers={lineNumbers}');
    expect(mainTsx).toContain('wrapLongLines={wrap}');
    expect(mainTsx).toContain('wrapLines={wrap}');
    expect(mainTsx).toContain("codeTagProps={{style: {whiteSpace: wrap ? 'pre-wrap' : 'pre', background: 'transparent', fontFamily: VS_CODE_EDITOR_FONT_FAMILY, fontWeight: 400, fontVariantLigatures: 'none', fontFeatureSettings: '\"liga\" 0, \"calt\" 0'}}}");
    expect(mainTsx).toContain("lineProps={{style: {background: 'transparent', whiteSpace: wrap ? 'pre-wrap' : 'pre', wordBreak: wrap ? 'break-word' : 'normal', overflowWrap: wrap ? 'anywhere' : 'normal'}}}");
    expect(mainTsx).toContain('styles={getDiffViewerStyles(wrapLines)}');
    expect(mainTsx).toContain("whiteSpace: wrap ? 'pre-wrap' : 'pre'");
    expect(mainTsx).toContain('disableWordDiff={true}');
    expect(mainTsx).toContain('compareMethod={DiffMethod.LINES}');
    expect(mainTsx).toContain('linesOffset={linesOffset}');
    expect(mainTsx).toContain('renderContent={line => <PrismInlineCode content={line} language={language} wrap={wrapLines} />}');
    expect(mainTsx).toContain('showDiffOnly={foldContext}');
    expect(mainTsx).toContain('extraLinesSurroundingDiff={3}');
    expect(mainTsx).toContain('const [foldContext, setFoldContext] = useState(true);');
    expect(mainTsx).toContain('<span>Fold Context</span>');
    expect(mainTsx).toContain("overflow: 'visible'");
    expect(mainTsx).toContain("const VS_CODE_EDITOR_FONT_FAMILY = \"Consolas, 'Courier New', monospace\";");
    expect(mainTsx).toContain('fontFamily: VS_CODE_EDITOR_FONT_FAMILY');
    expect(mainTsx).not.toContain("@fontsource/jetbrains-mono");
    expect(mainTsx).not.toContain("from 'prismjs'");
    expect(mainTsx).not.toContain("import 'prismjs/");
  });
});
