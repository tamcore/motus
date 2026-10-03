import type { HeatLayer, HeatLatLngTuple, HeatMapOptions, LatLng } from 'leaflet';

export type HeatLayerFactory = (
	latlngs: Array<LatLng | HeatLatLngTuple>,
	options?: HeatMapOptions
) => HeatLayer;

type LeafletRoot = Record<string, unknown> & { heatLayer?: HeatLayerFactory };

let factory: HeatLayerFactory | null = null;

/**
 * Loads leaflet.heat and returns its `heatLayer` factory.
 *
 * leaflet.heat is a legacy script that extends the global `L` once, when its
 * module is first evaluated. `await import('leaflet')` returns a fresh interop
 * wrapper on every call, so the factory has to be captured on first load:
 * reading it from a later wrapper (e.g. when the heatmap page is revisited
 * via client-side navigation) finds nothing.
 */
export async function loadHeatLayer(leaflet: typeof import('leaflet')): Promise<HeatLayerFactory> {
	if (factory) return factory;

	// Leaflet's UMD build registers its exports object as window.L. Prefer
	// that stable object over the per-import wrapper.
	const w = window as unknown as { L?: LeafletRoot };
	const mod = leaflet as unknown as LeafletRoot & { default?: LeafletRoot };
	const root = mod.default ?? mod;
	if (!w.L) w.L = root;

	await import('leaflet.heat');

	const found = w.L?.heatLayer ?? root.heatLayer;
	if (typeof found !== 'function') {
		throw new Error('leaflet.heat did not register L.heatLayer');
	}
	factory = found;
	return factory;
}
