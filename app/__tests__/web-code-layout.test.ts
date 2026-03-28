import fs from 'fs';
import path from 'path';

describe('web code layout', () => {
  test('uses prism line numbers plugin instead of custom line-number grid', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("import 'prismjs/plugins/line-numbers/prism-line-numbers.css';");
    expect(mainTsx).toContain("import 'prismjs/plugins/line-numbers/prism-line-numbers';");
    expect(mainTsx).toContain("lineNumbers ? 'line-numbers' : ''");
    expect(mainTsx).not.toContain('className="line-number"');
    expect(mainTsx).not.toContain('className={`code-grid');
  });
});
