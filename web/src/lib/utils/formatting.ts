import { get } from "svelte/store";
import { settings } from "$lib/stores/settings";

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
    case "iso": {
      const opts: Intl.DateTimeFormatOptions = {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      };
      if (timezone) opts.timeZone = timezone;

      return new Intl.DateTimeFormat("en-CA", opts)
        .formatToParts(d)
        .reduce((acc, part) => {
          if (part.type === "literal")
            return acc + (acc.length < 10 ? "-" : acc.length === 10 ? " " : part.value);
          return acc + part.value;
        }, "")
        .replace(",", "");
    }
    case "locale":
      return d.toLocaleString(undefined, { timeZone: timezone });
    case "relative":
      return formatRelative(d);
    default: {
      const opts: Intl.DateTimeFormatOptions = {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      };
      if (timezone) opts.timeZone = timezone;

      return new Intl.DateTimeFormat("en-CA", opts)
        .formatToParts(d)
        .reduce((acc, part) => {
          if (part.type === "literal")
            return acc + (acc.length < 10 ? "-" : acc.length === 10 ? " " : part.value);
          return acc + part.value;
        }, "")
        .replace(",", "");
    }
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

  if (seconds < 0) return "just now";
  if (seconds < 60) return "just now";
  if (minutes < 60) return `${minutes} minute${minutes > 1 ? "s" : ""} ago`;
  if (hours < 24) return `${hours} hour${hours > 1 ? "s" : ""} ago`;
  if (days < 30) return `${days} day${days > 1 ? "s" : ""} ago`;
  return date.toLocaleDateString();
}

/**
 * Format speed according to user unit preference.
 * Input speed is always in km/h.
 */
export function formatSpeed(kmh: number | undefined | null): string {
  if (kmh === undefined || kmh === null) return "0 km/h";

  const s = get(settings);

  if (s.units === "imperial") {
    const mph = kmh * 0.621371;
    return `${mph.toFixed(1)} mph`;
  }

  return `${kmh.toFixed(1)} km/h`;
}

/**
 * Format distance according to user unit preference.
 * Input distance is always in km.
 */
export function formatDistance(km: number): string {
  const s = get(settings);

  if (s.units === "imperial") {
    const miles = km * 0.621371;
    return `${miles.toFixed(2)} mi`;
  }

  return `${km.toFixed(2)} km`;
}

/**
 * Format mileage (odometer) according to user unit preference.
 * Input is always in km. Displays whole numbers with locale grouping.
 */
export function formatMileage(km: number | undefined | null): string {
  if (km === undefined || km === null) return "—";

  const s = get(settings);

  if (s.units === "imperial") {
    const miles = km * 0.621371;
    return `${Math.round(miles).toLocaleString()} mi`;
  }

  return `${Math.round(km).toLocaleString()} km`;
}

/**
 * Convert mileage from km to the user's display unit.
 */
export function mileageToDisplay(km: number): number {
  const s = get(settings);
  return s.units === "imperial" ? km * 0.621371 : km;
}

/**
 * Convert mileage from the user's display unit back to km.
 */
export function mileageFromDisplay(value: number): number {
  const s = get(settings);
  return s.units === "imperial" ? value / 0.621371 : value;
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
