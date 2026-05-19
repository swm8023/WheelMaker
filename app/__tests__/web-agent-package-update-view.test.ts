import {deriveRegistryHubIds, packageStatusLabel, wheelMakerUpdateStatusLabel, withAgentPackageTimeout} from '../web/src/agentPackageUpdateView';
import type {RegistryHub} from '../web/src/types/registry';

describe('agent package update view helpers', () => {
  test('derives unique hub ids from project.list hubs in stable sorted order', () => {
    const hubs: RegistryHub[] = [
      {hubId: 'hub-b'},
      {hubId: 'hub-a'},
      {hubId: 'hub-b'},
      {hubId: ' '},
    ];

    expect(deriveRegistryHubIds(hubs)).toEqual(['hub-a', 'hub-b']);
  });

  test('maps package status values to concise labels', () => {
    expect(packageStatusLabel('checking_latest')).toBe('Checking latest');
    expect(packageStatusLabel('not_installed')).toBe('Not installed');
    expect(packageStatusLabel('up_to_date')).toBe('Up to date');
    expect(packageStatusLabel('update_available')).toBe('Update available');
    expect(packageStatusLabel('latest_unknown')).toBe('Latest unknown');
    expect(packageStatusLabel('checking_failed')).toBe('Checking failed');
    expect(packageStatusLabel('deprecated')).toBe('Deprecated');
    expect(packageStatusLabel('running')).toBe('Running');
  });

  test('maps WheelMaker update status values to concise labels', () => {
    expect(wheelMakerUpdateStatusLabel('update_pending')).toBe('Update pending');
    expect(wheelMakerUpdateStatusLabel('not_published')).toBe('Not published');
    expect(wheelMakerUpdateStatusLabel('ahead_of_remote')).toBe('Ahead of remote');
    expect(wheelMakerUpdateStatusLabel('diverged')).toBe('Diverged');
    expect(wheelMakerUpdateStatusLabel('custom_status')).toBe('custom_status');
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
