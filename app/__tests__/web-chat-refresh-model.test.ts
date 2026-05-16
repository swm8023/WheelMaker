import fs from 'fs';
import path from 'path';

function readMain(): string {
  const projectRoot = path.join(__dirname, '..');
  return fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
}

describe('web chat refresh model', () => {
  test('mobile drawer open does not trigger an online chat session fan-out', () => {
    const main = readMain();

    expect(main).not.toContain("if (isWide || tab !== 'chat' || !drawerOpen || !connected)");
    expect(main).not.toContain('refreshMobileChatProjectSessions().catch(() => undefined);\n  }, [isWide, tab, drawerOpen, connected, projectIdListKey]);');
  });

  test('manual and reconnect refresh use coalesced chat index refresh helpers', () => {
    const main = readMain();

    expect(main).toContain('const refreshChatIndex = async');
    expect(main).toContain('const refreshChatProjectSessions = async');
    expect(main).toContain('chatIndexFullRefreshInFlightRef');
    expect(main).toContain('chatProjectRefreshInFlightRef');
    expect(main).toContain('await refreshChatIndex();');
  });

  test('session events use envelope project id and never fall back to workspace project', () => {
    const main = readMain();

    expect(main).toContain('if (!eventProjectId) {');
    expect(main).toContain('projectsRef.current.some(item => item.projectId === eventProjectId)');
    expect(main).toContain('refreshChatProjectSessions(eventProjectId)');
    expect(main).toContain('const runtimeKey = buildChatRuntimeKey(eventProjectId, sessionId);');
    expect(main).not.toContain('const targetProjectId = eventProjectId || projectIdRef.current;');
  });
});
