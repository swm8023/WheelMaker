const fs = require('fs');
const os = require('os');
const path = require('path');

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

ensureDir(target);
copyFileIfExists(path.join(webPublic, 'manifest.webmanifest'), path.join(target, 'manifest.webmanifest'));
copyFileIfExists(path.join(webPublic, 'service-worker.js'), path.join(target, 'service-worker.js'));
copyDir(path.join(webPublic, 'icons'), path.join(target, 'icons'));

console.log(`Exported web release to ${target}`);
