import { Chart, registerables } from "chart.js";
import { isDark } from "$lib/stores/theme";
import { chartColors } from "$lib/utils/chart-metrics";

Chart.register(...registerables);

isDark.subscribe((dark) => {
  const c = chartColors(dark);
  Chart.defaults.color = c.tick;
  Chart.defaults.borderColor = c.grid;
  Object.assign(Chart.defaults.plugins.tooltip, {
    backgroundColor: c.tooltipBg,
    titleColor: c.tooltipText,
    bodyColor: c.tooltipText,
    borderColor: c.tooltipBorder,
    borderWidth: 1,
  });
});

export { Chart };
