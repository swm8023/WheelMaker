import fs from 'fs';
import path from 'path';

describe('web chat project scoping', () => {
  test('guards stale chat loads by selected composite key instead of workspace project', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'main.tsx'),
      'utf8',
    );

    expect(mainTsx).not.toContain('if (activeProjectId !== projectIdRef.current) {');
    expect(mainTsx).toContain('const result = await service.readProjectSession(');
    expect(mainTsx).toContain('activeProjectId,');
    expect(mainTsx).toContain('const currentSelectedRuntimeKey = encodeChatSessionKey(selectedChatKeyRef.current);');
    expect(mainTsx).toContain('currentSelectedRuntimeKey !== selectionSnapshot');
    expect(mainTsx).toContain('selectionSnapshot: runtimeKey,');
  });

  test('workspace service exposes project-scoped session methods for wide navigation', () => {
    const projectRoot = path.join(__dirname, '..');
    const serviceTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'),
      'utf8',
    );

    expect(serviceTs).toContain('async listProjectSessions(projectId: string)');
    expect(serviceTs).toContain('return this.repository.listSessions(projectId);');
    expect(serviceTs).toContain('async createProjectSession(projectId: string, agentType: string, title?: string)');
    expect(serviceTs).toContain('return this.repository.createSession(projectId, agentType, title);');
    expect(serviceTs).toContain('async listProjectResumableSessions(projectId: string, agentType: string)');
    expect(serviceTs).toContain('return this.repository.listResumableSessions(projectId, agentType);');
    expect(serviceTs).toContain('async importProjectResumedSession(projectId: string, agentType: string, sessionId: string)');
    expect(serviceTs).toContain('return this.repository.importResumedSession(projectId, agentType, sessionId);');
    expect(serviceTs).toContain('async reloadProjectSession(projectId: string, sessionId: string)');
    expect(serviceTs).toContain('return this.repository.reloadSession(projectId, sessionId);');
    expect(serviceTs).toContain('async archiveProjectSession(projectId: string, sessionId: string)');
    expect(serviceTs).toContain('return this.repository.archiveSession(projectId, sessionId);');
    expect(serviceTs).toContain('async deleteProjectSession(projectId: string, sessionId: string)');
    expect(serviceTs).toContain('return this.repository.deleteSession(projectId, sessionId);');
  });
});
