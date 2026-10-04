import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import { readable } from "svelte/store";

vi.mock("$app/stores", () => ({
  page: readable({ url: new URL("http://localhost/notifications/history?rule=42") }),
}));

vi.mock("$lib/api/client", () => ({
  // Admins with "All users" see other users' rules only through fetchNotifications.
  fetchNotifications: vi.fn().mockResolvedValue([
    { id: 42, name: "Other user's rule", eventTypes: ["deviceOnline"], channel: "webhook" },
  ]),
  api: {
    getNotifications: vi.fn().mockResolvedValue([]),
    // API shape: sentAt only, null while queued (no createdAt).
    getNotificationLogs: vi.fn().mockResolvedValue([
      { id: 1, ruleId: 42, status: "queued", sentAt: null },
      { id: 2, ruleId: 42, status: "sent", sentAt: "2026-10-01T10:00:00Z" },
    ]),
  },
}));

import { formatDate } from "$lib/utils/formatting";
import HistoryPage from "../routes/notifications/history/+page.svelte";

describe("notification history page", () => {
  it("filters by a ?rule= link to a rule loaded with the All users scope", async () => {
    render(HistoryPage);

    await waitFor(() => expect(screen.getByText("Showing 2 entries")).toBeInTheDocument());
    const select = screen.getByLabelText("Rule:") as HTMLSelectElement;
    expect(select.value).toBe("42");
    expect(select.selectedOptions[0].textContent).toBe("Other user's rule");
  });

  it("shows sentAt and a dash for queued logs, queued (newest) first", async () => {
    render(HistoryPage);

    await waitFor(() => expect(screen.getByText("Showing 2 entries")).toBeInTheDocument());
    const times = [...document.querySelectorAll("td.cell-time")].map((td) => td.textContent?.trim());
    expect(times).toEqual(["-", formatDate("2026-10-01T10:00:00Z")]);
  });
});
