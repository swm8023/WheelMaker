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
    expect(panelTsx).toContain('Include multi-session records');
    expect(panelTsx).toContain('Jump to latest');
    expect(panelTsx).toContain('Clear');
    expect(panelTsx).toContain('Copy');
    expect(panelTsx).toContain('JSON.stringify(selectedEnvelopeOrLifecycle, null, 2)');
    expect(panelTsx).not.toContain('payload summary');
    expect(panelTsx).not.toContain('registry-debug-payload-summary');
  });

  test('main renders the panel only for open desktop debug mode', () => {
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("import {RegistryDebugPanel} from './debug/RegistryDebugPanel';");
    expect(mainTsx).toContain('const [registryDebugRecords, setRegistryDebugRecords] = useState(');
    expect(mainTsx).toContain('const [selectedRegistryDebugRecordId, setSelectedRegistryDebugRecordId] = useState<number | null>(null);');
    expect(mainTsx).toContain('const registryDebugPanel = isWide && registryDebug && registryDebugPanelOpen ? (');
    expect(mainTsx).toContain('<RegistryDebugPanel');
    expect(mainTsx).toContain('records={registryDebugRecords}');
    expect(mainTsx).toContain('onClear={() => registryDebugStore.clear()}');
  });

  test('styles include floating panel, list, and detail panes', () => {
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(stylesCss).toContain('.registry-debug-panel');
    expect(stylesCss).toContain('.registry-debug-list-pane');
    expect(stylesCss).toContain('.registry-debug-detail-pane');
    expect(stylesCss).toContain('.registry-debug-resize-handle');
  });
});
