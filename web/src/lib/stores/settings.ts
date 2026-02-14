import { writable, get } from "svelte/store";
import { browser } from "$app/environment";

export interface UserSettings {
  dateFormat: "iso" | "locale" | "relative";
  timezone: string;
  units: "metric" | "imperial";
  defaultMapLat: number;
  defaultMapLng: number;
  defaultMapZoom: number;
  mapLocationSet: boolean;
  /** Selected map overlay id (e.g. "none", "topo", "cyclosm"). */
  mapOverlay: string;
  /** Overlay opacity as integer percent 0-100. */
  mapOverlayOpacity: number;
  /** Admin only: show all resources in the instance, not just assigned ones. */
  showAllDevices: boolean;
}

const STORAGE_KEY = "motus_settings";

const defaultSettings: UserSettings = {
  dateFormat: "iso",
  timezone: "local",
  units: "metric",
  defaultMapLat: 49.79,
  defaultMapLng: 9.95,
  defaultMapZoom: 13,
  mapLocationSet: false,
  mapOverlay: "none",
  mapOverlayOpacity: 80,
  showAllDevices: false,
};

function loadSettings(): UserSettings {
  if (!browser) return defaultSettings;

  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
      return { ...defaultSettings, ...JSON.parse(saved) };
    }
  } catch {
    // Corrupted storage, use defaults
  }

  return defaultSettings;
}

function createSettingsStore() {
  const { subscribe, set, update } = writable<UserSettings>(loadSettings());

  if (browser) {
    subscribe((value) => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    });
  }

  return {
    subscribe,
    set,
    update,
    reset: () => {
      set(defaultSettings);
    },
  };
}

export const settings = createSettingsStore();

/**
 * Get a snapshot of current settings (for use outside reactive contexts).
 */
export function getSettings(): UserSettings {
  return get(settings);
}
