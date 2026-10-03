import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import TrailBookmarkList from "$lib/components/TrailBookmarkList.svelte";
import type { TrailBookmark } from "$lib/types/api";
import { bookmarkRangeLabel } from "$lib/utils/trail-bookmarks";

function bookmark(id: number, name: string, day: number): TrailBookmark {
  return {
    id,
    deviceId: 4,
    deviceName: "Backpack",
    name,
    description: `notes ${name}`,
    from: new Date(2026, 5, day, 8, 0).toISOString(),
    to: new Date(2026, 5, day, 16, 30).toISOString(),
    createdAt: new Date(2026, 5, day).toISOString(),
    updatedAt: new Date(2026, 5, day).toISOString(),
  };
}

const list = [bookmark(1, "Zugspitze", 6), bookmark(2, "Watzmann", 2)];

function props(overrides: Record<string, unknown> = {}) {
  return {
    bookmarks: list,
    activeRange: { preset: "24h" as const },
    loading: false,
    error: "",
    onOpen: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
}

describe("TrailBookmarkList", () => {
  it("lists bookmarks with name and date range", () => {
    const { container } = render(TrailBookmarkList, { props: props() });
    const items = container.querySelectorAll(".trail-bookmark-item");
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain("Zugspitze");
    expect(items[0].textContent).toContain(bookmarkRangeLabel(list[0]));
    expect(container.querySelector('a[href="/bookmarks"]')).not.toBeNull();
  });

  it("opens, edits and deletes bookmarks via callbacks", async () => {
    const p = props();
    const { getByRole } = render(TrailBookmarkList, { props: p });
    await fireEvent.click(getByRole("button", { name: /Show bookmark Zugspitze/ }));
    expect(p.onOpen).toHaveBeenCalledWith(list[0]);
    await fireEvent.click(getByRole("button", { name: "Edit bookmark Watzmann" }));
    expect(p.onEdit).toHaveBeenCalledWith(list[1]);
    await fireEvent.click(getByRole("button", { name: "Delete bookmark Zugspitze" }));
    expect(p.onDelete).toHaveBeenCalledWith(list[0]);
  });

  it("highlights the bookmark matching the applied range", () => {
    const { container } = render(TrailBookmarkList, {
      props: props({ activeRange: { preset: "custom", from: list[1].from, to: list[1].to } }),
    });
    const active = container.querySelectorAll(".trail-bookmark-item.active");
    expect(active).toHaveLength(1);
    expect(active[0].textContent).toContain("Watzmann");
  });

  it("shows an empty hint", () => {
    const { container } = render(TrailBookmarkList, { props: props({ bookmarks: [] }) });
    expect(container.querySelector(".trail-bookmarks-empty")?.textContent).toContain(
      "No bookmarks",
    );
  });

  it("shows load errors", () => {
    const { container } = render(TrailBookmarkList, {
      props: props({ bookmarks: [], error: "Failed to load bookmarks" }),
    });
    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "Failed to load bookmarks",
    );
  });
});
