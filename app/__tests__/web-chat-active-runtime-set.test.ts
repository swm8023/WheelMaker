import {createChatActiveRuntimeSet} from '../web/src/chat/chatActiveRuntimeSet';

describe('chat active runtime set', () => {
  test('keeps selected session and evicts least recently used non-selected sessions at capacity five', async () => {
    const flushed: string[] = [];
    const runtime = createChatActiveRuntimeSet({
      capacity: 5,
      flushSession: async key => {
        flushed.push(key);
      },
    });

    for (const key of ['a', 'b', 'c', 'd', 'e']) {
      await runtime.activate(key, {selected: key === 'a'});
    }
    runtime.markDirty('a');
    await runtime.activate('f', {selected: true});

    expect(runtime.keys()).toEqual(['b', 'c', 'd', 'e', 'f']);
    expect(flushed).toEqual(['a']);
    expect(runtime.isActive('f')).toBe(true);
    expect(runtime.selectedKey()).toBe('f');
  });

  test('reports first activation separately from already active selection', async () => {
    const runtime = createChatActiveRuntimeSet();

    await expect(runtime.activate('sess-1', {selected: true})).resolves.toEqual({
      key: 'sess-1',
      firstActivation: true,
      evicted: [],
    });
    await expect(runtime.activate('sess-1', {selected: true})).resolves.toEqual({
      key: 'sess-1',
      firstActivation: false,
      evicted: [],
    });
  });
});
