import { writable } from 'svelte/store';
import { browser } from '$app/environment';

function createPersistedStore<T>(key: string, initialValue: T) {
	const stored = browser ? localStorage.getItem(key) : null;
	const initial = stored ? JSON.parse(stored) : initialValue;

	const store = writable<T>(initial);

	if (browser) {
		store.subscribe((value) => {
			localStorage.setItem(key, JSON.stringify(value));
		});
	}

	return store;
}

export const currentUser = createPersistedStore<Record<string, unknown> | null>(
	'motus_user',
	null
);
export const isAuthenticated = createPersistedStore<boolean>('motus_authenticated', false);
