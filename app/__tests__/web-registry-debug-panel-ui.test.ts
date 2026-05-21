import fs from 'fs';
import path from 'path';

describe('web registry debug panel ui', () => {
  const projectRoot = path.join(__dirname, '..');
  const panelPath = path.join(projectRoot, 'web', 'src', 'debug', 'RegistryDebugPanel.tsx');

  test('defines a two-pane virtualized debug panel', () => {
    expect(fs.existsSync(panelPath)).toBe(true);
    const panelTsx = fs.readFileSync(panelPath, 'utf8');

    expect(panelTsx).toContain("import {Virtuoso, type VirtuosoHandle} from 'react-virtuoso';");
    expect(panelTsx).toContain('className="registry-debug-panel"');
    expect(panelTsx).toContain('className="registry-debug-list-pane"');
    expect(panelTsx).toContain('className="registry-debug-detail-pane"');
    expect(panelTsx).toContain('className="registry-debug-splitter"');
    expect(panelTsx).toContain('<span>Conn</span>');
    expect(panelTsx).toContain('className={`registry-debug-connection-tag ${record.connection.toLowerCase()}`}');
    expect(panelTsx).toContain('{record.connection}');
    expect(panelTsx).toContain('Include multi-session records');
    expect(panelTsx).toContain('Jump to latest');
    expect(panelTsx).toContain('Clear');
    expect(panelTsx).toContain('Copy');
    expect(panelTsx).toContain('JSON.stringify(selectedEnvelopeOrLifecycle, null, 2)');
    expect(panelTsx).toContain('selectedScope');
    expect(panelTsx).toContain('onSelectedScopeChange');
    expect(panelTsx).toContain('sessionLabels');
    expect(panelTsx).toContain('registry-debug-detail-collapsed');
    expect(panelTsx).not.toContain('followOutput');
    expect(panelTsx).not.toContain('payload summary');
    expect(panelTsx).not.toContain('registry-debug-payload-summary');
  });

  test('main renders the panel only for open desktop debug mode', () => {
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("import {RegistryDebugPanel} from './debug/RegistryDebugPanel';");
    expect(mainTsx).toContain('const [registryDebugRecords, setRegistryDebugRecords] = useState(');
    expect(mainTsx).toContain('const [selectedRegistryDebugRecordId, setSelectedRegistryDebugRecordId] = useState<number | null>(null);');
    expect(mainTsx).toContain('const [selectedRegistryDebugScope, setSelectedRegistryDebugScope] = useState');
    expect(mainTsx).toContain('const registryDebugSessionLabels = useMemo');
    expect(mainTsx).toContain('const registryDebugPanel = isWide && registryDebug && registryDebugPanelOpen ? (');
    expect(mainTsx).toContain('<RegistryDebugPanel');
    expect(mainTsx).toContain('records={registryDebugRecords}');
    expect(mainTsx).toContain('selectedScope={selectedRegistryDebugScope}');
    expect(mainTsx).toContain('sessionLabels={registryDebugSessionLabels}');
    expect(mainTsx).toContain('onClear={() => registryDebugStore.clear()}');
  });

  test('styles include floating panel, list, and detail panes', () => {
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(stylesCss).toContain('.registry-debug-panel');
    expect(stylesCss).toContain('.registry-debug-list-pane');
    expect(stylesCss).toContain('.registry-debug-detail-pane');
    expect(stylesCss).toContain('.registry-debug-splitter');
    expect(stylesCss).toContain('flex: 0 0 4px;');
    expect(stylesCss).toContain('.registry-debug-connection-tag.local');
    expect(stylesCss).toContain('.registry-debug-connection-tag.remote');
    expect(stylesCss).toContain('minmax(72px, 150px)');
    expect(stylesCss).toContain('minmax(66px, 120px)');
    expect(stylesCss).toContain('.registry-debug-detail-collapsed');
    expect(stylesCss).toContain('.registry-debug-resize-handle');
  });
});
