import { describe, it, expect } from "vitest";
import { positionToRoutePosition, toRoutePositions } from "./route-points";
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

describe("positionToRoutePosition", () => {
  it("maps a full (e.g. live WebSocket) position to the route shape", () => {
    expect(
      positionToRoutePosition({
        id: 7,
        deviceId: 1,
        fixTime: "2025-01-01T00:02:00Z",
        valid: true,
        latitude: 51.52,
        longitude: -0.07,
        speed: 12,
        course: 45,
        altitude: 30,
        outdated: false,
      }),
    ).toEqual({
      latitude: 51.52,
      longitude: -0.07,
      speed: 12,
      fixTime: "2025-01-01T00:02:00Z",
      course: 45,
      altitude: 30,
    });
  });

  it("defaults missing speed to 0 and missing course/altitude to null", () => {
    expect(
      positionToRoutePosition({
        id: 8,
        deviceId: 1,
        fixTime: "2025-01-01T00:03:00Z",
        valid: true,
        latitude: 1,
        longitude: 2,
        outdated: false,
      }),
    ).toEqual({ latitude: 1, longitude: 2, speed: 0, fixTime: "2025-01-01T00:03:00Z", course: null, altitude: null });
  });
});
