import fs from 'fs';
import path from 'path';

describe('web registry debug settings', () => {
  const projectRoot = path.join(__dirname, '..');

  test('persists registry debug as a default-off global setting', () => {
    const workspacePersistence = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );

    expect(workspacePersistence).toContain('registryDebug: boolean;');
    expect(workspacePersistence).toContain("registryDebug: 'registryDebug',");
    expect(workspacePersistence).toContain('registryDebug: false,');
    expect(workspacePersistence).toContain(
      "registryDebug: typeof input.registryDebug === 'boolean' ? input.registryDebug : base.registryDebug",
    );
    expect(workspacePersistence).toContain(
      '{k: GLOBAL_KEYS.registryDebug, v: serialize(next.registryDebug), updatedAt: now}',
    );
  });

  test('adds a settings debug switch and open button without making records persistent', () => {
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("import {createRegistryDebugStore} from './debug/registryDebug';");
    expect(mainTsx).toContain('const registryDebugStore = createRegistryDebugStore();');
    expect(mainTsx).toContain('const service = new RegistryWorkspaceService(registryDebugStore.recordCaptureEvent);');
    expect(mainTsx).toContain('const [registryDebug, setRegistryDebug] = useState(');
    expect(mainTsx).toContain('const [registryDebugPanelOpen, setRegistryDebugPanelOpen] = useState(');
    expect(mainTsx).toContain('registryDebugStore.setEnabled(registryDebug);');
    expect(mainTsx).toContain("renderSettingsSection('Debug'");
    expect(mainTsx).toContain('<span>Debug</span>');
    expect(mainTsx).toContain('checked={registryDebug}');
    expect(mainTsx).toContain('onChange={event => setRegistryDebug(event.target.checked)}');
    expect(mainTsx).toContain('disabled={!registryDebug}');
    expect(mainTsx).toContain('setRegistryDebugPanelOpen(true)');
    expect(mainTsx).not.toContain('registryDebugRecordsJson');
  });
});
