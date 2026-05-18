import type {ChatDisplayIndexItem} from './chatDisplayIndex';

export const CHAT_VIRTUAL_WIDTH_BUCKET_PX = 32;

export type ChatVirtualItemKey = string | number | bigint;

export type ChatVirtualMeasurement = {
  key: ChatVirtualItemKey;
  index: number;
  start: number;
  end: number;
  size: number;
  lane: number;
};

export type ChatVirtualAnchor = {
  itemKey: string;
  offsetInsideItem: number;
};

export type ChatVirtualScrollDirection = 'forward' | 'backward' | null;

export type ChatVirtualHeightCache = Map<string, Map<string, number>>;

export function createChatVirtualHeightCache(): ChatVirtualHeightCache {
  return new Map();
}

export function resolveChatVirtualWidthBucket(
  contentWidth: number,
  bucketSize = CHAT_VIRTUAL_WIDTH_BUCKET_PX,
): number {
  if (!Number.isFinite(contentWidth) || contentWidth <= 0) {
    return 0;
  }
  const normalizedBucketSize = Math.max(1, Math.trunc(bucketSize));
  return Math.max(
    normalizedBucketSize,
    Math.round(contentWidth / normalizedBucketSize) * normalizedBucketSize,
  );
}

function cacheScope(runtimeKey: string, widthBucket: number): string {
  return `${runtimeKey || 'default'}:${Math.max(0, Math.trunc(widthBucket))}`;
}

function normalizeMeasuredHeight(measuredHeight: number): number {
  if (!Number.isFinite(measuredHeight) || measuredHeight <= 0) {
    return 0;
  }
  return Math.max(1, Math.round(measuredHeight));
}

export function readChatVirtualMeasuredHeight(
  cache: ChatVirtualHeightCache,
  input: {
    itemKey: string;
    runtimeKey: string;
    widthBucket: number;
  },
): number | undefined {
  if (input.widthBucket <= 0 || !input.itemKey) {
    return undefined;
  }
  return cache.get(cacheScope(input.runtimeKey, input.widthBucket))?.get(input.itemKey);
}

export function writeChatVirtualMeasuredHeight(
  cache: ChatVirtualHeightCache,
  input: {
    itemKey: string;
    measuredHeight: number;
    runtimeKey: string;
    widthBucket: number;
  },
): boolean {
  const measuredHeight = normalizeMeasuredHeight(input.measuredHeight);
  if (input.widthBucket <= 0 || !input.itemKey || measuredHeight <= 0) {
    return false;
  }
  const scope = cacheScope(input.runtimeKey, input.widthBucket);
  const scopedCache = cache.get(scope) ?? new Map<string, number>();
  const previousHeight = scopedCache.get(input.itemKey);
  if (previousHeight === measuredHeight) {
    return false;
  }
  scopedCache.set(input.itemKey, measuredHeight);
  cache.set(scope, scopedCache);
  return true;
}

export function resolveChatVirtualItemEstimate(input: {
  cache: ChatVirtualHeightCache;
  fallbackHeight: number;
  itemKey: string;
  rowGap: number;
  runtimeKey: string;
  widthBucket: number;
}): number {
  return (
    readChatVirtualMeasuredHeight(input.cache, {
      itemKey: input.itemKey,
      runtimeKey: input.runtimeKey,
      widthBucket: input.widthBucket,
    }) ??
    Math.max(1, Math.round(input.fallbackHeight + input.rowGap))
  );
}

export function resolveChatVirtualAnchor(input: {
  measurements: ChatVirtualMeasurement[];
  scrollTop: number;
}): ChatVirtualAnchor | null {
  const scrollTop = Math.max(0, input.scrollTop);
  const item = input.measurements.find(measurement => measurement.end > scrollTop);
  if (!item) {
    return null;
  }
  return {
    itemKey: String(item.key),
    offsetInsideItem: Math.max(0, scrollTop - item.start),
  };
}

export function resolveChatVirtualAnchorScrollTop(input: {
  anchor: ChatVirtualAnchor | null;
  measurements: ChatVirtualMeasurement[];
}): number | null {
  if (!input.anchor) {
    return null;
  }
  const item = input.measurements.find(measurement => String(measurement.key) === input.anchor?.itemKey);
  if (!item) {
    return null;
  }
  return Math.max(0, item.start + input.anchor.offsetInsideItem);
}

export function selectChatVirtualPremeasureItems(input: {
  cache: ChatVirtualHeightCache;
  displayItems: ChatDisplayIndexItem[];
  runtimeKey: string;
  scrollDirection: ChatVirtualScrollDirection;
  visibleStartIndex: number;
  widthBucket: number;
  windowSize: number;
}): ChatDisplayIndexItem[] {
  if (
    input.scrollDirection !== 'backward' ||
    input.widthBucket <= 0 ||
    input.visibleStartIndex <= 0 ||
    input.windowSize <= 0
  ) {
    return [];
  }
  const startIndex = Math.max(0, input.visibleStartIndex - Math.max(0, Math.trunc(input.windowSize)));
  return input.displayItems
    .slice(startIndex, input.visibleStartIndex)
    .filter(item => readChatVirtualMeasuredHeight(input.cache, {
      itemKey: item.key,
      runtimeKey: input.runtimeKey,
      widthBucket: input.widthBucket,
    }) === undefined);
}
