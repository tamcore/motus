import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockGetApiKeys, mockCreateApiKey } = vi.hoisted(() => ({
  mockGetApiKeys: vi.fn(),
  mockCreateApiKey: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  api: {
    getApiKeys: mockGetApiKeys,
    createApiKey: mockCreateApiKey,
    deleteApiKey: vi.fn(),
  },
  APIError: class APIError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

import { render, screen, waitFor, fireEvent } from "@testing-library/svelte";
import type { ApiKey, CreateApiKeyPayload } from "$lib/types/api";
import ApiKeyManager from "$lib/components/ApiKeyManager.svelte";

const createdKey: ApiKey = {
  id: 1,
  userId: 1,
  token: "mts_full_token_value",
  name: "HA",
  permissions: "full",
  expiresAt: null,
  createdAt: "2026-02-15T10:00:00Z",
  lastUsedAt: null,
};

describe("ApiKeyManager create modal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  async function createViaModal(expiration: string) {
    mockGetApiKeys.mockResolvedValue([]);
    mockCreateApiKey.mockResolvedValue(createdKey);
    render(ApiKeyManager, { props: { showUsageInstructions: false } });
    await fireEvent.click(await screen.findByRole("button", { name: "Create API Key" }));
    await fireEvent.input(screen.getByLabelText(/Key Name/), { target: { value: "HA" } });
    await fireEvent.change(screen.getByLabelText("Expiration"), { target: { value: expiration } });
    await fireEvent.submit(document.querySelector("form.create-form")!);
    await waitFor(() => expect(mockCreateApiKey).toHaveBeenCalledOnce());
    return mockCreateApiKey.mock.calls[0][0] as CreateApiKeyPayload;
  }

  it("sends a preset as an expiresAt timestamp", async () => {
    const before = Date.now();
    const payload = await createViaModal("24");

    const ms = Date.parse(payload.expiresAt!);
    expect(ms).toBeGreaterThanOrEqual(before + 24 * 3_600_000);
    expect(ms).toBeLessThanOrEqual(Date.now() + 24 * 3_600_000);
    expect(payload).not.toHaveProperty("expiresInHours");
  });

  it("sends no expiration for the 'never' option", async () => {
    const payload = await createViaModal("never");

    expect(payload).toEqual({ name: "HA", permissions: "full" });
  });
});
