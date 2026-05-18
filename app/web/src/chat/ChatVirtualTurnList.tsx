import React from 'react';
import {
  measureElement as measureVirtualElement,
  useVirtualizer,
  type VirtualItem,
  type Virtualizer,
} from '@tanstack/react-virtual';
import type {ChatDisplayIndex, ChatDisplayIndexItem} from './chatDisplayIndex';
import {shouldAdjustChatVirtualItemSizeChange} from './chatScrollIntent';
import {
  createChatVirtualHeightCache,
  resolveChatVirtualAnchor,
  resolveChatVirtualAnchorScrollTop,
  resolveChatVirtualItemEstimate,
  resolveChatVirtualWidthBucket,
  selectChatVirtualPremeasureItems,
  writeChatVirtualMeasuredHeight,
  type ChatVirtualAnchor,
  type ChatVirtualMeasurement,
} from './chatVirtualMeasurements';

const chatVirtualHeightCache = createChatVirtualHeightCache();

type ChatVirtualizer = Virtualizer<HTMLElement, Element>;

export type ChatVirtualTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  overscan?: number;
  rowGap?: number;
  renderItem: (item: ChatDisplayIndexItem, virtualItem: VirtualItem) => React.ReactNode;
};

export function ChatVirtualTurnList({
  scrollRef,
  displayIndex,
  runtimeKey,
  overscan = 8,
  rowGap = 7,
  renderItem,
}: ChatVirtualTurnListProps) {
  const listRef = React.useRef<HTMLDivElement | null>(null);
  const anchorRef = React.useRef<ChatVirtualAnchor | null>(null);
  const anchorRestoreFrameRef = React.useRef(0);
  const virtualMeasureFrameRef = React.useRef(0);
  const preserveAnchorDuringMeasureRef = React.useRef(false);
  const scheduleAnchorRestoreRef = React.useRef<() => void>(() => undefined);
  const scheduleVirtualMeasureRef = React.useRef<() => void>(() => undefined);
  const [widthBucket, setWidthBucket] = React.useState(0);
  const normalizedRuntimeKey = runtimeKey || 'default';

  React.useLayoutEffect(() => {
    const element = listRef.current;
    if (!element) {
      return;
    }
    const updateWidthBucket = () => {
      setWidthBucket(resolveChatVirtualWidthBucket(element.clientWidth));
    };
    updateWidthBucket();
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', updateWidthBucket);
      return () => window.removeEventListener('resize', updateWidthBucket);
    }
    const observer = new ResizeObserver(updateWidthBucket);
    observer.observe(element);
    return () => observer.disconnect();
  }, [normalizedRuntimeKey]);

  const captureAnchorFromInstance = React.useCallback(
    (instance: ChatVirtualizer) => {
      if (preserveAnchorDuringMeasureRef.current) {
        return;
      }
      const container = scrollRef.current;
      if (!container) {
        return;
      }
      const nearBottom =
        container.scrollHeight - container.scrollTop - container.clientHeight <= 2;
      if (nearBottom) {
        return;
      }
      const measurements: ChatVirtualMeasurement[] =
        instance.measurementsCache.length > 0
          ? instance.measurementsCache
          : instance.getVirtualItems();
      const anchor = resolveChatVirtualAnchor({
        measurements,
        scrollTop: container.scrollTop,
      });
      if (anchor) {
        anchorRef.current = anchor;
      }
    },
    [scrollRef],
  );

  const virtualizer = useVirtualizer({
    count: displayIndex.items.length,
    getScrollElement: () => scrollRef.current,
    getItemKey: index => displayIndex.items[index]?.key ?? index,
    estimateSize: index => resolveChatVirtualItemEstimate({
      cache: chatVirtualHeightCache,
      fallbackHeight: displayIndex.items[index]?.estimatedHeight ?? 120,
      itemKey: displayIndex.items[index]?.key ?? String(index),
      rowGap,
      runtimeKey: normalizedRuntimeKey,
      widthBucket,
    }),
    measureElement: (element, entry, instance) => {
      const measuredHeight = measureVirtualElement(element, entry, instance);
      const index = Number(element.getAttribute('data-index') ?? -1);
      const item = displayIndex.items[index];
      if (item && writeChatVirtualMeasuredHeight(chatVirtualHeightCache, {
        itemKey: item.key,
        measuredHeight,
        runtimeKey: normalizedRuntimeKey,
        widthBucket,
      })) {
        scheduleAnchorRestoreRef.current();
      }
      return measuredHeight;
    },
    onChange: instance => {
      captureAnchorFromInstance(instance);
    },
    useAnimationFrameWithResizeObserver: true,
    overscan,
  });

  virtualizer.shouldAdjustScrollPositionOnItemSizeChange = (item, _delta, instance) =>
    shouldAdjustChatVirtualItemSizeChange({
      isScrolling: instance.isScrolling,
      itemEnd: item.end,
      itemStart: item.start,
      scrollDirection: instance.scrollDirection,
      scrollOffset: instance.scrollOffset,
    });

  const restoreAnchor = React.useCallback(() => {
    const container = scrollRef.current;
    if (!container || !anchorRef.current) {
      return;
    }
    const nearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight <= 2;
    if (nearBottom) {
      return;
    }
    const nextScrollTop = resolveChatVirtualAnchorScrollTop({
      anchor: anchorRef.current,
      measurements: virtualizer.measurementsCache,
    });
    if (nextScrollTop === null || Math.abs(container.scrollTop - nextScrollTop) <= 1) {
      return;
    }
    container.scrollTop = nextScrollTop;
  }, [scrollRef, virtualizer]);

  const scheduleAnchorRestore = React.useCallback(() => {
    preserveAnchorDuringMeasureRef.current = true;
    if (anchorRestoreFrameRef.current) {
      window.cancelAnimationFrame(anchorRestoreFrameRef.current);
    }
    anchorRestoreFrameRef.current = window.requestAnimationFrame(() => {
      anchorRestoreFrameRef.current = 0;
      restoreAnchor();
      preserveAnchorDuringMeasureRef.current = false;
    });
  }, [restoreAnchor]);
  scheduleAnchorRestoreRef.current = scheduleAnchorRestore;

  const scheduleVirtualMeasure = React.useCallback(() => {
    if (virtualMeasureFrameRef.current) {
      return;
    }
    virtualMeasureFrameRef.current = window.requestAnimationFrame(() => {
      virtualMeasureFrameRef.current = 0;
      captureAnchorFromInstance(virtualizer);
      preserveAnchorDuringMeasureRef.current = true;
      virtualizer.measure();
      scheduleAnchorRestore();
    });
  }, [captureAnchorFromInstance, scheduleAnchorRestore, virtualizer]);
  scheduleVirtualMeasureRef.current = scheduleVirtualMeasure;

  React.useEffect(() => () => {
    if (anchorRestoreFrameRef.current) {
      window.cancelAnimationFrame(anchorRestoreFrameRef.current);
    }
    if (virtualMeasureFrameRef.current) {
      window.cancelAnimationFrame(virtualMeasureFrameRef.current);
    }
  }, []);

  const layoutScope = `${normalizedRuntimeKey}:${widthBucket}`;
  const previousLayoutScopeRef = React.useRef(layoutScope);
  React.useLayoutEffect(() => {
    if (previousLayoutScopeRef.current === layoutScope) {
      return;
    }
    previousLayoutScopeRef.current = layoutScope;
    captureAnchorFromInstance(virtualizer);
    virtualizer.measure();
    scheduleAnchorRestore();
  }, [captureAnchorFromInstance, layoutScope, scheduleAnchorRestore, virtualizer]);

  const virtualItems = virtualizer.getVirtualItems();
  const premeasureItems = selectChatVirtualPremeasureItems({
    cache: chatVirtualHeightCache,
    displayItems: displayIndex.items,
    runtimeKey: normalizedRuntimeKey,
    scrollDirection: virtualizer.scrollDirection,
    visibleStartIndex: virtualItems[0]?.index ?? displayIndex.items.length,
    widthBucket,
    windowSize: overscan,
  });

  const measurePreheatedItem = (item: ChatDisplayIndexItem, node: HTMLDivElement | null) => {
    if (!node) {
      return;
    }
    if (writeChatVirtualMeasuredHeight(chatVirtualHeightCache, {
      itemKey: item.key,
      measuredHeight: node.getBoundingClientRect().height,
      runtimeKey: normalizedRuntimeKey,
      widthBucket,
    })) {
      scheduleVirtualMeasureRef.current();
    }
  };

  return (
    <div
      ref={listRef}
      className="chat-virtual-list"
      style={{
        height: `${virtualizer.getTotalSize()}px`,
        position: 'relative',
        width: '100%',
      }}
    >
      {premeasureItems.length > 0 ? (
        <div
          aria-hidden="true"
          className="chat-virtual-premeasure"
        >
          {premeasureItems.map(displayItem => {
            const index = displayIndex.items.findIndex(item => item.key === displayItem.key);
            const size = resolveChatVirtualItemEstimate({
              cache: chatVirtualHeightCache,
              fallbackHeight: displayItem.estimatedHeight,
              itemKey: displayItem.key,
              rowGap,
              runtimeKey: normalizedRuntimeKey,
              widthBucket,
            });
            return (
              <div
                key={`premeasure:${displayItem.key}`}
                ref={node => measurePreheatedItem(displayItem, node)}
                className="chat-virtual-premeasure-row"
                style={{
                  paddingBottom: `${rowGap}px`,
                }}
              >
                {renderItem(displayItem, {
                  end: size,
                  index,
                  key: displayItem.key,
                  lane: 0,
                  size,
                  start: 0,
                })}
              </div>
            );
          })}
        </div>
      ) : null}
      {virtualItems.map(virtualItem => {
        const displayItem = displayIndex.items[virtualItem.index];
        if (!displayItem) return null;
        return (
          <div
            key={virtualItem.key}
            ref={virtualizer.measureElement}
            data-index={virtualItem.index}
            className="chat-virtual-row"
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              boxSizing: 'border-box',
              paddingBottom: `${rowGap}px`,
              transform: `translateY(${virtualItem.start}px)`,
            }}
          >
            {renderItem(displayItem, virtualItem)}
          </div>
        );
      })}
    </div>
  );
}
