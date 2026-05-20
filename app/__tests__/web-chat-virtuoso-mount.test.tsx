import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {ChatVirtuosoTurnList, type ChatVirtuosoTurnListHandle} from '../web/src/chat/ChatVirtuosoTurnList';
import type {ChatDisplayIndexItem} from '../web/src/chat/chatDisplayIndex';

const mockVirtuosoProps: any[] = [];
const mockScrollToIndexCalls: any[] = [];
const mockAutoscrollToBottomCalls: any[] = [];

jest.mock('react-virtuoso', () => {
  const React = require('react');
  return {
    Virtuoso: React.forwardRef((props: any, ref: any) => {
      mockVirtuosoProps.push(props);
      React.useImperativeHandle(ref, () => ({
        autoscrollToBottom: () => {
          mockAutoscrollToBottomCalls.push({});
        },
        scrollToIndex: (location: any) => {
          mockScrollToIndexCalls.push(location);
        },
      }));
      return React.createElement(
        'div',
        {className: 'mock-virtuoso'},
        props.data.map((item: ChatDisplayIndexItem, index: number) =>
          React.createElement(
            'div',
            {key: item.key, className: 'mock-virtuoso-row'},
            props.itemContent(index, item),
          ),
        ),
      );
    }),
  };
});

function turnItem(turnIndex: number): ChatDisplayIndexItem {
  return {
    kind: 'turn',
    key: `sess-1:${turnIndex}:agent_message_chunk`,
    turnIndex,
    sourceIndex: turnIndex - 1,
    estimatedHeight: 80,
  };
}

function installAnimationFrameQueue(): {
  frameCallbacks: Array<{callback: FrameRequestCallback; cancelled: boolean; id: number}>;
  restore: () => void;
} {
  const originalRequestAnimationFrame = window.requestAnimationFrame;
  const originalCancelAnimationFrame = window.cancelAnimationFrame;
  const frameCallbacks: Array<{callback: FrameRequestCallback; cancelled: boolean; id: number}> = [];
  const frameById = new Map<number, {callback: FrameRequestCallback; cancelled: boolean; id: number}>();
  let nextFrameId = 1;
  window.requestAnimationFrame = ((callback: FrameRequestCallback) => {
    const id = nextFrameId;
    nextFrameId += 1;
    const frame = {id, callback, cancelled: false};
    frameCallbacks.push(frame);
    frameById.set(id, frame);
    return id;
  }) as typeof window.requestAnimationFrame;
  window.cancelAnimationFrame = ((id: number) => {
    const frame = frameById.get(id);
    if (frame) {
      frame.cancelled = true;
    }
  }) as typeof window.cancelAnimationFrame;
  return {
    frameCallbacks,
    restore: () => {
      window.requestAnimationFrame = originalRequestAnimationFrame;
      window.cancelAnimationFrame = originalCancelAnimationFrame;
    },
  };
}

async function flushAnimationFrames(
  frameCallbacks: Array<{callback: FrameRequestCallback; cancelled: boolean; id: number}>,
): Promise<void> {
  await ReactTestRenderer.act(() => {
    let flushCount = 0;
    while (frameCallbacks.length > 0 && flushCount < 10) {
      const callbacks = frameCallbacks.splice(0);
      callbacks
        .filter(frame => !frame.cancelled)
        .forEach(frame => frame.callback(16 + flushCount));
      flushCount += 1;
    }
  });
}

function createScrollParent(input: {
  clientHeight: number;
  scrollHeight: number;
  scrollTo: jest.Mock;
}): HTMLElement {
  return {
    clientHeight: input.clientHeight,
    scrollHeight: input.scrollHeight,
    scrollTo: input.scrollTo,
  } as unknown as HTMLElement;
}

describe('chat virtuoso mount fallback', () => {
  beforeEach(() => {
    mockVirtuosoProps.length = 0;
    mockScrollToIndexCalls.length = 0;
    mockAutoscrollToBottomCalls.length = 0;
  });

  test('does not render chat rows while the custom scroll parent ref is not attached yet', async () => {
    const scrollRef = {current: null};
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
    const renderItem = jest.fn((item: ChatDisplayIndexItem) => (
      <span>{item.key}</span>
    ));

    await ReactTestRenderer.act(() => {
      renderer = ReactTestRenderer.create(
        <ChatVirtuosoTurnList
          scrollRef={scrollRef}
          displayIndex={{items: [turnItem(1), turnItem(2)]}}
          runtimeKey="project-a/session-a"
          renderItem={renderItem}
        />,
      );
    });

    const rows = renderer!.root.findAllByProps({className: 'chat-virtuoso-row'});
    expect(rows).toHaveLength(0);
    expect(renderItem).not.toHaveBeenCalled();
    expect(renderer!.toJSON()).toEqual(
      expect.objectContaining({
        props: expect.objectContaining({
          className: 'chat-virtuoso-list',
          'data-scroll-parent-pending': true,
        }),
      }),
    );

    await ReactTestRenderer.act(() => {
      renderer!.unmount();
    });
  });

  test('mounts Virtuoso when the custom scroll parent appears after the first layout pass', async () => {
    const animationFrames = installAnimationFrameQueue();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      const scrollRef = {current: null as HTMLElement | null};

      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            scrollRef={scrollRef}
            displayIndex={{items: [turnItem(1)]}}
            runtimeKey="project-a/session-a"
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      expect(renderer!.root.findAllByProps({className: 'mock-virtuoso'})).toHaveLength(0);

      scrollRef.current = {} as HTMLElement;
      await flushAnimationFrames(animationFrames.frameCallbacks);

      expect(renderer!.root.findAllByProps({className: 'mock-virtuoso'})).toHaveLength(1);
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
      animationFrames.restore();
    }
  });

  test('follows app autoscroll intent even when Virtuoso bottom state is stale', async () => {
    const scrollRef = {current: {} as HTMLElement};
    let allowAutoscroll = true;
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            scrollRef={scrollRef}
            displayIndex={{items: [turnItem(1), turnItem(2)]}}
            runtimeKey="project-a/session-a"
            shouldAutoscroll={() => allowAutoscroll}
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      const props = mockVirtuosoProps[mockVirtuosoProps.length - 1];
      expect(props.followOutput(false)).toBe('auto');
      allowAutoscroll = false;
      expect(props.followOutput(true)).toBe(false);
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
    }
  });

  test('keeps height estimates stable while the same turn streams', async () => {
    const scrollRef = {current: {} as HTMLElement};
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            scrollRef={scrollRef}
            displayIndex={{items: [{...turnItem(1), estimatedHeight: 80}]}}
            runtimeKey="project-a/session-a"
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      const firstProps = mockVirtuosoProps[mockVirtuosoProps.length - 1];
      const firstHeightEstimates = firstProps.heightEstimates;
      expect(firstHeightEstimates).toEqual([87]);

      await ReactTestRenderer.act(() => {
        renderer!.update(
          <ChatVirtuosoTurnList
            scrollRef={scrollRef}
            displayIndex={{items: [{...turnItem(1), estimatedHeight: 240}]}}
            runtimeKey="project-a/session-a"
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      const latestProps = mockVirtuosoProps[mockVirtuosoProps.length - 1];
      expect(latestProps.heightEstimates).toBe(firstHeightEstimates);
      expect(latestProps.heightEstimates).toEqual([87]);
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
    }
  });

  test('imperative bottom scroll settles the scroll parent to its physical bottom', async () => {
    const animationFrames = installAnimationFrameQueue();
    const scrollTo = jest.fn();
    const scrollParent = createScrollParent({
      clientHeight: 500,
      scrollHeight: 1200,
      scrollTo,
    });
    const listRef = React.createRef<ChatVirtuosoTurnListHandle>();
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            ref={listRef}
            scrollRef={{current: scrollParent}}
            displayIndex={{items: [turnItem(1), turnItem(2)]}}
            runtimeKey="project-a/session-a"
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      await ReactTestRenderer.act(() => {
        listRef.current?.scrollToBottom('auto');
      });
      await flushAnimationFrames(animationFrames.frameCallbacks);

      expect(mockScrollToIndexCalls).toContainEqual({
        index: 'LAST',
        align: 'end',
        behavior: 'auto',
      });
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: 700,
        behavior: 'auto',
      });
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
      animationFrames.restore();
    }
  });

  test('tail-locked height changes settle the scroll parent to its physical bottom', async () => {
    const animationFrames = installAnimationFrameQueue();
    const scrollTo = jest.fn();
    const scrollParent = createScrollParent({
      clientHeight: 420,
      scrollHeight: 900,
      scrollTo,
    });
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            scrollRef={{current: scrollParent}}
            displayIndex={{items: [turnItem(1), turnItem(2)]}}
            runtimeKey="project-a/session-a"
            shouldAutoscroll={() => true}
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      const props = mockVirtuosoProps[mockVirtuosoProps.length - 1];
      await ReactTestRenderer.act(() => {
        props.totalListHeightChanged();
      });

      expect(mockAutoscrollToBottomCalls).toHaveLength(0);
      expect(mockScrollToIndexCalls).toHaveLength(0);
      expect(scrollTo).not.toHaveBeenCalled();

      await flushAnimationFrames(animationFrames.frameCallbacks);

      expect(mockAutoscrollToBottomCalls).toHaveLength(1);
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: 480,
        behavior: 'auto',
      });
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
      animationFrames.restore();
    }
  });

  test('coalesces repeated tail-locked streaming height changes into one settling frame', async () => {
    const animationFrames = installAnimationFrameQueue();
    const scrollTo = jest.fn();
    const scrollParent = createScrollParent({
      clientHeight: 420,
      scrollHeight: 900,
      scrollTo,
    });
    let renderer: ReactTestRenderer.ReactTestRenderer | undefined;

    try {
      await ReactTestRenderer.act(() => {
        renderer = ReactTestRenderer.create(
          <ChatVirtuosoTurnList
            scrollRef={{current: scrollParent}}
            displayIndex={{items: [turnItem(1), turnItem(2)]}}
            runtimeKey="project-a/session-a"
            shouldAutoscroll={() => true}
            renderItem={item => (
              <span>{item.key}</span>
            )}
          />,
        );
      });

      const props = mockVirtuosoProps[mockVirtuosoProps.length - 1];
      await ReactTestRenderer.act(() => {
        props.totalListHeightChanged(900);
        props.totalListHeightChanged(920);
        props.totalListHeightChanged(940);
      });

      expect(mockAutoscrollToBottomCalls).toHaveLength(0);
      expect(mockScrollToIndexCalls).toHaveLength(0);
      expect(scrollTo).not.toHaveBeenCalled();

      await flushAnimationFrames(animationFrames.frameCallbacks);

      expect(mockAutoscrollToBottomCalls).toHaveLength(1);
      expect(mockScrollToIndexCalls).toHaveLength(2);
      expect(scrollTo).toHaveBeenCalledTimes(2);
      expect(scrollTo).toHaveBeenLastCalledWith({
        top: 480,
        behavior: 'auto',
      });
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
      animationFrames.restore();
    }
  });
});
