import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";

const mocks = vi.hoisted(() => ({
  createDeviceShare: vi.fn(),
  listDeviceShares: vi.fn(),
  deleteShare: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({ api: mocks }));

import ShareModal from "$lib/components/ShareModal.svelte";

const HOUR_MS = 3_600_000;

async function generate(expiry: string) {
  render(ShareModal, { props: { deviceId: 7, deviceName: "Car", open: true } });
  await fireEvent.change(screen.getByLabelText("Expiry"), { target: { value: expiry } });
  await fireEvent.click(screen.getByRole("button", { name: "Generate Link" }));
  await waitFor(() => expect(mocks.createDeviceShare).toHaveBeenCalled());
}

describe("ShareModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.listDeviceShares.mockResolvedValue([]);
    mocks.createDeviceShare.mockResolvedValue({
      id: 1,
      deviceId: 7,
      token: "tok123",
      createdBy: 1,
      expiresAt: null,
      createdAt: "2026-10-04T00:00:00Z",
    });
  });

  it("sends an expiresAt timestamp for an expiry preset", async () => {
    const before = Date.now();
    await generate("1h");

    const [deviceId, expiresAt] = mocks.createDeviceShare.mock.calls[0];
    expect(deviceId).toBe(7);
    const ms = Date.parse(expiresAt);
    expect(ms).toBeGreaterThanOrEqual(before + HOUR_MS);
    expect(ms).toBeLessThanOrEqual(Date.now() + HOUR_MS);
  });

  it("sends no expiry for 'Never'", async () => {
    await generate("never");

    expect(mocks.createDeviceShare).toHaveBeenCalledWith(7, null);
  });

  it("builds the share link from the token", async () => {
    await generate("24h");

    await waitFor(() =>
      expect((document.querySelector(".share-link-input") as HTMLInputElement).value).toBe(
        `${window.location.origin}/share/tok123`,
      ),
    );
  });
});
