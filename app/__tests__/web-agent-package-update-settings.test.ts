import fs from 'fs';
import path from 'path';

describe('agent package update settings UI source structure', () => {
  test('moves shortcut details out of More and keeps Chat focused on chat options', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("type SettingsDetailView = 'update' | 'skills' | 'tokenStats' | 'ccSwitch' | 'database' | 'portRelay' | 'debugLogs' | null;");
    expect(mainTsx).toContain("settingsDetailView === 'update'");
    expect(mainTsx).toContain('renderUpdateSettingsDetail(options)');
    expect(mainTsx).not.toContain("renderSettingsSection('More'");
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

    const debugStart = mainTsx.indexOf("renderSettingsSection('Debug'");
    expect(debugStart).toBeGreaterThan(codeDisplayStart);
    const debugSection = mainTsx.slice(debugStart);
    expect(debugSection).not.toContain("setSettingsDetailView('update')");
    expect(debugSection).not.toContain("setSettingsDetailView('skills')");
    expect(debugSection).not.toContain("setSettingsDetailView('tokenStats')");
    expect(debugSection).not.toContain("setSettingsDetailView('ccSwitch')");
    expect(debugSection).not.toContain("setSettingsDetailView('portRelay')");
    expect(debugSection.indexOf("setSettingsDetailView('database')")).toBeGreaterThanOrEqual(0);
    expect(debugSection.indexOf("setSettingsDetailView('database')")).toBeLessThan(debugSection.indexOf('requestClearLocalCache'));
    expect(debugSection.indexOf('requestClearLocalCache')).toBeLessThan(debugSection.indexOf('handleRegistryDebugLogout'));
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
    expect(mainTsx).toContain('refreshWheelMakerUpdates({force: true})');
    expect(mainTsx).toContain('const wheelMakerUpdatePollTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null);');
    expect(mainTsx).toContain('const scheduleWheelMakerUpdatePoll = useCallback((hubIds: string | string[]) =>');
    expect(mainTsx).toContain('result.remoteRefreshRunning');
    expect(mainTsx).toContain('scheduleWheelMakerUpdatePoll(hubId)');
    expect(mainTsx).toContain('refreshWheelMakerUpdateHubRef.current?.(hubId)');
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
    expect(mainTsx).toContain('service.installNpmPackages');
    expect(mainTsx).toContain('service.uninstallNpmPackage');
    expect(mainTsx).not.toContain('service.queryNpmPackageTask');
    expect(mainTsx).not.toContain('pollAgentPackageTask');
    expect(mainTsx).toContain("kind: 'npmPackage'");
    expect(mainTsx).toContain("kind: 'npmPackageHubUpdate'");
    expect(mainTsx).toContain('requestAgentPackageAction');
    expect(mainTsx).toContain('requestAgentPackageHubUpdate(card.hubId, npmUpdateTargets)');
    expect(mainTsx).toContain('handleAgentPackageConfirmedAction');
    expect(mainTsx).toContain('handleAgentPackageHubUpdateConfirmedAction');
    expect(mainTsx).toContain("await service.installNpmPackages(target.hubId, target.packages.map(pkg => pkg.packageName), 'latest');");
    expect(mainTsx).not.toContain("for (const pkg of target.packages)");
    expect(mainTsx).toContain('packageStatusLabel');
    expect(mainTsx).toContain('deriveNpmPackageUpdateTargets(hub?.packages ?? [])');
    expect(mainTsx).toContain('npmPackageUpdateSummary(npmUpdateTargets.length)');
    expect(mainTsx).toContain('const [expandedNpmUpdateHubIds, setExpandedNpmUpdateHubIds] = useState<Record<string, boolean>>({});');
    expect(mainTsx).toContain("const [agentPackageHubUpdatePendingId, setAgentPackageHubUpdatePendingId] = useState('');");
    expect(mainTsx).toContain('const npmExpanded = expandedNpmUpdateHubIds[card.hubId] === true;');
    expect(mainTsx).toContain('aria-expanded={npmExpanded}');
    expect(mainTsx).toContain('{npmExpanded ? (');
    expect(mainTsx).not.toContain('<span className="npm-update-title">NPM Update</span>');
    expect(mainTsx).toContain("npmHubUpdatePending ? 'Updating...' : 'Update All'");
    expect(mainTsx).not.toContain("npmHubUpdatePending ? 'Updating...' : 'Update NPM'");
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
    expect(mainTsx).not.toContain('<span className="wheelmaker-update-product" title={card.hubId}>{card.hubId}</span>');
    expect(mainTsx).toContain('<span className="wheelmaker-update-scope">Release</span>');

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

  test('shows npm hub Update All only from the expanded summary row', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const disclosureStart = mainTsx.indexOf('className="npm-update-disclosure"');
    const bodyStart = mainTsx.indexOf('className="npm-update-body"', disclosureStart);
    expect(disclosureStart).toBeGreaterThanOrEqual(0);
    expect(bodyStart).toBeGreaterThan(disclosureStart);

    const disclosureBlock = mainTsx.slice(disclosureStart, bodyStart);
    const expandedGateIndex = disclosureBlock.indexOf('{npmExpanded ? (');
    const actionIndex = disclosureBlock.indexOf('className="npm-update-action-btn"');
    expect(expandedGateIndex).toBeGreaterThanOrEqual(0);
    expect(actionIndex).toBeGreaterThan(expandedGateIndex);
    expect(disclosureBlock).toContain("npmHubUpdatePending ? 'Updating...' : 'Update All'");

    const mobileNpmBlock = stylesCss.match(/@media \(max-width: 560px\) \{[\s\S]*?\.wheelmaker-update-panel \{/m)?.[0] ?? '';
    expect(mobileNpmBlock).not.toContain('grid-template-columns: 1fr;');
    expect(mobileNpmBlock).not.toContain('width: 100%;');
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
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-scope"');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-version-line"');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-ref-tag"');
    expect(wheelMakerBlock).toContain('wheelMakerReleaseRef(wheelMakerData)');
    expect(wheelMakerBlock).toContain('className="wheelmaker-update-sha-line"');
    expect(wheelMakerBlock).toContain('wheelMakerCurrentTime');
    expect(wheelMakerBlock).toContain('wheelMakerLatestTime');
    expect(wheelMakerBlock).toContain(": 'Update'}");
    expect(mainTsx).not.toContain('Update+Publish');

    const hubCardBlock = stylesCss.match(/\.agent-package-hub-card \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(hubCardBlock).toContain('border-left: 3px solid');

    const panelBlock = stylesCss.match(/\.wheelmaker-update-panel \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(panelBlock).not.toContain('border: 1px solid');
    expect(panelBlock).not.toContain('border-left: 3px solid');
    expect(panelBlock).not.toContain('background:');
    expect(panelBlock).not.toContain('border-radius:');
    expect(panelBlock).toContain('grid-template-columns: minmax(0, 1fr) auto;');
    expect(panelBlock).toContain('grid-template-rows: auto auto auto;');

    const npmSectionBlock = stylesCss.match(/\.npm-update-section \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(npmSectionBlock).not.toContain('border: 1px solid');
    expect(npmSectionBlock).not.toContain('background:');
    expect(npmSectionBlock).not.toContain('border-radius:');

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

  test('keeps WheelMaker release SHA metadata on one line inside the mobile settings screen', () => {
    const projectRoot = path.join(__dirname, '..');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const mobileShaLineBlock = stylesCss.match(/\.mobile-settings-screen \.wheelmaker-update-sha-line \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileShaLineBlock).toContain('grid-template-columns: 52px 7ch max-content;');
    expect(mobileShaLineBlock).toContain('column-gap: 10px;');
    expect(mobileShaLineBlock).toContain('white-space: nowrap;');

    const mobileShaValueBlock = stylesCss.match(/\.mobile-settings-screen \.wheelmaker-update-sha-value \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileShaValueBlock).toContain('min-width: 7ch;');
    expect(mobileShaValueBlock).not.toContain('grid-column: 2;');

    const mobileShaTimeBlock = stylesCss.match(/\.mobile-settings-screen \.wheelmaker-update-sha-time \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileShaTimeBlock).toContain('overflow: visible;');
    expect(mobileShaTimeBlock).not.toContain('text-overflow: ellipsis;');
    expect(mobileShaTimeBlock).not.toContain('grid-row: 2;');
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

  test('adds desktop shortcuts and a mobile Settings-only shortcut bar', () => {
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
    const mobileBarStart = mainTsx.indexOf('const mobileSettingsShortcutBar = !isWide && sidebarSettingsOpen ? (', floatingStart);
    const mobileOnly = mainTsx.slice(floatingStart, mobileBarStart);
    expect(mobileOnly).not.toContain("openSettingsDetail('update')");

    const mobileBarEnd = mainTsx.indexOf('const mobileSettingsScreen = !isWide && sidebarSettingsOpen ? (', mobileBarStart);
    expect(mobileBarStart).toBeGreaterThanOrEqual(0);
    expect(mobileBarEnd).toBeGreaterThan(mobileBarStart);
    const mobileBar = mainTsx.slice(mobileBarStart, mobileBarEnd);
    expect(mobileBar.indexOf('title="Settings"')).toBeLessThan(mobileBar.indexOf('title="Update"'));
    expect(mobileBar.indexOf('title="Update"')).toBeLessThan(mobileBar.indexOf('title="Skills"'));
    expect(mobileBar.indexOf('title="Skills"')).toBeLessThan(mobileBar.indexOf('title="Port Relay"'));
    expect(mobileBar.indexOf('title="Port Relay"')).toBeLessThan(mobileBar.indexOf('title="Token Stats"'));
    expect(mobileBar.indexOf('title="Token Stats"')).toBeLessThan(mobileBar.indexOf('title="CC Switch"'));
    expect(mobileBar).toContain("openMobileSettingsShortcutDetail('update')");
    expect(mobileBar).toContain("openMobileSettingsShortcutDetail('skills')");
    expect(mobileBar).toContain("openMobileSettingsShortcutDetail('portRelay')");
    expect(mobileBar).toContain("openMobileSettingsShortcutDetail('tokenStats')");
    expect(mobileBar).toContain("openMobileSettingsShortcutDetail('ccSwitch')");
    expect(mobileBar).toContain('onClick={handleMobileSettingsRootShortcut}');
    expect(mobileBar).toContain('data-active-index={mobileSettingsShortcutActiveIndex}');
    expect(mobileBar).toContain('className="mobile-settings-shortcut-label">Settings</span>');
    expect(mobileBar).toContain('className="mobile-settings-shortcut-label">CC Switch</span>');

    const mobileToolbarStart = mainTsx.indexOf('<div className="mobile-chat-toolbar"');
    const mobileToolbarEnd = mainTsx.indexOf('{renderChatHubSummary(true)}', mobileToolbarStart);
    expect(mobileToolbarStart).toBeGreaterThanOrEqual(0);
    expect(mobileToolbarEnd).toBeGreaterThan(mobileToolbarStart);
    const mobileToolbar = mainTsx.slice(mobileToolbarStart, mobileToolbarEnd);
    expect(mobileToolbar).toContain('title="Open settings"');
    expect(mobileToolbar).not.toContain('title="Update"');
    expect(mobileToolbar).not.toContain('title="Port Relay"');
    expect(mobileToolbar).not.toContain("openSettingsDetail('update')");
    expect(mobileToolbar).not.toContain("openSettingsDetail('portRelay')");
    expect(mobileToolbar).not.toContain('refreshMobileChatProjectSessions()');
    expect(mobileToolbar).not.toContain('title={reconnecting ? \'Reconnecting...\' : \'Refresh chats\'}');

    const mobileToolbarBlock = stylesCss.match(/\.mobile-chat-toolbar \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileToolbarBlock).toContain('gap: 4px;');
    expect(mobileToolbarBlock).toContain('background: transparent;');
    expect(mobileToolbarBlock).not.toContain('border: 1px solid');
    expect(mobileToolbarBlock).not.toContain('border-radius: 10px;');

    expect(stylesCss).toContain('.mobile-settings-shortcut-bar {');
    expect(stylesCss).toContain('.mobile-settings-shortcut-bar::before {');
    expect(stylesCss).toContain('.mobile-settings-shortcut-button {');
    expect(stylesCss).toContain('.mobile-settings-shortcut-label {');
    const mobileShortcutButtonBlock = stylesCss.match(/\.mobile-settings-shortcut-button \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileShortcutButtonBlock).toContain('height: 58px;');
    expect(mobileShortcutButtonBlock).toContain('flex-direction: column;');
    const mobileShortcutIndicatorBlock = stylesCss.match(/\.mobile-settings-shortcut-bar::before \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(mobileShortcutIndicatorBlock).toContain('top: 0;');
    expect(mobileShortcutIndicatorBlock).toContain('transition: transform');
    expect(stylesCss).toContain(".mobile-settings-shortcut-bar[data-active-index='5']::before");
    expect(stylesCss).not.toContain('.mobile-settings-shortcut-button.active::before');
    expect(stylesCss).toContain('padding: 0 0 env(safe-area-inset-bottom, 0px);');
    expect(stylesCss).not.toContain('.mobile-chat-toolbar-icon.active');
  });
});
