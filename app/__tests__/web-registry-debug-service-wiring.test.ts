import fs from 'fs';
import path from 'path';

describe('registry debug service wiring', () => {
  const projectRoot = path.join(__dirname, '..');

  test('passes an optional debug sink from workspace service to registry client', () => {
    const repositoryTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'registryRepository.ts'),
      'utf8',
    );
    const workspaceServiceTs = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'services', 'registryWorkspaceService.ts'),
      'utf8',
    );

    expect(repositoryTs).toContain("import { RegistryClient, type RegistryDebugSink } from './registryClient';");
    expect(repositoryTs).toContain('export const createRegistryRepository = (debugSink?: RegistryDebugSink): RegistryRepository => {');
    expect(repositoryTs).toContain('return new RegistryRepository(new RegistryClient(8000, debugSink));');

    expect(workspaceServiceTs).toContain("import type {RegistryDebugSink} from './registryClient';");
    expect(workspaceServiceTs).toContain('constructor(private readonly debugSink?: RegistryDebugSink) {}');
    expect(workspaceServiceTs).toContain('const repository = createRegistryRepository(this.debugSink);');
  });
});
