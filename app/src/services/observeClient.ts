import type { RegistryEnvelope } from '../types/observe';

type PendingRequest = {
  resolve: (value: RegistryEnvelope) => void;
  reject: (reason?: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
};

export class RegistryClient {
  private ws: WebSocket | null = null;
  private seq = 0;
  private readonly pending = new Map<string, PendingRequest>();

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
        // Browser WebSocket error events do not expose details.
        // Prefer close code/reason from onclose when available.
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

  async hello(clientName = 'wheelmaker-rn', clientVersion = '0.1.0'): Promise<void> {
    await this.request({
      method: 'hello',
      payload: {
        clientName,
        clientVersion,
        protocolVersion: '1.0',
      },
    });
  }

  async auth(token: string): Promise<void> {
    if (!token) return;
    await this.request({
      method: 'auth',
      payload: { token },
    });
  }

  async request(args: {
    method: string;
    payload: Record<string, unknown>;
    projectId?: string;
    timeoutMs?: number;
  }): Promise<RegistryEnvelope> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('registry websocket is not connected');
    }
    const requestId = `req-${this.seq++}`;
    const envelope: RegistryEnvelope = {
      version: '1.0',
      requestId,
      type: 'request',
      method: args.method,
      payload: args.payload,
      ...(args.projectId ? { projectId: args.projectId } : {}),
    };

    const timeoutMs = args.timeoutMs ?? this.timeoutMs;
    return new Promise<RegistryEnvelope>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error(`registry request timed out (${timeoutMs}ms): ${args.method}`));
      }, timeoutMs);
      this.pending.set(requestId, { resolve, reject, timer });
      this.ws?.send(JSON.stringify(envelope));
    });
  }

  close(): void {
    for (const [id, pending] of this.pending.entries()) {
      clearTimeout(pending.timer);
      pending.reject(new Error(`connection closed before response: ${id}`));
    }
    this.pending.clear();
    this.ws?.close();
    this.ws = null;
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
      if (!envelope.requestId) return;
      const pending = this.pending.get(envelope.requestId);
      if (!pending) return;
      this.pending.delete(envelope.requestId);
      clearTimeout(pending.timer);
      if (envelope.type === 'error') {
        pending.reject(new Error(envelope.error?.message ?? 'registry error'));
        return;
      }
      pending.resolve(envelope);
    };
    ws.onclose = () => this.close();
    ws.onerror = () => this.close();
  }
}

export { RegistryClient as ObserveClient };
