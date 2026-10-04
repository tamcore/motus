import { get } from "svelte/store";
import { settings, type UserSettings } from "$lib/stores/settings";

const KM_TO_MI = 0.621371;

/**
 * Format a date string or Date object according to user preferences.
 * Respects the user's timezone setting for proper local time display.
 */
export function formatDate(date: string | Date): string {
  const s = get(settings);
  const d = typeof date === "string" ? new Date(date) : date;

  if (isNaN(d.getTime())) return String(date);

  // Determine timezone to use — undefined means browser-local
  const timezone = s.timezone === "local" ? undefined : s.timezone;

  switch (s.dateFormat) {
    case "locale":
      return d.toLocaleString(undefined, { timeZone: timezone });
    case "relative":
      return formatRelative(d);
    default:
      return new Intl.DateTimeFormat("sv-SE", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        timeZone: timezone,
      }).format(d);
  }
}

/**
 * Format a date as a relative time string (e.g. "2 hours ago").
 */
export function formatRelative(date: Date): string {
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (seconds < 60) return "just now";
  if (minutes < 60) return `${minutes} minute${minutes > 1 ? "s" : ""} ago`;
  if (hours < 24) return `${hours} hour${hours > 1 ? "s" : ""} ago`;
  if (days < 30) return `${days} day${days > 1 ? "s" : ""} ago`;
  return date.toLocaleDateString();
}

export function formatLastUsed(lastUsedAt: string | null | undefined): string {
  return lastUsedAt ? formatDate(lastUsedAt) : "Never";
}

export function isExpired(expiresAt: string | null | undefined): boolean {
  return !!expiresAt && new Date(expiresAt) < new Date();
}

/** Input speed is always in km/h. */
export function formatSpeed(
  kmh: number | undefined | null,
  units: UserSettings["units"] = get(settings).units,
): string {
  if (kmh === undefined || kmh === null) return "0 km/h";
  return units === "imperial" ? `${(kmh * KM_TO_MI).toFixed(1)} mph` : `${kmh.toFixed(1)} km/h`;
}

const isImperial = () => get(settings).units === "imperial";
const distanceUnit = () => (isImperial() ? "mi" : "km");

/** Converts km to the user's distance unit. */
export function mileageToDisplay(km: number): number {
  return isImperial() ? km * KM_TO_MI : km;
}

/** Converts the user's distance unit back to km. */
export function mileageFromDisplay(value: number): number {
  return isImperial() ? value / KM_TO_MI : value;
}

export function formatDistance(km: number): string {
  return `${mileageToDisplay(km).toFixed(2)} ${distanceUnit()}`;
}

/** Odometer in whole units with locale grouping. */
export function formatMileage(km: number | undefined | null): string {
  if (km === undefined || km === null) return "—";
  return `${Math.round(mileageToDisplay(km)).toLocaleString()} ${distanceUnit()}`;
}

/**
 * Format duration in seconds to a human-readable string.
 */
export function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

/**
 * Convert a course angle (0-360 degrees) to a compass direction string.
 * 0/360 = N, 45 = NE, 90 = E, 135 = SE, 180 = S, 225 = SW, 270 = W, 315 = NW.
 */
export function getCardinalDirection(degrees: number): string {
  const dirs = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
  const normalized = ((degrees % 360) + 360) % 360;
  const idx = Math.round(normalized / 45) % 8;
  return dirs[idx];
}

/**
 * Format latitude and longitude as a compact coordinate string.
 * Uses 4 decimal places (~11m precision).
 */
export function formatCoordinates(lat: number, lng: number): string {
  return `${lat.toFixed(4)}, ${lng.toFixed(4)}`;
}
