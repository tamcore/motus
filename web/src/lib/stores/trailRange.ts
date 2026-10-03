import { writable } from "svelte/store";
import {
  DEFAULT_TRAIL_RANGE,
  normalizeTrailRange,
  type TrailRange,
} from "$lib/utils/trail-range";

const STORAGE_KEY = "motus_trail_range";

function load(): TrailRange {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) return normalizeTrailRange(JSON.parse(saved)) ?? DEFAULT_TRAIL_RANGE;
  } catch {
    // Corrupted or unavailable storage, use the default.
  }
  return DEFAULT_TRAIL_RANGE;
}

/** The user's saved trail range, persisted to localStorage. */
function createTrailRangeStore() {
  const { subscribe, set } = writable<TrailRange>(load());

  subscribe((value) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    } catch {
      // Storage full/unavailable: selection just isn't persisted.
    }
  });

  return {
    subscribe,
    set: (range: TrailRange) => set(normalizeTrailRange(range) ?? DEFAULT_TRAIL_RANGE),
  };
}

export const trailRange = createTrailRangeStore();
