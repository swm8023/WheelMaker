import type {
  RegistryEnvelope,
  RegistrySpeechCancelPayload,
  RegistrySpeechChunkPayload,
  RegistrySpeechErrorEvent,
  RegistrySpeechFinishPayload,
  RegistrySpeechStartPayload,
  RegistrySpeechStartResponse,
  RegistrySpeechTranscriptEvent,
} from '../../types/registry';

export type RegistrySpeechTransport = {
  request: (args: {method: string; payload: unknown; timeoutMs?: number}) => Promise<RegistryEnvelope>;
  onEvent: (listener: (event: RegistryEnvelope) => void) => () => void;
  onClose: (listener: () => void) => () => void;
};

export type RegistrySpeechClient = {
  start: (payload: RegistrySpeechStartPayload) => Promise<RegistrySpeechStartResponse>;
  chunk: (payload: RegistrySpeechChunkPayload) => Promise<void>;
  finish: (payload: RegistrySpeechFinishPayload) => Promise<void>;
  cancel: (payload: RegistrySpeechCancelPayload) => Promise<void>;
  onTranscript: (listener: (event: RegistrySpeechTranscriptEvent) => void) => () => void;
  onError: (listener: (event: RegistrySpeechErrorEvent) => void) => () => void;
  onClose: (listener: () => void) => () => void;
};

export function isSpeechTranscriptEvent(
  event: RegistryEnvelope,
): event is RegistryEnvelope<RegistrySpeechTranscriptEvent> {
  const payload = event.payload;
  return event.type === 'event' &&
    event.method === 'speech.transcript' &&
    !!payload &&
    typeof payload === 'object' &&
    !Array.isArray(payload) &&
    typeof (payload as Partial<RegistrySpeechTranscriptEvent>).streamId === 'string' &&
    typeof (payload as Partial<RegistrySpeechTranscriptEvent>).text === 'string' &&
    typeof (payload as Partial<RegistrySpeechTranscriptEvent>).final === 'boolean';
}

export function isSpeechErrorEvent(
  event: RegistryEnvelope,
): event is RegistryEnvelope<RegistrySpeechErrorEvent> {
  const payload = event.payload;
  return event.type === 'event' &&
    event.method === 'speech.error' &&
    !!payload &&
    typeof payload === 'object' &&
    !Array.isArray(payload) &&
    typeof (payload as Partial<RegistrySpeechErrorEvent>).code === 'string' &&
    typeof (payload as Partial<RegistrySpeechErrorEvent>).message === 'string' &&
    typeof (payload as Partial<RegistrySpeechErrorEvent>).retryable === 'boolean';
}

export function createRegistrySpeechClient(transport: RegistrySpeechTransport): RegistrySpeechClient {
  return {
    start: async payload => {
      const response = await transport.request({
        method: 'speech.start',
        payload,
        timeoutMs: 15000,
      });
      const body = (response.payload ?? {}) as Partial<RegistrySpeechStartResponse>;
      return {
        streamId: body.streamId ?? '',
      };
    },
    chunk: async payload => {
      await transport.request({
        method: 'speech.chunk',
        payload,
        timeoutMs: 8000,
      });
    },
    finish: async payload => {
      await transport.request({
        method: 'speech.finish',
        payload,
        timeoutMs: 15000,
      });
    },
    cancel: async payload => {
      await transport.request({
        method: 'speech.cancel',
        payload,
        timeoutMs: 8000,
      });
    },
    onTranscript: listener => transport.onEvent(event => {
      if (isSpeechTranscriptEvent(event)) {
        const payload = event.payload;
        if (!payload) {
          return;
        }
        listener(payload);
      }
    }),
    onError: listener => transport.onEvent(event => {
      if (isSpeechErrorEvent(event)) {
        const payload = event.payload;
        if (!payload) {
          return;
        }
        listener(payload);
      }
    }),
    onClose: listener => transport.onClose(listener),
  };
}
