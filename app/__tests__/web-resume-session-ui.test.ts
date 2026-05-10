import fs from 'fs';
import path from 'path';

describe('web resume session ui', () => {
  test('renders unified session picker controls and reloads immediately after import', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );
    const styles = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'styles.css'),
      'utf8',
    );

    // Resume flow: import + reload
    expect(mainTsx).toContain('const handleResumeBackToAgents = () => {');
    expect(mainTsx).toContain('const handleDismissNewChatPicker = () => {');
    expect(mainTsx).toContain('const handleDismissResume = () => {');
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
    
    // Shared picker structure: both resume and new should use same card/overlay/header/close
    const resumePickerMatch = mainTsx.match(
      /\{resumeAgentPickerOpen[\s\S]*?<div className="chat-agent-picker-card chat-agent-picker-overlay">/,
    );
    const newPickerMatch = mainTsx.match(
      /\{newChatAgentPickerOpen[\s\S]*?<div className="chat-agent-picker-card chat-agent-picker-overlay">/,
    );
    expect(resumePickerMatch).toBeTruthy();
    expect(newPickerMatch).toBeTruthy();
    
    // Both pickers should have: card, overlay, header, header-main, title, close button
    expect(mainTsx).toContain('className="chat-agent-picker-card chat-agent-picker-overlay"');
    expect(mainTsx).toContain('className="chat-agent-picker-header"');
    expect(mainTsx).toContain('className="chat-agent-picker-header-main"');
    expect(mainTsx).toContain('className="chat-agent-picker-title"');
    expect(mainTsx).toContain('className="chat-agent-picker-close"');
    expect(mainTsx).toContain('className="chat-agent-picker-subtitle"');
    expect(mainTsx).toContain('className="chat-agent-picker-actions"');
    
    // Resume picker specific: back button and session list
    expect(mainTsx).toContain('className="chat-agent-picker-back"');
    expect(mainTsx).toContain('className="chat-resume-list"');
    expect(mainTsx).toContain('className="chat-resume-item"');
    
    // Check resume flow dismissal
    expect(mainTsx).toContain('handleDismissResume();');
    
    // Check new flow dismissal
    expect(mainTsx).toContain('handleDismissNewChatPicker();');
    
    // Ensure no conflicting cancel class
    expect(mainTsx).not.toContain('className="chat-agent-picker-cancel"');
    
    // CSS: shared picker styles
    expect(styles).toContain('.chat-agent-picker-card {');
    expect(styles).toContain('.chat-agent-picker-overlay {');
    expect(styles).toContain('.chat-agent-picker-header {');
    expect(styles).toContain('.chat-agent-picker-header-main {');
    expect(styles).toContain('.chat-agent-picker-title {');
    expect(styles).toContain('.chat-agent-picker-close {');
    expect(styles).toContain('.chat-agent-picker-subtitle {');
    expect(styles).toContain('.chat-agent-picker-actions {');
    expect(styles).toContain('.chat-agent-picker-back {');
    expect(styles).toContain('.chat-resume-list {');
    expect(styles).toContain('.chat-resume-item {');
  });
});
