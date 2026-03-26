import type { ObserveEnvelope } from '../types/observe';

type PendingRequest = {
  resolve: (value: ObserveEnvelope) => void;
  reject: (reason?: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
};

export class ObserveClient {
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
      ws.onopen = () => {
        this.ws = ws;
        this.bind(ws);
        resolve();
      };
      ws.onerror = event => {
        reject(new Error(`observe websocket connect failed: ${String(event)}`));
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
  }): Promise<ObserveEnvelope> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('observe websocket is not connected');
    }
    const requestId = `req-${this.seq++}`;
    const envelope: ObserveEnvelope = {
      version: '1.0',
      requestId,
      type: 'request',
      method: args.method,
      payload: args.payload,
      ...(args.projectId ? { projectId: args.projectId } : {}),
    };

    return new Promise<ObserveEnvelope>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error(`observe request timed out: ${args.method}`));
      }, this.timeoutMs);
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
      let envelope: ObserveEnvelope;
      try {
        envelope = JSON.parse(event.data) as ObserveEnvelope;
      } catch {
        return;
      }
      if (!envelope.requestId) return;
      const pending = this.pending.get(envelope.requestId);
      if (!pending) return;
      this.pending.delete(envelope.requestId);
      clearTimeout(pending.timer);
      if (envelope.type === 'error') {
        pending.reject(new Error(envelope.error?.message ?? 'observe error'));
        return;
      }
      pending.resolve(envelope);
    };
    ws.onclose = () => this.close();
    ws.onerror = () => this.close();
  }
}
