import {
  deriveNpmPackageUpdateTargets,
  deriveRegistryHubIds,
  npmPackageUpdateSummary,
  packageStatusLabel,
  shouldShowWheelMakerUpdateAction,
  wheelMakerUpdateStatusLabel,
  withAgentPackageTimeout,
} from '../web/src/agentPackageUpdateView';
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

  test('derives hub-level npm update targets without uninstalling deprecated packages', () => {
    const targets = deriveNpmPackageUpdateTargets([
      {
        packageName: '@openai/codex',
        displayName: 'Codex',
        agentTypes: ['codex'],
        kind: 'runtime',
        installed: true,
        installedVersion: '0.1.0',
        latestVersion: '0.2.0',
        status: 'update_available',
        error: '',
        canInstall: false,
        canUpdate: true,
        canUninstall: false,
      },
      {
        packageName: '@anthropic/claude-code',
        displayName: 'Claude Code',
        agentTypes: ['claude'],
        kind: 'runtime',
        installed: false,
        installedVersion: '',
        latestVersion: '1.0.0',
        status: 'not_installed',
        error: '',
        canInstall: true,
        canUpdate: false,
        canUninstall: false,
      },
      {
        packageName: '@zed-industries/claude-agent-acp',
        displayName: 'Deprecated Claude ACP',
        agentTypes: [],
        kind: 'deprecated',
        installed: true,
        installedVersion: '0.0.1',
        latestVersion: '',
        status: 'deprecated',
        error: '',
        canInstall: false,
        canUpdate: false,
        canUninstall: true,
      },
    ]);

    expect(targets).toEqual([
      {
        packageName: '@openai/codex',
        displayName: 'Codex',
        installedVersion: '0.1.0',
        latestVersion: '0.2.0',
      },
      {
        packageName: '@anthropic/claude-code',
        displayName: 'Claude Code',
        installedVersion: '',
        latestVersion: '1.0.0',
      },
    ]);
    expect(npmPackageUpdateSummary(targets.length)).toBe('2 npm updates');
    expect(npmPackageUpdateSummary(0)).toBe('No npm updates');
  });

  test('maps WheelMaker update status values to concise labels', () => {
    expect(wheelMakerUpdateStatusLabel('update_pending')).toBe('Update pending');
    expect(wheelMakerUpdateStatusLabel('not_published')).toBe('Not published');
    expect(wheelMakerUpdateStatusLabel('ahead_of_remote')).toBe('Ahead of remote');
    expect(wheelMakerUpdateStatusLabel('diverged')).toBe('Diverged');
    expect(wheelMakerUpdateStatusLabel('custom_status')).toBe('custom_status');
  });

  test('does not allow WheelMaker update while the hub is still checking', () => {
    expect(
      shouldShowWheelMakerUpdateAction({
        data: null,
        loading: true,
        pending: false,
      }),
    ).toBe(false);
    expect(
      shouldShowWheelMakerUpdateAction({
        data: {
          ok: true,
          status: 'up_to_date',
          hubId: 'hub-a',
          pendingSignal: false,
          canUpdatePublish: true,
        },
        loading: false,
        pending: false,
      }),
    ).toBe(false);
    expect(
      shouldShowWheelMakerUpdateAction({
        data: {
          ok: true,
          status: 'update_available',
          hubId: 'hub-a',
          pendingSignal: false,
          canUpdatePublish: true,
        },
        loading: false,
        pending: false,
      }),
    ).toBe(true);
    expect(
      shouldShowWheelMakerUpdateAction({
        data: {
          ok: true,
          status: 'update_pending',
          hubId: 'hub-a',
          pendingSignal: true,
          canUpdatePublish: true,
        },
        loading: true,
        pending: false,
      }),
    ).toBe(true);
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
