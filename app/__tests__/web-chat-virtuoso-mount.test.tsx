import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {ChatVirtuosoTurnList} from '../web/src/chat/ChatVirtuosoTurnList';
import type {ChatDisplayIndexItem} from '../web/src/chat/chatDisplayIndex';

const mockVirtuosoProps: any[] = [];

jest.mock('react-virtuoso', () => {
  const React = require('react');
  return {
    Virtuoso: React.forwardRef((props: any, ref: any) => {
      mockVirtuosoProps.push(props);
      React.useImperativeHandle(ref, () => ({
        autoscrollToBottom: () => undefined,
        scrollToIndex: () => undefined,
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

describe('chat virtuoso mount fallback', () => {
  beforeEach(() => {
    mockVirtuosoProps.length = 0;
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
    const originalRequestAnimationFrame = window.requestAnimationFrame;
    const originalCancelAnimationFrame = window.cancelAnimationFrame;
    const frameCallbacks: FrameRequestCallback[] = [];
    window.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    }) as typeof window.requestAnimationFrame;
    window.cancelAnimationFrame = (() => undefined) as typeof window.cancelAnimationFrame;
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
      await ReactTestRenderer.act(() => {
        const callbacks = frameCallbacks.splice(0);
        callbacks.forEach(callback => callback(16));
      });

      expect(renderer!.root.findAllByProps({className: 'mock-virtuoso'})).toHaveLength(1);
    } finally {
      if (renderer) {
        await ReactTestRenderer.act(() => {
          renderer!.unmount();
        });
      }
      window.requestAnimationFrame = originalRequestAnimationFrame;
      window.cancelAnimationFrame = originalCancelAnimationFrame;
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
});
