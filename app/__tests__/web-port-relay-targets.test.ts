import fs from 'fs';
import path from 'path';
import {
  normalizePortRelayTarget,
  orderPortRelayTargetsForMenu,
  reconcilePortRelayTargetSelection,
  samePortRelayTargets,
  upsertPortRelayTarget,
  type PortRelayTarget,
} from '../web/src/portRelayTargets';

const root = path.resolve(__dirname, '..');

describe('port relay target presets', () => {
  test('normalizes hub local port targets and deduplicates by hub and port', () => {
    const targets = [
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-a', targetPort: '5173'},
      {hubId: '', targetPort: 3000},
      {hubId: 'hub-b', targetPort: 0},
      {hubId: 'hub-c', targetPort: 65536},
    ] as Array<Partial<PortRelayTarget> & {targetPort?: unknown}>;

    expect(targets.map(normalizePortRelayTarget).filter(Boolean)).toEqual([
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-a', targetPort: 5173},
    ]);

    expect(upsertPortRelayTarget(targets as PortRelayTarget[], {hubId: 'hub-a', targetPort: 5173})).toEqual([
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-a', targetPort: 5173},
    ]);
  });

  test('adds the active registry snapshot target and selects it locally', () => {
    const result = reconcilePortRelayTargetSelection({
      targets: [{hubId: 'hub-a', targetPort: 80}],
      selectedTarget: {hubId: 'hub-a', targetPort: 80},
      snapshot: {
        ok: true,
        enabled: true,
        status: 'Up',
        hubId: 'hub-b',
        targetPort: 5173,
      },
    });

    expect(result.targets).toEqual([
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-b', targetPort: 5173},
    ]);
    expect(result.selectedTarget).toEqual({hubId: 'hub-b', targetPort: 5173});
  });

  test('compares normalized target lists by value so status refreshes do not churn state', () => {
    expect(samePortRelayTargets(
      [{hubId: 'hub-a', targetPort: 80}],
      [{hubId: 'hub-a', targetPort: 80}],
    )).toBe(true);
    expect(samePortRelayTargets(
      [{hubId: 'hub-a', targetPort: 80}],
      [{hubId: 'hub-a', targetPort: 5173}],
    )).toBe(false);
  });

  test('orders the mobile relay switch menu with the current target first', () => {
    expect(orderPortRelayTargetsForMenu(
      [
        {hubId: 'hub-a', targetPort: 80},
        {hubId: 'hub-b', targetPort: 5173},
        {hubId: 'hub-c', targetPort: 3000},
      ],
      {hubId: 'hub-b', targetPort: 5173},
    )).toEqual([
      {hubId: 'hub-b', targetPort: 5173},
      {hubId: 'hub-a', targetPort: 80},
      {hubId: 'hub-c', targetPort: 3000},
    ]);
  });

  test('persists relay target presets under global workspace state', () => {
    const persistence = fs.readFileSync(
      path.join(root, 'web', 'src', 'services', 'workspacePersistence.ts'),
      'utf8',
    );
    const mainTsx = fs.readFileSync(path.join(root, 'web', 'src', 'main.tsx'), 'utf8');

    expect(persistence).toContain('portRelayTargets: PortRelayTarget[];');
    expect(persistence).toContain('selectedPortRelayTarget: PortRelayTarget | null;');
    expect(persistence).toContain('portRelayListenPort: number;');
    expect(persistence).toContain("portRelayTargets: 'portRelayTargets',");
    expect(persistence).toContain("selectedPortRelayTarget: 'selectedPortRelayTarget',");
    expect(persistence).toContain("portRelayListenPort: 'portRelayListenPort',");
    expect(persistence).toContain('normalizePortRelayTargets(input.portRelayTargets)');
    expect(persistence).toContain('normalizePortRelayTarget(input.selectedPortRelayTarget)');
    expect(persistence).toContain('{k: GLOBAL_KEYS.portRelayTargets, v: serialize(next.portRelayTargets), updatedAt: now}');
    expect(persistence).toContain('{k: GLOBAL_KEYS.selectedPortRelayTarget, v: serialize(next.selectedPortRelayTarget), updatedAt: now}');
    expect(persistence).toContain('{k: GLOBAL_KEYS.portRelayListenPort, v: serialize(next.portRelayListenPort), updatedAt: now}');

    expect(mainTsx).toContain('persistPortRelaySettings(');
    expect(mainTsx).toContain('reconcilePortRelayTargetSelection({');
    expect(mainTsx).toContain('samePortRelayTargets(portRelayTargets, reconciled.targets)');
    expect(mainTsx).toContain('}, [settingsDetailView]);');
    expect(mainTsx).not.toContain('}, [settingsDetailView, refreshPortRelayStatus]);');
  });
});
