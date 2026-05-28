export type AppDiagnosticCategory = 'voice';
export type AppDiagnosticLevel = 'debug' | 'warn' | 'error';

export type AppDiagnosticRecord = {
  id: number;
  timestamp: number;
  timeText: string;
  category: AppDiagnosticCategory;
  level: AppDiagnosticLevel;
  event: string;
  details: Record<string, unknown>;
};

export type AppDiagnosticInput = {
  category: AppDiagnosticCategory;
  level: AppDiagnosticLevel;
  event: string;
  details?: Record<string, unknown>;
};

export type AppDiagnosticFilter = {
  category: AppDiagnosticCategory;
  levels: AppDiagnosticLevel[];
};

export type AppDiagnosticSubscriber = (records: AppDiagnosticRecord[]) => void;

export type AppDiagnosticStore = {
  clear: () => void;
  getRecords: () => AppDiagnosticRecord[];
  record: (input: AppDiagnosticInput) => void;
  subscribe: (subscriber: AppDiagnosticSubscriber) => () => void;
};

const MAX_APP_DIAGNOSTIC_RECORDS = 200;
const SENSITIVE_DETAIL_KEYS = new Set(['apiKey', 'streamId']);
const OMITTED_DETAIL_KEYS = new Set(['pcm', 'base64']);

function pad(value: number, length: number): string {
  return String(value).padStart(length, '0');
}

export function formatAppDiagnosticTime(timestamp: number): string {
  const date = new Date(timestamp);
  return [
    pad(date.getHours(), 2),
    pad(date.getMinutes(), 2),
    pad(date.getSeconds(), 2),
  ].join(':') + `.${pad(date.getMilliseconds(), 3)}`;
}

function sanitizeDiagnosticValue(value: unknown, depth: number): unknown {
  if (depth > 4) {
    return '[truncated]';
  }
  if (!value || typeof value !== 'object') {
    return value;
  }
  if (Array.isArray(value)) {
    return value.map(item => sanitizeDiagnosticValue(item, depth + 1));
  }
  const output: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (SENSITIVE_DETAIL_KEYS.has(key)) {
      output[key] = '[redacted]';
      continue;
    }
    if (OMITTED_DETAIL_KEYS.has(key)) {
      output[key] = '[omitted]';
      continue;
    }
    output[key] = sanitizeDiagnosticValue(item, depth + 1);
  }
  return output;
}

export function sanitizeAppDiagnosticDetails(details: Record<string, unknown> = {}): Record<string, unknown> {
  return sanitizeDiagnosticValue(details, 0) as Record<string, unknown>;
}

export function filterAppDiagnosticRecords(
  records: AppDiagnosticRecord[],
  filter: AppDiagnosticFilter,
): AppDiagnosticRecord[] {
  const levels = new Set(filter.levels);
  return records.filter(record => record.category === filter.category && levels.has(record.level));
}

function cloneRecords(records: AppDiagnosticRecord[]): AppDiagnosticRecord[] {
  return records.slice();
}

export function createAppDiagnosticStore(now: () => number = () => Date.now()): AppDiagnosticStore {
  let nextId = 1;
  let records: AppDiagnosticRecord[] = [];
  const subscribers = new Set<AppDiagnosticSubscriber>();

  const notify = () => {
    const snapshot = cloneRecords(records);
    for (const subscriber of subscribers) {
      subscriber(snapshot);
    }
  };

  return {
    clear: () => {
      records = [];
      notify();
    },
    getRecords: () => cloneRecords(records),
    record: input => {
      const timestamp = now();
      records = [
        ...records,
        {
          id: nextId,
          timestamp,
          timeText: formatAppDiagnosticTime(timestamp),
          category: input.category,
          level: input.level,
          event: input.event,
          details: sanitizeAppDiagnosticDetails(input.details),
        },
      ].slice(-MAX_APP_DIAGNOSTIC_RECORDS);
      nextId += 1;
      notify();
    },
    subscribe: subscriber => {
      subscribers.add(subscriber);
      subscriber(cloneRecords(records));
      return () => {
        subscribers.delete(subscriber);
      };
    },
  };
}

export const appDiagnosticStore = createAppDiagnosticStore();
