import fs from 'fs';
import path from 'path';

describe('agent package update settings UI source structure', () => {
  test('adds Update to More settings and keeps Chat focused on chat options', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'update'");
    expect(mainTsx).toContain('renderUpdateSettingsDetail(options)');
    expect(mainTsx).toContain("renderSettingsSection('More'");
    expect(mainTsx).not.toContain("renderSettingsSection('Storage'");

    const chatStart = mainTsx.indexOf("renderSettingsSection('Chat'");
    const codeDisplayStart = mainTsx.indexOf("renderSettingsSection('Code Display'", chatStart);
    expect(chatStart).toBeGreaterThanOrEqual(0);
    expect(codeDisplayStart).toBeGreaterThan(chatStart);
    const chatSection = mainTsx.slice(chatStart, codeDisplayStart);
    expect(chatSection).not.toContain('Use Latest Prompt Title');
    expect(chatSection).toContain('Hide Tool Calls');
    expect(chatSection).not.toContain('Token Stats');
    expect(chatSection).not.toContain('CC Switch');

    const moreStart = mainTsx.indexOf("renderSettingsSection('More'");
    expect(moreStart).toBeGreaterThan(codeDisplayStart);
    const moreSection = mainTsx.slice(moreStart);
    const updateIndex = moreSection.indexOf("setSettingsDetailView('update')");
    const skillsIndex = moreSection.indexOf("setSettingsDetailView('skills')");
    const tokenStatsIndex = moreSection.indexOf("setSettingsDetailView('tokenStats')");
    const ccSwitchIndex = moreSection.indexOf("setSettingsDetailView('ccSwitch')");
    const databaseIndex = moreSection.indexOf("setSettingsDetailView('database')");
    const clearCacheIndex = moreSection.indexOf('requestClearLocalCache');
    expect(updateIndex).toBeGreaterThanOrEqual(0);
    expect(updateIndex).toBeLessThan(skillsIndex);
    expect(skillsIndex).toBeLessThan(tokenStatsIndex);
    expect(tokenStatsIndex).toBeLessThan(ccSwitchIndex);
    expect(ccSwitchIndex).toBeLessThan(databaseIndex);
    expect(databaseIndex).toBeLessThan(clearCacheIndex);
  });

  test('renders Update detail with scan, task polling, and npm confirmation flow hooks', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const renderUpdateSettingsDetail = (options?: SettingsDetailShellOptions) =>');
    expect(mainTsx).toContain("'Update'");
    expect(mainTsx).toContain('WheelMaker');
    expect(mainTsx).toContain('refreshWheelMakerUpdates');
    expect(mainTsx).toContain('service.queryWheelMakerUpdate');
    expect(mainTsx).toContain('service.requestWheelMakerUpdatePublish');
    expect(mainTsx).toContain("kind: 'wheelMakerUpdate'");
    expect(mainTsx).toContain("kind: 'wheelMakerUpdateAll'");
    expect(mainTsx).toContain('requestWheelMakerUpdatePublish');
    expect(mainTsx).toContain('requestWheelMakerUpdateAll');
    expect(mainTsx).toContain('handleWheelMakerUpdateAllConfirmedAction');
    expect(mainTsx).toContain('const [wheelMakerUpdateAllPending, setWheelMakerUpdateAllPending] = useState(false);');
    expect(mainTsx).toContain('Promise.all(target.hubIds.map(async hubId =>');
    expect(mainTsx).toContain('await refreshWheelMakerUpdates();');
    expect(mainTsx).toContain('Failed to update ${failedUpdates.length} of ${target.hubIds.length} hubs: ${failedUpdates.map(entry => entry.hubId).join');
    expect(mainTsx).toContain('wheelMakerUpdateStatusLabel');
    expect(mainTsx).toContain('wheelMakerReleaseRef');
    expect(mainTsx).toContain('formatWheelMakerDateTime');
    expect(mainTsx).toContain('wheelMakerData?.git?.latestCommittedAt');
    expect(mainTsx).toContain('refreshAgentPackages');
    expect(mainTsx).toContain('deriveRegistryHubIds');
    expect(mainTsx).toContain('withAgentPackageTimeout(');
    expect(mainTsx).toContain('service.scanNpmPackages');
    expect(mainTsx).toContain('service.installNpmPackage');
    expect(mainTsx).toContain('service.uninstallNpmPackage');
    expect(mainTsx).not.toContain('service.queryNpmPackageTask');
    expect(mainTsx).not.toContain('pollAgentPackageTask');
    expect(mainTsx).toContain("kind: 'npmPackage'");
    expect(mainTsx).toContain("kind: 'npmPackageHubUpdate'");
    expect(mainTsx).toContain('requestAgentPackageAction');
    expect(mainTsx).toContain('requestAgentPackageHubUpdate(card.hubId, npmUpdateTargets)');
    expect(mainTsx).toContain('handleAgentPackageConfirmedAction');
    expect(mainTsx).toContain('handleAgentPackageHubUpdateConfirmedAction');
    expect(mainTsx).toContain('packageStatusLabel');
    expect(mainTsx).toContain('deriveNpmPackageUpdateTargets(hub?.packages ?? [])');
    expect(mainTsx).toContain('npmPackageUpdateSummary(npmUpdateTargets.length)');
    expect(mainTsx).toContain('const [expandedNpmUpdateHubIds, setExpandedNpmUpdateHubIds] = useState<Record<string, boolean>>({});');
    expect(mainTsx).toContain("const [agentPackageHubUpdatePendingId, setAgentPackageHubUpdatePendingId] = useState('');");
    expect(mainTsx).toContain('const npmExpanded = expandedNpmUpdateHubIds[card.hubId] === true;');
    expect(mainTsx).toContain('aria-expanded={npmExpanded}');
    expect(mainTsx).toContain('{npmExpanded ? (');
    expect(mainTsx).toContain('const showWheelMakerUpdateAction =');
    expect(mainTsx).toContain('shouldShowWheelMakerUpdateAction({');
    expect(mainTsx).toContain('loading: wheelMaker?.loading === true,');
    expect(mainTsx).toContain('pending: wheelMakerPending || wheelMakerUpdateAllPending,');
    expect(mainTsx).toContain('disabled={wheelMakerUpdateAllPending || wheelMakerPending || wheelMakerData?.pendingSignal === true}');
    expect(mainTsx).toContain('className="wheelmaker-update-all-btn"');
    expect(mainTsx).toContain("requestWheelMakerUpdateAll(updateHubCards.map(card => card.hubId))");
    expect(mainTsx).toContain("wheelMakerUpdateAllPending ? 'Updating All Hubs...' : 'Update All Hubs'");
    expect(mainTsx).toContain('disabled={updateHubCards.length === 0 || wheelMakerUpdateAllPending}');
    expect(mainTsx).not.toContain("wheelMakerStatus !== 'up_to_date'");
    expect(mainTsx).not.toContain('Agent Packages');
    expect(mainTsx).not.toContain('>Prefix:');
    expect(mainTsx).not.toContain('title={hub?.npmPrefix');
    expect(mainTsx).not.toContain('Updated: {agentCard.updatedAt}');
    expect(mainTsx).not.toContain('<span className="wheelmaker-update-product">WheelMaker</span>');

    expect(stylesCss).toContain('.agent-package-hub-list');
    expect(stylesCss).toContain('.update-hub-header .wide-project-hub-tag');
    expect(stylesCss).toContain('font-size: 12.5px;');
    const settingsDetailPageBlock = stylesCss.match(/^\.settings-detail-page \{[\s\S]*?\n\}/m)?.[0] ?? '';
    expect(settingsDetailPageBlock).toContain('flex: 1 1 auto;');
    expect(settingsDetailPageBlock).toContain('overflow: hidden;');
    const settingsDetailBodyBlock = stylesCss.match(/^\.settings-detail-body \{[\s\S]*?\n\}/m)?.[0] ?? '';
    expect(settingsDetailBodyBlock).toContain('overflow-y: auto;');
    expect(settingsDetailBodyBlock).toContain('scrollbar-gutter: stable;');
    expect(stylesCss).toContain('.wheelmaker-update-panel');
    expect(stylesCss).toContain('.wheelmaker-update-all-btn');
    expect(stylesCss).toContain('.wheelmaker-update-version-line');
    expect(stylesCss).toContain('.wheelmaker-update-ref-tag');
    expect(stylesCss).toContain('.wheelmaker-update-sha-line');
    expect(stylesCss).toContain('.wheelmaker-update-action-btn');
    expect(stylesCss).toContain('.npm-update-disclosure');
    expect(stylesCss).toContain('.npm-update-section');
    expect(stylesCss).toContain('.npm-update-action-btn');
    expect(stylesCss).toContain('.npm-update-body');
    expect(stylesCss).toContain('.agent-package-row');
    expect(stylesCss).toContain('.agent-package-name-line');
    expect(stylesCss).toContain('.agent-package-agent-tags');
    expect(stylesCss).toContain('.agent-package-version-status');
    expect(stylesCss).toContain('.agent-package-action-btn');
  });

  test('does not use one hub package operation to disable every hub action', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const pending = agentPackageActionPendingKey === pendingKey || operation?.running === true || npmHubUpdatePending;');
    expect(mainTsx).toContain('disabled={pending}');
    expect(mainTsx).not.toContain('agentPackageAnyOperationRunning');
    expect(mainTsx).not.toContain('disabled={pending || agentPackageAnyOperationRunning}');
  });

  test('makes WheelMaker release identity and SHA rows visually distinct', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const wheelMakerBlockStart = mainTsx.indexOf('className="wheelmaker-update-panel"');
    const agentPackagesStart = mainTsx.indexOf('className="agent-package-row-list"', wheelMakerBlockStart);
    expect(wheelMakerBlockStart).toBeGreaterThanOrEqual(0);
    expect(agentPackagesStart).toBeGreaterThan(wheelMakerBlockStart);
    const wheelMakerBlock = mainTsx.slice(wheelMakerBlockStart, agentPackagesStart);
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-product"');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-version-line"');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-ref-tag"');
    expect(wheelMakerBlock).toContain('wheelMakerReleaseRef(wheelMakerData)');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-sha-line"');
    expect(wheelMakerBlock).toContain('wheelMakerCurrentTime');
    expect(wheelMakerBlock).toContain('wheelMakerLatestTime');
    expect(wheelMakerBlock).toContain(": 'Update'}");
    expect(mainTsx).not.toContain('Update+Publish');

    const panelBlock = stylesCss.match(/\.wheelmaker-update-panel \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(panelBlock).toContain('border-left: 3px solid');
    expect(panelBlock).toContain('grid-template-columns: minmax(0, 1fr) auto;');
    expect(panelBlock).toContain('grid-template-rows: auto auto auto;');

    const versionLineBlock = stylesCss.match(/\.wheelmaker-update-version-line \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(versionLineBlock).toContain('grid-row: 2;');
    expect(versionLineBlock).toContain('overflow: hidden;');

    const shaLineBlock = stylesCss.match(/\.wheelmaker-update-sha-line \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(shaLineBlock).toContain('white-space: nowrap;');
    expect(shaLineBlock).toContain('grid-template-columns: 52px auto minmax(0, 1fr);');

    const shaLinesBlock = stylesCss.match(/\.wheelmaker-update-sha-lines \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(shaLinesBlock).toContain('grid-column: 1 / -1;');

    const refTagBlock = stylesCss.match(/\.wheelmaker-update-ref-tag \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(refTagBlock).toContain('text-overflow: ellipsis;');
    expect(refTagBlock).toContain('font-family: \'JetBrains Mono\', Consolas, \'Courier New\', monospace;');

    const actionButtonBlock = stylesCss.match(/\.wheelmaker-update-action-btn \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(actionButtonBlock).toContain('grid-row: 1 / 3;');
    expect(actionButtonBlock).toContain('min-width: 74px;');
  });

  test('places agent tags beside display names and lets versions span under the action button', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('className="agent-package-name-line"');
    expect(mainTsx).toContain('className="agent-package-agent-tags"');
    expect(mainTsx).toContain('className={`agent-package-status agent-package-version-status status-${pkg.status}`}');
    expect(mainTsx).not.toContain('className={`agent-package-status status-${pkg.status}`}');

    const titleLineStart = mainTsx.indexOf('className="agent-package-title-line"');
    const nameLineStart = mainTsx.indexOf('className="agent-package-name-line"', titleLineStart);
    expect(titleLineStart).toBeGreaterThanOrEqual(0);
    expect(nameLineStart).toBeGreaterThan(titleLineStart);
    const titleLineBlock = mainTsx.slice(titleLineStart, nameLineStart);
    expect(titleLineBlock).toContain('className="agent-package-agent-tags"');
    expect(titleLineBlock).toContain("tagVariantClass('wide-session-agent', agent)");

    const nameLineEnd = mainTsx.indexOf('className="agent-package-version-line"', nameLineStart);
    const nameLineBlock = mainTsx.slice(nameLineStart, nameLineEnd);
    expect(nameLineBlock).not.toContain('className="agent-package-agent-tags"');

    const actionButtonBlock = stylesCss.match(/\.agent-package-action-btn \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(actionButtonBlock).toContain('grid-column: 2;');
    expect(actionButtonBlock).toContain('grid-row: 1 / 3;');

    const versionLineBlock = stylesCss.match(/\.agent-package-version-line \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(versionLineBlock).toContain('grid-column: 1 / -1;');
  });

  test('uses explicit agent tag variants and softly sized capsules', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain('const AGENT_TAG_VARIANT_INDEX');
    expect(mainTsx).toContain("claude: 2");
    expect(mainTsx).not.toContain("codexapp: 3");
    expect(mainTsx).not.toContain(`${['my', 'flicker'].join('')}:`);
    expect(mainTsx).toContain('if (prefix === \'wide-session-agent\' || prefix === \'token-stats-pill-agent\')');

    const agentTagBlock = stylesCss.match(/\.wide-session-agent-tag \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(agentTagBlock).toContain('display: inline-flex;');
    expect(agentTagBlock).toContain('min-height: 20px;');
    expect(agentTagBlock).toContain('max-width: 80px;');
    expect(agentTagBlock).toContain('padding: 1px 7px;');
    expect(agentTagBlock).toContain('font-size: 10.5px;');
    expect(agentTagBlock).toContain('font-weight: 600;');
    expect(agentTagBlock).toContain('background: color-mix(in srgb, var(--agent-accent) 6%, transparent);');
    expect(agentTagBlock).toContain('text-transform: none;');
  });

  test('adds desktop shortcuts and a compact mobile Update toolbar shortcut', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const activityBarStart = mainTsx.indexOf('const desktopActivityBar = isWide ? (');
    const activityBarEnd = mainTsx.indexOf('const floatingControlStack = !isWide ? (', activityBarStart);
    expect(activityBarStart).toBeGreaterThanOrEqual(0);
    expect(activityBarEnd).toBeGreaterThan(activityBarStart);
    const activityBar = mainTsx.slice(activityBarStart, activityBarEnd);

    expect(activityBar).toContain('codicon-cloud-download');
    expect(activityBar).toContain('codicon-graph-line');
    expect(activityBar).toContain('codicon-radio-tower');
    expect(activityBar).toContain("openSettingsDetail('update')");
    expect(activityBar).toContain("openSettingsDetail('tokenStats')");
    expect(activityBar).toContain('handleDesktopPortRelaySelect');
    expect(activityBar.indexOf("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}")).toBeLessThan(
      activityBar.indexOf('title="Update"'),
    );
    expect(activityBar.indexOf('title="Update"')).toBeLessThan(activityBar.indexOf('title="Token Stats"'));
    expect(activityBar.indexOf('title="Token Stats"')).toBeLessThan(activityBar.indexOf('title="Settings"'));
    expect(activityBar.indexOf('title="Port Relay"')).toBeLessThan(
      activityBar.indexOf("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}"),
    );
    expect(activityBar).toContain("settingsDetailView === 'update'");
    expect(activityBar).toContain("settingsDetailView === 'tokenStats'");
    expect(activityBar).toContain("settingsDetailView === 'portRelay'");
    expect(activityBar).toContain("!isShortcutSettingsDetailActive");
    expect(mainTsx).toContain("settingsDetailView === 'skills' ||");
    expect(mainTsx).toContain("settingsDetailView === 'portRelay'");

    const floatingStart = mainTsx.indexOf('const floatingControlStack = !isWide ? (');
    const mobileSettingsStart = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', floatingStart);
    const mobileOnly = mainTsx.slice(floatingStart, mobileSettingsStart);
    expect(mobileOnly).not.toContain("openSettingsDetail('update')");

    const mobileToolbarStart = mainTsx.indexOf('<div className="mobile-chat-toolbar"');
    const mobileToolbarEnd = mainTsx.indexOf('{renderChatHubSummary()}', mobileToolbarStart);
    expect(mobileToolbarStart).toBeGreaterThanOrEqual(0);
    expect(mobileToolbarEnd).toBeGreaterThan(mobileToolbarStart);
    const mobileToolbar = mainTsx.slice(mobileToolbarStart, mobileToolbarEnd);
    expect(mobileToolbar).toContain("openSettingsDetail('update')");
    expect(mobileToolbar).toContain('codicon-cloud-download');
    expect(mobileToolbar.indexOf('title="Open settings"')).toBeLessThan(mobileToolbar.indexOf('title="Update"'));
    expect(mobileToolbar.indexOf('title="Update"')).toBeLessThan(mobileToolbar.indexOf('title="Port Relay"'));

    const mobileToolbarBlock = stylesCss.match(/\.mobile-chat-toolbar \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileToolbarBlock).toContain('gap: 4px;');
    expect(mobileToolbarBlock).toContain('background: transparent;');
    expect(mobileToolbarBlock).not.toContain('border: 1px solid');
    expect(mobileToolbarBlock).not.toContain('border-radius: 10px;');

    const mobileToolbarIconBlock = stylesCss.match(/\.mobile-chat-toolbar-icon \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileToolbarIconBlock).toContain('width: 36px;');
    expect(mobileToolbarIconBlock).toContain('height: 34px;');
    expect(mobileToolbarIconBlock).toContain('border-radius: 8px;');
    expect(mobileToolbarIconBlock).toContain('box-shadow: none;');
    expect(mobileToolbarIconBlock).not.toContain('border-radius: 999px;');
    expect(stylesCss).toContain('.mobile-chat-toolbar-icon.active');
  });
});
