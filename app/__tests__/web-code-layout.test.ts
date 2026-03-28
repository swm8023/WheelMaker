import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses react-syntax-highlighter for file code rendering', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("from 'react-syntax-highlighter'");
    expect(mainTsx).toContain('showLineNumbers={lineNumbers}');
    expect(mainTsx).toContain('wrapLongLines={wrap}');
    expect(mainTsx).not.toContain("prismjs/plugins/line-numbers/prism-line-numbers");
  });
});
