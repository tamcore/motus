// iOS clears localStorage on PWA cold starts; the token is mirrored to the more durable IndexedDB.

const DB_NAME = "motus_auth";
const DB_VERSION = 1;
const STORE = "tokens";
const KEY = "auth";
const LS_KEY = "motus_auth_token";

function openDB(): Promise<IDBDatabase | null> {
  return new Promise((resolve) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE);
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => resolve(null);
  });
}

async function idbGet(): Promise<string | null> {
  const db = await openDB();
  if (!db) return null;
  return new Promise((resolve) => {
    const tx = db.transaction(STORE, "readonly");
    const req = tx.objectStore(STORE).get(KEY);
    req.onsuccess = () =>
      resolve(typeof req.result === "string" ? req.result : null);
    req.onerror = () => resolve(null);
  });
}

async function idbPut(value: string | null): Promise<void> {
  const db = await openDB();
  if (!db) return;
  return new Promise((resolve) => {
    const tx = db.transaction(STORE, "readwrite");
    if (value == null) {
      tx.objectStore(STORE).delete(KEY);
    } else {
      tx.objectStore(STORE).put(value, KEY);
    }
    tx.oncomplete = () => resolve();
    tx.onerror = () => resolve();
  });
}

export async function setAuthToken(value: string | null): Promise<void> {
  if (value == null) {
    localStorage.removeItem(LS_KEY);
  } else {
    localStorage.setItem(LS_KEY, value);
  }
  await idbPut(value);
}

/** localStorage first, else re-hydrated from IndexedDB. */
export async function getStoredAuthToken(): Promise<string | null> {
  const local = localStorage.getItem(LS_KEY);
  if (local) return local;
  const idb = await idbGet();
  if (idb) localStorage.setItem(LS_KEY, idb);
  return idb;
}
