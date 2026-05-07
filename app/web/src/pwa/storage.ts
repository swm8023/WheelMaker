export type PWAStorageKind = 'indexeddb';

export type PWAStorageAdapter = {
  kind: PWAStorageKind;
  get(key: string): Promise<string | null>;
  set(key: string, value: string): Promise<void>;
  remove(key: string): Promise<void>;
  clear(): Promise<void>;
};

type StorageEnv = {
  indexedDB?: IDBFactory;
};

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
  if (!env.indexedDB) {
    throw new Error('IndexedDB is unavailable in this environment.');
  }
  const db = await openStorageDB(env.indexedDB, `${namespace}.db`);
  return new IndexedDBStorageAdapter(db);
}
