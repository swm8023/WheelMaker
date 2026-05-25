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
    expect(mainTsx).toContain("import { resolvePortRelayOpenUrl } from './portRelayUrl';");
    expect(mainTsx).toContain('resolvePortRelayOpenUrl({');
    expect(mainTsx).toContain('relayUrl: portRelaySnapshot.relayUrl');
    expect(mainTsx).not.toContain('window.open(openUrl, \'_blank\', \'noopener,noreferrer\')');

    expect(stylesCss).toContain('.port-relay-panel');
    expect(stylesCss).toContain('.port-relay-form-grid');
    expect(stylesCss).toContain('.port-relay-code-row');
    expect(stylesCss).toContain('.port-relay-status-pill');
  });

  test('embeds relay pages through desktop main pane and mobile floating overlay', () => {
    expect(mainTsx).toContain("const PORT_RELAY_FLOATING_SLOT_STORAGE_KEY = 'wheelmaker:portRelayFloatingSlot';");
    expect(mainTsx).toContain('readPortRelayFloatingSlot()');
    expect(mainTsx).toContain('window.localStorage.setItem(PORT_RELAY_FLOATING_SLOT_STORAGE_KEY, nextSlot);');
    expect(mainTsx).toContain('const [portRelayFrameOpen, setPortRelayFrameOpen] = useState(false);');
    expect(mainTsx).toContain("portRelaySnapshot.enabled && portRelaySnapshot.status === 'Up'");
    expect(mainTsx).toContain('setPortRelayFrameOpen(false);');
    expect(mainTsx).toContain('const handleDesktopPortRelaySelect = useCallback(() => {');
    expect(mainTsx).toContain('setPortRelayFrameOpen(open => !open);');
    expect(mainTsx).toContain('onClick={handleDesktopPortRelaySelect}');
    expect(mainTsx).toContain('className={`port-relay-frame-surface ${mode}`}');
    expect(mainTsx).toContain("renderPortRelayFrameSurface('desktop')");
    expect(mainTsx).toContain("renderPortRelayFrameSurface('mobile')");
    expect(mainTsx).toContain('<iframe');
    expect(mainTsx).toContain('src={portRelayFrameUrl}');
    expect(mainTsx).toContain('className="port-relay-frame"');
    expect(mainTsx).toContain('className="drawer-toggle-bubble port-relay-floating-bubble"');
    expect(mainTsx).toContain("title={portRelayFrameOpen ? 'Close relay page' : 'Open relay page'}");

    const renderMainStart = mainTsx.indexOf('const renderMain = () => {');
    const chatBranchStart = mainTsx.indexOf("if (tab === 'chat')", renderMainStart);
    const renderMainPrologue = mainTsx.slice(renderMainStart, chatBranchStart);
    expect(renderMainPrologue).toContain('isWide && portRelayFrameOpen && portRelayFrameUrl');
    expect(renderMainPrologue).toContain('renderPortRelayFrameSurface');

    expect(stylesCss).toContain('.port-relay-frame-surface');
    expect(stylesCss).toContain('.port-relay-frame-surface.mobile');
    expect(stylesCss).toContain('.port-relay-frame');
    expect(stylesCss).toContain('.port-relay-floating-bubble');
  });

  test('keeps the mobile relay iframe locked to the visible viewport', () => {
    const mobileSurfaceStart = stylesCss.indexOf('.port-relay-frame-surface.mobile');
    const mobileSurfaceEnd = stylesCss.indexOf('.port-relay-frame {', mobileSurfaceStart);
    const mobileSurfaceCss = stylesCss.slice(mobileSurfaceStart, mobileSurfaceEnd);

    expect(mobileSurfaceCss).toContain('width: 100dvw;');
    expect(mobileSurfaceCss).toContain('height: 100dvh;');
    expect(mobileSurfaceCss).toContain('max-width: 100dvw;');
    expect(mobileSurfaceCss).toContain('overflow: clip;');
    expect(mobileSurfaceCss).toContain('overscroll-behavior: none;');
    expect(mobileSurfaceCss).toContain('touch-action: pan-y;');

    const iframeStart = stylesCss.indexOf('.port-relay-frame {');
    const iframeEnd = stylesCss.indexOf('.floating-control-stack-layer', iframeStart);
    const iframeCss = stylesCss.slice(iframeStart, iframeEnd);

    expect(iframeCss).toContain('max-width: 100%;');
  });

  test('hides mobile navigation and drawer while the relay iframe is open', () => {
    expect(mainTsx).toContain('const mobilePortRelayFrameOpen = !isWide && portRelayFrameOpen && !!portRelayFrameUrl;');
    expect(mainTsx).toContain('if (!mobilePortRelayFrameOpen) {');
    expect(mainTsx).toContain('setDrawerOpen(false);');
    expect(mainTsx).toContain('setSidebarSettingsOpen(false);');
    expect(mainTsx).toContain('}, [mobilePortRelayFrameOpen, setDrawerOpen, setSidebarSettingsOpen]);');
    expect(mainTsx).toContain('{mobilePortRelayFrameOpen ? null : gestureNavigation ? (');
    expect(mainTsx).toContain('drawerOpen={mobilePortRelayFrameOpen ? false : drawerOpen}');
    expect(mainTsx).toContain('const portRelayMobileFrameOverlay = mobilePortRelayFrameOpen');
  });

  test('keeps relay settings focused on hub local port mapping', () => {
    expect(mainTsx).toContain("const [portRelayTargets, setPortRelayTargets] = useState<PortRelayTarget[]>(");
    expect(mainTsx).toContain("const [selectedPortRelayTarget, setSelectedPortRelayTarget] = useState<PortRelayTarget | null>(");
    expect(mainTsx).toContain("const [portRelayDraftPort, setPortRelayDraftPort] = useState('80');");
    expect(mainTsx).not.toContain('const [portRelayTargetHost, setPortRelayTargetHost]');
    expect(mainTsx).not.toContain('setPortRelayTargetHost(snapshot.targetHost)');
    expect(mainTsx).toContain("targetHost: '127.0.0.1',");
    expect(mainTsx).toContain("const portRelayTargetDisplay = selectedTarget ? `${selectedTarget.hubId} -> 127.0.0.1:${selectedTarget.targetPort}` : 'No target';");
    expect(mainTsx).toContain('className="port-relay-target-inline"');
    expect(mainTsx).toContain('{portRelayTargetDisplay}');
    expect(mainTsx).toContain('className="port-relay-target-list"');
    expect(mainTsx).toContain('type="checkbox"');
    expect(mainTsx).toContain('selectPortRelayTarget(target)');
    expect(mainTsx).toContain('deletePortRelayTarget(target)');
    expect(mainTsx).toContain('commitPortRelayDraftTarget();');
    expect(mainTsx).not.toContain('<span>Target Host</span>');
    expect(mainTsx).not.toContain('onClick={openPortRelay}');
    expect(mainTsx).toContain("if (settingsDetailView !== 'portRelay' || portRelayAccessCode || portRelaySnapshot.enabled) {");
    expect(mainTsx).toContain('setPortRelayAccessCode(generatePortRelayAccessCode());');

    expect(stylesCss).toContain('.port-relay-section');
    expect(stylesCss).toContain('.port-relay-target-inline');
    expect(stylesCss).toContain('.port-relay-target-list-row');
    expect(stylesCss).toContain('.port-relay-target-delete');
  });

  test('adds a copy action beside generated relay access code and polished relay panel styling', () => {
    const codeRowStart = mainTsx.indexOf('className="port-relay-code-row"');
    const codeRowEnd = mainTsx.indexOf('</div>', codeRowStart);
    const codeRow = mainTsx.slice(codeRowStart, codeRowEnd);

    expect(codeRow.indexOf('Generate')).toBeGreaterThan(-1);
    expect(codeRow.indexOf('Copy')).toBeGreaterThan(codeRow.indexOf('Generate'));
    expect(mainTsx).toContain('const [portRelayCodeCopied, setPortRelayCodeCopied] = useState(false);');
    expect(mainTsx).toContain('writeTextToClipboard(portRelayAccessCode);');
    expect(mainTsx).toContain('aria-label="Copy port relay access code"');
    expect(mainTsx).toContain("{portRelayCodeCopied ? 'Copied' : 'Copy'}");

    expect(stylesCss).toContain('.port-relay-panel-shell');
    expect(stylesCss).toContain('.port-relay-control-section');
    expect(stylesCss).toContain('.port-relay-section-title');
    expect(stylesCss).toContain('.port-relay-copy-btn');
    expect(stylesCss).toContain('.port-relay-status-section::before');
  });

  test('keeps the relay status area to one line without widening the settings panel', () => {
    const statusStart = mainTsx.indexOf('className="port-relay-section port-relay-status-section"');
    const statusEnd = mainTsx.indexOf('{portRelayError || portRelaySnapshot.error ?', statusStart);
    const statusMarkup = mainTsx.slice(statusStart, statusEnd);

    expect(statusMarkup).toContain('className="port-relay-header"');
    expect(statusMarkup).toContain('className="port-relay-target-inline"');
    expect(statusMarkup).not.toContain('className="port-relay-target-row"');
    expect(statusMarkup).not.toContain('className="port-relay-url"');

    const statusCssStart = stylesCss.indexOf('.port-relay-status-section');
    const statusCssEnd = stylesCss.indexOf('.port-relay-form-grid', statusCssStart);
    const statusCss = stylesCss.slice(statusCssStart, statusCssEnd);

    expect(statusCss).toContain('min-width: 0;');
    expect(statusCss).toContain('overflow: hidden;');
    expect(statusCss).toContain('.port-relay-target-inline');
    expect(statusCss).toContain('text-overflow: ellipsis;');
    expect(stylesCss).toContain('.port-relay-code-row {');
    expect(stylesCss).toContain('flex-wrap: wrap;');
  });

  test('does not invent an access code for an already-enabled relay from another device', () => {
    expect(mainTsx).toContain('const [portRelayKnownAccessCodeGeneration, setPortRelayKnownAccessCodeGeneration] = useState<number | null>(null);');
    expect(mainTsx).toContain('const portRelayAccessCodeUnknown = portRelaySnapshot.enabled &&');
    expect(mainTsx).toContain("settingsDetailView !== 'portRelay' || portRelayAccessCode || portRelaySnapshot.enabled");
    expect(mainTsx).toContain('setPortRelayError(\'Access code is unknown on this device. Generate a new code before switching target.\');');
    expect(mainTsx).toContain("placeholder={portRelayAccessCodeUnknown ? 'Unknown' : ''}");
    expect(mainTsx).toContain('disabled={portRelayAccessCodeUnknown || !portRelayAccessCode}');
    expect(mainTsx).toContain("portRelayAccessCodeUnknown ? 'Reset Code' : 'Generate'");
    expect(mainTsx).toContain("setPortRelayError('Access code is unknown on this device. Generate a new code before copying.');");
    expect(mainTsx).toContain('setPortRelayKnownAccessCodeGeneration(typeof snapshot.accessCodeGeneration === \'number\' ? snapshot.accessCodeGeneration : null);');
  });
});
