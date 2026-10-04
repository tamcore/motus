import { describe, it, expect, vi, afterEach } from 'vitest';
import { readable } from 'svelte/store';
import {
	useUserLocation,
	userLocationLayers,
	type UseUserLocationReturn
} from '$lib/composables/useUserLocation';

function fakeLocation(active: boolean): UseUserLocationReturn {
	const { subscribe } = readable({ active, position: null, heading: null, error: null });
	return { subscribe, start: vi.fn(async () => {}), stop: vi.fn() };
}

function fakeLeaflet() {
	const marker = { addTo: vi.fn().mockReturnThis(), setLatLng: vi.fn() };
	const circle = { addTo: vi.fn().mockReturnThis(), setLatLng: vi.fn(), setRadius: vi.fn() };
	const L = {
		circle: vi.fn(() => circle),
		marker: vi.fn(() => marker),
		divIcon: vi.fn()
	} as unknown as typeof import('leaflet');
	const map = {
		setView: vi.fn(),
		getZoom: vi.fn(() => 10),
		removeLayer: vi.fn()
	} as unknown as import('leaflet').Map;
	return { L, map, marker };
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
		userLocationLayers(() => L, () => map).sync({
			active: true,
			position: null,
			heading: null,
			error: null
		});
		expect(L.circle).not.toHaveBeenCalled();
		expect(L.marker).not.toHaveBeenCalled();
	});
});

describe('useUserLocation', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('notifies subscribers on each fix so the marker is redrawn', async () => {
		let onFix: PositionCallback = () => {};
		vi.stubGlobal('navigator', {
			geolocation: {
				watchPosition: vi.fn((cb: PositionCallback) => {
					onFix = cb;
					return 1;
				}),
				clearWatch: vi.fn()
			}
		});
		const fix = (lat: number, lng: number) =>
			onFix({ coords: { latitude: lat, longitude: lng, accuracy: 5 } } as GeolocationPosition);

		const location = useUserLocation();
		const { L, map, marker } = fakeLeaflet();
		const layers = userLocationLayers(() => L, () => map);
		const unsubscribe = location.subscribe(layers.sync);

		await location.start();
		fix(52.5, 13.4);
		fix(48.1, 11.6);

		expect(L.marker).toHaveBeenCalledOnce();
		expect(marker.setLatLng).toHaveBeenCalledWith([48.1, 11.6]);

		location.stop();
		unsubscribe();
	});
});
