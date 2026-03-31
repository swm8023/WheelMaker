import type {RegistryConnectInitPayload, RegistryEnvelope, RegistryErrorPayload} from '../types/registry';

type PendingRequest = {
  resolve: (value: RegistryEnvelope) => void;
  reject: (reason?: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
};

function parseErrorPayload(payload: unknown): RegistryErrorPayload {
  if (!payload || typeof payload !== 'object') {
    return {};
  }
  const input = payload as Record<string, unknown>;
  return {
    code: typeof input.code === 'string' ? input.code : undefined,
    message: typeof input.message === 'string' ? input.message : undefined,
    details: input.details,
  };
}

export class RegistryClient {
  private ws: WebSocket | null = null;
  private seq = 1;
  private readonly pending = new Map<number, PendingRequest>();
  private readonly eventListeners = new Set<(event: RegistryEnvelope) => void>();
  private readonly closeListeners = new Set<() => void>();
  private closing = false;

  constructor(private readonly timeoutMs = 8000) {}

  async connect(url: string): Promise<void> {
    if (this.ws?.readyState === WebSocket.OPEN) {
      return;
    }
    await new Promise<void>((resolve, reject) => {
      const ws = new WebSocket(url);
      let settled = false;
      let sawErrorEvent = false;
      const connectTimer = setTimeout(() => {
        if (settled) return;
        settled = true;
        reject(
          new Error(
            `registry websocket connect timeout: url=${url} (check network, TLS cert, and nginx websocket proxy)`,
          ),
        );
        try {
          ws.close();
        } catch {}
      }, this.timeoutMs);

      const fail = (message: string) => {
        if (settled) return;
        settled = true;
        clearTimeout(connectTimer);
        reject(new Error(message));
        try {
          ws.close();
        } catch {}
      };

      ws.onopen = () => {
        if (settled) return;
        settled = true;
        clearTimeout(connectTimer);
        this.ws = ws;
        this.bind(ws);
        resolve();
      };
      ws.onerror = () => {
        sawErrorEvent = true;
      };
      ws.onclose = event => {
        if (!settled) {
          const suffix = sawErrorEvent ? ' (after websocket error event)' : '';
          fail(
            `registry websocket closed during connect: code=${event.code} reason=${event.reason || 'n/a'} url=${url}${suffix}`,
          );
        }
      };
    });
  }

  async connectInit(payload: RegistryConnectInitPayload): Promise<void> {
    await this.request({
      method: 'connect.init',
      payload,
    });
  }

  async request(args: {
    method: string;
    payload: unknown;
    projectId?: string;
    timeoutMs?: number;
  }): Promise<RegistryEnvelope> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('registry websocket is not connected');
    }
    const requestId = this.seq++;
    const envelope: RegistryEnvelope = {
      requestId,
      type: 'request',
      method: args.method,
      payload: args.payload,
      ...(args.projectId ? {projectId: args.projectId} : {}),
    };

    const timeoutMs = args.timeoutMs ?? this.timeoutMs;
    return new Promise<RegistryEnvelope>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error(`registry request timed out (${timeoutMs}ms): ${args.method}`));
      }, timeoutMs);
      this.pending.set(requestId, {resolve, reject, timer});
      this.ws?.send(JSON.stringify(envelope));
    });
  }

  close(): void {
    this.closing = true;
    for (const [id, pending] of this.pending.entries()) {
      clearTimeout(pending.timer);
      pending.reject(new Error(`connection closed before response: ${id}`));
    }
    this.pending.clear();
    this.ws?.close();
    this.ws = null;
    this.emitClosed();
    this.closing = false;
  }

  onEvent(listener: (event: RegistryEnvelope) => void): () => void {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  onClose(listener: () => void): () => void {
    this.closeListeners.add(listener);
    return () => {
      this.closeListeners.delete(listener);
    };
  }

  private bind(ws: WebSocket): void {
    ws.onmessage = event => {
      if (typeof event.data !== 'string') return;
      let envelope: RegistryEnvelope;
      try {
        envelope = JSON.parse(event.data) as RegistryEnvelope;
      } catch {
        return;
      }
      if (envelope.type === 'event') {
        this.emitEvent(envelope);
        return;
      }
      if (!envelope.requestId) return;
      const pending = this.pending.get(envelope.requestId);
      if (!pending) return;
      this.pending.delete(envelope.requestId);
      clearTimeout(pending.timer);
      if (envelope.type === 'error') {
        const payload = parseErrorPayload(envelope.payload);
        pending.reject(new Error(payload.message ?? 'registry error'));
        return;
      }
      pending.resolve(envelope);
    };
    ws.onclose = () => this.handleSocketClosed(ws);
    ws.onerror = () => this.handleSocketClosed(ws);
  }

  private emitEvent(event: RegistryEnvelope): void {
    for (const listener of this.eventListeners) {
      listener(event);
    }
  }

  private emitClosed(): void {
    for (const listener of this.closeListeners) {
      listener();
    }
  }

  private handleSocketClosed(ws: WebSocket): void {
    if (this.ws !== ws) {
      return;
    }
    this.ws = null;
    for (const [id, pending] of this.pending.entries()) {
      clearTimeout(pending.timer);
      pending.reject(new Error(`connection closed before response: ${id}`));
    }
    this.pending.clear();
    if (!this.closing) {
      this.emitClosed();
    }
  }
}
