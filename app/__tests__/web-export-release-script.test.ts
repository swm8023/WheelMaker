import fs from 'fs';
import os from 'os';
import path from 'path';
import { spawnSync } from 'child_process';

describe('web release exporter', () => {
  test('copies public PWA assets to the configured release target', () => {
    const appRoot = path.join(__dirname, '..');
    const target = fs.mkdtempSync(path.join(os.tmpdir(), 'wheelmaker-web-release-'));
    const script = path.join(appRoot, 'scripts', 'export_web_release.js');

    const result = spawnSync(process.execPath, [script], {
      cwd: appRoot,
      env: { ...process.env, WHEELMAKER_WEB_TARGET: target },
      encoding: 'utf8',
    });

    expect(result.status).toBe(0);
    expect(fs.existsSync(path.join(target, 'manifest.webmanifest'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'service-worker.js'))).toBe(true);
    expect(fs.existsSync(path.join(target, 'icons', 'icon.png'))).toBe(true);
  });

  test('release script is wired through npm without powershell', () => {
    const appRoot = path.join(__dirname, '..');
    const packageJson = JSON.parse(fs.readFileSync(path.join(appRoot, 'package.json'), 'utf8'));

    expect(packageJson.scripts['build:web:release']).toBe(
      'npm run build:web && node scripts/export_web_release.js',
    );
  });
});
