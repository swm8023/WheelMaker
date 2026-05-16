import fs from 'fs';
import path from 'path';

describe('web resume session ui', () => {
  test('uses project-scoped resume controls without legacy chat pickers', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );
    const styles = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'styles.css'),
      'utf8',
    );

    // Project scoped resume flow: import + reload
    expect(mainTsx).toContain('const handleWideProjectResumeAgent = async (targetProjectId: string, agentType: string) => {');
    expect(mainTsx).toContain('const handleMobileProjectResumeAgent = async (targetProjectId: string, agentType: string) => {');
    expect(mainTsx).toContain(
      'const imported = await service.importProjectResumedSession(targetProjectId, agentType, sessionId);',
    );
    expect(mainTsx).toContain("let importedSessionId = '';");
    expect(mainTsx).toContain(
      'setResumeSessions(prev => prev.filter(item => item.sessionId !== sessionId));',
    );
    expect(mainTsx).toContain(
      'const reloaded = await service.reloadProjectSession(targetProjectId, importedSessionId);',
    );
    expect(mainTsx).toContain('if (importedSessionId) {');

    // Legacy chat picker should be gone; project action menus are the only session creation/resume UI.
    expect(mainTsx).not.toContain('resumeAgentPickerOpen');
    expect(mainTsx).not.toContain('newChatAgentPickerOpen');
    expect(mainTsx).not.toContain('className="chat-agent-picker-card chat-agent-picker-overlay"');
    expect(mainTsx).not.toContain('className="chat-resume-list"');
    expect(mainTsx).toContain('className="wide-project-action-popover"');
    expect(mainTsx).toContain('className="mobile-project-action-panel"');
    expect(mainTsx).toContain("wideProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'");
    expect(mainTsx).toContain("activeMobileProjectActionMenu.kind === 'new' ? 'New Session' : 'Resume Session'");

    // Ensure no conflicting cancel class.
    expect(mainTsx).not.toContain('className="chat-agent-picker-cancel"');

    // CSS: legacy picker styles are removed with the old chat page.
    expect(styles).not.toContain('.chat-agent-picker-card {');
    expect(styles).not.toContain('.chat-agent-picker-overlay {');
    expect(styles).not.toContain('.chat-resume-list {');
    expect(styles).not.toContain('.chat-resume-item {');
    expect(styles).toContain('.wide-project-action-popover {');
    expect(styles).toContain('.mobile-project-action-panel {');
  });
});
