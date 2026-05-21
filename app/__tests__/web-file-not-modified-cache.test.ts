import fs from 'fs';
import path from 'path';

describe('web file read cache on notModified', () => {
  test('restores cached content when fs.read returns notModified', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');

    expect(mainTsx).toContain("const fileMemoryCacheKey = (activeProjectId: string, path: string) => `${activeProjectId}\\n${path}`;");
    expect(mainTsx).toContain('const fileCacheRef = useRef<Record<string, string>>({});');
    expect(mainTsx).toContain('const cacheKey = fileMemoryCacheKey(targetProjectId, path);');
    expect(mainTsx).toContain('const cachedContent = fileCacheRef.current[cacheKey] ?? persistedFile?.content;');
    expect(mainTsx).toContain("const knownHash = typeof cachedContent === 'string'");
    expect(mainTsx).toContain('if (result.notModified) {');
    expect(mainTsx).toContain("setFileContent(cachedContent);");
    expect(mainTsx).not.toContain("fileCacheRef.current[path] ?? persistedFile?.content ?? '';");
    expect(mainTsx).toContain('fileCacheRef.current[cacheKey] = result.content;');
    expect(mainTsx).toContain('fileHashRef.current[cacheKey] = nextHash;');
  });

  test('reads files against an explicit project instead of mutable service selection', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const serviceTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'),
      'utf8',
    );

    expect(serviceTs).toContain('async getProjectFileInfo(projectId: string, path: string): Promise<RegistryFsInfo>');
    expect(serviceTs).toContain('async readProjectFile(path: string, projectId: string, options?: {knownHash?: string})');
    expect(mainTsx).toContain('const targetProjectId = projectIdRef.current || projectId;');
    expect(mainTsx).toContain('const info = await service.getProjectFileInfo(targetProjectId, path);');
    expect(mainTsx).toContain('const result = await service.readProjectFile(path, targetProjectId, {');
    expect(mainTsx).toContain('if (requestSeq !== fileReadSeqRef.current || projectIdRef.current !== targetProjectId) return;');
  });
});
