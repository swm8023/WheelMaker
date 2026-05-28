import {base64FromBytes} from './audioCapture';

export type VoiceInputSendChunkPayload = {
  streamId: string;
  seq: number;
  pcm: string;
};

export type VoiceInputSendQueueOptions = {
  streamId: string;
  sendChunk: (payload: VoiceInputSendChunkPayload) => Promise<void>;
};

export type VoiceInputSendQueueStats = {
  queuedBytes: number;
  queuedChunks: number;
  sentChunks: number;
  seq: number;
  failed: boolean;
  cancelled: boolean;
};

export type VoiceInputSendQueue = {
  enqueue: (bytes: Uint8Array<ArrayBufferLike>) => Promise<void>;
  drain: () => Promise<void>;
  cancel: () => void;
  stats: () => VoiceInputSendQueueStats;
};

export function createVoiceInputSendQueue(options: VoiceInputSendQueueOptions): VoiceInputSendQueue {
  let seq = 0;
  let sentChunks = 0;
  let queuedChunks = 0;
  let queuedBytes = 0;
  let failed: unknown = null;
  let cancelled = false;
  let chain = Promise.resolve();

  const runChunk = async (bytes: Uint8Array<ArrayBufferLike>) => {
    if (cancelled) {
      throw new Error('voice input send queue cancelled');
    }
    if (failed) {
      throw failed;
    }
    seq += 1;
    queuedChunks = Math.max(0, queuedChunks - 1);
    queuedBytes = Math.max(0, queuedBytes - bytes.length);
    await options.sendChunk({
      streamId: options.streamId,
      seq,
      pcm: base64FromBytes(bytes),
    });
    sentChunks += 1;
  };

  const rememberFailure = (error: unknown) => {
    failed = error;
    queuedChunks = 0;
    queuedBytes = 0;
    throw error;
  };

  return {
    enqueue: bytes => {
      if (cancelled) {
        return Promise.reject(new Error('voice input send queue cancelled'));
      }
      if (failed) {
        return Promise.reject(failed);
      }
      const copy = bytes.slice();
      queuedChunks += 1;
      queuedBytes += copy.length;
      const job = chain.then(() => runChunk(copy)).catch(rememberFailure);
      chain = job.catch(() => undefined);
      return job;
    },
    drain: async () => {
      await chain;
      if (failed) {
        throw failed;
      }
    },
    cancel: () => {
      cancelled = true;
      queuedChunks = 0;
      queuedBytes = 0;
    },
    stats: () => ({
      queuedBytes,
      queuedChunks,
      sentChunks,
      seq,
      failed: !!failed,
      cancelled,
    }),
  };
}
