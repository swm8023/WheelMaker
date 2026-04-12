import fs from 'fs';
import path from 'path';

describe('web clear local cache settings', () => {
  test('exposes settings action that clears local cache while preserving token', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const workspaceStore = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'workspaceStore.ts'), 'utf8');
    const workspacePersistence = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'services', 'workspacePersistence.ts'), 'utf8');

    expect(mainTsx).toContain('Clear Local Cache (Keep Token)');
    expect(mainTsx).toContain("window.confirm('Clear all local cache data except token?')");
    expect(mainTsx).toContain('workspaceStore.clearLocalCachePreservingToken();');
    expect(mainTsx).toContain('window.location.reload();');

    expect(workspaceStore).toContain('clearLocalCachePreservingToken(): void {');
    expect(workspaceStore).toContain('this.persistence.clearCachePreservingToken();');

    expect(workspacePersistence).toContain('clearCachePreservingToken(): void {');
    expect(workspacePersistence).toContain('const preservedToken = this.state.global.token;');
    expect(workspacePersistence).toContain('this.state = defaultWorkspaceState();');
    expect(workspacePersistence).toContain('this.state.global.token = preservedToken;');
  });
});