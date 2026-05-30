import fs from 'fs';
import path from 'path';

describe('connection settings UI source structure', () => {
  test('adds a Connection section with status detail and local hub read settings', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("'connectionStatus'");
    expect(mainTsx).toContain("case 'connectionStatus':");
    expect(mainTsx).toContain('renderConnectionStatusSettingsDetail(options)');
    expect(mainTsx).toContain("renderSettingsSection('Connection'");
    expect(mainTsx).toContain("openSettingsDetail('connectionStatus')");
    expect(mainTsx).toContain('Connection Status');
    expect(mainTsx).toContain('Local Hub Read');
    expect(mainTsx).toContain('checked={localHubReadEnabled}');

    const chatSectionStart = mainTsx.indexOf("renderSettingsSection('Chat'");
    const connectionSectionStart = mainTsx.indexOf("renderSettingsSection('Connection'");
    const codeSectionStart = mainTsx.indexOf("renderSettingsSection('Code Display'");
    const connectionSection = mainTsx.slice(connectionSectionStart, codeSectionStart);
    const localHubReadIndex = connectionSection.indexOf('Local Hub Read');
    const connectionStatusIndex = connectionSection.indexOf('Connection Status');

    expect(connectionSectionStart).toBeGreaterThan(chatSectionStart);
    expect(connectionSectionStart).toBeLessThan(codeSectionStart);
    expect(localHubReadIndex).toBeGreaterThanOrEqual(0);
    expect(connectionStatusIndex).toBeGreaterThanOrEqual(0);
    expect(localHubReadIndex).toBeLessThan(connectionStatusIndex);
  });
});
