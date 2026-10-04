import { describe, it, expect, vi } from "vitest";
import { render, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";

const mocks = vi.hoisted(() => ({ getUsers: vi.fn() }));

vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$lib/api/client", () => ({ api: { getUsers: mocks.getUsers } }));
vi.mock("$lib/stores/auth", () => ({
  isAdmin: writable(true),
  currentUser: writable({ id: 1, name: "Admin", email: "admin@example.com" }),
}));

import UsersPage from "../routes/admin/users/+page.svelte";

const user = (id: number, administrator: boolean, readonly: boolean) => ({
  id,
  email: `u${id}@example.com`,
  name: `User ${id}`,
  administrator,
  readonly,
  disabled: false,
  createdAt: "2026-01-01T00:00:00Z",
});

describe("admin users page", () => {
  it("counts roles from the administrator/readonly flags", async () => {
    mocks.getUsers.mockResolvedValue([
      user(1, true, false),
      user(2, false, false),
      user(3, false, false),
      user(4, false, true),
    ]);

    render(UsersPage);

    await waitFor(() => expect(document.querySelector(".role-distribution")).toBeTruthy());
    const counts = Object.fromEntries(
      [...document.querySelectorAll(".role-distribution .role-stat")].map((el) => [
        el.querySelector(".role-badge")?.textContent?.trim(),
        el.querySelector(".role-count")?.textContent?.trim(),
      ]),
    );
    expect(counts).toEqual({ Admin: "1", User: "2", "Read Only": "1" });
  });
});
