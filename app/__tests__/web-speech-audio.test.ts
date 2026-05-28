import {
  base64FromBytes,
  chunkPCMBytes,
  floatTo16BitPCM,
  resampleLinear,
} from '../web/src/features/speech/audioCapture';

describe('speech audio helpers', () => {
  test('converts float samples into little-endian signed 16-bit PCM', () => {
    const pcm = new Uint8Array(floatTo16BitPCM(new Float32Array([-1, 0, 1])));

    expect(Array.from(pcm)).toEqual([
      0x00, 0x80,
      0x00, 0x00,
      0xff, 0x7f,
    ]);
  });

  test('resamples mono audio linearly', () => {
    const out = resampleLinear(new Float32Array([0, 1, 0]), 48000, 16000);

    expect(Array.from(out)).toEqual([0]);
  });

  test('encodes bytes to base64 and chunks PCM by size', () => {
    expect(base64FromBytes(new Uint8Array([1, 2, 3]))).toBe('AQID');
    expect(chunkPCMBytes(new Uint8Array([1, 2, 3, 4, 5]), 2).map(item => Array.from(item))).toEqual([
      [1, 2],
      [3, 4],
      [5],
    ]);
  });
});
