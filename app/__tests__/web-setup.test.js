const fs = require('fs');
const path = require('path');

describe('web runtime setup', () => {
  test('defines web script and pure React web entrypoints', () => {
    const projectRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(
      fs.readFileSync(path.join(projectRoot, 'package.json'), 'utf8'),
    );

    expect(packageJson.scripts?.web).toBeDefined();
    expect(
      fs.existsSync(path.join(projectRoot, 'web', 'webpack.config.js')),
    ).toBe(true);
    expect(
      fs.existsSync(path.join(projectRoot, 'web', 'src', 'main.tsx')),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(projectRoot, 'web', 'public', 'runtime-config.js'),
      ),
    ).toBe(true);
  });

  test('includes pwa foundation modules and runtime integration', () => {
    const projectRoot = path.join(__dirname, '..');
    expect(
      fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'index.ts')),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(projectRoot, 'web', 'src', 'pwa', 'capabilities.ts'),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'storage.ts')),
    ).toBe(true);
    expect(
      fs.existsSync(
        path.join(projectRoot, 'web', 'src', 'pwa', 'connection.ts'),
      ),
    ).toBe(true);
    expect(
      fs.existsSync(path.join(projectRoot, 'web', 'src', 'pwa', 'push.ts')),
    ).toBe(true);

    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );
    expect(mainTsx).toMatch(
      /import\s+\{\s*initializePWAFoundation\s*\}\s+from\s+'\.\/pwa';/,
    );
    expect(mainTsx).toContain('initializePWAFoundation();');
  });

  test('service worker handles push and demo notification messages', () => {
    const projectRoot = path.join(__dirname, '..');
    const sw = fs.readFileSync(
      path.join(projectRoot, 'web', 'public', 'service-worker.js'),
      'utf8',
    );
    expect(sw).toContain("self.addEventListener('push'");
    expect(sw).toContain("self.addEventListener('notificationclick'");
    expect(sw).toContain("event.data?.type === 'WM_PWA_DEMO_NOTIFY'");
    expect(sw).toContain("event.data?.type === 'WM_PWA_NOTIFY'");
  });

  test('uses the current WheelMaker brand icon for PWA and desktop publishing', () => {
    const projectRoot = path.join(__dirname, '..');
    const iconPath = path.join(projectRoot, 'web', 'public', 'icons', 'icon.svg');
    const manifest = JSON.parse(
      fs.readFileSync(path.join(projectRoot, 'web', 'public', 'manifest.webmanifest'), 'utf8'),
    );
    const icon = fs.readFileSync(iconPath, 'utf8');

    expect(manifest.icons?.[0]?.src).toBe('/icons/icon.svg');
    expect(icon).toContain('id="appTile"');
    expect(icon).toContain('id="wmMarkBlue"');
    expect(icon).toContain('id="wmMarkWhite"');
    expect(icon).toContain('id="wmMarkTop"');
    expect(icon).toContain('rx="78"');
    expect(icon).toContain('filter="url(#tileShadow)"');
  });

  test('webpack output path can be redirected for desktop staging', () => {
    const projectRoot = path.join(__dirname, '..');
    const webpackConfigPath = path.join(projectRoot, 'web', 'webpack.config.js');
    const target = path.join(projectRoot, '..', 'server', 'cmd', 'wheelmaker-desktop', 'webroot');
    const previous = process.env.WHEELMAKER_WEB_TARGET;

    jest.resetModules();
    process.env.WHEELMAKER_WEB_TARGET = target;
    const redirected = require(webpackConfigPath);

    if (previous === undefined) {
      delete process.env.WHEELMAKER_WEB_TARGET;
    } else {
      process.env.WHEELMAKER_WEB_TARGET = previous;
    }
    jest.resetModules();
    const normal = require(webpackConfigPath);

    expect(redirected.output.path).toBe(path.resolve(target));
    expect(normal.output.path).toBe(path.join(require('os').homedir(), '.wheelmaker', 'web'));
  });
});
