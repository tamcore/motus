import { describe, it, expect } from "vitest";

// Tests for the protocol field payload construction logic used in the
// Edit/Create Device modal. The modal trims whitespace and sends undefined
// when the field is blank so the backend ignores it on updates.

function buildProtocolPayload(formProtocol: string): string | undefined {
  return formProtocol.trim() || undefined;
}

describe("Device modal — protocol payload construction", () => {
  it("includes protocol when set to a non-empty value", () => {
    expect(buildProtocolPayload("h02")).toBe("h02");
  });

  it("includes protocol when set to 'watch'", () => {
    expect(buildProtocolPayload("watch")).toBe("watch");
  });

  it("trims whitespace around the protocol value", () => {
    expect(buildProtocolPayload("  h02  ")).toBe("h02");
  });

  it("returns undefined when protocol is empty string (not sent to API)", () => {
    expect(buildProtocolPayload("")).toBeUndefined();
  });

  it("returns undefined when protocol is only whitespace", () => {
    expect(buildProtocolPayload("   ")).toBeUndefined();
  });
});
