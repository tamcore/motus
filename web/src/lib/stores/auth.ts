import { writable } from 'svelte/store';

function createPersistedStore<T>(key: string, initialValue: T) {
	const stored = localStorage.getItem(key);
	const initial = stored ? JSON.parse(stored) : initialValue;

	const store = writable<T>(initial);

	store.subscribe((value) => {
		localStorage.setItem(key, JSON.stringify(value));
	});

	return store;
}

export const currentUser = createPersistedStore<Record<string, unknown> | null>(
	'motus_user',
	null
);
export const isAuthenticated = createPersistedStore<boolean>('motus_authenticated', false);
