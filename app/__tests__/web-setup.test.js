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

  test('includes pwa foundation modules and runtime integration', () => {
    const projectRoot = path.join(__dirname, '..');
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'index.ts'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'capabilities.ts'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'storage.ts'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'connection.ts'))).toBe(true);
    expect(fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'push.ts'))).toBe(true);

    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    expect(mainTsx).toContain("import {initializePWAFoundation} from './pwa';");
    expect(mainTsx).toContain('initializePWAFoundation();');
    expect(mainTsx).toContain('PWA Debug');
    expect(mainTsx).toContain('Check Capabilities');
    expect(mainTsx).toContain('Test Storage');
    expect(mainTsx).toContain('Test Push Notification');
    expect(mainTsx).toContain('Start Connection Probe');
  });

  test('service worker handles push and demo notification messages', () => {
    const projectRoot = path.join(__dirname, '..');
    const sw = fs.readFileSync(path.join(projectRoot, 'web', 'public', 'service-worker.js'), 'utf8');
    expect(sw).toContain("self.addEventListener('push'");
    expect(sw).toContain("self.addEventListener('notificationclick'");
    expect(sw).toContain("event.data?.type === 'WM_PWA_DEMO_NOTIFY'");
  });
});