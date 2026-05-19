import React from 'react';
import {Virtuoso} from 'react-virtuoso';
import type {ChatDisplayIndex, ChatDisplayIndexItem} from './chatDisplayIndex';

const DEFAULT_AT_BOTTOM_THRESHOLD = 80;

export type ChatVirtualItem = {
  end: number;
  index: number;
  key: string;
  lane: number;
  size: number;
  start: number;
};

export type ChatVirtualTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  overscan?: number;
  rowGap?: number;
  atBottomThreshold?: number;
  renderItem: (item: ChatDisplayIndexItem, virtualItem: ChatVirtualItem) => React.ReactNode;
};

function resolveEstimatedItemHeight(item: ChatDisplayIndexItem | undefined, rowGap: number): number {
  return Math.max(1, Math.round((item?.estimatedHeight ?? 120) + rowGap));
}

function resolveDefaultItemHeight(heightEstimates: number[], rowGap: number): number {
  if (heightEstimates.length === 0) {
    return resolveEstimatedItemHeight(undefined, rowGap);
  }
  const totalHeight = heightEstimates.reduce((sum, height) => sum + height, 0);
  return Math.max(1, Math.round(totalHeight / heightEstimates.length));
}

export function ChatVirtualTurnList({
  scrollRef,
  displayIndex,
  runtimeKey,
  overscan = 8,
  rowGap = 7,
  atBottomThreshold = DEFAULT_AT_BOTTOM_THRESHOLD,
  renderItem,
}: ChatVirtualTurnListProps) {
  const [scrollParent, setScrollParent] = React.useState<HTMLElement | null>(null);

  React.useLayoutEffect(() => {
    const nextScrollParent = scrollRef.current;
    setScrollParent(current => current === nextScrollParent ? current : nextScrollParent);
  }, [runtimeKey, scrollRef]);

  const heightEstimates = React.useMemo(
    () => displayIndex.items.map(item => resolveEstimatedItemHeight(item, rowGap)),
    [displayIndex.items, rowGap],
  );
  const defaultItemHeight = React.useMemo(
    () => resolveDefaultItemHeight(heightEstimates, rowGap),
    [heightEstimates, rowGap],
  );
  const viewportIncrease = Math.max(0, Math.round(defaultItemHeight * Math.max(0, overscan)));
  const minOverscanItemCount = Math.max(1, Math.trunc(overscan));
  const initialTopMostItemIndex = displayIndex.items.length > 0
    ? {index: displayIndex.items.length - 1, align: 'end' as const}
    : 0;

  if (!scrollParent) {
    return <div className="chat-virtual-list" />;
  }

  return (
    <Virtuoso<ChatDisplayIndexItem>
      key={runtimeKey}
      className="chat-virtual-list"
      customScrollParent={scrollParent}
      data={displayIndex.items}
      defaultItemHeight={defaultItemHeight}
      heightEstimates={heightEstimates}
      initialTopMostItemIndex={initialTopMostItemIndex}
      alignToBottom={true}
      atBottomThreshold={atBottomThreshold}
      computeItemKey={(index, item) => item.key}
      increaseViewportBy={{top: viewportIncrease, bottom: viewportIncrease}}
      minOverscanItemCount={{top: minOverscanItemCount, bottom: minOverscanItemCount}}
      followOutput={isAtBottom => (isAtBottom ? 'auto' : false)}
      itemContent={(index, displayItem) => {
        const size = heightEstimates[index] ?? defaultItemHeight;
        return (
          <div
            className="chat-virtual-row"
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
      }}
    />
  );
}
