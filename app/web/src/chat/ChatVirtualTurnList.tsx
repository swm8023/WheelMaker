import React from 'react';
import {useVirtualizer, type VirtualItem} from '@tanstack/react-virtual';
import type {ChatDisplayIndex, ChatDisplayIndexItem} from './chatDisplayIndex';

export type ChatVirtualTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  overscan?: number;
  rowGap?: number;
  renderItem: (item: ChatDisplayIndexItem, virtualItem: VirtualItem) => React.ReactNode;
};

export function ChatVirtualTurnList({
  scrollRef,
  displayIndex,
  overscan = 8,
  rowGap = 7,
  renderItem,
}: ChatVirtualTurnListProps) {
  const virtualizer = useVirtualizer({
    count: displayIndex.items.length,
    getScrollElement: () => scrollRef.current,
    getItemKey: index => displayIndex.items[index]?.key ?? index,
    estimateSize: index => (displayIndex.items[index]?.estimatedHeight ?? 120) + rowGap,
    overscan,
  });

  return (
    <div
      className="chat-virtual-list"
      style={{
        height: `${virtualizer.getTotalSize()}px`,
        position: 'relative',
        width: '100%',
      }}
    >
      {virtualizer.getVirtualItems().map(virtualItem => {
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
