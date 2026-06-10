import { describe, it, expect } from 'vitest';
import { sanitizeReturnTo, buildLoginUrl } from '$lib/utils/returnTo';

describe('sanitizeReturnTo', () => {
	it('accepts a same-origin path', () => {
		expect(sanitizeReturnTo('/devices')).toBe('/devices');
	});

	it('preserves query strings', () => {
		expect(sanitizeReturnTo('/reports?from=2026-01-01&to=2026-02-01')).toBe(
			'/reports?from=2026-01-01&to=2026-02-01'
		);
	});

	it('defaults to / for null or empty input', () => {
		expect(sanitizeReturnTo(null)).toBe('/');
		expect(sanitizeReturnTo('')).toBe('/');
	});

	it('rejects protocol-relative URLs (open redirect)', () => {
		expect(sanitizeReturnTo('//evil.com')).toBe('/');
		expect(sanitizeReturnTo('//evil.com/devices')).toBe('/');
	});

	it('rejects backslash variants (browser normalization)', () => {
		expect(sanitizeReturnTo('/\\evil.com')).toBe('/');
		expect(sanitizeReturnTo('\\/evil.com')).toBe('/');
	});

	it('rejects absolute URLs', () => {
		expect(sanitizeReturnTo('https://evil.com/devices')).toBe('/');
		expect(sanitizeReturnTo('javascript:alert(1)')).toBe('/');
	});

	it('rejects anything containing a colon', () => {
		expect(sanitizeReturnTo('/devices?next=https://evil.com')).toBe('/');
	});

	it('rejects /login to avoid redirect loops', () => {
		expect(sanitizeReturnTo('/login')).toBe('/');
		expect(sanitizeReturnTo('/login?returnTo=%2Fdevices')).toBe('/');
	});
});

describe('buildLoginUrl', () => {
	it('appends the current path as returnTo', () => {
		expect(buildLoginUrl('/devices', '')).toBe('/login?returnTo=%2Fdevices');
	});

	it('includes the query string', () => {
		expect(buildLoginUrl('/reports', '?from=a')).toBe('/login?returnTo=%2Freports%3Ffrom%3Da');
	});

	it('omits returnTo for the root path', () => {
		expect(buildLoginUrl('/', '')).toBe('/login');
	});

	it('omits returnTo for the login page itself', () => {
		expect(buildLoginUrl('/login', '')).toBe('/login');
	});
});
