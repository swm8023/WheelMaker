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
    const icon = fs.readFileSync(iconPath);
    const serviceWorker = fs.readFileSync(
      path.join(projectRoot, 'web', 'public', 'service-worker.js'),
      'utf8',
    );
    const indexHtml = fs.readFileSync(
      path.join(projectRoot, 'web', 'public', 'index.html'),
      'utf8',
    );

    expect(manifest.icons?.[0]).toMatchObject({
      src: '/icons/icon.svg',
      sizes: '1536x1536',
      type: 'image/svg+xml',
      purpose: 'any maskable',
    });
    const iconText = icon.toString('utf8');
    expect(iconText).toContain('<svg');
    expect(iconText).toContain('viewBox="0 0 1536 1536"');
    expect(serviceWorker).toContain('/icons/icon.svg');
    expect(serviceWorker).not.toContain('/icons/icon.png');
    expect(indexHtml).toContain('href="/icons/icon.svg"');
    expect(indexHtml).not.toContain('href="/icons/icon.png"');
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

  test('webpack does not emit bundle size performance warnings for local PWA releases', () => {
    const projectRoot = path.join(__dirname, '..');
    const webpackConfig = require(path.join(projectRoot, 'web', 'webpack.config.js'));

    expect(webpackConfig.performance).toEqual({hints: false});
  });
});
