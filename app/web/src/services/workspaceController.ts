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

  async validateExpandedDirectories(rootEntries: RegistryFsEntry[], expandedSnapshot: string[]): Promise<ValidatedDirectoryState> {
    const dirEntries: Record<string, RegistryFsEntry[]> = {
      '.': sortEntries(rootEntries),
    };
    const expandedDirs: string[] = ['.'];
    for (const dirPath of expandedSnapshot) {
      if (dirPath === '.') continue;
      try {
        dirEntries[dirPath] = sortEntries(await this.service.listDirectory(dirPath));
        expandedDirs.push(dirPath);
      } catch {
        // drop stale directory cache entry
      }
    }
    return {dirEntries, expandedDirs};
  }

  async refreshProject(projectId: string, expandedSnapshot: string[]): Promise<ValidatedDirectoryState> {
    const session = await this.service.selectProject(projectId);
    return this.validateExpandedDirectories(session.fileEntries, expandedSnapshot);
  }
}
