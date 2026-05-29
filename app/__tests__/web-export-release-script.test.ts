import fs from 'fs';
import os from 'os';
import path from 'path';
import { spawnSync } from 'child_process';

describe('web release exporter', () => {
  test('copies public PWA assets to the configured release target', () => {
    const appRoot = path.join(__dirname, '..');
    const target = fs.mkdtempSync(path.join(os.tmpdir(), 'wheelmaker-web-release-'));
    const script = path.join(appRoot, 'scripts', 'export_web_release.js');
    fs.writeFileSync(path.join(target, 'bundle.test.js'), "console.log('fresh');");
    fs.writeFileSync(path.join(target, 'bundle.test.css'), 'body{color:#111;}');

    const result = spawnSync(process.execPath, [script], {
      cwd: appRoot,
      env: {
        ...process.env,
        WHEELMAKER_WEB_TARGET: target,
        WHEELMAKER_WEB_BUILD_SHA: 'sha-for-test',
        WHEELMAKER_WEB_BUILD_TIME: '2026-05-29T00:00:00.000Z',
      },
      encoding: 'utf8',
    });

    expect(result.status).toBe(0);
    expect(fs.existsSync(path.join(target, 'manifest.webmanifest'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'service-worker.js'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'icons', 'icon.svg'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'icons', 'icon.png'))).toBe(false);

    const buildManifest = JSON.parse(fs.readFileSync(path.join(target, 'web-build.json'), 'utf8'));
    expect(buildManifest).toMatchObject({
      schemaVersion: 1,
      sha: 'sha-for-test',
      builtAt: '2026-05-29T00:00:00.000Z',
    });
    expect(buildManifest.assets['bundle.test.js']).toMatch(/^sha256:[a-f0-9]{64}$/);
    expect(buildManifest.assets['bundle.test.css']).toMatch(/^sha256:[a-f0-9]{64}$/);
  });

  test('release script is wired through npm without powershell', () => {
    const appRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(fs.readFileSync(path.join(appRoot, 'package.json'), 'utf8'));

    expect(packageJson.scripts['build:web:release']).toBe(
      'npm run build:web && node scripts/export_web_release.js',
    );
  });
});
