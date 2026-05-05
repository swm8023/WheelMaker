import fs from 'fs';
import path from 'path';

describe('web resume session ui', () => {
  test('renders resume overlay controls and reloads immediately after import', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );
    const styles = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'styles.css'),
      'utf8',
    );

    expect(mainTsx).toContain('const handleResumeBackToAgents = () => {');
    expect(mainTsx).toContain(
      'const imported = await service.importResumedSession(agentType, sessionId);',
    );
    expect(mainTsx).toContain("let importedSessionId = '';");
    expect(mainTsx).toContain(
      'setResumeSessions(prev => prev.filter(item => item.sessionId !== importedSessionId));',
    );
    expect(mainTsx).toContain(
      'const reloaded = await service.reloadSession(importedSessionId);',
    );
    expect(mainTsx).toContain(
      'const loaded = await loadChatSession(importedSessionId, projectIdRef.current, { forceFull: true });',
    );
    expect(mainTsx).toContain('if (importedSessionId) {');
    expect(mainTsx).toContain('handleDismissResume();');
    expect(mainTsx).toContain('className="chat-agent-picker-close"');
    expect(mainTsx).toContain('className="chat-agent-picker-back"');
    expect(styles).toContain('.chat-agent-picker-close {');
    expect(styles).toContain('.chat-agent-picker-back {');
  });
});
