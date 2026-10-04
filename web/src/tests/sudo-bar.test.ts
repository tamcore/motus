import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";

const mocks = vi.hoisted(() => ({ getSudoStatus: vi.fn(), endSudo: vi.fn() }));

vi.mock("$lib/api/client", () => ({
  api: { getSudoStatus: mocks.getSudoStatus, endSudo: mocks.endSudo },
}));
vi.mock("$lib/stores/auth", () => ({ currentUserName: writable("Target User") }));

import SudoBar from "$lib/components/SudoBar.svelte";

describe("SudoBar", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows the bar when the API reports an active sudo session", async () => {
    mocks.getSudoStatus.mockResolvedValue({ active: true, originalUserId: 1, targetUserId: 2 });

    render(SudoBar);

    await waitFor(() => expect(screen.getByText("SUDO MODE")).toBeTruthy());
    expect(screen.getByText("Target User")).toBeTruthy();
  });

  it("stays hidden when sudo is not active", async () => {
    mocks.getSudoStatus.mockResolvedValue({ active: false });

    render(SudoBar);

    await waitFor(() => expect(mocks.getSudoStatus).toHaveBeenCalled());
    expect(screen.queryByText("SUDO MODE")).toBeNull();
  });
});
