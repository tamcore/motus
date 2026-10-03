import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import { readable, writable } from "svelte/store";

vi.mock("$app/stores", () => ({
  page: readable({ url: new URL("http://localhost/notifications/history?rule=42") }),
}));
vi.mock("$lib/stores/refresh", () => ({ refreshHandler: writable(null) }));

vi.mock("$lib/api/client", () => ({
  // Admins with "All users" see other users' rules only through fetchNotifications.
  fetchNotifications: vi.fn().mockResolvedValue([
    { id: 42, name: "Other user's rule", eventTypes: ["deviceOnline"], channel: "webhook" },
  ]),
  api: {
    getNotifications: vi.fn().mockResolvedValue([]),
    getNotificationLogs: vi.fn().mockResolvedValue([
      { id: 1, ruleId: 42, status: "sent", createdAt: "2026-10-01T10:00:00Z" },
    ]),
  },
}));

import HistoryPage from "../routes/notifications/history/+page.svelte";

describe("notification history page", () => {
  it("filters by a ?rule= link to a rule loaded with the All users scope", async () => {
    render(HistoryPage);

    await waitFor(() => expect(screen.getByText("Showing 1 entry")).toBeInTheDocument());
    const select = screen.getByLabelText("Rule:") as HTMLSelectElement;
    expect(select.value).toBe("42");
    expect(select.selectedOptions[0].textContent).toBe("Other user's rule");
  });
});
