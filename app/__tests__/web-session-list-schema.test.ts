import fs from 'fs';
import path from 'path';

describe('web session list schema', () => {
  test('uses sessionId without legacy chatId compatibility', () => {
    const projectRoot = path.join(__dirname, '..');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');
    const serviceTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'), 'utf8');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const registryTypes = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'types', 'registry.ts'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(repositoryTs).toContain("method: 'session.list'");
    expect(repositoryTs).toContain("method: 'session.read'");
    expect(repositoryTs).toContain("method: 'session.markRead'");
    expect(repositoryTs).toContain("typeof input.sessionId === 'string'");
    expect(repositoryTs).toContain('input.running === true');
    expect(repositoryTs).toContain('typeof input.lastDoneTurnIndex ===');
    expect(repositoryTs).toContain('typeof input.lastDoneSuccess ===');
    expect(repositoryTs).toContain('typeof input.lastReadTurnIndex ===');
    expect(serviceTs).toContain('async markProjectSessionRead(');
    expect(registryTypes).toContain('running?: boolean;');
    expect(registryTypes).toContain('lastDoneTurnIndex?: number;');
    expect(registryTypes).toContain('lastDoneSuccess?: boolean;');
    expect(registryTypes).toContain('lastReadTurnIndex?: number;');
    expect(mainTsx).toContain('const renderSessionStateMarker = (session: RegistryChatSession, activeProjectId = projectIdRef.current) => {');
    expect(mainTsx).toContain('const runtimeKey = buildChatRuntimeKey(activeProjectId, session.sessionId);');
    expect(mainTsx).toContain('resolveChatSessionVisualStateValue(session, {');
    expect(mainTsx).toContain('service.markProjectSessionRead(activeProjectId, sessionId, cursor)');
    expect(mainTsx).toContain('rememberChatSessionSummary(eventProjectId, payload.session);');
    expect(mainTsx).toContain('workspaceStore.rememberChatSession(eventProjectId, payload.session, {');
    expect(stylesCss).toContain('grid-template-columns: 6px minmax(0, 1fr) auto auto;');
    expect(stylesCss).toContain('.session-state-marker.running');
    expect(stylesCss).toContain('.session-state-marker.failed-unviewed .session-state-dot');
    expect(repositoryTs).not.toContain('input.chatId');
    expect(mainTsx).toContain("const baseTitle = 'WheelMaker';");
    expect(mainTsx).toContain('const currentProjectTitle = useMemo(');
    expect(mainTsx).toContain("document.title = projectTitle ? `${baseTitle} - ${projectTitle}` : baseTitle;");
    expect(mainTsx).toContain("}, [currentProjectTitle]);");
  });
});

