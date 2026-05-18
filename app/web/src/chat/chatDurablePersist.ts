export type ChatDurablePersistFlush = (sessionKey: string) => Promise<void> | void;

export class ChatDurablePersistQueue {
  private readonly dirtyKeys = new Set<string>();
  private readonly timers = new Map<string, ReturnType<typeof setTimeout>>();

  constructor(
    private readonly flushSession: ChatDurablePersistFlush,
    private readonly delayMs = 5000,
  ) {}

  markDirty(sessionKey: string): void {
    const key = this.normalizeKey(sessionKey);
    if (!key) return;
    this.dirtyKeys.add(key);
    this.schedule(key);
  }

  async flush(sessionKey: string): Promise<void> {
    const key = this.normalizeKey(sessionKey);
    if (!key || !this.dirtyKeys.has(key)) return;
    this.clearTimer(key);
    this.dirtyKeys.delete(key);
    await this.flushSession(key);
  }

  async flushAll(): Promise<void> {
    const keys = Array.from(this.dirtyKeys);
    await Promise.all(keys.map(key => this.flush(key)));
  }

  clear(sessionKey: string): void {
    const key = this.normalizeKey(sessionKey);
    if (!key) return;
    this.clearTimer(key);
    this.dirtyKeys.delete(key);
  }

  private schedule(key: string): void {
    this.clearTimer(key);
    const timer = setTimeout(() => {
      this.timers.delete(key);
      void this.flush(key);
    }, Math.max(0, this.delayMs));
    this.timers.set(key, timer);
  }

  private clearTimer(key: string): void {
    const timer = this.timers.get(key);
    if (timer) {
      clearTimeout(timer);
      this.timers.delete(key);
    }
  }

  private normalizeKey(sessionKey: string): string {
    return sessionKey.trim();
  }
}

export function createChatDurablePersistQueue(
  flushSession: ChatDurablePersistFlush,
  delayMs = 5000,
): ChatDurablePersistQueue {
  return new ChatDurablePersistQueue(flushSession, delayMs);
}
