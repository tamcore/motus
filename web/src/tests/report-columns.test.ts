import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { readable, writable } from "svelte/store";

vi.mock("$app/stores", () => ({
  page: readable({ url: new URL("http://localhost/reports") }),
}));
vi.mock("$lib/stores/theme", () => ({ isDark: writable(false) }));
vi.mock("$lib/api/client", () => ({
  api: {
    getActivityReport: vi.fn().mockResolvedValue({
      trips: [
        {
          id: "t1",
          deviceId: 1,
          deviceName: "Car",
          startTime: "2026-10-01T08:00:00Z",
          endTime: "2026-10-01T08:30:00Z",
          duration: 1800,
          distance: 12,
          avgSpeed: 24,
          maxSpeed: 50,
        },
      ],
      stops: [],
    }),
  },
  fetchDevices: vi.fn().mockResolvedValue([{ id: 1, name: "Car" }]),
}));

import ReportsPage from "../routes/reports/+page.svelte";

const KEY = "motus_report_columns";

describe("reports column config", () => {
  beforeEach(() => localStorage.clear());

  it("reads stored columns and persists changes", async () => {
    localStorage.setItem(KEY, JSON.stringify({ device: false }));
    render(ReportsPage);

    await screen.findByRole("button", { name: "Configure columns" });
    const device = screen.getByRole("checkbox", { name: "Device", hidden: true }) as HTMLInputElement;
    const distance = screen.getByRole("checkbox", { name: "Distance", hidden: true }) as HTMLInputElement;
    expect(device.checked).toBe(false);
    expect(distance.checked).toBe(true);

    await fireEvent.click(distance);
    await waitFor(() =>
      expect(JSON.parse(localStorage.getItem(KEY)!)).toMatchObject({ device: false, distance: false }),
    );
  });
});
