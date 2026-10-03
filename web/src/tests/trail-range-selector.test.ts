import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import TrailRangeSelector from "$lib/components/TrailRangeSelector.svelte";

function select(container: HTMLElement): HTMLSelectElement {
  return container.querySelector<HTMLSelectElement>("select.trail-range-select")!;
}

function input(container: HTMLElement, id: string): HTMLInputElement {
  return container.querySelector<HTMLInputElement>(`#${id}`)!;
}

async function openCustom(onChange = vi.fn()) {
  const result = render(TrailRangeSelector, {
    props: { range: { preset: "24h" }, onChange },
  });
  await fireEvent.change(select(result.container), { target: { value: "custom" } });
  await tick();
  return { ...result, onChange };
}

describe("TrailRangeSelector", () => {
  it("shows all presets with the current selection", () => {
    const { container } = render(TrailRangeSelector, {
      props: { range: { preset: "7d" }, onChange: vi.fn() },
    });
    const options = Array.from(select(container).options).map((o) => o.textContent?.trim());
    expect(options).toEqual([
      "Last 24h",
      "Last 48h",
      "Last 7 days",
      "Last 30 days",
      "All time",
      "Custom range",
    ]);
    expect(select(container).value).toBe("7d");
    expect(container.querySelector(".trail-custom-range")).toBeNull();
  });

  it("emits a preset immediately when chosen", async () => {
    const onChange = vi.fn();
    const { container } = render(TrailRangeSelector, {
      props: { range: { preset: "24h" }, onChange },
    });
    await fireEvent.change(select(container), { target: { value: "30d" } });
    expect(onChange).toHaveBeenCalledWith({ preset: "30d" });
  });

  it("reveals native date and optional time inputs for custom without emitting", async () => {
    const { container, onChange } = await openCustom();
    expect(input(container, "trail-from-date").type).toBe("date");
    expect(input(container, "trail-from-time").type).toBe("time");
    expect(input(container, "trail-to-date").type).toBe("date");
    expect(input(container, "trail-to-time").type).toBe("time");
    // Prefilled with the dates of the previous range; times left empty (whole days).
    expect(input(container, "trail-from-date").value).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(input(container, "trail-from-time").value).toBe("");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("applies whole days when no time is given", async () => {
    const { container, onChange } = await openCustom();
    await fireEvent.input(input(container, "trail-from-date"), {
      target: { value: "2026-09-01" },
    });
    await fireEvent.input(input(container, "trail-to-date"), {
      target: { value: "2026-09-02" },
    });
    await fireEvent.click(container.querySelector("button.trail-apply")!);
    expect(onChange).toHaveBeenCalledWith({
      preset: "custom",
      from: new Date(2026, 8, 1, 0, 0, 0).toISOString(),
      to: new Date(2026, 8, 2, 23, 59, 59).toISOString(),
    });
  });

  it("applies optional times", async () => {
    const { container, onChange } = await openCustom();
    await fireEvent.input(input(container, "trail-from-date"), {
      target: { value: "2026-09-01" },
    });
    await fireEvent.input(input(container, "trail-from-time"), {
      target: { value: "06:30" },
    });
    await fireEvent.input(input(container, "trail-to-date"), {
      target: { value: "2026-09-01" },
    });
    await fireEvent.input(input(container, "trail-to-time"), {
      target: { value: "18:00" },
    });
    await fireEvent.click(container.querySelector("button.trail-apply")!);
    expect(onChange).toHaveBeenCalledWith({
      preset: "custom",
      from: new Date(2026, 8, 1, 6, 30, 0).toISOString(),
      to: new Date(2026, 8, 1, 18, 0, 59).toISOString(),
    });
  });

  it("shows an error when the start is after the end and does not emit", async () => {
    const { container, onChange } = await openCustom();
    await fireEvent.input(input(container, "trail-from-date"), {
      target: { value: "2026-09-03" },
    });
    await fireEvent.input(input(container, "trail-to-date"), {
      target: { value: "2026-09-02" },
    });
    await fireEvent.click(container.querySelector("button.trail-apply")!);
    await tick();
    expect(onChange).not.toHaveBeenCalled();
    expect(container.querySelector(".trail-range-error")?.textContent).toMatch(/before/i);
  });

  it("prefills the inputs from an existing custom range", () => {
    const from = new Date(2026, 8, 1, 6, 30);
    const to = new Date(2026, 8, 2, 18, 0);
    const { container } = render(TrailRangeSelector, {
      props: {
        range: { preset: "custom", from: from.toISOString(), to: to.toISOString() },
        onChange: vi.fn(),
      },
    });
    expect(select(container).value).toBe("custom");
    expect(input(container, "trail-from-date").value).toBe("2026-09-01");
    expect(input(container, "trail-from-time").value).toBe("06:30");
    expect(input(container, "trail-to-date").value).toBe("2026-09-02");
    expect(input(container, "trail-to-time").value).toBe("18:00");
  });

  it("leaves times empty for a whole-day custom range", () => {
    const { container } = render(TrailRangeSelector, {
      props: {
        range: {
          preset: "custom",
          from: new Date(2026, 8, 1, 0, 0, 0).toISOString(),
          to: new Date(2026, 8, 2, 23, 59, 59).toISOString(),
        },
        onChange: vi.fn(),
      },
    });
    expect(input(container, "trail-from-time").value).toBe("");
    expect(input(container, "trail-to-time").value).toBe("");
  });
});
