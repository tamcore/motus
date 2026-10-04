import { describe, it, expect, vi } from 'vitest';
import { userLocationLayers, type UseUserLocationReturn } from '$lib/composables/useUserLocation';

function fakeLocation(active: boolean): UseUserLocationReturn {
	return { active, position: null, heading: null, error: null, start: vi.fn(async () => {}), stop: vi.fn() };
}

describe('userLocationLayers', () => {
	it('toggle starts tracking when inactive', async () => {
		const loc = fakeLocation(false);
		await userLocationLayers(() => null, () => null).toggle(loc);
		expect(loc.start).toHaveBeenCalledOnce();
		expect(loc.stop).not.toHaveBeenCalled();
	});

	it('toggle stops tracking when active', async () => {
		const loc = fakeLocation(true);
		const map = { removeLayer: vi.fn() } as unknown as import('leaflet').Map;
		await userLocationLayers(() => null, () => map).toggle(loc);
		expect(loc.stop).toHaveBeenCalledOnce();
		expect(loc.start).not.toHaveBeenCalled();
	});

	it('sync without a position adds no layers', () => {
		const L = { circle: vi.fn(), marker: vi.fn() } as unknown as typeof import('leaflet');
		const map = {} as unknown as import('leaflet').Map;
		userLocationLayers(() => L, () => map).sync(fakeLocation(true));
		expect(L.circle).not.toHaveBeenCalled();
		expect(L.marker).not.toHaveBeenCalled();
	});
});
