import { persisted } from "./persisted";

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

const store = persisted<UserSettings>(STORAGE_KEY, defaultSettings, (saved) =>
  saved && typeof saved === "object" ? { ...defaultSettings, ...saved } : null,
);

export const settings = {
  ...store,
  reset: () => store.set(defaultSettings),
};
