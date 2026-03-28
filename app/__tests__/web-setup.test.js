const fs = require('fs');
const path = require('path');

describe('web runtime setup', () => {
  test('defines web script and pure React web entrypoints', () => {
    const projectRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(fs.readFileSync(path.join(projectRoot, 'package.json'), 'utf8'));

    expect(packageJson.scripts?.web).toBeDefined();
    expect(fs.existsSync(path.join(projectRoot, 'web', 'webpack.config.js'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'main.tsx'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'public', 'runtime-config.js'))).toBe(true);
  });
});
