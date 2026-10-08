type NamedGeofence = { id: number; name: string };

/**
 * Names of the geofences attached to a device. Attachments the user cannot
 * see (another owner's geofences on a shared device) are counted as "N other".
 * Empty means no attachments: all geofences of the device's users apply.
 */
export function describeDeviceGeofences(
  ids: number[] | undefined,
  geofences: ReadonlyArray<NamedGeofence>,
): string {
  if (!ids || ids.length === 0) return "";
  const names: string[] = [];
  let hidden = 0;
  for (const id of ids) {
    const g = geofences.find((g) => g.id === id);
    if (g) names.push(g.name);
    else hidden++;
  }
  if (hidden > 0) names.push(`${hidden} other`);
  return names.join(", ");
}

/**
 * The geofenceIds to send for a device: the selected geofences the user can
 * see. The server keeps attachments to geofences the user cannot see.
 */
export function deviceGeofencePayload(
  selected: number[],
  geofences: ReadonlyArray<{ id: number }>,
): number[] {
  return [...new Set(selected)]
    .filter((id) => geofences.some((g) => g.id === id))
    .sort((a, b) => a - b);
}

/** Adds or removes id from a checkbox selection. */
export function toggleId(ids: number[], id: number, checked: boolean): number[] {
  const rest = ids.filter((x) => x !== id);
  return checked ? [...rest, id] : rest;
}
