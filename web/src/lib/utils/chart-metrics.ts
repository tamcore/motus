import type { Position } from "$lib/types/api";
import { haversineDistance } from "$lib/utils/trips";

/**
 * Metric definitions for device analytics charts.
 *
 * Each metric describes how to extract a value from a Position,
 * what unit it uses, and which Y-axis it should bind to.
 */

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

/**
 * Look up a metric by its id.
 */
export function getMetricById(id: string): MetricDefinition | undefined {
  return METRICS.find((m) => m.id === id);
}

/**
 * Check whether a metric has meaningful (non-null, non-zero) data in the given positions.
 * Returns false for unknown metric ids or empty positions.
 */
export function hasMetricData(
  positions: Position[],
  metricId: string,
): boolean {
  if (positions.length === 0) return false;
  const metric = getMetricById(metricId);
  if (!metric) return false;
  return positions.some((pos, idx) => {
    const val = metric.extract(pos, idx, positions);
    return val !== null && val !== 0;
  });
}

/**
 * Filter METRICS to only those with meaningful data in the given positions.
 */
export function getAvailableMetrics(
  positions: Position[],
): MetricDefinition[] {
  if (positions.length === 0) return [];
  return METRICS.filter((m) => hasMetricData(positions, m.id));
}

/**
 * Build Chart.js dataset objects for the selected metrics and positions.
 */
export function buildDatasets(
  positions: Position[],
  selectedMetricIds: string[],
): { labels: string[]; datasets: ChartDataset[] } {
  const labels = positions.map((p) => p.fixTime);

  const datasets: ChartDataset[] = [];

  for (const metricId of selectedMetricIds) {
    const metric = getMetricById(metricId);
    if (!metric) continue;

    const data = positions.map((pos, idx) =>
      metric.extract(pos, idx, positions),
    );

    datasets.push({
      label: `${metric.label} (${metric.unit})`,
      data,
      borderColor: metric.color,
      backgroundColor: hexToRgba(metric.color, 0.1),
      yAxisID: metric.axisId,
      tension: 0.3,
      pointRadius: positions.length > 200 ? 0 : 2,
      pointHoverRadius: 4,
      borderWidth: 2,
      fill: false,
    });
  }

  return { labels, datasets };
}

export interface ChartDataset {
  label: string;
  data: (number | null)[];
  borderColor: string;
  backgroundColor: string;
  yAxisID: string;
  tension: number;
  pointRadius: number;
  pointHoverRadius: number;
  borderWidth: number;
  fill: boolean;
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

  // Collect unique axes needed
  const seenAxes = new Set<string>();
  const axisMetrics: { axisId: string; unit: string; label: string }[] = [];

  for (const metricId of selectedMetricIds) {
    const metric = getMetricById(metricId);
    if (!metric || seenAxes.has(metric.axisId)) continue;
    seenAxes.add(metric.axisId);
    axisMetrics.push({
      axisId: metric.axisId,
      unit: metric.unit,
      label: metric.label,
    });
  }

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
 * Convert hex color to rgba string.
 */
function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

/**
 * Export chart data to CSV.
 */
export function exportChartDataToCSV(
  positions: Position[],
  selectedMetricIds: string[],
  deviceName: string,
): void {
  const metrics = selectedMetricIds
    .map(getMetricById)
    .filter((m): m is MetricDefinition => m !== undefined);

  const headers = ["Time", ...metrics.map((m) => `${m.label} (${m.unit})`)];

  const rows = positions.map((pos, idx) => {
    const time = pos.fixTime;
    const values = metrics.map((m) => {
      const val = m.extract(pos, idx, positions);
      return val !== null ? String(val) : "";
    });
    return [time, ...values];
  });

  const csv = [headers.join(","), ...rows.map((row) => row.join(","))].join(
    "\n",
  );

  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `motus-charts-${deviceName}-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}
