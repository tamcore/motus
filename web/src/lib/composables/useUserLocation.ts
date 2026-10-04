import { get, writable, type Readable } from "svelte/store";

interface UserPosition {
  lat: number;
  lng: number;
  accuracy: number;
}

interface UserLocationState {
  active: boolean;
  position: UserPosition | null;
  /** Degrees from true north, or null if unavailable. */
  heading: number | null;
  error: string | null;
}

export interface UseUserLocationReturn extends Readable<UserLocationState> {
  /** Call from a user gesture (iOS compass permission); the first fix may come later. */
  start: () => Promise<void>;
  stop: () => void;
}

const IDLE: UserLocationState = { active: false, position: null, heading: null, error: null };

export function useUserLocation(): UseUserLocationReturn {
  const store = writable<UserLocationState>(IDLE);
  const patch = (p: Partial<UserLocationState>) => store.update((s) => ({ ...s, ...p }));

  let watchId: number | null = null;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let orientationHandler: ((e: any) => void) | null = null;
  let orientationEventName: string | null = null;

  async function start(): Promise<void> {
    if (get(store).active) return;

    patch({ error: null });

    if (!navigator.geolocation) {
      patch({ error: 'Geolocation is not supported by your browser.' });
      return;
    }

    // Start geolocation watch
    watchId = navigator.geolocation.watchPosition(
      (pos) => {
        patch({
          position: { lat: pos.coords.latitude, lng: pos.coords.longitude, accuracy: pos.coords.accuracy },
          error: null,
        });
      },
      (err) => {
        patch({ error: geolocationErrorMessage(err) });
      },
      {
        enableHighAccuracy: true,
        maximumAge: 0,
        timeout: 10000,
      }
    );

    patch({ active: true });

    // Start compass heading (best effort — failures don't prevent location dot)
    await startCompass();
  }

  function stop(): void {
    if (watchId !== null) {
      navigator.geolocation.clearWatch(watchId);
      watchId = null;
    }

    if (orientationHandler && orientationEventName) {
      window.removeEventListener(orientationEventName, orientationHandler);
      orientationHandler = null;
      orientationEventName = null;
    }

    store.set(IDLE);
  }

  async function startCompass(): Promise<void> {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const DevOrEvent = (window as any).DeviceOrientationEvent;

    // iOS 13+ requires explicit permission
    if (typeof DevOrEvent?.requestPermission === 'function') {
      try {
        const permission = await DevOrEvent.requestPermission();
        if (permission !== 'granted') {
          // No heading — location dot will still show, just without compass cone
          return;
        }
      } catch {
        // Permission request failed; continue without heading
        return;
      }
    }

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const handler = (e: any) => {
      if (e.webkitCompassHeading != null) {
        // iOS Safari: webkitCompassHeading is degrees from magnetic north (0 = North)
        patch({ heading: e.webkitCompassHeading });
      } else if (e.absolute && e.alpha != null) {
        // Chrome/Android: deviceorientationabsolute gives true heading
        // alpha = rotation around z-axis, 0 = North when absolute = true
        // Must negate and normalize to get compass bearing
        patch({ heading: (360 - e.alpha) % 360 });
      } else {
        patch({ heading: null });
      }
    };

    // Prefer deviceorientationabsolute (true north, Chrome/Android)
    if ('ondeviceorientationabsolute' in window) {
      orientationEventName = 'deviceorientationabsolute';
    } else {
      orientationEventName = 'deviceorientation';
    }

    orientationHandler = handler;
    window.addEventListener(orientationEventName, handler);
  }

  return { subscribe: store.subscribe, start, stop };
}

function geolocationErrorMessage(err: GeolocationPositionError): string {
  switch (err.code) {
    case err.PERMISSION_DENIED:
      return 'Location access denied. Please allow location in browser settings.';
    case err.POSITION_UNAVAILABLE:
      return 'Location unavailable. Check GPS signal.';
    case err.TIMEOUT:
      return 'Location request timed out.';
    default:
      return 'An unknown location error occurred.';
  }
}

// Map layers for the user's position: accuracy circle, dot, compass cone.

type Leaflet = typeof import("leaflet");
type LeafletMap = import("leaflet").Map;

interface UserLocationLayers {
  sync: (location: UserLocationState) => void;
  toggle: (location: UseUserLocationReturn) => Promise<void>;
}

export function userLocationLayers(
  getLeaflet: () => Leaflet | null,
  getMap: () => LeafletMap | null,
  panOnFirstFix = true,
): UserLocationLayers {
  let accuracyCircle: import("leaflet").Circle | null = null;
  let dotMarker: import("leaflet").Marker | null = null;
  let headingMarker: import("leaflet").Marker | null = null;
  let firstFix = false;

  function updatePosition(pos: UserPosition | null): void {
    const L = getLeaflet();
    const map = getMap();
    if (!L || !map || !pos) return;
    const latlng: [number, number] = [pos.lat, pos.lng];

    if (accuracyCircle) {
      accuracyCircle.setLatLng(latlng);
      accuracyCircle.setRadius(pos.accuracy);
    } else {
      accuracyCircle = L.circle(latlng, {
        radius: pos.accuracy,
        color: "#4285F4",
        fillColor: "#4285F4",
        fillOpacity: 0.1,
        weight: 1,
        interactive: false,
      }).addTo(map);
    }

    if (dotMarker) {
      dotMarker.setLatLng(latlng);
    } else {
      dotMarker = L.marker(latlng, {
        icon: L.divIcon({
          className: "user-location-marker",
          html: '<div class="user-location-dot"><div class="user-location-dot-inner"></div></div>',
          iconSize: [16, 16],
          iconAnchor: [8, 8],
        }),
        interactive: false,
        zIndexOffset: 1000,
      }).addTo(map);
    }

    if (!firstFix) {
      firstFix = true;
      if (panOnFirstFix) map.setView(latlng, Math.max(map.getZoom(), 15));
    }
  }

  function updateHeading(pos: UserPosition | null, heading: number | null, active: boolean): void {
    const L = getLeaflet();
    const map = getMap();
    if (!L || !map || !pos) return;

    if (headingMarker) {
      map.removeLayer(headingMarker);
      headingMarker = null;
    }
    if (heading === null || !active) return;

    headingMarker = L.marker([pos.lat, pos.lng], {
      icon: L.divIcon({
        className: "user-heading-marker",
        html: `<svg width="40" height="40" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg"
          style="transform: rotate(${heading}deg); transform-origin: 20px 20px;">
          <path d="M20 0 L26 20 L20 16 L14 20 Z" fill="#4285F4" fill-opacity="0.5"/>
        </svg>`,
        iconSize: [40, 40],
        iconAnchor: [20, 20],
      }),
      interactive: false,
      zIndexOffset: 999,
    }).addTo(map);
  }

  function remove(): void {
    firstFix = false;
    const map = getMap();
    if (!map) return;
    for (const layer of [accuracyCircle, dotMarker, headingMarker]) {
      if (layer) map.removeLayer(layer);
    }
    accuracyCircle = dotMarker = headingMarker = null;
  }

  function sync(location: UserLocationState): void {
    updatePosition(location.position);
    updateHeading(location.position, location.heading, location.active);
  }

  async function toggle(location: UseUserLocationReturn): Promise<void> {
    if (get(location).active) {
      location.stop();
      remove();
    } else {
      await location.start();
    }
  }

  return { sync, toggle };
}
