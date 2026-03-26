const fs = require('fs');
const path = require('path');

describe('react-native CLI setup', () => {
  test('declares community CLI plugin so start/run commands are registered', () => {
    const packageJsonPath = path.join(__dirname, '..', 'package.json');
    const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
    const version = packageJson.devDependencies?.['@react-native/community-cli-plugin'] ?? packageJson.dependencies?.['@react-native/community-cli-plugin'];

    expect(version).toBeDefined();
  });
});
