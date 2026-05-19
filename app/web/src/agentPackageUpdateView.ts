import type {RegistryNpmPackageStatus, RegistryProject} from './types/registry';

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
