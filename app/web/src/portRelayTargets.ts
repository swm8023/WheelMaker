import type {RegistryPortRelaySnapshot} from './types/registry';

export type PortRelayTarget = {
  hubId: string;
  targetPort: number;
};

export type PortRelayTargetSelection = {
  targets: PortRelayTarget[];
  selectedTarget: PortRelayTarget | null;
};

export function normalizePortRelayTarget(input: unknown): PortRelayTarget | null {
  if (!input || typeof input !== 'object') {
    return null;
  }
  const candidate = input as {hubId?: unknown; targetPort?: unknown};
  if (typeof candidate.hubId !== 'string' || !candidate.hubId) {
    return null;
  }
  const numericPort = typeof candidate.targetPort === 'string'
    ? Number(candidate.targetPort)
    : candidate.targetPort;
  if (!Number.isInteger(numericPort) || Number(numericPort) < 1 || Number(numericPort) > 65535) {
    return null;
  }
  return {
    hubId: candidate.hubId,
    targetPort: Number(numericPort),
  };
}

export function portRelayTargetKey(target: PortRelayTarget): string {
  return `${target.hubId}:${target.targetPort}`;
}

export function samePortRelayTarget(left: PortRelayTarget | null, right: PortRelayTarget | null): boolean {
  if (!left || !right) {
    return false;
  }
  return left.hubId === right.hubId && left.targetPort === right.targetPort;
}

export function samePortRelayTargets(left: unknown, right: unknown): boolean {
  const normalizedLeft = normalizePortRelayTargets(left);
  const normalizedRight = normalizePortRelayTargets(right);
  if (normalizedLeft.length !== normalizedRight.length) {
    return false;
  }
  return normalizedLeft.every((target, index) => samePortRelayTarget(target, normalizedRight[index]));
}

export function normalizePortRelayTargets(input: unknown): PortRelayTarget[] {
  if (!Array.isArray(input)) {
    return [];
  }
  const seen = new Set<string>();
  const targets: PortRelayTarget[] = [];
  for (const item of input) {
    const target = normalizePortRelayTarget(item);
    if (!target) {
      continue;
    }
    const key = portRelayTargetKey(target);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    targets.push(target);
  }
  return targets;
}

export function upsertPortRelayTarget(targets: unknown, target: unknown): PortRelayTarget[] {
  const existing = normalizePortRelayTargets(targets);
  const normalizedTarget = normalizePortRelayTarget(target);
  if (!normalizedTarget) {
    return existing;
  }
  const key = portRelayTargetKey(normalizedTarget);
  if (existing.some(item => portRelayTargetKey(item) === key)) {
    return existing;
  }
  return [...existing, normalizedTarget];
}

export function removePortRelayTarget(targets: unknown, target: unknown): PortRelayTarget[] {
  const normalizedTarget = normalizePortRelayTarget(target);
  const existing = normalizePortRelayTargets(targets);
  if (!normalizedTarget) {
    return existing;
  }
  const key = portRelayTargetKey(normalizedTarget);
  return existing.filter(item => portRelayTargetKey(item) !== key);
}

export function orderPortRelayTargetsForMenu(targets: unknown, activeTarget: unknown): PortRelayTarget[] {
  const existing = normalizePortRelayTargets(targets);
  const normalizedActiveTarget = normalizePortRelayTarget(activeTarget);
  if (!normalizedActiveTarget) {
    return existing;
  }
  return [
    normalizedActiveTarget,
    ...existing.filter(target => !samePortRelayTarget(target, normalizedActiveTarget)),
  ];
}

export function reconcilePortRelayTargetSelection(input: {
  targets: unknown;
  selectedTarget: unknown;
  snapshot: RegistryPortRelaySnapshot;
}): PortRelayTargetSelection {
  const targets = normalizePortRelayTargets(input.targets);
  const selectedTarget = normalizePortRelayTarget(input.selectedTarget);
  const snapshotTarget = input.snapshot.enabled
    ? normalizePortRelayTarget({
        hubId: input.snapshot.hubId,
        targetPort: input.snapshot.targetPort,
      })
    : null;
  if (!snapshotTarget) {
    return {
      targets,
      selectedTarget,
    };
  }
  return {
    targets: upsertPortRelayTarget(targets, snapshotTarget),
    selectedTarget: snapshotTarget,
  };
}

export function normalizePortRelayListenPort(input: unknown, fallback = 28810): number {
  const numericPort = typeof input === 'string' ? Number(input) : input;
  if (!Number.isInteger(numericPort) || Number(numericPort) < 1 || Number(numericPort) > 65535) {
    return fallback;
  }
  return Number(numericPort);
}
