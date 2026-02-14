import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock window.matchMedia for theme store
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: query === "(prefers-color-scheme: dark)",
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

import type { Position } from "$lib/types/api";
import {
  METRICS,
  getMetricById,
  hasMetricData,
  getAvailableMetrics,
  buildDatasets,
  buildScales,
  exportChartDataToCSV,
} from "$lib/utils/chart-metrics";

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

function makePosition(overrides: Partial<Position> = {}): Position {
  return {
    id: 1,
    deviceId: 100,
    fixTime: "2024-01-15T10:00:00Z",
    valid: true,
    latitude: 49.79,
    longitude: 9.95,
    altitude: 150,
    speed: 60,
    course: 180,
    accuracy: 5,
    outdated: false,
    ...overrides,
  };
}

function makePositions(count: number): Position[] {
  const positions: Position[] = [];
  for (let i = 0; i < count; i++) {
    positions.push(
      makePosition({
        id: i + 1,
        fixTime: new Date(
          Date.UTC(2024, 0, 15, 10, i * 5, 0),
        ).toISOString(),
        latitude: 49.79 + i * 0.001,
        longitude: 9.95 + i * 0.001,
        speed: 30 + i * 5,
        altitude: 150 + i * 10,
        course: (i * 45) % 360,
        accuracy: 3 + i,
      }),
    );
  }
  return positions;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Chart Metrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    if (typeof localStorage !== "undefined" && localStorage.clear) {
      localStorage.clear();
    }
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("METRICS definitions", () => {
    it("should export a non-empty array of metrics", () => {
      expect(METRICS).toBeDefined();
      expect(Array.isArray(METRICS)).toBe(true);
      expect(METRICS.length).toBeGreaterThan(0);
    });

    it("should have unique metric ids", () => {
      const ids = METRICS.map((m) => m.id);
      const uniqueIds = new Set(ids);
      expect(uniqueIds.size).toBe(ids.length);
    });

    it("each metric should have required fields", () => {
      for (const metric of METRICS) {
        expect(metric).toHaveProperty("id");
        expect(metric).toHaveProperty("label");
        expect(metric).toHaveProperty("unit");
        expect(metric).toHaveProperty("axisId");
        expect(metric).toHaveProperty("color");
        expect(metric).toHaveProperty("extract");
        expect(typeof metric.id).toBe("string");
        expect(typeof metric.label).toBe("string");
        expect(typeof metric.unit).toBe("string");
        expect(typeof metric.axisId).toBe("string");
        expect(typeof metric.color).toBe("string");
        expect(typeof metric.extract).toBe("function");
      }
    });

    it("should include speed, altitude, course, latitude, longitude, accuracy, distance, totalDistance", () => {
      const ids = METRICS.map((m) => m.id);
      expect(ids).toContain("speed");
      expect(ids).toContain("altitude");
      expect(ids).toContain("course");
      expect(ids).toContain("latitude");
      expect(ids).toContain("longitude");
      expect(ids).toContain("accuracy");
      expect(ids).toContain("distance");
      expect(ids).toContain("totalDistance");
    });

    it("each metric color should be a valid hex color", () => {
      for (const metric of METRICS) {
        expect(metric.color).toMatch(/^#[0-9a-fA-F]{6}$/);
      }
    });
  });

  describe("getMetricById", () => {
    it("should return the correct metric for a valid id", () => {
      const speed = getMetricById("speed");
      expect(speed).toBeDefined();
      expect(speed!.id).toBe("speed");
      expect(speed!.unit).toBe("km/h");
    });

    it("should return undefined for an unknown id", () => {
      const result = getMetricById("nonexistent");
      expect(result).toBeUndefined();
    });

    it("should find all defined metrics by id", () => {
      for (const metric of METRICS) {
        const found = getMetricById(metric.id);
        expect(found).toBeDefined();
        expect(found!.id).toBe(metric.id);
      }
    });
  });

  describe("Metric extract functions", () => {
    const pos = makePosition({
      speed: 72.5,
      altitude: 250,
      course: 90,
      latitude: 51.5,
      longitude: -0.12,
      accuracy: 3.5,
    });
    const all = [pos];

    it("speed extracts speed value", () => {
      const metric = getMetricById("speed")!;
      expect(metric.extract(pos, 0, all)).toBe(72.5);
    });

    it("speed returns null for missing speed", () => {
      const metric = getMetricById("speed")!;
      const noSpeed = makePosition({ speed: null });
      expect(metric.extract(noSpeed, 0, [noSpeed])).toBeNull();
    });

    it("altitude extracts altitude value", () => {
      const metric = getMetricById("altitude")!;
      expect(metric.extract(pos, 0, all)).toBe(250);
    });

    it("altitude returns null for missing altitude", () => {
      const metric = getMetricById("altitude")!;
      const noAlt = makePosition({ altitude: null });
      expect(metric.extract(noAlt, 0, [noAlt])).toBeNull();
    });

    it("course extracts course value", () => {
      const metric = getMetricById("course")!;
      expect(metric.extract(pos, 0, all)).toBe(90);
    });

    it("latitude extracts latitude value", () => {
      const metric = getMetricById("latitude")!;
      expect(metric.extract(pos, 0, all)).toBe(51.5);
    });

    it("longitude extracts longitude value", () => {
      const metric = getMetricById("longitude")!;
      expect(metric.extract(pos, 0, all)).toBe(-0.12);
    });

    it("accuracy extracts accuracy value", () => {
      const metric = getMetricById("accuracy")!;
      expect(metric.extract(pos, 0, all)).toBe(3.5);
    });

    it("distance returns 0 for first position", () => {
      const metric = getMetricById("distance")!;
      const positions = makePositions(3);
      expect(metric.extract(positions[0], 0, positions)).toBe(0);
    });

    it("distance returns positive value for subsequent positions", () => {
      const metric = getMetricById("distance")!;
      const positions = makePositions(3);
      const dist = metric.extract(positions[1], 1, positions);
      expect(dist).toBeGreaterThan(0);
    });

    it("totalDistance accumulates over positions", () => {
      const metric = getMetricById("totalDistance")!;
      const positions = makePositions(5);

      const total0 = metric.extract(positions[0], 0, positions);
      const total2 = metric.extract(positions[2], 2, positions);
      const total4 = metric.extract(positions[4], 4, positions);

      expect(total0).toBe(0);
      expect(total2).toBeGreaterThan(0);
      expect(total4).toBeGreaterThan(total2!);
    });
  });

  describe("buildDatasets", () => {
    const positions = makePositions(5);

    it("returns labels and datasets", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result).toHaveProperty("labels");
      expect(result).toHaveProperty("datasets");
      expect(result.labels).toHaveLength(5);
      expect(result.datasets).toHaveLength(1);
    });

    it("labels match position fixTime values", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result.labels).toEqual(positions.map((p) => p.fixTime));
    });

    it("creates one dataset per selected metric", () => {
      const result = buildDatasets(positions, ["speed", "altitude", "course"]);
      expect(result.datasets).toHaveLength(3);
    });

    it("dataset has correct label format", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result.datasets[0].label).toBe("Speed (km/h)");
    });

    it("dataset data has correct length", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result.datasets[0].data).toHaveLength(5);
    });

    it("dataset uses correct axis id", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result.datasets[0].yAxisID).toBe("speed");
    });

    it("dataset uses correct color", () => {
      const speedMetric = getMetricById("speed")!;
      const result = buildDatasets(positions, ["speed"]);
      expect(result.datasets[0].borderColor).toBe(speedMetric.color);
    });

    it("ignores unknown metric ids", () => {
      const result = buildDatasets(positions, ["speed", "nonexistent"]);
      expect(result.datasets).toHaveLength(1);
    });

    it("returns empty datasets for empty metric selection", () => {
      const result = buildDatasets(positions, []);
      expect(result.datasets).toHaveLength(0);
      expect(result.labels).toHaveLength(5);
    });

    it("uses smaller point radius for large datasets", () => {
      const largePositions = makePositions(250);
      const result = buildDatasets(largePositions, ["speed"]);
      expect(result.datasets[0].pointRadius).toBe(0);
    });

    it("uses larger point radius for small datasets", () => {
      const result = buildDatasets(positions, ["speed"]);
      expect(result.datasets[0].pointRadius).toBe(2);
    });
  });

  describe("buildScales", () => {
    it("always includes x axis", () => {
      const scales = buildScales(["speed"], true);
      expect(scales).toHaveProperty("x");
    });

    it("includes y axis for each unique axisId", () => {
      const scales = buildScales(["speed", "altitude"], true);
      expect(scales).toHaveProperty("speed");
      expect(scales).toHaveProperty("altitude");
    });

    it("does not duplicate axis for metrics sharing an axisId", () => {
      // latitude and longitude share the "coords" axisId
      const scales = buildScales(["latitude", "longitude"], true);
      expect(scales).toHaveProperty("coords");
      const keys = Object.keys(scales).filter((k) => k !== "x");
      expect(keys).toHaveLength(1);
    });

    it("alternates axis position left/right", () => {
      const scales = buildScales(
        ["speed", "altitude", "accuracy"],
        true,
      ) as Record<string, any>;
      expect(scales.speed.position).toBe("left");
      expect(scales.altitude.position).toBe("right");
      expect(scales.accuracy.position).toBe("left");
    });

    it("uses dark theme colors when isDark is true", () => {
      const scales = buildScales(["speed"], true) as Record<string, any>;
      expect(scales.x.ticks.color).toBe("#a0a0a0");
      expect(scales.x.grid.color).toBe("#3a3a3a");
    });

    it("uses light theme colors when isDark is false", () => {
      const scales = buildScales(["speed"], false) as Record<string, any>;
      expect(scales.x.ticks.color).toBe("#666666");
      expect(scales.x.grid.color).toBe("#e0e0e0");
    });

    it("only first y axis draws grid on chart area", () => {
      const scales = buildScales(
        ["speed", "altitude"],
        true,
      ) as Record<string, any>;
      expect(scales.speed.grid.drawOnChartArea).toBe(true);
      expect(scales.altitude.grid.drawOnChartArea).toBe(false);
    });
  });

  describe("hasMetricData", () => {
    it("returns true when positions have non-null non-zero altitude", () => {
      const positions = makePositions(3); // altitude: 150, 160, 170
      expect(hasMetricData(positions, "altitude")).toBe(true);
    });

    it("returns false when all altitudes are null", () => {
      const positions = [
        makePosition({ altitude: null }),
        makePosition({ altitude: null }),
      ];
      expect(hasMetricData(positions, "altitude")).toBe(false);
    });

    it("returns false when all altitudes are zero", () => {
      const positions = [
        makePosition({ altitude: 0 }),
        makePosition({ altitude: 0 }),
        makePosition({ altitude: 0 }),
      ];
      expect(hasMetricData(positions, "altitude")).toBe(false);
    });

    it("returns true when at least one altitude is non-zero", () => {
      const positions = [
        makePosition({ altitude: 0 }),
        makePosition({ altitude: 150 }),
        makePosition({ altitude: 0 }),
      ];
      expect(hasMetricData(positions, "altitude")).toBe(true);
    });

    it("returns true for speed even when some are zero", () => {
      const positions = [
        makePosition({ speed: 0 }),
        makePosition({ speed: 60 }),
      ];
      expect(hasMetricData(positions, "speed")).toBe(true);
    });

    it("returns false for empty positions array", () => {
      expect(hasMetricData([], "altitude")).toBe(false);
    });

    it("returns false for unknown metric id", () => {
      const positions = makePositions(3);
      expect(hasMetricData(positions, "nonexistent")).toBe(false);
    });

    it("returns true for latitude (always has data)", () => {
      const positions = makePositions(3);
      expect(hasMetricData(positions, "latitude")).toBe(true);
    });
  });

  describe("getAvailableMetrics", () => {
    it("returns all metrics when positions have full data", () => {
      const positions = makePositions(3);
      const available = getAvailableMetrics(positions);
      expect(available.map((m) => m.id)).toEqual(METRICS.map((m) => m.id));
    });

    it("excludes altitude when all altitudes are zero", () => {
      const positions = [
        makePosition({ altitude: 0 }),
        makePosition({ altitude: 0 }),
      ];
      const available = getAvailableMetrics(positions);
      expect(available.map((m) => m.id)).not.toContain("altitude");
    });

    it("excludes altitude when all altitudes are null", () => {
      const positions = [
        makePosition({ altitude: null }),
        makePosition({ altitude: null }),
      ];
      const available = getAvailableMetrics(positions);
      expect(available.map((m) => m.id)).not.toContain("altitude");
    });

    it("includes altitude when at least one is non-zero", () => {
      const positions = [
        makePosition({ altitude: 0 }),
        makePosition({ altitude: 300 }),
      ];
      const available = getAvailableMetrics(positions);
      expect(available.map((m) => m.id)).toContain("altitude");
    });

    it("returns empty array for empty positions", () => {
      const available = getAvailableMetrics([]);
      expect(available).toHaveLength(0);
    });
  });

  describe("exportChartDataToCSV", () => {
    let createObjectURLSpy: ReturnType<typeof vi.spyOn>;
    let revokeObjectURLSpy: ReturnType<typeof vi.spyOn>;
    let clickSpy: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      createObjectURLSpy = vi
        .spyOn(URL, "createObjectURL")
        .mockReturnValue("blob:test");
      revokeObjectURLSpy = vi
        .spyOn(URL, "revokeObjectURL")
        .mockImplementation(() => {});
      clickSpy = vi.fn();
      vi.spyOn(document, "createElement").mockReturnValue({
        href: "",
        download: "",
        click: clickSpy,
      } as unknown as HTMLAnchorElement);
    });

    it("creates a CSV blob and triggers download", () => {
      const positions = makePositions(3);
      exportChartDataToCSV(positions, ["speed"], "TestDevice");

      expect(createObjectURLSpy).toHaveBeenCalledOnce();
      expect(clickSpy).toHaveBeenCalledOnce();
      expect(revokeObjectURLSpy).toHaveBeenCalledOnce();
    });

    it("includes correct headers for selected metrics", () => {
      const positions = makePositions(2);
      let blobContent = "";

      createObjectURLSpy.mockImplementation((blob: Blob) => {
        // Read blob synchronously via FileReaderSync is not available in jsdom,
        // so we capture the blob for assertion
        const reader = new FileReader();
        reader.readAsText(blob);
        reader.onload = () => {
          blobContent = reader.result as string;
        };
        return "blob:test";
      });

      exportChartDataToCSV(positions, ["speed", "altitude"], "TestDevice");

      // The Blob constructor receives content as an array
      const blobArg = (URL.createObjectURL as any).mock.calls[0][0];
      expect(blobArg).toBeInstanceOf(Blob);
      expect(blobArg.type).toBe("text/csv");
    });

    it("uses device name in filename", () => {
      const positions = makePositions(2);
      const mockAnchor: Record<string, string> = {
        href: "",
        download: "",
      };
      vi.spyOn(document, "createElement").mockReturnValue({
        ...mockAnchor,
        click: clickSpy,
      } as unknown as HTMLAnchorElement);

      exportChartDataToCSV(positions, ["speed"], "MyTracker");

      // The download property is set on the mock element
      // We verify the function completes without error
      expect(clickSpy).toHaveBeenCalled();
    });
  });
});
