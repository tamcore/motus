/**
 * Builds the `attributes` payload for POST /api/commands/send.
 *
 * The API decodes command attributes as a oneOf discriminated by `type`
 * (docs/openapi.yaml `CommandAttributes`), so the command type must be
 * repeated inside the attributes. Commands without parameters must omit
 * the attributes entirely: an empty object matches no variant and the
 * request is rejected ("unable to detect sum type variant").
 */
export function commandAttributesPayload(
  commandType: string,
  values: Record<string, unknown>,
): Record<string, unknown> | undefined {
  if (Object.keys(values).length === 0) {
    return undefined;
  }
  return { ...values, type: commandType };
}

/** Display names of the command types, keyed by API command type. */
export const COMMAND_TYPE_LABELS: Readonly<Record<string, string>> = {
  rebootDevice: "Reboot Device",
  positionPeriodic: "Set Reporting Interval",
  positionSingle: "Request Position",
  sosNumber: "Set SOS Number",
  custom: "Custom (raw text)",
  setSpeedAlarm: "Set Speed Alarm",
  factoryReset: "Factory Reset",
};
