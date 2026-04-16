export type PWAStorageKind = 'indexeddb' | 'localstorage' | 'memory';

export type PWAStorageAdapter = {
  kind: PWAStorageKind;
  get(key: string): Promise<string | null>;
  set(key: string, value: string): Promise<void>;
  remove(key: string): Promise<void>;
  clear(): Promise<void>;
};

type StorageEnv = {
  indexedDB?: IDBFactory;
  localStorage?: Storage;
};

class MemoryStorageAdapter implements PWAStorageAdapter {
  kind: PWAStorageKind = 'memory';
  private readonly data = new Map<string, string>();

  async get(key: string): Promise<string | null> {
    return this.data.has(key) ? this.data.get(key)! : null;
  }

  async set(key: string, value: string): Promise<void> {
    this.data.set(key, value);
  }

  async remove(key: string): Promise<void> {
    this.data.delete(key);
  }

  async clear(): Promise<void> {
    this.data.clear();
  }
}

class LocalStorageAdapter implements PWAStorageAdapter {
  kind: PWAStorageKind = 'localstorage';
  constructor(private readonly storage: Storage, private readonly prefix: string) {}

  async get(key: string): Promise<string | null> {
    return this.storage.getItem(this.fullKey(key));
  }

  async set(key: string, value: string): Promise<void> {
    this.storage.setItem(this.fullKey(key), value);
  }

  async remove(key: string): Promise<void> {
    this.storage.removeItem(this.fullKey(key));
  }

  async clear(): Promise<void> {
    const keys: string[] = [];
    for (let i = 0; i < this.storage.length; i += 1) {
      const key = this.storage.key(i);
      if (key && key.startsWith(this.prefix + ':')) {
        keys.push(key);
      }
    }
    for (const key of keys) {
      this.storage.removeItem(key);
    }
  }

  private fullKey(key: string): string {
    return `${this.prefix}:${key}`;
  }
}

class IndexedDBStorageAdapter implements PWAStorageAdapter {
  kind: PWAStorageKind = 'indexeddb';
  constructor(private readonly db: IDBDatabase) {}

  async get(key: string): Promise<string | null> {
    const value = await this.run<string | undefined>('readonly', store => store.get(key));
    return typeof value === 'string' ? value : null;
  }

  async set(key: string, value: string): Promise<void> {
    await this.run('readwrite', store => store.put(value, key));
  }

  async remove(key: string): Promise<void> {
    await this.run('readwrite', store => store.delete(key));
  }

  async clear(): Promise<void> {
    await this.run('readwrite', store => store.clear());
  }

  private run<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const tx = this.db.transaction('kv', mode);
      const store = tx.objectStore('kv');
      const req = action(store);
      req.onsuccess = () => {
        resolve(req.result);
      };
      req.onerror = () => {
        reject(req.error ?? new Error('indexeddb request failed'));
      };
      tx.onerror = () => {
        reject(tx.error ?? new Error('indexeddb transaction failed'));
      };
    });
  }
}

function openStorageDB(indexedDB: IDBFactory, dbName: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(dbName, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains('kv')) {
        db.createObjectStore('kv');
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('open indexeddb failed'));
  });
}

export async function createPWAStorageAdapter(
  namespace = 'wheelmaker.pwa',
  env: StorageEnv = globalThis as StorageEnv,
): Promise<PWAStorageAdapter> {
  if (env.indexedDB) {
    try {
      const db = await openStorageDB(env.indexedDB, `${namespace}.db`);
      return new IndexedDBStorageAdapter(db);
    } catch {
      // fall through to local storage
    }
  }
  if (env.localStorage) {
    try {
      const testKey = `${namespace}:__probe__`;
      env.localStorage.setItem(testKey, '1');
      env.localStorage.removeItem(testKey);
      return new LocalStorageAdapter(env.localStorage, namespace);
    } catch {
      // fall through to in-memory
    }
  }
  return new MemoryStorageAdapter();
}