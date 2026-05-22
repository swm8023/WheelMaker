import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web/src/main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web/src/styles.css'), 'utf8');

describe('port relay settings UI source structure', () => {
  test('adds Port Relay as a settings detail in More', () => {
    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'portRelay'");
    expect(mainTsx).toContain('renderPortRelaySettingsDetail(options)');
    expect(mainTsx).toContain("setSettingsDetailView('portRelay')");

    const moreStart = mainTsx.indexOf("renderSettingsSection('More'");
    const moreEnd = mainTsx.indexOf("renderSettingsSection(", moreStart + 1);
    const moreSection = mainTsx.slice(moreStart, moreEnd > moreStart ? moreEnd : undefined);
    expect(moreSection.indexOf("setSettingsDetailView('database')")).toBeLessThan(moreSection.indexOf("setSettingsDetailView('portRelay')"));
    expect(moreSection.indexOf("setSettingsDetailView('portRelay')")).toBeLessThan(moreSection.indexOf('requestClearLocalCache'));
  });

  test('renders Port Relay controls and service hooks', () => {
    expect(mainTsx).toContain('const renderPortRelaySettingsDetail = (options?: SettingsDetailShellOptions) =>');
    expect(mainTsx).toContain('refreshPortRelayStatus');
    expect(mainTsx).toContain('service.getPortRelayStatus');
    expect(mainTsx).toContain('service.enablePortRelay');
    expect(mainTsx).toContain('service.disablePortRelay');
    expect(mainTsx).toContain('service.regeneratePortRelayAccessCode');
    expect(mainTsx).toContain('generatePortRelayAccessCode');
    expect(mainTsx).toContain('buildPortRelayOpenUrl(address, portRelayListenPort)');
    expect(mainTsx).toContain('window.open(openUrl, \'_blank\', \'noopener,noreferrer\')');

    expect(stylesCss).toContain('.port-relay-panel');
    expect(stylesCss).toContain('.port-relay-form-grid');
    expect(stylesCss).toContain('.port-relay-code-row');
    expect(stylesCss).toContain('.port-relay-status-pill');
  });
});
