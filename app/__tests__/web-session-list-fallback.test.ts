import fs from 'fs';
import path from 'path';

describe('web session list fallback compatibility', () => {
  test('maps chatId to sessionId compatibility in normalization', () => {
    const projectRoot = path.join(__dirname, '..');
    const repositoryTs = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'), 'utf8');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(repositoryTs).toContain("method: 'session.list'");
    expect(repositoryTs).toContain("method: 'session.read'");
    expect(repositoryTs).toContain("typeof input.sessionId === 'string'");
    expect(repositoryTs).toContain("typeof input.chatId === 'string'");
    expect(mainTsx).toContain("const baseTitle = 'WheelMaker';");
    expect(mainTsx).toContain('const currentProjectTitle = useMemo(');
    expect(mainTsx).toContain("document.title = projectTitle ? `${baseTitle} - ${projectTitle}` : baseTitle;");
    expect(mainTsx).toContain("}, [currentProjectTitle]);");
  });
});

