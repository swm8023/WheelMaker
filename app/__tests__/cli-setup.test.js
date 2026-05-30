const fs = require('fs');
const path = require('path');

const projectRoot = path.join(__dirname, '..');
const packageJsonPath = path.join(projectRoot, 'package.json');
const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));

describe('web app package setup', () => {
  test('does not declare React Native shell commands', () => {
    const nativeScripts = ['android', 'android:web', 'build:web:native', 'ios'];

    for (const scriptName of nativeScripts) {
      expect(packageJson.scripts).not.toHaveProperty(scriptName);
    }
    expect(packageJson.scripts?.start ?? '').not.toContain('react-native');
  });

  test('does not declare React Native runtime or CLI packages', () => {
    const dependencies = {
      ...(packageJson.dependencies ?? {}),
      ...(packageJson.devDependencies ?? {}),
    };
    const forbiddenPackageNames = Object.keys(dependencies).filter(
      packageName =>
        packageName === 'react-native' ||
        packageName.startsWith('react-native-') ||
        packageName.startsWith('@react-native/') ||
        packageName.startsWith('@react-native-community/cli'),
    );

    expect(forbiddenPackageNames).toEqual([]);
  });

  test('does not keep React Native shell project files in the web app workspace', () => {
    const nativePaths = [
      path.join('android'),
      path.join('ios'),
      'App.tsx',
      'App.native.tsx',
      'index.js',
      'metro.config.js',
      'babel.config.js',
      'app.json',
      path.join('scripts', 'sync_web_assets.ps1'),
    ];
    const existingPaths = nativePaths.filter(nativePath =>
      fs.existsSync(path.join(projectRoot, nativePath)),
    );

    expect(existingPaths).toEqual([]);
  });
});
