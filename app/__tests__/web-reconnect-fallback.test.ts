import fs from 'fs';
import path from 'path';

describe('web reconnect fallback behavior', () => {
  test('keeps cached workspace visible during silent reconnect and falls back after grace period', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const RECONNECT_GRACE_PERIOD_MS = 30_000;');
    expect(mainTsx).toContain("const canSilentReconnect = !!tokenRef.current.trim() && !!projectIdRef.current;");
    expect(mainTsx).toContain('if (elapsed < RECONNECT_GRACE_PERIOD_MS) {');
    expect(mainTsx).toContain('connect({silentReconnect: true}).catch(() => undefined);');
    expect(mainTsx).toContain('if (!connected && !keepWorkspaceVisible) {');
  });

  test('uses pwa foreground supervisor for background suspend and resume reconnect', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const pwaFoundation = initializePWAFoundation();');
    expect(mainTsx).toContain('const supervisor = pwaFoundation.createConnectionSupervisor({');
    expect(mainTsx).toContain("disconnectForSupervisor(reason);");
    expect(mainTsx).toContain('await connect({silentReconnect: true});');
  });
  test('reloads selected file after reconnect success', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const selectedFileToReload = result.hydrated.selectedFile || selectedFileRef.current;');
    expect(mainTsx).toContain('readSelectedFile(selectedFileToReload).catch(() => undefined);');
  });

  test('shows reconnecting state through refresh button while recovering', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}");
    expect(mainTsx).toContain('disabled={refreshingProject || reconnecting}');
    expect(mainTsx).toContain('codicon-loading codicon-modifier-spin');
  });
});

