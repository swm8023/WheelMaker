import fs from 'fs';
import path from 'path';

describe('web reconnect fallback behavior', () => {
  test('keeps cached workspace visible during silent reconnect and falls back after grace period', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain('const RECONNECT_GRACE_PERIOD_MS = 30_000;');
    expect(mainTsx).toContain("const canSilentReconnect = !!token.trim() && !!projectIdRef.current;");
    expect(mainTsx).toContain('if (elapsed < RECONNECT_GRACE_PERIOD_MS) {');
    expect(mainTsx).toContain('connect({silentReconnect: true}).catch(() => undefined);');
    expect(mainTsx).toContain('if (!connected && !keepWorkspaceVisible) {');
  });

  test('shows reconnecting state through refresh button while recovering', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("title={reconnecting ? 'Reconnecting...' : 'Refresh project'}");
    expect(mainTsx).toContain('disabled={refreshingProject || reconnecting}');
    expect(mainTsx).toContain('codicon-loading codicon-modifier-spin');
  });
});