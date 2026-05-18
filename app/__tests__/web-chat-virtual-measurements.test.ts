import {
  createChatVirtualHeightCache,
  readChatVirtualMeasuredHeight,
  resolveChatVirtualAnchor,
  resolveChatVirtualAnchorScrollTop,
  resolveChatVirtualItemEstimate,
  resolveChatVirtualWidthBucket,
  selectChatVirtualPremeasureItems,
  writeChatVirtualMeasuredHeight,
} from '../web/src/chat/chatVirtualMeasurements';

const displayItems = [
  {kind: 'turn' as const, key: 'turn:1', turnIndex: 1, sourceIndex: 0, estimatedHeight: 80},
  {kind: 'turn' as const, key: 'turn:2', turnIndex: 2, sourceIndex: 1, estimatedHeight: 90},
  {kind: 'turn' as const, key: 'turn:3', turnIndex: 3, sourceIndex: 2, estimatedHeight: 100},
  {kind: 'turn' as const, key: 'turn:4', turnIndex: 4, sourceIndex: 3, estimatedHeight: 110},
];

describe('web chat virtual measurements', () => {
  test('buckets measured heights by runtime key and content width', () => {
    const cache = createChatVirtualHeightCache();

    expect(resolveChatVirtualWidthBucket(0)).toBe(0);
    expect(resolveChatVirtualWidthBucket(641)).toBe(640);
    expect(resolveChatVirtualWidthBucket(657)).toBe(672);
    expect(writeChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:1',
      measuredHeight: 123.4,
      runtimeKey: 'project:session',
      widthBucket: 640,
    })).toBe(true);
    expect(readChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:1',
      runtimeKey: 'project:session',
      widthBucket: 640,
    })).toBe(123);
    expect(readChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:1',
      runtimeKey: 'project:session',
      widthBucket: 672,
    })).toBeUndefined();
    expect(readChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:1',
      runtimeKey: 'other:session',
      widthBucket: 640,
    })).toBeUndefined();
  });

  test('uses measured height as the estimate source before falling back', () => {
    const cache = createChatVirtualHeightCache();
    writeChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:2',
      measuredHeight: 188,
      runtimeKey: 'runtime',
      widthBucket: 640,
    });

    expect(resolveChatVirtualItemEstimate({
      cache,
      fallbackHeight: 90,
      itemKey: 'turn:2',
      rowGap: 7,
      runtimeKey: 'runtime',
      widthBucket: 640,
    })).toBe(188);
    expect(resolveChatVirtualItemEstimate({
      cache,
      fallbackHeight: 90,
      itemKey: 'turn:2',
      rowGap: 7,
      runtimeKey: 'runtime',
      widthBucket: 672,
    })).toBe(97);
  });

  test('captures and restores the top visible turn anchor', () => {
    const measurements = [
      {key: 'turn:1', index: 0, start: 0, end: 100, size: 100, lane: 0},
      {key: 'turn:2', index: 1, start: 100, end: 260, size: 160, lane: 0},
      {key: 'turn:3', index: 2, start: 260, end: 380, size: 120, lane: 0},
    ];

    const anchor = resolveChatVirtualAnchor({
      measurements,
      scrollTop: 135,
    });

    expect(anchor).toEqual({itemKey: 'turn:2', offsetInsideItem: 35});
    expect(resolveChatVirtualAnchorScrollTop({
      anchor,
      measurements: [
        {key: 'turn:1', index: 0, start: 0, end: 80, size: 80, lane: 0},
        {key: 'turn:2', index: 1, start: 80, end: 260, size: 180, lane: 0},
      ],
    })).toBe(115);
  });

  test('premeasures only uncached turns before the visible range while scrolling backward', () => {
    const cache = createChatVirtualHeightCache();
    writeChatVirtualMeasuredHeight(cache, {
      itemKey: 'turn:2',
      measuredHeight: 144,
      runtimeKey: 'runtime',
      widthBucket: 640,
    });

    expect(selectChatVirtualPremeasureItems({
      cache,
      displayItems,
      runtimeKey: 'runtime',
      scrollDirection: 'backward',
      visibleStartIndex: 3,
      widthBucket: 640,
      windowSize: 3,
    }).map(item => item.key)).toEqual(['turn:1', 'turn:3']);
    expect(selectChatVirtualPremeasureItems({
      cache,
      displayItems,
      runtimeKey: 'runtime',
      scrollDirection: 'forward',
      visibleStartIndex: 3,
      widthBucket: 640,
      windowSize: 3,
    })).toEqual([]);
  });
});
