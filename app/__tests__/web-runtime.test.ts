import fs from 'fs';
import path from 'path';

describe('web runtime defaults', () => {
  test('uses registry port 9630 for localhost ws default', () => {
    const projectRoot = path.join(__dirname, '..');
    const runtimeTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'runtime.ts'), 'utf8');

    expect(runtimeTs).toContain("return 'ws://127.0.0.1:9630/ws';");
    expect(runtimeTs).not.toContain("return 'ws://127.0.0.1:6930/ws';");
  });
});
