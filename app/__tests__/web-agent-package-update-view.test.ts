import {deriveAgentPackageHubIds, packageStatusLabel, withAgentPackageTimeout} from '../web/src/agentPackageUpdateView';
import type {RegistryProject} from '../web/src/types/registry';

describe('agent package update view helpers', () => {
  test('derives unique online hub ids in stable sorted order', () => {
    const projects: RegistryProject[] = [
      {projectId: 'hub-b:server', name: 'server', online: true, path: '', hubId: 'hub-b'},
      {projectId: 'hub-a:app', name: 'app', online: true, path: '', hubId: 'hub-a'},
      {projectId: 'hub-b:docs', name: 'docs', online: true, path: '', hubId: 'hub-b'},
      {projectId: 'hub-c:old', name: 'old', online: false, path: '', hubId: 'hub-c'},
      {projectId: 'hub-d:legacy', name: 'legacy', online: true, path: ''},
    ];

    expect(deriveAgentPackageHubIds(projects)).toEqual(['hub-a', 'hub-b', 'hub-d']);
  });

  test('maps package status values to concise labels', () => {
    expect(packageStatusLabel('not_installed')).toBe('Not installed');
    expect(packageStatusLabel('up_to_date')).toBe('Up to date');
    expect(packageStatusLabel('update_available')).toBe('Update available');
    expect(packageStatusLabel('latest_unknown')).toBe('Latest unknown');
    expect(packageStatusLabel('checking_failed')).toBe('Checking failed');
    expect(packageStatusLabel('deprecated')).toBe('Deprecated');
    expect(packageStatusLabel('running')).toBe('Running');
  });

  test('turns a stuck scan promise into a timeout error', async () => {
    jest.useFakeTimers();
    const pending = withAgentPackageTimeout(
      new Promise(() => undefined),
      25,
      'hub-a scan timed out',
    );

    jest.advanceTimersByTime(25);

    await expect(pending).rejects.toThrow('hub-a scan timed out');
    jest.useRealTimers();
  });
});
