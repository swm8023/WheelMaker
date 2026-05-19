import React from 'react';
import {Virtuoso, type Components, type VirtuosoHandle} from 'react-virtuoso';
import type {ChatDisplayIndex, ChatDisplayIndexItem} from './chatDisplayIndex';

const DEFAULT_AT_BOTTOM_THRESHOLD = 80;
const DEFAULT_BOTTOM_BUFFER = 12;

type ChatVirtualScrollBehavior = 'auto' | 'smooth';

type ChatVirtuosoContext = {
  bottomBuffer: number;
  rowGap: number;
};

export type ChatVirtualItem = {
  end: number;
  index: number;
  key: string;
  lane: number;
  size: number;
  start: number;
};

export type ChatVirtualTurnListHandle = {
  autoscrollToBottom: () => void;
  scrollToBottom: (behavior?: ChatVirtualScrollBehavior) => void;
};

export type ChatVirtualTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  overscan?: number;
  rowGap?: number;
  bottomBuffer?: number;
  atBottomThreshold?: number;
  onAtBottomChange?: (atBottom: boolean) => void;
  shouldAutoscroll?: () => boolean;
  renderItem: (item: ChatDisplayIndexItem, virtualItem: ChatVirtualItem) => React.ReactNode;
};

const ChatVirtuosoList: Components<ChatDisplayIndexItem, ChatVirtuosoContext>['List'] =
  React.forwardRef<HTMLDivElement, React.ComponentProps<'div'>>(
    ({children, style, ...props}, ref) => (
      <div
        {...props}
        ref={ref}
        className="chat-virtual-list"
        style={style}
      >
        {children}
      </div>
    ),
  );

const ChatVirtuosoItem: Components<ChatDisplayIndexItem, ChatVirtuosoContext>['Item'] = ({
  children,
  context,
  style,
  ...props
}) => (
  <div
    {...props}
    className="chat-virtual-row"
    style={{
      ...style,
      paddingBottom: `${context.rowGap}px`,
    }}
  >
    {children}
  </div>
);

const ChatVirtuosoFooter: Components<ChatDisplayIndexItem, ChatVirtuosoContext>['Footer'] = ({
  context,
}) => (
  <div
    aria-hidden="true"
    className="chat-virtual-footer"
    style={{
      height: `${context.bottomBuffer}px`,
    }}
  />
);

const ChatVirtuosoComponents: Components<ChatDisplayIndexItem, ChatVirtuosoContext> = {
  Footer: ChatVirtuosoFooter,
  Item: ChatVirtuosoItem,
  List: ChatVirtuosoList,
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

export const ChatVirtualTurnList = React.forwardRef<
  ChatVirtualTurnListHandle,
  ChatVirtualTurnListProps
>(function ChatVirtualTurnList({
  scrollRef,
  displayIndex,
  runtimeKey,
  overscan = 8,
  rowGap = 7,
  bottomBuffer = DEFAULT_BOTTOM_BUFFER,
  atBottomThreshold = DEFAULT_AT_BOTTOM_THRESHOLD,
  onAtBottomChange,
  shouldAutoscroll,
  renderItem,
}: ChatVirtualTurnListProps, ref) {
  const virtuosoRef = React.useRef<VirtuosoHandle | null>(null);
  const atBottomRef = React.useRef(true);
  const [scrollParent, setScrollParent] = React.useState<HTMLElement | null>(null);

  React.useLayoutEffect(() => {
    const nextScrollParent = scrollRef.current;
    setScrollParent(current => current === nextScrollParent ? current : nextScrollParent);
  }, [runtimeKey, scrollRef]);

  React.useLayoutEffect(() => {
    atBottomRef.current = true;
  }, [runtimeKey]);

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
  const virtuosoContext = React.useMemo<ChatVirtuosoContext>(
    () => ({
      bottomBuffer: Math.max(0, Math.round(bottomBuffer)),
      rowGap: Math.max(0, Math.round(rowGap)),
    }),
    [bottomBuffer, rowGap],
  );

  const shouldAutoscrollNow = React.useCallback(
    () => shouldAutoscroll?.() ?? true,
    [shouldAutoscroll],
  );

  const handleAtBottomStateChange = React.useCallback(
    (atBottom: boolean) => {
      atBottomRef.current = atBottom;
      onAtBottomChange?.(atBottom);
    },
    [onAtBottomChange],
  );

  const handleTotalListHeightChanged = React.useCallback(() => {
    if (atBottomRef.current && shouldAutoscrollNow()) {
      virtuosoRef.current?.autoscrollToBottom();
    }
  }, [shouldAutoscrollNow]);

  React.useImperativeHandle(ref, () => ({
    autoscrollToBottom: () => {
      virtuosoRef.current?.autoscrollToBottom();
    },
    scrollToBottom: (behavior: ChatVirtualScrollBehavior = 'auto') => {
      if (displayIndex.items.length === 0) {
        return;
      }
      virtuosoRef.current?.scrollToIndex({index: 'LAST', align: 'end', behavior});
    },
  }), [displayIndex.items.length]);

  if (!scrollParent) {
    return <div className="chat-virtual-list" />;
  }

  return (
    <Virtuoso<ChatDisplayIndexItem, ChatVirtuosoContext>
      ref={virtuosoRef}
      key={runtimeKey}
      customScrollParent={scrollParent}
      data={displayIndex.items}
      components={ChatVirtuosoComponents}
      context={virtuosoContext}
      defaultItemHeight={defaultItemHeight}
      heightEstimates={heightEstimates}
      initialTopMostItemIndex={displayIndex.items.length > 0 ? {index: 'LAST', align: 'end'} : 0}
      alignToBottom={true}
      atBottomThreshold={atBottomThreshold}
      atBottomStateChange={handleAtBottomStateChange}
      computeItemKey={(index, item) => item.key}
      increaseViewportBy={{top: viewportIncrease, bottom: viewportIncrease}}
      minOverscanItemCount={{top: minOverscanItemCount, bottom: minOverscanItemCount}}
      followOutput={isAtBottom => (isAtBottom && shouldAutoscrollNow() ? 'auto' : false)}
      totalListHeightChanged={handleTotalListHeightChanged}
      itemContent={(index, displayItem) => {
        const size = heightEstimates[index] ?? defaultItemHeight;
        return renderItem(displayItem, {
          end: size,
          index,
          key: displayItem.key,
          lane: 0,
          size,
          start: 0,
        });
      }}
    />
  );
});
