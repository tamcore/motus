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
