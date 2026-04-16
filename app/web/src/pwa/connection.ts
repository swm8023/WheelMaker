export type ConnectionHooks = {
  connect: () => Promise<void> | void;
  disconnect: (reason: 'background' | 'offline' | 'stop') => void;
};

export type ForegroundConnectionSupervisorOptions = {
  reconnectDelayMs?: number;
};

export class ForegroundConnectionSupervisor {
  private readonly reconnectDelayMs: number;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private started = false;

  constructor(
    private readonly hooks: ConnectionHooks,
    private readonly env: {
      document?: {hidden?: boolean; addEventListener?: (name: string, cb: () => void) => void; removeEventListener?: (name: string, cb: () => void) => void};
      window?: {addEventListener?: (name: string, cb: () => void) => void; removeEventListener?: (name: string, cb: () => void) => void};
      navigator?: {onLine?: boolean};
      setTimeoutImpl?: typeof setTimeout;
      clearTimeoutImpl?: typeof clearTimeout;
    } = {
      document: typeof document !== 'undefined' ? document : undefined,
      window: typeof window !== 'undefined' ? window : undefined,
      navigator: typeof navigator !== 'undefined' ? navigator : undefined,
      setTimeoutImpl: setTimeout,
      clearTimeoutImpl: clearTimeout,
    },
    options: ForegroundConnectionSupervisorOptions = {},
  ) {
    this.reconnectDelayMs = options.reconnectDelayMs ?? 1200;
  }

  start(): void {
    if (this.started) return;
    this.started = true;
    this.env.document?.addEventListener?.('visibilitychange', this.handleVisibilityChange);
    this.env.window?.addEventListener?.('online', this.handleOnline);
    this.env.window?.addEventListener?.('offline', this.handleOffline);
    void this.tryConnect();
  }

  stop(): void {
    if (!this.started) return;
    this.started = false;
    this.clearReconnectTimer();
    this.env.document?.removeEventListener?.('visibilitychange', this.handleVisibilityChange);
    this.env.window?.removeEventListener?.('online', this.handleOnline);
    this.env.window?.removeEventListener?.('offline', this.handleOffline);
    this.hooks.disconnect('stop');
  }

  private readonly handleVisibilityChange = (): void => {
    if (!this.started) return;
    if (this.env.document?.hidden) {
      this.clearReconnectTimer();
      this.hooks.disconnect('background');
      return;
    }
    void this.tryConnect();
  };

  private readonly handleOnline = (): void => {
    if (!this.started) return;
    void this.tryConnect();
  };

  private readonly handleOffline = (): void => {
    if (!this.started) return;
    this.clearReconnectTimer();
    this.hooks.disconnect('offline');
  };

  private isForeground(): boolean {
    return !this.env.document?.hidden;
  }

  private isOnline(): boolean {
    return this.env.navigator?.onLine !== false;
  }

  private scheduleRetry(): void {
    if (!this.started || !this.isForeground() || !this.isOnline() || this.reconnectTimer) {
      return;
    }
    const setTimeoutImpl = this.env.setTimeoutImpl ?? setTimeout;
    this.reconnectTimer = setTimeoutImpl(() => {
      this.reconnectTimer = null;
      void this.tryConnect();
    }, this.reconnectDelayMs);
  }

  private clearReconnectTimer(): void {
    if (!this.reconnectTimer) return;
    const clearTimeoutImpl = this.env.clearTimeoutImpl ?? clearTimeout;
    clearTimeoutImpl(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private async tryConnect(): Promise<void> {
    if (!this.started || !this.isForeground() || !this.isOnline()) {
      return;
    }
    try {
      await this.hooks.connect();
      this.clearReconnectTimer();
    } catch {
      this.scheduleRetry();
    }
  }
}