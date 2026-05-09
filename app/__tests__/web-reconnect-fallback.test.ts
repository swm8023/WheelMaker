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
      /const\s+canSilentReconnect\s*=\s*!!addressRef\.current\.trim\(\)\s*&&\s*!!projectIdRef\.current;/,
    );
    expect(mainTsx).toContain('if (elapsed < RECONNECT_GRACE_PERIOD_MS) {');
    expect(mainTsx).toMatch(
      /connect\(\{\s*silentReconnect:\s*true\s*\}\)\.catch\(\(\)\s*=>\s*undefined\);/,
    );
    expect(mainTsx).toContain('if (!connected && !keepWorkspaceVisible) {');
    expect(mainTsx).toContain("tabRef.current === 'chat'");
    expect(mainTsx).toContain('const shouldSyncSelectedSession =');
    expect(mainTsx).toContain('incremental: true,');
    expect(mainTsx).toContain('preserveUserSelection: true,');
    expect(mainTsx).toContain('selectionSnapshot: preferredSelectedChatId,');
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
    expect(mainTsx).toContain('const message = decodeSessionMessageFromEventPayload(payload);');
    expect(mainTsx).toContain('maybeNotifyChatMessage(message);');
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
      'readSelectedFile(selectedFileToReload, { restoreScroll: true, silent: silentReconnect }).catch(() => undefined);',
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
      'readSelectedFile(selectedFileToReload, { restoreScroll: true, silent: silentReconnect }).catch(() => undefined);',
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

  test('keeps workspace visible while background-disconnected and reconnecting', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain('const shouldKeepWorkspaceVisible =');
    expect(mainTsx).toContain(
      "reason !== 'stop' && !!addressRef.current.trim() && !!projectIdRef.current;",
    );
    expect(mainTsx).toContain('setReconnecting(shouldKeepWorkspaceVisible);');
  });

  test('supports silent file reads during reconnect to avoid loading flicker', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).toContain(
      'const readSelectedFile = async (path: string, options?: {restoreScroll?: boolean; silent?: boolean}) => {',
    );
    expect(mainTsx).toContain('const silentRead = options?.silent === true;');
    expect(mainTsx).toContain('if (!silentRead) {');
    expect(mainTsx).toContain('setFileLoading(true);');
  });
});
