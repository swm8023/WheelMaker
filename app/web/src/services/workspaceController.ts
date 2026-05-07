import type {RegistryFsEntry, RegistryProject} from '../types/registry';
import type {RegistryWorkspaceService} from './registryWorkspaceService';
import type {HydratedProjectState, WorkspaceStore} from './workspaceStore';

function sortEntries(entries: RegistryFsEntry[]): RegistryFsEntry[] {
  return [...entries].sort((a, b) => {
    if (a.kind === 'dir' && b.kind !== 'dir') return -1;
    if (a.kind !== 'dir' && b.kind === 'dir') return 1;
    return a.name.localeCompare(b.name);
  });
}

export type ProjectLoadResult = {
  projects: RegistryProject[];
  rootEntries: RegistryFsEntry[];
  hydrated: HydratedProjectState;
};

export type ValidatedDirectoryState = {
  dirEntries: Record<string, RegistryFsEntry[]>;
  expandedDirs: string[];
};

export class WorkspaceController {
  constructor(
    private readonly service: RegistryWorkspaceService,
    private readonly store: WorkspaceStore,
  ) {}

  async connect(wsUrl: string, token: string): Promise<ProjectLoadResult> {
    const baseSession = await this.service.connect(wsUrl, token.trim());
    const targetProjectId = this.store.selectProjectOnConnect(baseSession.projects, baseSession.selectedProjectId);
    const session = targetProjectId !== baseSession.selectedProjectId
      ? await this.service.selectProject(targetProjectId)
      : baseSession;
    return {
      projects: session.projects,
      rootEntries: session.fileEntries,
      hydrated: this.store.hydrateProject(session.selectedProjectId, session.fileEntries),
    };
  }

  async switchProject(projectId: string): Promise<ProjectLoadResult> {
    const session = await this.service.selectProject(projectId);
    return {
      projects: session.projects,
      rootEntries: session.fileEntries,
      hydrated: this.store.hydrateProject(session.selectedProjectId, session.fileEntries),
    };
  }

  async validateExpandedDirectories(
    projectId: string,
    rootEntries: RegistryFsEntry[],
    expandedSnapshot: string[],
  ): Promise<ValidatedDirectoryState> {
    const dirEntries: Record<string, RegistryFsEntry[]> = {
      '.': sortEntries(rootEntries),
    };
    this.store.cacheDirectory(projectId, '.', '', dirEntries['.']);
    const expandedDirs: string[] = ['.'];
    for (const dirPath of expandedSnapshot) {
      if (dirPath === '.') continue;
      try {
        const cached = this.store.getCachedDirectory(projectId, dirPath);
        const result = await this.service.listDirectory(dirPath, cached?.hash || undefined);
        if (result.notModified && cached) {
          dirEntries[dirPath] = sortEntries(cached.entries);
          expandedDirs.push(dirPath);
          continue;
        }
        const entries = sortEntries(result.entries);
        dirEntries[dirPath] = entries;
        expandedDirs.push(dirPath);
        this.store.cacheDirectory(projectId, dirPath, result.hash || cached?.hash || '', entries);
      } catch {
        // drop stale directory cache entry
      }
    }
    return {dirEntries, expandedDirs};
  }

  async refreshProject(projectId: string, expandedSnapshot: string[]): Promise<ValidatedDirectoryState> {
    const session = await this.service.selectProject(projectId);
    return this.validateExpandedDirectories(projectId, session.fileEntries, expandedSnapshot);
  }
}
