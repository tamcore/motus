import { writable, type Writable } from "svelte/store";

function readJson(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    // Older versions stored some values (e.g. the theme) as plain strings.
    return raw;
  }
}

/**
 * A writable store mirrored to localStorage. `parse` validates the stored
 * value and returns null to fall back to `initial`. Unavailable or full
 * storage only means the value is not persisted.
 */
export function persisted<T>(
  key: string,
  initial: T,
  parse: (value: unknown) => T | null = (value) => value as T,
): Writable<T> {
  let value = initial;
  try {
    const raw = localStorage.getItem(key);
    if (raw !== null) value = parse(readJson(raw)) ?? initial;
  } catch {
    // Storage unavailable: use the initial value.
  }

  const store = writable<T>(value);
  store.subscribe((v) => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      // Storage full or unavailable.
    }
  });
  return store;
}
