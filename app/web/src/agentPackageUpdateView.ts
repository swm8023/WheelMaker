import type {RegistryNpmPackageStatus, RegistryProject} from './types/registry';

export const AGENT_PACKAGE_SCAN_TIMEOUT_MS = 65000;

export function deriveAgentPackageHubIds(projects: RegistryProject[]): string[] {
  const hubIds = new Set<string>();
  projects.forEach(project => {
    if (project.online === false) return;
    const hubId = (project.hubId || project.projectId.split(':', 1)[0] || '').trim();
    if (hubId) hubIds.add(hubId);
  });
  return Array.from(hubIds).sort((a, b) => {
    if (a < b) return -1;
    if (a > b) return 1;
    return 0;
  });
}

export function packageStatusLabel(status: RegistryNpmPackageStatus | string): string {
  switch (status) {
    case 'not_installed':
      return 'Not installed';
    case 'up_to_date':
      return 'Up to date';
    case 'update_available':
      return 'Update available';
    case 'latest_unknown':
      return 'Latest unknown';
    case 'checking_failed':
      return 'Checking failed';
    case 'deprecated':
      return 'Deprecated';
    case 'installing':
      return 'Installing';
    case 'updating':
      return 'Updating';
    case 'uninstalling':
      return 'Uninstalling';
    case 'running':
      return 'Running';
    case 'succeeded':
      return 'Succeeded';
    case 'failed':
      return 'Failed';
    default:
      return status || 'Unknown';
  }
}

export function wheelMakerUpdateStatusLabel(status: string): string {
  switch (status) {
    case 'up_to_date':
      return 'Up to date';
    case 'update_available':
      return 'Update available';
    case 'update_pending':
      return 'Update pending';
    case 'not_published':
      return 'Not published';
    case 'checking_failed':
      return 'Checking failed';
    case 'ahead_of_remote':
      return 'Ahead of remote';
    case 'diverged':
      return 'Diverged';
    default:
      return status || 'Unknown';
  }
}

export function withAgentPackageTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  message: string,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      reject(new Error(message));
    }, timeoutMs);
    promise.then(
      value => {
        globalThis.clearTimeout(timer);
        resolve(value);
      },
      error => {
        globalThis.clearTimeout(timer);
        reject(error);
      },
    );
  });
}
