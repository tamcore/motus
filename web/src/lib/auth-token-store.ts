/**
 * Durable auth token storage for iOS PWA contexts.
 *
 * iOS WebKit aggressively clears localStorage on PWA cold starts (when the
 * user closes the PWA and the OS later kills the process). IndexedDB is more
 * resilient and survives most cold starts. This module mirrors the auth
 * token to IndexedDB so the next launch can re-hydrate localStorage even
 * when iOS purges it.
 *
 * getStoredAuthToken() reads localStorage first and falls back to (and
 * re-hydrates from) IndexedDB, so every request can attach the token.
 */

const DB_NAME = "motus_auth";
const DB_VERSION = 1;
const STORE = "tokens";
const KEY = "auth";
const LS_KEY = "motus_auth_token";

function openDB(): Promise<IDBDatabase | null> {
  return new Promise((resolve) => {
    if (typeof indexedDB === "undefined") {
      resolve(null);
      return;
    }
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

/**
 * Persist the auth token to both localStorage (for synchronous request
 * reads) and IndexedDB (for cross-session durability on iOS).
 */
export async function setAuthToken(value: string | null): Promise<void> {
  if (typeof localStorage !== "undefined") {
    if (value == null) {
      localStorage.removeItem(LS_KEY);
    } else {
      localStorage.setItem(LS_KEY, value);
    }
  }
  await idbPut(value);
}

/**
 * Returns the stored auth token, checking localStorage first and falling
 * back to IndexedDB. Used by the API client to ensure every request can
 * attach the X-Auth-Token header.
 */
export async function getStoredAuthToken(): Promise<string | null> {
  if (typeof localStorage !== "undefined") {
    const local = localStorage.getItem(LS_KEY);
    if (local) return local;
  }
  const idb = await idbGet();
  if (idb && typeof localStorage !== "undefined") {
    localStorage.setItem(LS_KEY, idb);
  }
  return idb;
}
