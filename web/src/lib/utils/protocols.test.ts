import { describe, it, expect } from "vitest";
import { DEVICE_PROTOCOLS } from "./protocols";

describe("DEVICE_PROTOCOLS", () => {
  it("lists every protocol the backend serves", () => {
    // Values must match the backend protocol names (device.protocol).
    expect(DEVICE_PROTOCOLS.map((p) => p.value)).toEqual(["h02", "watch", "osmand"]);
  });

  it("labels the OsmAnd protocol with the Traccar Client app", () => {
    const osmand = DEVICE_PROTOCOLS.find((p) => p.value === "osmand");
    expect(osmand?.label).toBe("OsmAnd (Traccar Client)");
  });

  it("has unique values and non-empty labels", () => {
    const values = DEVICE_PROTOCOLS.map((p) => p.value);
    expect(new Set(values).size).toBe(values.length);
    for (const p of DEVICE_PROTOCOLS) {
      expect(p.label.trim()).not.toBe("");
    }
  });
});
