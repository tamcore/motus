import { derived } from 'svelte/store';
import type { User } from '$lib/types/api';
import { persisted } from './persisted';

export const currentUser = persisted<User | null>('motus_user', null);
export const isAuthenticated = persisted<boolean>('motus_authenticated', false);
export const isAdmin = derived(currentUser, (user) => user?.administrator === true);
export const currentUserName = derived(currentUser, (user) => user?.name || user?.email || '');
