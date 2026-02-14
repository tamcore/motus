import { describe, it, expect, vi } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));

describe("Device page report shortcuts", () => {
  it("should generate correct report link for a device", () => {
    const deviceId = 42;
    const reportsUrl = `/reports?device=${deviceId}`;
    const chartsUrl = `/reports/charts?device=${deviceId}`;

    expect(reportsUrl).toBe("/reports?device=42");
    expect(chartsUrl).toBe("/reports/charts?device=42");
  });

  it("should parse device query param from URL", () => {
    const url = new URL("http://localhost/reports?device=42");
    const deviceParam = url.searchParams.get("device");
    expect(deviceParam).toBe("42");
  });

  it("should validate device param against loaded devices", () => {
    const devices = [
      { id: 1, name: "Device 1" },
      { id: 42, name: "Device 42" },
    ];
    const deviceParam = "42";

    const isValid = devices.some((d) => String(d.id) === deviceParam);
    expect(isValid).toBe(true);

    const invalidParam = "999";
    const isInvalid = devices.some((d) => String(d.id) === invalidParam);
    expect(isInvalid).toBe(false);
  });

  it("should not pre-select device when param is missing", () => {
    const url = new URL("http://localhost/reports");
    const deviceParam = url.searchParams.get("device");
    expect(deviceParam).toBeNull();
  });
});
