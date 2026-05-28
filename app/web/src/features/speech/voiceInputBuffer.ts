export const VOICE_INPUT_PCM_BYTES_PER_SECOND = 16000 * 2 * 1;
export const DEFAULT_VOICE_INPUT_BUFFER_MAX_MS = 5000;

export type VoiceInputBufferStats = {
  byteCount: number;
  chunkCount: number;
  durationMs: number;
  maxBytes: number;
  overflowed: boolean;
};

export type VoiceInputBufferAppendResult = {
  ok: boolean;
  overflow: boolean;
  stats: VoiceInputBufferStats;
};

export type VoiceInputBufferOptions = {
  maxDurationMs?: number;
  bytesPerSecond?: number;
};

export type VoiceInputBuffer = {
  append: (bytes: Uint8Array<ArrayBufferLike>) => VoiceInputBufferAppendResult;
  drain: () => Uint8Array<ArrayBufferLike>[];
  clear: () => void;
  stats: () => VoiceInputBufferStats;
};

function resolveMaxBytes(maxDurationMs: number, bytesPerSecond: number): number {
  return Math.max(1, Math.floor((Math.max(1, maxDurationMs) * Math.max(1, bytesPerSecond)) / 1000));
}

function cloneBytes(bytes: Uint8Array<ArrayBufferLike>): Uint8Array<ArrayBufferLike> {
  return bytes.slice();
}

export function createVoiceInputBuffer(options: VoiceInputBufferOptions = {}): VoiceInputBuffer {
  const bytesPerSecond = options.bytesPerSecond ?? VOICE_INPUT_PCM_BYTES_PER_SECOND;
  const maxDurationMs = options.maxDurationMs ?? DEFAULT_VOICE_INPUT_BUFFER_MAX_MS;
  const maxBytes = resolveMaxBytes(maxDurationMs, bytesPerSecond);
  let chunks: Uint8Array<ArrayBufferLike>[] = [];
  let byteCount = 0;
  let overflowed = false;

  const stats = (): VoiceInputBufferStats => ({
    byteCount,
    chunkCount: chunks.length,
    durationMs: Math.round((byteCount / Math.max(1, bytesPerSecond)) * 1000),
    maxBytes,
    overflowed,
  });

  return {
    append: bytes => {
      if (overflowed || byteCount + bytes.length > maxBytes) {
        overflowed = true;
        return {
          ok: false,
          overflow: true,
          stats: stats(),
        };
      }
      if (bytes.length > 0) {
        chunks.push(cloneBytes(bytes));
        byteCount += bytes.length;
      }
      return {
        ok: true,
        overflow: false,
        stats: stats(),
      };
    },
    drain: () => {
      const drained = chunks;
      chunks = [];
      byteCount = 0;
      overflowed = false;
      return drained;
    },
    clear: () => {
      chunks = [];
      byteCount = 0;
      overflowed = false;
    },
    stats,
  };
}
