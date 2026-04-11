import fs from 'fs';
import path from 'path';

describe('web chat integration', () => {
  test('defines registry chat protocol and uses real chat UI instead of placeholder sessions', () => {
    const projectRoot = path.join(__dirname, '..');
    const registryTypes = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'types', 'registry.ts'), 'utf8');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');
    const workspaceServiceTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'), 'utf8');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(registryTypes).toContain('export interface RegistryChatSession');
    expect(registryTypes).toContain('export interface RegistryChatMessage');
    expect(repositoryTs).toContain("method: 'chat.session.list'");
    expect(repositoryTs).toContain("method: 'chat.session.read'");
    expect(repositoryTs).toContain("method: 'chat.send'");
    expect(repositoryTs).toContain("method: 'chat.permission.respond'");
    expect(workspaceServiceTs).toContain('async listChatSessions(');
    expect(workspaceServiceTs).toContain('async readChatSession(');
    expect(workspaceServiceTs).toContain('async sendChatMessage(');
    expect(workspaceServiceTs).toContain('async respondToChatPermission(');
    expect(mainTsx).toContain('chatComposerText');
    expect(mainTsx).toContain('chatMessages');
    expect(mainTsx).toContain('chat.message');
    expect(mainTsx).toContain('type="file"');
    expect(mainTsx).not.toContain("const [chatSessions] = useState(['General', 'WheelMaker App', 'Go Service']);");
    expect(stylesCss).toContain('.chat-composer');
    expect(stylesCss).toContain('.chat-message');
  });
});
