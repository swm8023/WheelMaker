import {createVoiceInputSendQueue} from '../web/src/features/speech/voiceInputSendQueue';

describe('voice input serial send queue', () => {
  test('sends queued PCM chunks one at a time with send-time sequence numbers', async () => {
    const releases: Array<() => void> = [];
    const sendChunk = jest.fn(({streamId, seq, pcm}: {streamId: string; seq: number; pcm: string}) => {
      return new Promise<void>(resolve => {
        releases.push(() => {
          expect(streamId).toBe('speech-1');
          expect(typeof seq).toBe('number');
          expect(typeof pcm).toBe('string');
          resolve();
        });
      });
    });
    const queue = createVoiceInputSendQueue({streamId: 'speech-1', sendChunk});

    const first = queue.enqueue(new Uint8Array([1]));
    const second = queue.enqueue(new Uint8Array([2]));
    await Promise.resolve();

    expect(sendChunk).toHaveBeenCalledTimes(1);
    expect(sendChunk).toHaveBeenNthCalledWith(1, {
      streamId: 'speech-1',
      seq: 1,
      pcm: 'AQ==',
    });
    expect(queue.stats()).toMatchObject({
      queuedChunks: 1,
      queuedBytes: 1,
      sentChunks: 0,
      seq: 1,
    });

    releases[0]();
    await first;
    await Promise.resolve();

    expect(sendChunk).toHaveBeenCalledTimes(2);
    expect(sendChunk).toHaveBeenNthCalledWith(2, {
      streamId: 'speech-1',
      seq: 2,
      pcm: 'Ag==',
    });

    releases[1]();
    await second;
    await queue.drain();
    expect(queue.stats()).toMatchObject({
      queuedChunks: 0,
      queuedBytes: 0,
      sentChunks: 2,
      seq: 2,
      failed: false,
    });
  });

  test('fails the queue on the first chunk send error and does not send later chunks', async () => {
    const sendChunk = jest.fn(async ({seq}: {streamId: string; seq: number; pcm: string}) => {
      if (seq === 1) {
        throw new Error('network down');
      }
    });
    const queue = createVoiceInputSendQueue({streamId: 'speech-1', sendChunk});

    const first = queue.enqueue(new Uint8Array([1])).catch(error => error);
    const second = queue.enqueue(new Uint8Array([2])).catch(error => error);

    await expect(queue.drain()).rejects.toThrow('network down');
    await expect(first).resolves.toBeInstanceOf(Error);
    await expect(second).resolves.toBeInstanceOf(Error);
    expect(sendChunk).toHaveBeenCalledTimes(1);
    expect(queue.stats()).toMatchObject({
      queuedChunks: 0,
      queuedBytes: 0,
      sentChunks: 0,
      failed: true,
    });
  });
});
