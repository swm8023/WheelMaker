export type ChatActiveRuntimeActivation = {
  key: string;
  firstActivation: boolean;
  evicted: string[];
};

export type ChatActiveRuntimeSetOptions = {
  capacity?: number;
  flushSession?: (key: string) => Promise<void> | void;
};

type RuntimeEntry = {
  key: string;
  selected: boolean;
  running: boolean;
  dirty: boolean;
  accessedAt: number;
};

export class ChatActiveRuntimeSet {
  private readonly capacity: number;
  private readonly flushSession: (key: string) => Promise<void> | void;
  private readonly entries = new Map<string, RuntimeEntry>();
  private selected = '';
  private clock = 0;

  constructor(options: ChatActiveRuntimeSetOptions = {}) {
    this.capacity = Math.max(1, Math.trunc(options.capacity ?? 5));
    this.flushSession = options.flushSession ?? (() => undefined);
  }

  keys(): string[] {
    return Array.from(this.entries.keys());
  }

  selectedKey(): string {
    return this.selected;
  }

  isActive(key: string): boolean {
    return this.entries.has(key);
  }

  markDirty(key: string): void {
    const entry = this.entries.get(key);
    if (entry) {
      entry.dirty = true;
    }
  }

  markClean(key: string): void {
    const entry = this.entries.get(key);
    if (entry) {
      entry.dirty = false;
    }
  }

  setRunning(key: string, running: boolean): void {
    const entry = this.entries.get(key);
    if (entry) {
      entry.running = running;
    }
  }

  async activate(key: string, options: {selected?: boolean; running?: boolean} = {}): Promise<ChatActiveRuntimeActivation> {
    const normalizedKey = key.trim();
    if (!normalizedKey) {
      return {key: '', firstActivation: false, evicted: []};
    }
    const existing = this.entries.get(normalizedKey);
    const firstActivation = !existing;
    this.clock += 1;
    this.entries.set(normalizedKey, {
      key: normalizedKey,
      selected: options.selected === true,
      running: options.running ?? existing?.running ?? false,
      dirty: existing?.dirty ?? false,
      accessedAt: this.clock,
    });
    if (options.selected) {
      this.selected = normalizedKey;
      for (const entry of this.entries.values()) {
        entry.selected = entry.key === normalizedKey;
      }
    }
    const evicted = await this.evictOverflow();
    return {key: normalizedKey, firstActivation, evicted};
  }

  private async evictOverflow(): Promise<string[]> {
    const evicted: string[] = [];
    while (this.entries.size > this.capacity) {
      const candidate = this.pickEvictionCandidate();
      if (!candidate) break;
      if (candidate.dirty) {
        try {
          await this.flushSession(candidate.key);
        } catch {
          // Eviction must not be blocked by cache flush failure.
        }
      }
      this.entries.delete(candidate.key);
      evicted.push(candidate.key);
    }
    return evicted;
  }

  private pickEvictionCandidate(): RuntimeEntry | null {
    const candidates = Array.from(this.entries.values())
      .filter(entry => entry.key !== this.selected);
    if (candidates.length === 0) return null;
    const nonRunning = candidates.filter(entry => !entry.running);
    const pool = nonRunning.length > 0 ? nonRunning : candidates;
    return [...pool].sort((a, b) => a.accessedAt - b.accessedAt)[0] ?? null;
  }
}

export function createChatActiveRuntimeSet(options?: ChatActiveRuntimeSetOptions): ChatActiveRuntimeSet {
  return new ChatActiveRuntimeSet(options);
}
