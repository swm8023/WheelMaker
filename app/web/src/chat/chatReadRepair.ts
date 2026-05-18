export type ChatReadRepairRun = (cursor: number) => Promise<void> | void;

type RepairState = {
  inFlight: boolean;
  pending: boolean;
  cursor: number;
  nextCursor: number;
  run: ChatReadRepairRun | null;
  nextRun: ChatReadRepairRun | null;
  promise: Promise<void> | null;
};

export class ChatReadRepairQueue {
  private readonly states = new Map<string, RepairState>();

  request(sessionKey: string, cursor: number, run: ChatReadRepairRun): Promise<void> {
    const key = sessionKey.trim();
    if (!key) return Promise.resolve();
    const safeCursor = Math.max(0, Math.trunc(cursor));
    const existing = this.states.get(key);
    if (existing?.inFlight) {
      existing.nextCursor = Math.max(existing.nextCursor, safeCursor);
      existing.nextRun = run;
      existing.pending = true;
      return existing.promise ?? Promise.resolve();
    }
    const state: RepairState = existing ?? {
      inFlight: true,
      pending: false,
      cursor: safeCursor,
      nextCursor: safeCursor,
      run,
      nextRun: null,
      promise: null,
    };
    state.inFlight = true;
    state.pending = false;
    state.cursor = safeCursor;
    state.nextCursor = safeCursor;
    state.run = run;
    state.nextRun = null;
    this.states.set(key, state);
    state.promise = Promise.resolve().then(() => this.drain(key, state));
    return state.promise;
  }

  private async drain(key: string, state: RepairState): Promise<void> {
    try {
      do {
        state.pending = false;
        const run = state.run;
        if (run) {
          await run(state.cursor);
        }
        if (!state.pending) {
          continue;
        }
        state.cursor = state.nextCursor;
        state.run = state.nextRun ?? state.run;
        state.nextRun = null;
      } while (state.pending);
    } finally {
      state.inFlight = false;
      state.promise = null;
      if (state.pending) {
        state.inFlight = true;
        state.promise = Promise.resolve().then(() => this.drain(key, state));
      } else {
        this.states.delete(key);
      }
    }
  }
}

export function createChatReadRepairQueue(): ChatReadRepairQueue {
  return new ChatReadRepairQueue();
}
