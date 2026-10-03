/** Raw form inputs of the command parameters. */
export interface CommandFormValues {
  frequency?: string;
  phoneNumber?: string;
  speed?: string;
  text?: string;
}

function parsePositiveInt(raw: string | undefined): number | null {
  const s = (raw ?? "").trim();
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  return n > 0 ? n : null;
}

/** Parameters of one command type, or a user-facing error. */
function commandParams(
  commandType: string,
  values: CommandFormValues,
): { params: Record<string, unknown> } | { error: string } {
  switch (commandType) {
    case "positionPeriodic": {
      const frequency = parsePositiveInt(values.frequency);
      if (frequency === null) return { error: "Interval must be a positive whole number of seconds" };
      if (frequency > MAX_REPORTING_INTERVAL_SECONDS) {
        return { error: `Interval must be at most ${MAX_REPORTING_INTERVAL_SECONDS} seconds (1 day)` };
      }
      return { params: { frequency } };
    }
    case "sosNumber": {
      const phoneNumber = (values.phoneNumber ?? "").trim();
      return phoneNumber ? { params: { phoneNumber } } : { error: "SOS number is required" };
    }
    case "setSpeedAlarm": {
      const s = (values.speed ?? "").trim();
      const speed = Number(s);
      if (s === "" || !Number.isFinite(speed) || speed < 0) {
        return { error: "Speed must be 0 or a positive number" };
      }
      return { params: { speed } };
    }
    case "custom": {
      const text = (values.text ?? "").trim();
      return text ? { params: { text } } : { error: "Command text is required" };
    }
    default:
      return { params: {} };
  }
}

/**
 * Validates the command form and builds the `attributes` payload for
 * POST /api/commands/send and command notification rules.
 *
 * The API decodes command attributes as a oneOf discriminated by `type`
 * (docs/openapi.yaml `CommandAttributes`), so the command type must be
 * repeated inside the attributes. Commands without parameters must omit
 * the attributes entirely: an empty object matches no variant and the
 * request is rejected ("unable to detect sum type variant").
 */
export function buildCommandAttributes(
  commandType: string,
  values: CommandFormValues,
): { attributes?: Record<string, unknown>; error?: string } {
  const result = commandParams(commandType, values);
  if ("error" in result) return { error: result.error };
  if (Object.keys(result.params).length === 0) return {};
  return { attributes: { ...result.params, type: commandType } };
}

/** Display names of the command types, keyed by API command type, in display order. */
export const COMMAND_TYPE_LABELS: Readonly<Record<string, string>> = {
  positionPeriodic: "Set Reporting Interval",
  positionSingle: "Request Position",
  rebootDevice: "Reboot Device",
  sosNumber: "Set SOS Number",
  setSpeedAlarm: "Set Speed Alarm",
  custom: "Custom (raw text)",
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
  const seconds = cmd.attributes?.frequency;
  if (typeof seconds !== "number" || !Number.isFinite(seconds) || seconds <= 0) return null;
  return `Interval: ${formatInterval(seconds)}`;
}
