import React from 'react';
import {Virtuoso, type Components, type VirtuosoHandle} from 'react-virtuoso';
import type {ChatDisplayIndex, ChatDisplayIndexItem} from './chatDisplayIndex';
import {resolveChatDisplayScrollIndex} from './chatDisplayIndex';
import {resolveChatScrollBottomTop} from './chatScrollIntent';

const DEFAULT_AT_BOTTOM_THRESHOLD = 80;
const DEFAULT_BOTTOM_BUFFER = 0;

type ChatVirtuosoScrollBehavior = 'auto' | 'smooth';

type ChatVirtuosoContext = {
  bottomBuffer: number;
  rowGap: number;
};

export type ChatVirtuosoItem = {
  end: number;
  index: number;
  key: string;
  lane: number;
  size: number;
  start: number;
};

export type ChatVirtuosoTurnListHandle = {
  autoscrollToBottom: () => void;
  scrollToBottom: (behavior?: ChatVirtuosoScrollBehavior) => void;
  scrollToTurnIndex: (turnIndex: number, behavior?: ChatVirtuosoScrollBehavior) => void;
};

export type ChatVirtuosoTurnListProps = {
  scrollRef: React.RefObject<HTMLElement | null>;
  displayIndex: ChatDisplayIndex;
  runtimeKey: string;
  overscan?: number;
  rowGap?: number;
  bottomBuffer?: number;
  atBottomThreshold?: number;
  onAtBottomChange?: (atBottom: boolean) => void;
  shouldAutoscroll?: () => boolean;
  renderItem: (item: ChatDisplayIndexItem, virtualItem: ChatVirtuosoItem) => React.ReactNode;
};

const ChatVirtuosoList: Components<ChatDisplayIndexItem, ChatVirtuosoContext>['List'] =
  React.forwardRef<HTMLDivElement, React.ComponentProps<'div'>>(
    ({children, style, ...props}, ref) => (
      <div
        {...props}
        ref={ref}
        className="chat-virtuoso-list"
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
    className="chat-virtuoso-row"
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
    className="chat-virtuoso-footer"
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

function scrollElementToBottom(element: HTMLElement, behavior: ChatVirtuosoScrollBehavior): void {
  element.scrollTo({
    top: resolveChatScrollBottomTop({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }),
    behavior,
  });
}

export const ChatVirtuosoTurnList = React.forwardRef<
  ChatVirtuosoTurnListHandle,
  ChatVirtuosoTurnListProps
>(function ChatVirtuosoTurnList({
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
}: ChatVirtuosoTurnListProps, ref) {
  const virtuosoRef = React.useRef<VirtuosoHandle | null>(null);
  const tailLockSettleFrameRef = React.useRef<number | null>(null);
  const tailLockSettleFollowupFrameRef = React.useRef<number | null>(null);
  const [scrollParent, setScrollParent] = React.useState<HTMLElement | null>(null);

  React.useLayoutEffect(() => {
    let cancelled = false;
    let frameId = 0;
    let attempts = 0;

    const syncScrollParent = () => {
      if (cancelled) {
        return;
      }
      const nextScrollParent = scrollRef.current;
      setScrollParent(current => current === nextScrollParent ? current : nextScrollParent);
      if (!nextScrollParent && attempts < 3) {
        attempts += 1;
        frameId = window.requestAnimationFrame(syncScrollParent);
      }
    };

    syncScrollParent();
    return () => {
      cancelled = true;
      if (frameId) {
        window.cancelAnimationFrame(frameId);
      }
    };
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
  const virtuosoContext = React.useMemo<ChatVirtuosoContext>(
    () => ({
      bottomBuffer: Math.max(0, Math.round(bottomBuffer)),
      rowGap: Math.max(0, Math.round(rowGap)),
    }),
    [bottomBuffer, rowGap],
  );
  const initialTopMostItemIndex = React.useMemo(
    () => displayIndex.items.length > 0
      ? {index: 'LAST' as const, align: 'end' as const}
      : 0,
    [displayIndex.items.length],
  );

  const shouldAutoscrollNow = React.useCallback(
    () => shouldAutoscroll?.() ?? true,
    [shouldAutoscroll],
  );

  const handleAtBottomStateChange = React.useCallback(
    (atBottom: boolean) => {
      onAtBottomChange?.(atBottom);
    },
    [onAtBottomChange],
  );

  const scrollToLastDisplayItem = React.useCallback(
    (behavior: ChatVirtuosoScrollBehavior = 'auto') => {
      if (displayIndex.items.length === 0) {
        return;
      }
      virtuosoRef.current?.scrollToIndex({
        index: 'LAST',
        align: 'end',
        behavior,
      });
    },
    [displayIndex.items.length],
  );

  const scrollToTurnIndex = React.useCallback(
    (turnIndex: number, behavior: ChatVirtuosoScrollBehavior = 'auto') => {
      const displayIndexPosition = resolveChatDisplayScrollIndex(displayIndex, turnIndex);
      if (displayIndexPosition === null) {
        return;
      }
      virtuosoRef.current?.scrollToIndex({
        index: displayIndexPosition,
        align: 'start',
        behavior,
      });
    },
    [displayIndex],
  );

  const settleScrollParentToBottom = React.useCallback(
    (behavior: ChatVirtuosoScrollBehavior = 'auto') => {
      if (!scrollParent) {
        return;
      }
      scrollElementToBottom(scrollParent, behavior);
      onAtBottomChange?.(true);
    },
    [onAtBottomChange, scrollParent],
  );

  const cancelTailLockSettle = React.useCallback(() => {
    if (tailLockSettleFrameRef.current !== null) {
      window.cancelAnimationFrame(tailLockSettleFrameRef.current);
      tailLockSettleFrameRef.current = null;
    }
    if (tailLockSettleFollowupFrameRef.current !== null) {
      window.cancelAnimationFrame(tailLockSettleFollowupFrameRef.current);
      tailLockSettleFollowupFrameRef.current = null;
    }
  }, []);

  const requestScrollToLastDisplayItem = React.useCallback(
    (
      behavior: ChatVirtuosoScrollBehavior = 'auto',
      options: {includeIndexScroll?: boolean; includeVirtuosoAutoscroll?: boolean} = {},
    ) => {
      cancelTailLockSettle();
      tailLockSettleFrameRef.current = window.requestAnimationFrame(() => {
        tailLockSettleFrameRef.current = null;
        if (options.includeVirtuosoAutoscroll) {
          virtuosoRef.current?.autoscrollToBottom();
        }
        if (options.includeIndexScroll) {
          scrollToLastDisplayItem(behavior);
        }
        settleScrollParentToBottom(behavior);
        tailLockSettleFollowupFrameRef.current = window.requestAnimationFrame(() => {
          tailLockSettleFollowupFrameRef.current = null;
          if (options.includeIndexScroll) {
            scrollToLastDisplayItem('auto');
          }
          settleScrollParentToBottom('auto');
        });
      });
    },
    [cancelTailLockSettle, scrollToLastDisplayItem, settleScrollParentToBottom],
  );

  const handleTotalListHeightChanged = React.useCallback(() => {
    if (shouldAutoscrollNow()) {
      requestScrollToLastDisplayItem('auto');
    }
  }, [
    requestScrollToLastDisplayItem,
    shouldAutoscrollNow,
  ]);

  React.useEffect(() => cancelTailLockSettle, [cancelTailLockSettle]);

  React.useImperativeHandle(ref, () => ({
    autoscrollToBottom: () => {
      requestScrollToLastDisplayItem('auto', {
        includeIndexScroll: true,
        includeVirtuosoAutoscroll: true,
      });
    },
    scrollToBottom: (behavior: ChatVirtuosoScrollBehavior = 'auto') => {
      scrollToLastDisplayItem(behavior);
      settleScrollParentToBottom(behavior);
      requestScrollToLastDisplayItem(behavior, {includeIndexScroll: true});
    },
    scrollToTurnIndex,
  }), [requestScrollToLastDisplayItem, scrollToLastDisplayItem, scrollToTurnIndex, settleScrollParentToBottom]);

  if (!scrollParent) {
    return (
      <div className="chat-virtuoso-list" data-scroll-parent-pending={true}>
        <div
          aria-hidden="true"
          className="chat-virtuoso-footer"
          style={{height: `${virtuosoContext.bottomBuffer}px`}}
        />
      </div>
    );
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
      initialTopMostItemIndex={initialTopMostItemIndex}
      alignToBottom={true}
      atBottomThreshold={atBottomThreshold}
      atBottomStateChange={handleAtBottomStateChange}
      computeItemKey={(index, item) => item.key}
      increaseViewportBy={{top: viewportIncrease, bottom: viewportIncrease}}
      minOverscanItemCount={{top: minOverscanItemCount, bottom: minOverscanItemCount}}
      followOutput={() => (shouldAutoscrollNow() ? 'auto' : false)}
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
