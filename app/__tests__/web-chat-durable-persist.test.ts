import {createChatDurablePersistQueue} from '../web/src/chat/chatDurablePersist';

describe('chat durable persist queue', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('flushes a dirty session after the configured delay', async () => {
    const flushed: string[] = [];
    const queue = createChatDurablePersistQueue(key => {
      flushed.push(key);
    });

    queue.markDirty('project::session');
    jest.advanceTimersByTime(4999);
    await Promise.resolve();
    expect(flushed).toEqual([]);

    jest.advanceTimersByTime(1);
    await Promise.resolve();
    expect(flushed).toEqual(['project::session']);
  });

  test('reschedules repeated dirty marks and supports explicit flush', async () => {
    const flushed: string[] = [];
    const queue = createChatDurablePersistQueue(key => {
      flushed.push(key);
    }, 5000);

    queue.markDirty('project::session');
    jest.advanceTimersByTime(3000);
    queue.markDirty('project::session');
    jest.advanceTimersByTime(4999);
    await Promise.resolve();
    expect(flushed).toEqual([]);

    await queue.flush('project::session');
    expect(flushed).toEqual(['project::session']);

    jest.advanceTimersByTime(1);
    await Promise.resolve();
    expect(flushed).toEqual(['project::session']);
  });
});
