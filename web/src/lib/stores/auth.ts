import { derived } from 'svelte/store';
import type { User } from '$lib/types/api';
import { setAuthToken } from '$lib/auth-token-store';
import { generateLoginToken } from '$lib/utils/native-interface';
import { persisted } from './persisted';
import { wsManager } from './websocket';

export const currentUser = persisted<User | null>('motus_user', null);
export const isAuthenticated = persisted<boolean>('motus_authenticated', false);
export const isAdmin = derived(currentUser, (user) => user?.administrator === true);
export const currentUserName = derived(currentUser, (user) => user?.name || user?.email || '');

interface LoginOptions {
	/** Persisted when given; null clears the stored token. */
	authToken?: string | null;
	sendNativeLoginToken?: boolean;
}

export async function completeLogin(
	user: User,
	{ authToken, sendNativeLoginToken = false }: LoginOptions = {},
): Promise<void> {
	currentUser.set(user);
	isAuthenticated.set(true);
	if (authToken !== undefined) await setAuthToken(authToken);
	wsManager.connect();
	if (sendNativeLoginToken) generateLoginToken();
}
