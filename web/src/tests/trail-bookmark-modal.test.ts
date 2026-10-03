import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/svelte";
import { tick } from "svelte";
import TrailBookmarkModal from "$lib/components/TrailBookmarkModal.svelte";
import type { TrailBookmark } from "$lib/types/api";

const range = {
  preset: "custom" as const,
  from: new Date(2026, 5, 6, 8, 0).toISOString(),
  to: new Date(2026, 5, 6, 16, 30).toISOString(),
};

const existing: TrailBookmark = {
  id: 3,
  deviceId: 9,
  deviceName: "Backpack",
  name: "Watzmann",
  description: "Ostwand",
  // Created through the API: seconds the form cannot show.
  from: new Date(2026, 6, 1, 5, 15, 20, 250).toISOString(),
  to: new Date(2026, 6, 2, 19, 0, 0, 0).toISOString(),
  createdAt: new Date(2026, 6, 3).toISOString(),
  updatedAt: new Date(2026, 6, 3).toISOString(),
};

function field(container: HTMLElement, id: string): HTMLInputElement {
  const el = container.ownerDocument.querySelector<HTMLInputElement>(`#${id}`);
  if (!el) throw new Error(`missing #${id}`);
  return el;
}

async function submit(container: HTMLElement) {
  const form = container.ownerDocument.querySelector<HTMLFormElement>("form.bookmark-form")!;
  await fireEvent.submit(form);
  await tick();
}

function renderModal(props: Record<string, unknown>) {
  return render(TrailBookmarkModal, {
    props: {
      open: true,
      bookmark: null,
      deviceId: 4,
      range,
      onSave: vi.fn().mockResolvedValue(undefined),
      onClose: vi.fn(),
      ...props,
    },
  });
}

describe("TrailBookmarkModal", () => {
  it("uses native date and time inputs", () => {
    const { container } = renderModal({});
    expect(field(container, "bookmark-from-date").type).toBe("date");
    expect(field(container, "bookmark-from-time").type).toBe("time");
    expect(field(container, "bookmark-to-date").type).toBe("date");
    expect(field(container, "bookmark-to-time").type).toBe("time");
  });

  it("prefills the current trail range when saving a new bookmark", () => {
    const { getByRole, container } = renderModal({});
    expect(getByRole("dialog").textContent).toContain("Save trail bookmark");
    expect(field(container, "bookmark-name").value).toBe("");
    expect(field(container, "bookmark-from-date").value).toBe("2026-06-06");
    expect(field(container, "bookmark-from-time").value).toBe("08:00");
    expect(field(container, "bookmark-to-date").value).toBe("2026-06-06");
    expect(field(container, "bookmark-to-time").value).toBe("16:30");
  });

  it("requires a name", async () => {
    const onSave = vi.fn();
    const { container } = renderModal({ onSave });
    await submit(container);
    expect(onSave).not.toHaveBeenCalled();
    expect(container.ownerDocument.querySelector(".bookmark-error")?.textContent).toContain(
      "Name is required",
    );
  });

  it("saves the applied range exactly when the fields are unchanged", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const { container } = renderModal({ onSave });
    await fireEvent.input(field(container, "bookmark-name"), { target: { value: "Zugspitze" } });
    const desc = container.ownerDocument.querySelector<HTMLTextAreaElement>("#bookmark-description")!;
    await fireEvent.input(desc, { target: { value: "Via Höllental" } });
    await submit(container);

    expect(onSave).toHaveBeenCalledWith({
      deviceId: 4,
      name: "Zugspitze",
      description: "Via Höllental",
      from: range.from,
      to: range.to,
    });
  });

  it("freezes a relative preset to the moment the dialog opened", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    try {
      const now = new Date(2026, 5, 6, 12, 0, 30, 500);
      vi.setSystemTime(now);
      const onSave = vi.fn().mockResolvedValue(undefined);
      const { container } = renderModal({ range: { preset: "24h" }, onSave });
      await fireEvent.input(field(container, "bookmark-name"), { target: { value: "Today" } });
      await submit(container);
      expect(onSave).toHaveBeenCalledWith(
        expect.objectContaining({
          from: new Date(now.getTime() - 24 * 3600_000).toISOString(),
          to: now.toISOString(),
        }),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("edits an existing bookmark", async () => {
    const { getByRole, container } = renderModal({ bookmark: existing });
    expect(getByRole("dialog").textContent).toContain("Edit trail bookmark");
    expect(field(container, "bookmark-name").value).toBe("Watzmann");
    expect(field(container, "bookmark-from-date").value).toBe("2026-07-01");
    expect(field(container, "bookmark-from-time").value).toBe("05:15");
    expect(field(container, "bookmark-to-date").value).toBe("2026-07-02");
    expect(field(container, "bookmark-to-time").value).toBe("19:00");
  });

  it("renaming keeps the original timestamps", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const { container } = renderModal({ bookmark: existing, onSave });
    await fireEvent.input(field(container, "bookmark-name"), { target: { value: "Watzmann II" } });
    await submit(container);
    expect(onSave).toHaveBeenCalledWith({
      deviceId: 9,
      name: "Watzmann II",
      description: "Ostwand",
      from: existing.from,
      to: existing.to,
    });
  });

  it("editing one boundary reparses only that one", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const { container } = renderModal({ bookmark: existing, onSave });
    await fireEvent.input(field(container, "bookmark-to-time"), { target: { value: "" } });
    await submit(container);
    expect(onSave).toHaveBeenCalledWith({
      deviceId: 9,
      name: "Watzmann",
      description: "Ostwand",
      from: existing.from,
      to: new Date(2026, 6, 2, 23, 59, 59, 999).toISOString(),
    });
  });

  it("shows a character counter and has no UTF-16 maxlength", async () => {
    const { container } = renderModal({});
    const name = field(container, "bookmark-name");
    expect(name.hasAttribute("maxlength")).toBe(false);
    await fireEvent.input(name, { target: { value: "🥾🥾" } });
    expect(container.ownerDocument.querySelector("#bookmark-name-count")?.textContent).toContain(
      "2/200",
    );
  });

  it("shows a save error and stays open", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("boom"));
    const onClose = vi.fn();
    const { container } = renderModal({ bookmark: existing, onSave, onClose });
    await submit(container);
    await tick();
    expect(container.ownerDocument.querySelector(".bookmark-error")?.textContent).toContain(
      "boom",
    );
    expect(onClose).not.toHaveBeenCalled();
  });
});
