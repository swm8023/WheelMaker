import fs from 'fs';
import path from 'path';

describe('web session list fallback compatibility', () => {
  test('maps chatId to sessionId and falls back to chat.session.list/read', () => {
    const projectRoot = path.join(__dirname, '..');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');

    expect(repositoryTs).toContain("method: 'session.list'");
    expect(repositoryTs).toContain("'chat.session.list'");
    expect(repositoryTs).toContain("method: 'session.read'");
    expect(repositoryTs).toContain("'chat.session.read'");
    expect(repositoryTs).toContain("typeof input.sessionId === 'string'");
    expect(repositoryTs).toContain("typeof input.chatId === 'string'");
  });
});
