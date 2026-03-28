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
    expect(mainTsx).toContain("codeTagProps={{style: {whiteSpace: wrap ? 'pre-wrap' : 'pre', background: 'transparent'}}}");
    expect(mainTsx).toContain("lineProps={{style: {background: 'transparent', whiteSpace: wrap ? 'pre-wrap' : 'pre', wordBreak: wrap ? 'break-word' : 'normal', overflowWrap: wrap ? 'anywhere' : 'normal'}}}");
    expect(mainTsx).toContain('styles={getDiffViewerStyles(wrapLines)}');
    expect(mainTsx).toContain("whiteSpace: wrap ? 'pre-wrap' : 'pre'");
    expect(mainTsx).not.toContain("from 'prismjs'");
    expect(mainTsx).not.toContain("import 'prismjs/");
  });
});
