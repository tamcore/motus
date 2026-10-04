import type { ChartDataset } from "chart.js";
import type { Position } from "$lib/types/api";
import { haversineDistance } from "$lib/utils/trips";
import { downloadCSV } from "$lib/utils/download";
import { dateValue } from "$lib/utils/date-range";

/**
 * Metric definitions for device analytics charts.
 *
 * Each metric describes how to extract a value from a Position,
 * what unit it uses, and which Y-axis it should bind to.
 */

type LineDataset = ChartDataset<"line", (number | null)[]>;

export interface MetricDefinition {
  id: string;
  label: string;
  unit: string;
  /** Which Y-axis this metric maps to (metrics sharing a unit share an axis). */
  axisId: string;
  /** Border color for the line in dark theme. */
  color: string;
  /** Extract the numeric value from a Position. Index is position index in array. */
  extract: (pos: Position, index: number, all: Position[]) => number | null;
}

export const METRICS: MetricDefinition[] = [
  {
    id: "speed",
    label: "Speed",
    unit: "km/h",
    axisId: "speed",
    color: "#00d4ff",
    extract: (pos) => pos.speed ?? null,
  },
  {
    id: "altitude",
    label: "Altitude",
    unit: "m",
    axisId: "altitude",
    color: "#00ff88",
    extract: (pos) => pos.altitude ?? null,
  },
  {
    id: "course",
    label: "Course",
    unit: "\u00b0",
    axisId: "course",
    color: "#ffaa00",
    extract: (pos) => pos.course ?? null,
  },
  {
    id: "latitude",
    label: "Latitude",
    unit: "\u00b0",
    axisId: "coords",
    color: "#ff6b6b",
    extract: (pos) => pos.latitude,
  },
  {
    id: "longitude",
    label: "Longitude",
    unit: "\u00b0",
    axisId: "coords",
    color: "#c084fc",
    extract: (pos) => pos.longitude,
  },
  {
    id: "accuracy",
    label: "Accuracy",
    unit: "m",
    axisId: "accuracy",
    color: "#f472b6",
    extract: (pos) => pos.accuracy ?? null,
  },
  {
    id: "distance",
    label: "Distance",
    unit: "km",
    axisId: "distance",
    color: "#34d399",
    extract: (_pos, index, all) => {
      if (index === 0) return 0;
      return haversineDistance(
        all[index - 1].latitude,
        all[index - 1].longitude,
        all[index].latitude,
        all[index].longitude,
      );
    },
  },
  {
    id: "totalDistance",
    label: "Total Distance",
    unit: "km",
    axisId: "distance",
    color: "#a78bfa",
    extract: (_pos, index, all) => {
      let total = 0;
      for (let i = 1; i <= index; i++) {
        total += haversineDistance(
          all[i - 1].latitude,
          all[i - 1].longitude,
          all[i].latitude,
          all[i].longitude,
        );
      }
      return total;
    },
  },
];

/** Known metrics for the ids, in id order. */
function selectedMetrics(ids: string[]): MetricDefinition[] {
  return ids.flatMap((id) => METRICS.find((m) => m.id === id) ?? []);
}

/** Metrics with meaningful (non-null, non-zero) data in the positions. */
export function getAvailableMetrics(
  positions: Position[],
): MetricDefinition[] {
  return METRICS.filter((m) =>
    positions.some((pos, idx) => {
      const val = m.extract(pos, idx, positions);
      return val !== null && val !== 0;
    }),
  );
}

/**
 * Build Chart.js dataset objects for the selected metrics and positions.
 */
export function buildDatasets(
  positions: Position[],
  selectedMetricIds: string[],
): { labels: string[]; datasets: LineDataset[] } {
  const labels = positions.map((p) => p.fixTime);

  const datasets = selectedMetrics(selectedMetricIds).map((metric): LineDataset => ({
    label: `${metric.label} (${metric.unit})`,
    data: positions.map((pos, idx) => metric.extract(pos, idx, positions)),
    borderColor: metric.color,
    backgroundColor: `${metric.color}1a`,
    yAxisID: metric.axisId,
    tension: 0.3,
    pointRadius: positions.length > 200 ? 0 : 2,
    pointHoverRadius: 4,
    borderWidth: 2,
    fill: false,
  }));

  return { labels, datasets };
}


/**
 * Build Chart.js scales config for selected metrics.
 * Groups metrics by axisId so metrics sharing the same unit share an axis.
 */
export function buildScales(
  selectedMetricIds: string[],
  isDark: boolean,
): Record<string, object> {
  const gridColor = isDark ? "#3a3a3a" : "#e0e0e0";
  const tickColor = isDark ? "#a0a0a0" : "#666666";

  const scales: Record<string, object> = {
    x: {
      type: "time" as const,
      time: {
        tooltipFormat: "MMM d, HH:mm:ss",
        displayFormats: {
          second: "HH:mm:ss",
          minute: "HH:mm",
          hour: "MMM d, HH:mm",
          day: "MMM d",
        },
      },
      ticks: { color: tickColor, maxRotation: 45, autoSkip: true },
      grid: { color: gridColor },
      title: { display: true, text: "Time", color: tickColor },
    },
  };

  const axisMetrics = selectedMetrics(selectedMetricIds).filter(
    (m, i, all) => all.findIndex((o) => o.axisId === m.axisId) === i,
  );

  // Alternate axes left/right
  axisMetrics.forEach((am, idx) => {
    const position = idx % 2 === 0 ? "left" : "right";
    scales[am.axisId] = {
      type: "linear" as const,
      display: true,
      position,
      ticks: { color: tickColor },
      grid: {
        color: idx === 0 ? gridColor : "transparent",
        drawOnChartArea: idx === 0,
      },
      title: {
        display: true,
        text: am.unit,
        color: tickColor,
      },
    };
  });

  return scales;
}

/**
 * Export chart data to CSV.
 */
export function exportChartDataToCSV(
  positions: Position[],
  selectedMetricIds: string[],
  deviceName: string,
): void {
  const metrics = selectedMetrics(selectedMetricIds);

  const headers = ["Time", ...metrics.map((m) => `${m.label} (${m.unit})`)];

  const rows = positions.map((pos, idx) => {
    const time = pos.fixTime;
    const values = metrics.map((m) => {
      const val = m.extract(pos, idx, positions);
      return val !== null ? String(val) : "";
    });
    return [time, ...values];
  });

  downloadCSV(
    headers,
    rows,
    `motus-charts-${deviceName}-${dateValue(new Date())}.csv`,
  );
}
