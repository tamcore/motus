/**
 * GPS protocols a device can be assigned to.
 *
 * `value` must match the backend protocol name stored in `device.protocol`
 * (the GPS servers in internal/protocol set it for auto-created devices).
 */

export interface DeviceProtocol {
  /** Backend protocol name. */
  value: string;
  /** Human-readable name shown in the device form. */
  label: string;
}

export const DEVICE_PROTOCOLS: readonly DeviceProtocol[] = [
  { value: "h02", label: "H02" },
  { value: "watch", label: "Watch" },
  { value: "osmand", label: "OsmAnd (Traccar Client)" },
];
