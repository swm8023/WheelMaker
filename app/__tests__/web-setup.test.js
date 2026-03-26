const fs = require('fs');
const path = require('path');

describe('web runtime setup', () => {
  test('defines npm web script and webpack config entrypoint', () => {
    const projectRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(fs.readFileSync(path.join(projectRoot, 'package.json'), 'utf8'));

    expect(packageJson.scripts?.web).toBeDefined();
    expect(fs.existsSync(path.join(projectRoot, 'webpack.config.js'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'index.web.js'))).toBe(true);
  });
});
