import { writable } from "svelte/store";

/**
 * Store holding the current page's refresh callback.
 * Pages set this in onMount and clear it in onDestroy.
 * The PullToRefresh component reads and calls it.
 */
export const refreshHandler = writable<(() => Promise<void>) | null>(null);
