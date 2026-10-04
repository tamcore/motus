import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";

const mocks = vi.hoisted(() => ({ getAuditLog: vi.fn() }));

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$lib/api/client", () => ({ api: { getAuditLog: mocks.getAuditLog } }));
vi.mock("$lib/stores/auth", () => ({ isAdmin: writable(true) }));
vi.mock("$lib/stores/refresh", () => ({ refreshHandler: writable(null) }));

import { formatDate } from "$lib/utils/formatting";
import AuditPage from "../routes/admin/audit/+page.svelte";

describe("admin audit page", () => {
  it("renders entries in the API shape (createdAt, metadata)", async () => {
    const createdAt = "2026-10-03T12:34:56Z";
    mocks.getAuditLog.mockResolvedValue({
      entries: [
        {
          id: 1,
          action: "device.create",
          userId: 7,
          userEmail: "someone@example.com",
          resourceType: "device",
          resourceId: "5",
          metadata: { name: "Car" },
          ipAddress: "192.0.2.1",
          createdAt,
        },
      ],
      total: 1,
    });

    render(AuditPage);

    await waitFor(() => expect(screen.getByText(formatDate(createdAt))).toBeTruthy());
    expect(screen.getByText("#5")).toBeTruthy();
    expect(document.body.textContent).not.toContain("Invalid Date");
    expect(screen.getByRole("button", { name: /show/i })).toBeTruthy();
  });
});
