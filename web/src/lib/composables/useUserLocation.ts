/**
 * Composable for tracking the user's own position on the map.
 *
 * Manages:
 *  - Geolocation watch (navigator.geolocation.watchPosition) for live lat/lng/accuracy
 *  - Compass heading via deviceorientationabsolute (Chrome/Android) or
 *    webkitCompassHeading (iOS Safari); handles iOS 13+ permission request
 *
 * Usage:
 *   const userLocation = useUserLocation();
 *   // Call start() from a user gesture (required for iOS compass permission):
 *   await userLocation.start();
 *   // Access reactive state:
 *   userLocation.active   // boolean
 *   userLocation.position // { lat, lng, accuracy } | null
 *   userLocation.heading  // degrees from true north | null
 *   userLocation.error    // error message | null
 *   // Stop tracking:
 *   userLocation.stop();
 */

export interface UserPosition {
  lat: number;
  lng: number;
  accuracy: number;
}

export interface UseUserLocationReturn {
  /** Whether location tracking is currently active. */
  active: boolean;
  /** Current user position, or null if not yet acquired. */
  position: UserPosition | null;
  /** Compass heading in degrees from true north (0–360), or null if unavailable. */
  heading: number | null;
  /** Human-readable error message, or null if no error. */
  error: string | null;
  /**
   * Start geolocation tracking and compass heading.
   * Must be called from a user gesture (required for iOS compass permission).
   * Returns a promise that resolves once the watch is started (first fix may come later).
   */
  start: () => Promise<void>;
  /** Stop tracking and clean up all watchers/listeners. */
  stop: () => void;
}

export function useUserLocation(): UseUserLocationReturn {
  const state: UseUserLocationReturn = {
    active: false,
    position: null,
    heading: null,
    error: null,
    start,
    stop,
  };

  let watchId: number | null = null;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let orientationHandler: ((e: any) => void) | null = null;
  let orientationEventName: string | null = null;

  async function start(): Promise<void> {
    if (state.active) return;

    state.error = null;

    if (!navigator.geolocation) {
      state.error = 'Geolocation is not supported by your browser.';
      return;
    }

    // Start geolocation watch
    watchId = navigator.geolocation.watchPosition(
      (pos) => {
        state.position = {
          lat: pos.coords.latitude,
          lng: pos.coords.longitude,
          accuracy: pos.coords.accuracy,
        };
        state.error = null;
      },
      (err) => {
        state.error = geolocationErrorMessage(err);
      },
      {
        enableHighAccuracy: true,
        maximumAge: 0,
        timeout: 10000,
      }
    );

    state.active = true;

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

    state.active = false;
    state.position = null;
    state.heading = null;
    state.error = null;
  }

  async function startCompass(): Promise<void> {
    if (typeof window === 'undefined') return;

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
        state.heading = e.webkitCompassHeading;
      } else if (e.absolute && e.alpha != null) {
        // Chrome/Android: deviceorientationabsolute gives true heading
        // alpha = rotation around z-axis, 0 = North when absolute = true
        // Must negate and normalize to get compass bearing
        state.heading = (360 - e.alpha) % 360;
      } else {
        state.heading = null;
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

  return state;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Map layers for the user's position (accuracy circle, dot, compass cone)
// ---------------------------------------------------------------------------

type Leaflet = typeof import("leaflet");
type LeafletMap = import("leaflet").Map;

export interface UserLocationLayers {
  updatePosition: (pos: UserPosition | null) => void;
  updateHeading: (pos: UserPosition | null, heading: number | null, active: boolean) => void;
  remove: () => void;
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

  return { updatePosition, updateHeading, remove };
}
