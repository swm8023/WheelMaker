import fs from 'fs';
import path from 'path';

describe('agent package update settings UI source structure', () => {
  test('adds Update to More settings and keeps Chat focused on chat options', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'tokenStats' | 'ccSwitch' | 'database' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'update'");
    expect(mainTsx).toContain('renderUpdateSettingsDetail()');
    expect(mainTsx).toContain("renderSettingsSection('More'");
    expect(mainTsx).not.toContain("renderSettingsSection('Storage'");

    const chatStart = mainTsx.indexOf("renderSettingsSection('Chat'");
    const codeDisplayStart = mainTsx.indexOf("renderSettingsSection('Code Display'", chatStart);
    expect(chatStart).toBeGreaterThanOrEqual(0);
    expect(codeDisplayStart).toBeGreaterThan(chatStart);
    const chatSection = mainTsx.slice(chatStart, codeDisplayStart);
    expect(chatSection).toContain('<span>Use Latest Prompt Title</span>');
    expect(chatSection).toContain('<span>Hide Tool Calls</span>');
    expect(chatSection).not.toContain('Token Stats');
    expect(chatSection).not.toContain('CC Switch');

    const moreStart = mainTsx.indexOf("renderSettingsSection('More'");
    expect(moreStart).toBeGreaterThan(codeDisplayStart);
    const moreSection = mainTsx.slice(moreStart);
    const updateIndex = moreSection.indexOf("setSettingsDetailView('update')");
    const tokenStatsIndex = moreSection.indexOf("setSettingsDetailView('tokenStats')");
    const ccSwitchIndex = moreSection.indexOf("setSettingsDetailView('ccSwitch')");
    const databaseIndex = moreSection.indexOf("setSettingsDetailView('database')");
    const clearCacheIndex = moreSection.indexOf('requestClearLocalCache');
    expect(updateIndex).toBeGreaterThanOrEqual(0);
    expect(updateIndex).toBeLessThan(tokenStatsIndex);
    expect(tokenStatsIndex).toBeLessThan(ccSwitchIndex);
    expect(ccSwitchIndex).toBeLessThan(databaseIndex);
    expect(databaseIndex).toBeLessThan(clearCacheIndex);
  });

  test('renders Update detail with scan, task polling, and npm confirmation flow hooks', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const renderUpdateSettingsDetail = () =>');
    expect(mainTsx).toContain("'Update'");
    expect(mainTsx).toContain('Agent Packages');
    expect(mainTsx).toContain('refreshAgentPackages');
    expect(mainTsx).toContain('deriveAgentPackageHubIds');
    expect(mainTsx).toContain('withAgentPackageTimeout(');
    expect(mainTsx).toContain('service.scanNpmPackages');
    expect(mainTsx).toContain('service.installNpmPackage');
    expect(mainTsx).toContain('service.uninstallNpmPackage');
    expect(mainTsx).toContain('service.queryNpmPackageTask');
    expect(mainTsx).toContain("kind: 'npmPackage'");
    expect(mainTsx).toContain('requestAgentPackageAction');
    expect(mainTsx).toContain('handleAgentPackageConfirmedAction');
    expect(mainTsx).toContain('packageStatusLabel');

    expect(stylesCss).toContain('.agent-package-hub-list');
    expect(stylesCss).toContain('.agent-package-row');
    expect(stylesCss).toContain('.agent-package-action-btn');
  });

  test('adds desktop Update and Token Stats shortcuts below refresh and above settings only', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
    const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
    expect(activityBarStart).toBeGreaterThanOrEqual(0);
    expect(activityBarEnd).toBeGreaterThan(activityBarStart);
    const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);

    expect(activityBar).toContain('codicon-cloud-download');
    expect(activityBar).toContain('codicon-pulse');
    expect(activityBar).toContain("openSettingsDetail('update')");
    expect(activityBar).toContain("openSettingsDetail('tokenStats')");
    expect(activityBar.indexOf("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}")).toBeLessThan(
      activityBar.indexOf('title="Update"'),
    );
    expect(activityBar.indexOf('title="Update"')).toBeLessThan(activityBar.indexOf('title="Token Stats"'));
    expect(activityBar.indexOf('title="Token Stats"')).toBeLessThan(activityBar.indexOf('title="Settings"'));
    expect(activityBar).toContain("settingsDetailView === 'update'");
    expect(activityBar).toContain("settingsDetailView === 'tokenStats'");
    expect(activityBar).toContain("!isShortcutSettingsDetailActive");

    const floatingStart = mainTsx.indexOf('const floatingControlStack = !isWide ? (');
    const mobileSettingsStart = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', floatingStart);
    const mobileOnly = mainTsx.slice(floatingStart, mobileSettingsStart);
    expect(mobileOnly).not.toContain('codicon-cloud-download');
    expect(mobileOnly).not.toContain('title="Update"');
  });
});
