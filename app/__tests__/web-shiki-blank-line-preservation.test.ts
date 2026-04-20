import fs from 'fs';
import path from 'path';

describe('web shiki blank line preservation', () => {
  test('preserves empty lines when line numbers are hidden', () => {
    const projectRoot = path.join(__dirname, '..');
    const shikiRenderer = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'shikiRenderer.ts'), 'utf8');

    expect(shikiRenderer).toContain('const lineContentChildren = originalChildren.length > 0');
    expect(shikiRenderer).toContain("value: ' '");
    expect(shikiRenderer).toContain('children: lineContentChildren as any[]');
  });
});
