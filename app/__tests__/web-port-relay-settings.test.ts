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
    expect(mainTsx).toContain('window.open(openUrl, \'_blank\', \'noopener,noreferrer\')');

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
});
