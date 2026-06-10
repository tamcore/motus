/**
 * Open-redirect-safe handling of the login `returnTo` query parameter.
 *
 * Only same-origin absolute paths are accepted. Everything else — absolute
 * URLs, protocol-relative URLs (`//evil.com`), backslash variants the
 * browser would normalize to `//`, or any value containing a scheme
 * separator — falls back to `/`.
 */

/** Sanitize a raw returnTo value into a safe same-origin path. */
export function sanitizeReturnTo(raw: string | null): string {
	if (!raw) return '/';
	if (!raw.startsWith('/')) return '/';
	if (raw.startsWith('//')) return '/';
	if (raw.includes('\\')) return '/';
	if (raw.includes(':')) return '/';
	if (raw === '/login' || raw.startsWith('/login?')) return '/';
	return raw;
}

/** Build the login URL for a session-expiry redirect, preserving the current location. */
export function buildLoginUrl(pathname: string, search: string): string {
	if (pathname === '/' || pathname === '/login') return '/login';
	return `/login?returnTo=${encodeURIComponent(pathname + search)}`;
}
