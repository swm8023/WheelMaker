const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const { execFileSync } = require('child_process');

const root = path.resolve(__dirname, '..');
const webPublic = path.join(root, 'web', 'public');
const target = process.env.WHEELMAKER_WEB_TARGET || path.join(os.homedir(), '.wheelmaker', 'web');

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function copyFileIfExists(source, dest) {
  if (!fs.existsSync(source)) return;
  ensureDir(path.dirname(dest));
  fs.copyFileSync(source, dest);
}

function copyDir(source, dest) {
  if (!fs.existsSync(source)) return;
  ensureDir(dest);
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    const sourcePath = path.join(source, entry.name);
    const destPath = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDir(sourcePath, destPath);
    } else if (entry.isFile()) {
      copyFileIfExists(sourcePath, destPath);
    }
  }
}

function gitValue(args) {
  try {
    return execFileSync('git', args, {
      cwd: path.resolve(root, '..'),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    return '';
  }
}

function hashFile(filePath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filePath));
  return `sha256:${hash.digest('hex')}`;
}

function collectAssetHashes(dir) {
  const assets = {};
  if (!fs.existsSync(dir)) return assets;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isFile()) continue;
    if (!/^bundle\..+\.(js|css)$/.test(entry.name)) continue;
    assets[entry.name] = hashFile(path.join(dir, entry.name));
  }
  return assets;
}

function writeBuildManifest(dir) {
  const manifest = {
    schemaVersion: 1,
    sha: process.env.WHEELMAKER_WEB_BUILD_SHA || gitValue(['rev-parse', 'HEAD']),
    builtAt: process.env.WHEELMAKER_WEB_BUILD_TIME || new Date().toISOString(),
    assets: collectAssetHashes(dir),
  };
  fs.writeFileSync(
    path.join(dir, 'web-build.json'),
    `${JSON.stringify(manifest, null, 2)}\n`,
    'utf8',
  );
}

ensureDir(target);
copyFileIfExists(path.join(webPublic, 'manifest.webmanifest'), path.join(target, 'manifest.webmanifest'));
copyFileIfExists(path.join(webPublic, 'service-worker.js'), path.join(target, 'service-worker.js'));
copyDir(path.join(webPublic, 'icons'), path.join(target, 'icons'));
writeBuildManifest(target);

console.log(`Exported web release to ${target}`);
