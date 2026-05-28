export type PCMChunk = {
  bytes: Uint8Array<ArrayBufferLike>;
  base64: string;
};

export type MicrophonePCMStreamOptions = {
  targetRate?: number;
  chunkBytes?: number;
  bufferSize?: number;
  onChunk: (chunk: PCMChunk) => void;
  onLevel?: (level: number) => void;
  onEnded?: (reason: string) => void;
};

export type MicrophonePCMStream = {
  stop: (options?: {flush?: boolean}) => void;
};

export function floatTo16BitPCM(input: Float32Array): ArrayBuffer {
  const output = new ArrayBuffer(input.length * 2);
  const view = new DataView(output);
  for (let index = 0; index < input.length; index += 1) {
    const sample = Math.max(-1, Math.min(1, input[index]));
    const value = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
    view.setInt16(index * 2, value, true);
  }
  return output;
}

export function resampleLinear(input: Float32Array, sourceRate: number, targetRate: number): Float32Array {
  if (sourceRate === targetRate) {
    return new Float32Array(input);
  }
  if (sourceRate <= 0 || targetRate <= 0 || input.length === 0) {
    return new Float32Array();
  }
  const outputLength = Math.max(1, Math.floor((input.length * targetRate) / sourceRate));
  const output = new Float32Array(outputLength);
  const ratio = sourceRate / targetRate;
  for (let index = 0; index < outputLength; index += 1) {
    const position = index * ratio;
    const left = Math.floor(position);
    const right = Math.min(input.length - 1, left + 1);
    const fraction = position - left;
    output[index] = input[left] + (input[right] - input[left]) * fraction;
  }
  return output;
}

export function base64FromBytes(bytes: Uint8Array): string {
  let binary = '';
  const chunkSize = 0x8000;
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    const chunk = bytes.subarray(offset, offset + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  if (typeof btoa === 'function') {
    return btoa(binary);
  }
  const bufferCtor = (globalThis as unknown as {Buffer?: {from(value: string, encoding: string): {toString(encoding: string): string}}}).Buffer;
  return bufferCtor ? bufferCtor.from(binary, 'binary').toString('base64') : '';
}

export function chunkPCMBytes(bytes: Uint8Array, chunkSize: number): Uint8Array[] {
  const safeChunkSize = Math.max(1, Math.floor(chunkSize));
  const chunks: Uint8Array[] = [];
  for (let offset = 0; offset < bytes.length; offset += safeChunkSize) {
    chunks.push(bytes.slice(offset, offset + safeChunkSize));
  }
  return chunks;
}

export function createPCMChunk(input: Float32Array, sourceRate: number, targetRate = 16000): PCMChunk {
  const resampled = resampleLinear(input, sourceRate, targetRate);
  const bytes = new Uint8Array(floatTo16BitPCM(resampled));
  return {
    bytes,
    base64: base64FromBytes(bytes),
  };
}

function appendBytes(
  left: Uint8Array<ArrayBufferLike>,
  right: Uint8Array<ArrayBufferLike>,
): Uint8Array<ArrayBufferLike> {
  if (left.length === 0) {
    return right;
  }
  const merged = new Uint8Array(left.length + right.length);
  merged.set(left, 0);
  merged.set(right, left.length);
  return merged;
}

function calculateLevel(input: Float32Array): number {
  if (input.length === 0) {
    return 0;
  }
  let sum = 0;
  for (let index = 0; index < input.length; index += 1) {
    sum += input[index] * input[index];
  }
  return Math.min(1, Math.sqrt(sum / input.length));
}

export async function startMicrophonePCMStream(
  options: MicrophonePCMStreamOptions,
): Promise<MicrophonePCMStream> {
  if (typeof navigator === 'undefined' || !navigator.mediaDevices?.getUserMedia) {
    throw new Error('Microphone capture is not available in this browser.');
  }
  if (typeof window === 'undefined') {
    throw new Error('Microphone capture requires a browser window.');
  }

  const AudioContextCtor =
    window.AudioContext ??
    (window as unknown as {webkitAudioContext?: typeof AudioContext}).webkitAudioContext;
  if (!AudioContextCtor) {
    throw new Error('Web Audio API is not available in this browser.');
  }

  const targetRate = options.targetRate ?? 16000;
  const chunkBytes = options.chunkBytes ?? 6400;
  const bufferSize = options.bufferSize ?? 4096;
  const context = new AudioContextCtor();
  let resumeError: unknown = null;
  const initialResume = context.state === 'suspended'
    ? context.resume().catch(error => {
      resumeError = error;
    })
    : Promise.resolve();
  let mediaStream: MediaStream | null = null;
  try {
    mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        autoGainControl: true,
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
      },
    });
    await initialResume;
    if (resumeError) {
      throw resumeError;
    }
    if (context.state === 'suspended') {
      await context.resume();
    }
  } catch (error) {
    mediaStream?.getTracks().forEach(track => track.stop());
    context.close().catch(() => undefined);
    throw error;
  }
  const source = context.createMediaStreamSource(mediaStream);
  const processor = context.createScriptProcessor(bufferSize, 1, 1);
  let pending: Uint8Array<ArrayBufferLike> = new Uint8Array();
  let stopped = false;
  let ended = false;
  const notifyEnded = (reason: string) => {
    if (stopped || ended) {
      return;
    }
    ended = true;
    options.onEnded?.(reason);
  };
  mediaStream.getTracks().forEach(track => {
    track.addEventListener?.('ended', () => notifyEnded('track_ended'));
  });

  processor.onaudioprocess = event => {
    if (stopped) {
      return;
    }
    const input = event.inputBuffer.getChannelData(0);
    options.onLevel?.(calculateLevel(input));
    const chunk = createPCMChunk(input, context.sampleRate, targetRate);
    pending = appendBytes(pending, chunk.bytes);
    while (pending.length >= chunkBytes) {
      const bytes = pending.slice(0, chunkBytes);
      pending = pending.slice(chunkBytes);
      options.onChunk({bytes, base64: base64FromBytes(bytes)});
    }
  };

  const flushPending = () => {
    if (pending.length === 0) {
      return;
    }
    const bytes = pending;
    pending = new Uint8Array();
    options.onChunk({bytes, base64: base64FromBytes(bytes)});
  };

  source.connect(processor);
  processor.connect(context.destination);

  return {
    stop: stopOptions => {
      if (stopped) {
        return;
      }
      if (stopOptions?.flush) {
        flushPending();
      }
      stopped = true;
      processor.disconnect();
      source.disconnect();
      mediaStream.getTracks().forEach(track => track.stop());
      context.close().catch(() => undefined);
    },
  };
}
