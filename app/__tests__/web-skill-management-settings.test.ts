import fs from 'fs';
import path from 'path';

const root = path.resolve(__dirname, '..');
const mainTsx = fs.readFileSync(path.join(root, 'web/src/main.tsx'), 'utf8');
const stylesCss = fs.readFileSync(path.join(root, 'web/src/styles.css'), 'utf8');

describe('skill management settings UI source structure', () => {
  test('adds Skills as a settings detail and More row after Update', () => {
    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'skills'");
    expect(mainTsx).toContain('renderSkillsSettingsDetail()');

    const moreStart = mainTsx.indexOf("renderSettingsSection('More'");
    const moreEnd = mainTsx.indexOf("renderSettingsSection(", moreStart + 1);
    const moreSection = mainTsx.slice(moreStart, moreEnd > moreStart ? moreEnd : undefined);
    expect(moreSection.indexOf("setSettingsDetailView('update')")).toBeLessThan(moreSection.indexOf("setSettingsDetailView('skills')"));
    expect(moreSection.indexOf("setSettingsDetailView('skills')")).toBeLessThan(moreSection.indexOf("setSettingsDetailView('tokenStats')"));
  });

  test('adds desktop Skills shortcut between Update and Token Stats', () => {
    const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
    const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
    const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);

    expect(activityBar).toContain('codicon-symbol-method');
    expect(activityBar).toContain("openSettingsDetail('skills')");
    expect(activityBar.indexOf('title="Update"')).toBeLessThan(activityBar.indexOf('title="Skills"'));
    expect(activityBar.indexOf('title="Skills"')).toBeLessThan(activityBar.indexOf('title="Token Stats"'));
    expect(activityBar).toContain("settingsDetailView === 'skills'");
  });

  test('renders Skills detail with controlled command hooks and confirmations', () => {
    expect(mainTsx).toContain('const renderSkillsSettingsDetail = () =>');
    expect(mainTsx).toContain('refreshSkillManagement');
    expect(mainTsx).toContain('service.scanSkills');
    expect(mainTsx).toContain('service.listSkillsSource');
    expect(mainTsx).toContain('service.installSkills');
    expect(mainTsx).toContain('service.uninstallSkills');
    expect(mainTsx).toContain('service.updateSkills');
    expect(mainTsx).toContain("kind: 'skillInstall'");
    expect(mainTsx).toContain("kind: 'skillUninstall'");
    expect(mainTsx).toContain("kind: 'skillUpdate'");
  });

  test('renders skill rows without linked agent labels', () => {
    expect(mainTsx).not.toContain('skillAgentsLabel');
    expect(mainTsx).not.toContain('No linked agents');
  });

  test('uses icon actions and operation polling for Skills tasks', () => {
    expect(mainTsx).toContain('skillOperationPollTimerRef');
    expect(mainTsx).toContain('operation?.running');
    expect(mainTsx).toContain('includeProjects: true');
    expect(mainTsx).toContain('settings-skill-icon-btn');
    expect(mainTsx).toContain('codicon-add');
    expect(mainTsx).toContain('codicon-sync');
    expect(mainTsx).toContain('codicon-trash');
  });

  test('expands skill install controls inline with select all', () => {
    expect(mainTsx).toContain('sameSkillInstallTarget');
    expect(mainTsx).toContain('toggleAllSkillSourceCandidates');
    expect(mainTsx).toContain('Select all');
    expect(mainTsx).toContain('renderSkillInstallPanel({hubId, scope: options.scope, projectName: options.projectName})');
    expect(mainTsx).not.toContain('renderSkillInstallPanel()}');
    expect(mainTsx).not.toContain('candidate?.description');
  });

  test('uses compact settings skill styles', () => {
    expect(stylesCss).toContain('.settings-skills-hub');
    expect(stylesCss).toContain('.settings-skill-row');
    expect(stylesCss).toContain('.settings-skill-category');
    expect(stylesCss).toContain('.settings-skill-icon-btn');
    expect(stylesCss).toContain('.settings-skill-select-all-row');
  });
});
