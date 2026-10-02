import { describe, it, expect } from "vitest";
import { toRoutePositions } from "./route-points";
import { generateGPX } from "./gpx";

describe("toRoutePositions", () => {
  it("maps compact points to the position fields route views use", () => {
    const positions = toRoutePositions([
      { lat: 51.5, lon: -0.09, speed: 30, fixTime: "2025-01-01T00:00:00Z", course: 90, altitude: 12 },
      { lat: 51.51, lon: -0.08, speed: 0, fixTime: "2025-01-01T00:01:00Z" },
    ]);

    expect(positions).toEqual([
      { latitude: 51.5, longitude: -0.09, speed: 30, fixTime: "2025-01-01T00:00:00Z", course: 90, altitude: 12 },
      { latitude: 51.51, longitude: -0.08, speed: 0, fixTime: "2025-01-01T00:01:00Z", course: null, altitude: null },
    ]);
  });

  it("feeds GPX export with coordinates, time and speed", () => {
    const gpx = generateGPX(
      toRoutePositions([{ lat: 51.5, lon: -0.09, speed: 30, fixTime: "2025-01-01T00:00:00Z" }]),
      "route",
    );

    expect(gpx).toContain('<trkpt lat="51.5" lon="-0.09">');
    expect(gpx).toContain("<time>2025-01-01T00:00:00Z</time>");
    expect(gpx).toContain("<speed>30</speed>");
  });
});
