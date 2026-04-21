import fs from 'fs';
import path from 'path';

describe('web reconnect fallback behavior', () => {
  test('keeps cached workspace visible during silent reconnect and falls back after grace period', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('const RECONNECT_GRACE_PERIOD_MS = 30_000;');
    expect(mainTsx).toMatch(
      /const\s+canSilentReconnect\s*=\s*!!tokenRef\.current\.trim\(\)\s*&&\s*!!projectIdRef\.current;/,
    );
    expect(mainTsx).toContain('if (elapsed < RECONNECT_GRACE_PERIOD_MS) {');
    expect(mainTsx).toMatch(
      /connect\(\{\s*silentReconnect:\s*true\s*\}\)\.catch\(\(\)\s*=>\s*undefined\);/,
    );
    expect(mainTsx).toContain('if (!connected && !keepWorkspaceVisible) {');
  });

  test('uses pwa foreground supervisor for background suspend and resume reconnect', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      'const pwaFoundation = initializePWAFoundation();',
    );
    expect(mainTsx).toContain(
      'const supervisor = pwaFoundation.createConnectionSupervisor({',
    );
    expect(mainTsx).toContain('disconnectForSupervisor(reason);');
    expect(mainTsx).toContain('await connect({ silentReconnect: true });');
  });

  test('triggers local push notification for incoming chat messages', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('const maybeNotifyChatMessage = (');
    expect(mainTsx).toContain('message: RegistryChatMessage,');
    expect(mainTsx).toContain('session?: RegistryChatSession,');
    expect(mainTsx).toContain('if (payload.message?.messageId) {');
    expect(mainTsx).toContain(
      'maybeNotifyChatMessage(payload.message, payload.session);',
    );
    expect(mainTsx).toContain(".showLocalNotification({ title, body, url: '/' })");
  });

  test('reloads selected file after reconnect success', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('const selectedFileToReload =');
    expect(mainTsx).toContain(
      'result.hydrated.selectedFile || selectedFileRef.current;',
    );
    expect(mainTsx).toContain(
      'readSelectedFile(selectedFileToReload, { restoreScroll: true }).catch(() => undefined);',
    );
  });
  test('restores selected file scroll position after reconnect success', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      'const fileScrollTopByPathRef = useRef<Record<string, number>>({});',
    );
    expect(mainTsx).toContain(
      'readSelectedFile(selectedFileToReload, { restoreScroll: true }).catch(() => undefined);',
    );
    expect(mainTsx).toContain('const savedTop = fileScrollTopByPathRef.current[path];');
    expect(mainTsx).toContain(
      'fileScrollTopByPathRef.current[path] = event.currentTarget.scrollTop;',
    );
  });
  test('shows reconnecting state through refresh button while recovering', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      "title={reconnecting ? 'Reconnecting...' : 'Refresh project'}",
    );
    expect(mainTsx).toContain('disabled={refreshingProject || reconnecting}');
    expect(mainTsx).toContain('codicon-loading codicon-modifier-spin');
  });
});
