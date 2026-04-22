import fs from 'fs';
import path from 'path';

describe('web chat integration', () => {
  test('defines registry session protocol and uses real chat UI instead of placeholder sessions', () => {
    const projectRoot = path.join(__dirname, '..');
    const registryTypes = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'types', 'registry.ts'), 'utf8');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');
    const workspaceServiceTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'), 'utf8');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(registryTypes).toContain('export interface RegistrySessionSummary');
    expect(registryTypes).toContain('export interface RegistrySessionMessage');
    expect(registryTypes).toContain('syncIndex');
    expect(registryTypes).toContain('lastIndex');
    expect(repositoryTs).toContain("method: 'session.list'");
    expect(repositoryTs).toContain("method: 'session.read'");
    expect(repositoryTs).toContain('afterIndex');
    expect(repositoryTs).toContain("method: 'session.new'");
    expect(repositoryTs).toContain("method: 'session.send'");
    expect(repositoryTs).not.toContain("method: 'session.markRead'");
    expect(repositoryTs).not.toContain('turnId = typeof input.turnId');
    expect(repositoryTs).toContain("method: 'chat.permission.respond'");
    expect(workspaceServiceTs).toContain('async listSessions(');
    expect(workspaceServiceTs).toContain('async readSession(');
    expect(workspaceServiceTs).toContain('async createSession(');
    expect(workspaceServiceTs).toContain('async sendSessionMessage(');
    expect(workspaceServiceTs).not.toContain('async markSessionRead(');
    expect(workspaceServiceTs).toContain('async respondToSessionPermission(');
    expect(workspaceServiceTs).toContain('private eventListeners = new Set');
    expect(workspaceServiceTs).toContain('private closeListeners = new Set');
    expect(registryTypes).not.toContain('status?: string;');
    expect(registryTypes).not.toContain('turnId: string;');
    expect(registryTypes).not.toContain('turnId?: string;');
    expect(mainTsx).toContain('chatComposerText');
    expect(mainTsx).toContain('chatMessages');
    expect(mainTsx).toContain('session.message');
    expect(mainTsx).toContain('`${sessionId}:${promptIndex}:${turnIndex}:${updateIndex}`');
    expect(mainTsx).not.toContain('await service.markSessionRead(');
    expect(mainTsx).toContain('chatSyncIndexRef');
    expect(mainTsx).toContain('sessions.some(session => session.sessionId === preferredSessionId)');
    expect(mainTsx).toContain('result.lastIndex < afterIndex');
    expect(mainTsx).toContain('preserveUserSelection');
    expect(mainTsx).toContain('selectionSnapshot');
    expect(mainTsx).toContain('chatSelectedIdRef.current = session.sessionId');
    expect(mainTsx).toContain('sessionId');
    expect(mainTsx).toContain('type="file"');
    expect(mainTsx).not.toContain("const [chatSessions] = useState(['General', 'WheelMaker App', 'Go Service']);");
    expect(stylesCss).toContain('.chat-composer');
    expect(stylesCss).toContain('.chat-message');
  });
});

