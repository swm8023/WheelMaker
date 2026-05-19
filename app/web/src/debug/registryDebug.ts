import type {RegistryEnvelope} from '../types/registry';

export type RegistryDebugDirection = 'out' | 'in' | 'lifecycle';

export type RegistryDebugPhase =
  | 'request'
  | 'response'
  | 'error'
  | 'event'
  | 'parse_error'
  | 'connect_start'
  | 'connect_open'
  | 'connect_close'
  | 'connect_error';

export type RegistryDebugLifecyclePayload = {
  phase: Extract<RegistryDebugPhase, 'connect_start' | 'connect_open' | 'connect_close' | 'connect_error'>;
  url?: string;
  code?: number;
  reason?: string;
  message?: string;
};

export type RegistryDebugRecord = {
  id: number;
  timestamp: number;
  timeText: string;
  direction: RegistryDebugDirection;
  phase: RegistryDebugPhase;
  method?: string;
  requestId?: number;
  projectId?: string;
  sessionIds: string[];
  multiSession: boolean;
  durationMs?: number;
  raw?: string;
  envelope?: RegistryEnvelope;
  lifecycle?: RegistryDebugLifecyclePayload;
  parseError?: string;
};

export type RegistryDebugCaptureEvent =
  | {
      kind: 'outbound';
      envelope: RegistryEnvelope;
      raw: string;
      timestamp?: number;
    }
  | {
      kind: 'inbound';
      envelope: RegistryEnvelope;
      raw: string;
      timestamp?: number;
    }
  | {
      kind: 'parse_error';
      raw: string;
      error: string;
      timestamp?: number;
    }
  | {
      kind: 'lifecycle';
      lifecycle: RegistryDebugLifecyclePayload;
      timestamp?: number;
    };

export type RegistryDebugOutboundInput = Extract<RegistryDebugCaptureEvent, {kind: 'outbound'}>;
export type RegistryDebugInboundInput = Extract<RegistryDebugCaptureEvent, {kind: 'inbound'}>;
export type RegistryDebugParseErrorInput = Extract<RegistryDebugCaptureEvent, {kind: 'parse_error'}>;
export type RegistryDebugLifecycleInput = Extract<RegistryDebugCaptureEvent, {kind: 'lifecycle'}>;
export type RegistryDebugSubscriber = (records: RegistryDebugRecord[]) => void;

type CorrelatedRequest = {
  method?: string;
  projectId?: string;
  sessionIds: string[];
  timestamp: number;
};

export type RegistryDebugStore = {
  setEnabled: (enabled: boolean) => void;
  isEnabled: () => boolean;
  clear: () => void;
  getRecords: () => RegistryDebugRecord[];
  subscribe: (subscriber: RegistryDebugSubscriber) => () => void;
  recordCaptureEvent: (event: RegistryDebugCaptureEvent) => void;
  recordOutbound: (input: Omit<RegistryDebugOutboundInput, 'kind'>) => void;
  recordInboundEnvelope: (input: Omit<RegistryDebugInboundInput, 'kind'>) => void;
  recordInboundParseError: (input: Omit<RegistryDebugParseErrorInput, 'kind'>) => void;
  recordLifecycle: (input: Omit<RegistryDebugLifecycleInput, 'kind'>) => void;
};

const MAX_EXTRACT_DEPTH = 8;
const MAX_EXTRACT_NODES = 500;

function appendUnique(items: string[], value: unknown): void {
  if (typeof value !== 'string' || value.length === 0) {
    return;
  }
  if (!items.includes(value)) {
    items.push(value);
  }
}

export function extractRegistryDebugSessionIds(value: unknown): string[] {
  const sessionIds: string[] = [];
  const seen = new Set<unknown>();
  let visitedNodes = 0;

  const visit = (input: unknown, depth: number) => {
    if (depth > MAX_EXTRACT_DEPTH || visitedNodes >= MAX_EXTRACT_NODES) {
      return;
    }
    if (!input || typeof input !== 'object') {
      return;
    }
    if (seen.has(input)) {
      return;
    }
    seen.add(input);
    visitedNodes += 1;

    if (Array.isArray(input)) {
      for (const item of input) {
        visit(item, depth + 1);
      }
      return;
    }

    const record = input as Record<string, unknown>;
    appendUnique(sessionIds, record.sessionId);
    for (const item of Object.values(record)) {
      visit(item, depth + 1);
    }
  };

  visit(value, 0);
  return sessionIds;
}

function pad(value: number, length: number): string {
  return String(value).padStart(length, '0');
}

export function formatRegistryDebugTime(timestamp: number): string {
  const date = new Date(timestamp);
  return [
    pad(date.getHours(), 2),
    pad(date.getMinutes(), 2),
    pad(date.getSeconds(), 2),
  ].join(':') + `.${pad(date.getMilliseconds(), 3)}`;
}

function resolveInboundPhase(type: RegistryEnvelope['type']): Extract<RegistryDebugPhase, 'response' | 'error' | 'event'> {
  if (type === 'error') {
    return 'error';
  }
  if (type === 'event') {
    return 'event';
  }
  return 'response';
}

function cloneRecords(records: RegistryDebugRecord[]): RegistryDebugRecord[] {
  return records.slice();
}

export function filterRegistryDebugRecords(
  records: RegistryDebugRecord[],
  selectedSessionId: string,
  includeMultiSessionRecords: boolean,
): RegistryDebugRecord[] {
  if (!selectedSessionId || selectedSessionId === 'All') {
    return records;
  }
  return records.filter(record => {
    if (!record.sessionIds.includes(selectedSessionId)) {
      return false;
    }
    if (record.multiSession && !includeMultiSessionRecords) {
      return false;
    }
    return true;
  });
}

export function createRegistryDebugStore(now: () => number = () => Date.now()): RegistryDebugStore {
  let enabled = false;
  let nextId = 1;
  let records: RegistryDebugRecord[] = [];
  const requests = new Map<number, CorrelatedRequest>();
  const subscribers = new Set<RegistryDebugSubscriber>();

  const notify = () => {
    const snapshot = cloneRecords(records);
    for (const subscriber of subscribers) {
      subscriber(snapshot);
    }
  };

  const appendRecord = (record: Omit<RegistryDebugRecord, 'id'>) => {
    records = [
      ...records,
      {
        id: nextId,
        ...record,
      },
    ];
    nextId += 1;
    notify();
  };

  const clear = () => {
    records = [];
    requests.clear();
    notify();
  };

  const recordOutbound = (input: Omit<RegistryDebugOutboundInput, 'kind'>) => {
    if (!enabled || input.envelope.method === 'connect.init') {
      return;
    }
    const timestamp = input.timestamp ?? now();
    const sessionIds = extractRegistryDebugSessionIds(input.envelope);
    if (typeof input.envelope.requestId === 'number') {
      requests.set(input.envelope.requestId, {
        method: input.envelope.method,
        projectId: input.envelope.projectId,
        sessionIds,
        timestamp,
      });
    }
    appendRecord({
      timestamp,
      timeText: formatRegistryDebugTime(timestamp),
      direction: 'out',
      phase: 'request',
      method: input.envelope.method,
      requestId: input.envelope.requestId,
      projectId: input.envelope.projectId,
      sessionIds,
      multiSession: sessionIds.length > 1,
      raw: input.raw,
      envelope: input.envelope,
    });
  };

  const recordInboundEnvelope = (input: Omit<RegistryDebugInboundInput, 'kind'>) => {
    if (!enabled) {
      return;
    }
    const timestamp = input.timestamp ?? now();
    const correlated = typeof input.envelope.requestId === 'number'
      ? requests.get(input.envelope.requestId)
      : undefined;
    const extractedSessionIds = extractRegistryDebugSessionIds(input.envelope);
    const sessionIds = extractedSessionIds.length > 0
      ? extractedSessionIds
      : correlated?.sessionIds ?? [];
    appendRecord({
      timestamp,
      timeText: formatRegistryDebugTime(timestamp),
      direction: 'in',
      phase: resolveInboundPhase(input.envelope.type),
      method: input.envelope.method ?? correlated?.method,
      requestId: input.envelope.requestId,
      projectId: input.envelope.projectId ?? correlated?.projectId,
      sessionIds,
      multiSession: sessionIds.length > 1,
      durationMs: correlated ? Math.max(0, timestamp - correlated.timestamp) : undefined,
      raw: input.raw,
      envelope: input.envelope,
    });
    if (
      typeof input.envelope.requestId === 'number' &&
      (input.envelope.type === 'response' || input.envelope.type === 'error')
    ) {
      requests.delete(input.envelope.requestId);
    }
  };

  const recordInboundParseError = (input: Omit<RegistryDebugParseErrorInput, 'kind'>) => {
    if (!enabled) {
      return;
    }
    const timestamp = input.timestamp ?? now();
    appendRecord({
      timestamp,
      timeText: formatRegistryDebugTime(timestamp),
      direction: 'in',
      phase: 'parse_error',
      sessionIds: [],
      multiSession: false,
      raw: input.raw,
      parseError: input.error,
    });
  };

  const recordLifecycle = (input: Omit<RegistryDebugLifecycleInput, 'kind'>) => {
    if (!enabled) {
      return;
    }
    const timestamp = input.timestamp ?? now();
    appendRecord({
      timestamp,
      timeText: formatRegistryDebugTime(timestamp),
      direction: 'lifecycle',
      phase: input.lifecycle.phase,
      sessionIds: [],
      multiSession: false,
      lifecycle: input.lifecycle,
    });
  };

  return {
    setEnabled: nextEnabled => {
      if (enabled === nextEnabled) {
        return;
      }
      enabled = nextEnabled;
      if (!enabled) {
        clear();
      }
    },
    isEnabled: () => enabled,
    clear,
    getRecords: () => cloneRecords(records),
    subscribe: subscriber => {
      subscribers.add(subscriber);
      subscriber(cloneRecords(records));
      return () => {
        subscribers.delete(subscriber);
      };
    },
    recordCaptureEvent: event => {
      if (event.kind === 'outbound') {
        recordOutbound(event);
        return;
      }
      if (event.kind === 'inbound') {
        recordInboundEnvelope(event);
        return;
      }
      if (event.kind === 'parse_error') {
        recordInboundParseError(event);
        return;
      }
      recordLifecycle(event);
    },
    recordOutbound,
    recordInboundEnvelope,
    recordInboundParseError,
    recordLifecycle,
  };
}
