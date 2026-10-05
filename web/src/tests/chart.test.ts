import { describe, it, expect, vi } from "vitest";

const dark = await vi.hoisted(async () => (await import("svelte/store")).writable(true));
vi.mock("$lib/stores/theme", () => ({ isDark: dark }));

import { Chart } from "$lib/utils/chart";

describe("chart defaults", () => {
  it("follows the theme at runtime", () => {
    expect(Chart.defaults.color).toBe("#a0a0a0");
    expect(Chart.defaults.borderColor).toBe("#3a3a3a");
    expect(Chart.defaults.plugins.tooltip.backgroundColor).toBe("#2d2d2d");

    dark.set(false);
    expect(Chart.defaults.color).toBe("#666666");
    expect(Chart.defaults.borderColor).toBe("#e0e0e0");
    expect(Chart.defaults.plugins.tooltip.backgroundColor).toBe("#ffffff");
  });
});
