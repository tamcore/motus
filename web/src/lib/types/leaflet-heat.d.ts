declare module 'leaflet.heat' {
	import * as L from 'leaflet';

	export interface HeatMapOptions {
		minOpacity?: number;
		maxZoom?: number;
		max?: number;
		radius?: number;
		blur?: number;
		gradient?: Record<number, string>;
	}

	export interface HeatLayer extends L.Layer {
		setLatLngs(latlngs: Array<[number, number, number?]>): this;
		addLatLng(latlng: [number, number, number?]): this;
		setOptions(options: HeatMapOptions): this;
		redraw(): this;
	}

	export function heatLayer(
		latlngs: Array<[number, number, number?]>,
		options?: HeatMapOptions
	): HeatLayer;
}
