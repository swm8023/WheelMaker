import {createChatReadRepairQueue} from '../web/src/chat/chatReadRepair';

describe('chat read repair queue', () => {
  test('serializes reads per session and reruns once when marked dirty during in-flight read', async () => {
    const calls: number[] = [];
    const queue = createChatReadRepairQueue();

    await Promise.all([
      queue.request('sess-1', 10, async cursor => {
        calls.push(cursor);
        queue.request('sess-1', 12, async nextCursor => {
          calls.push(nextCursor);
        });
      }),
      queue.request('sess-1', 11, async cursor => {
        calls.push(cursor);
      }),
    ]);

    expect(calls).toEqual([10, 12]);
  });
});
