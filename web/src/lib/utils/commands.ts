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

/** Reporting interval preselected for "Set Reporting Interval". */
export const DEFAULT_REPORTING_INTERVAL_SECONDS = 60;

/** Largest reporting interval the API accepts (one day, docs/openapi.yaml). */
export const MAX_REPORTING_INTERVAL_SECONDS = 86400;

/** Quick-select reporting intervals offered for "Set Reporting Interval". */
export const REPORTING_INTERVAL_PRESETS: ReadonlyArray<{ seconds: number; label: string }> = [
  { seconds: 5, label: "5 sec" },
  { seconds: 20, label: "20 sec" },
  { seconds: 60, label: "1 min" },
  { seconds: 300, label: "5 min" },
  { seconds: 600, label: "10 min" },
];

/** Formats a reporting interval, e.g. 60 -> "60 s (1 min)". */
export function formatInterval(seconds: number): string {
  if (seconds >= 60 && seconds % 60 === 0) {
    return `${seconds} s (${seconds / 60} min)`;
  }
  return `${seconds} s`;
}

/**
 * Confirmation shown after POST /api/commands/send. It only states what was
 * requested: the device has not confirmed anything yet. status is the
 * returned command status ("sent" when written to the live connection,
 * "pending" when queued for an offline device).
 */
export function commandSentMessage(
  commandType: string,
  attributes: Record<string, unknown>,
  status: string,
): string {
  const outcome = status === "sent" ? "command sent to device." : "device is offline, command queued.";
  if (commandType === "positionPeriodic" && typeof attributes.frequency === "number") {
    return `Reporting interval of ${formatInterval(attributes.frequency)} requested; ${outcome}`;
  }
  const label = COMMAND_TYPE_LABELS[commandType] ?? commandType;
  return `${label}: ${outcome}`;
}

/**
 * Requested interval of a positionPeriodic command from GET /api/commands
 * (attributes.frequency), e.g. "Interval: 60 s (1 min)"; null otherwise.
 * Shown next to the raw device reply, which is never rewritten.
 */
export function commandIntervalLabel(cmd: {
  type: string;
  attributes?: Record<string, unknown>;
}): string | null {
  if (cmd.type !== "positionPeriodic") return null;
  const raw = cmd.attributes?.frequency;
  const seconds =
    typeof raw === "number" ? raw : typeof raw === "string" && raw.trim() !== "" ? Number(raw) : NaN;
  if (!Number.isFinite(seconds) || seconds <= 0) return null;
  return `Interval: ${formatInterval(seconds)}`;
}
