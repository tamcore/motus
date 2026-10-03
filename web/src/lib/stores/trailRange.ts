import {
  DEFAULT_TRAIL_RANGE,
  normalizeTrailRange,
  type TrailRange,
} from "$lib/utils/trail-range";
import { persisted } from "./persisted";

const store = persisted<TrailRange>("motus_trail_range", DEFAULT_TRAIL_RANGE, normalizeTrailRange);

/** The user's saved trail range, persisted to localStorage. */
export const trailRange = {
  subscribe: store.subscribe,
  set: (range: TrailRange) => store.set(normalizeTrailRange(range) ?? DEFAULT_TRAIL_RANGE),
};
