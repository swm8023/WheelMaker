import {
  VOICE_INPUT_PCM_BYTES_PER_SECOND,
  createVoiceInputBuffer,
} from '../web/src/features/speech/voiceInputBuffer';

describe('voice input PCM buffer', () => {
  test('tracks buffered PCM bytes and drains chunks in capture order', () => {
    const buffer = createVoiceInputBuffer({
      maxDurationMs: 5000,
      bytesPerSecond: VOICE_INPUT_PCM_BYTES_PER_SECOND,
    });

    expect(buffer.append(new Uint8Array([1, 2, 3]))).toMatchObject({
      ok: true,
      overflow: false,
      stats: {byteCount: 3, chunkCount: 1},
    });
    expect(buffer.append(new Uint8Array([4, 5]))).toMatchObject({
      ok: true,
      overflow: false,
      stats: {byteCount: 5, chunkCount: 2},
    });

    expect(buffer.stats()).toMatchObject({
      byteCount: 5,
      chunkCount: 2,
      durationMs: 0,
      overflowed: false,
    });
    expect(buffer.drain().map(chunk => Array.from(chunk))).toEqual([
      [1, 2, 3],
      [4, 5],
    ]);
    expect(buffer.stats()).toMatchObject({
      byteCount: 0,
      chunkCount: 0,
      durationMs: 0,
      overflowed: false,
    });
  });

  test('reports overflow instead of dropping early audio', () => {
    const buffer = createVoiceInputBuffer({
      maxDurationMs: 10,
      bytesPerSecond: 1000,
    });

    expect(buffer.append(new Uint8Array(6))).toMatchObject({
      ok: true,
      overflow: false,
      stats: {byteCount: 6, chunkCount: 1, durationMs: 6},
    });
    expect(buffer.append(new Uint8Array(5))).toMatchObject({
      ok: false,
      overflow: true,
      stats: {byteCount: 6, chunkCount: 1, durationMs: 6, overflowed: true},
    });

    expect(buffer.drain().map(chunk => chunk.length)).toEqual([6]);
    expect(buffer.stats()).toMatchObject({
      byteCount: 0,
      chunkCount: 0,
      overflowed: false,
    });
  });
});
