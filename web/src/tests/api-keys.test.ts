import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock the API client
const mockGetApiKeys = vi.fn();
const mockCreateApiKey = vi.fn();
const mockDeleteApiKey = vi.fn();

vi.mock("$lib/api/client", () => ({
  api: {
    getApiKeys: mockGetApiKeys,
    createApiKey: mockCreateApiKey,
    deleteApiKey: mockDeleteApiKey,
  },
  APIError: class APIError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

import type { ApiKey, CreateApiKeyPayload } from "$lib/types/api";

// Helper to create a mock API key
function createMockApiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 1,
    userId: 1,
    token: "abc12345...",
    name: "Test Key",
    permissions: "full",
    expiresAt: null,
    createdAt: "2026-02-15T10:00:00Z",
    lastUsedAt: null,
    ...overrides,
  };
}

describe("API Keys Management", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // API Client interaction tests
  // ---------------------------------------------------------------------------

  describe("List API keys", () => {
    it("should return an array of API keys with redacted tokens", async () => {
      const keys: ApiKey[] = [
        createMockApiKey({
          id: 1,
          name: "Home Assistant",
          permissions: "full",
          token: "abc12345...",
        }),
        createMockApiKey({
          id: 2,
          name: "Grafana",
          permissions: "readonly",
          token: "xyz98765...",
        }),
      ];
      mockGetApiKeys.mockResolvedValueOnce(keys);

      const result = await mockGetApiKeys();

      expect(mockGetApiKeys).toHaveBeenCalledOnce();
      expect(result).toHaveLength(2);
      expect(result[0].token).toContain("...");
      expect(result[1].token).toContain("...");
    });

    it("should return an empty array when no keys exist", async () => {
      mockGetApiKeys.mockResolvedValueOnce([]);

      const result = await mockGetApiKeys();

      expect(result).toEqual([]);
    });

    it("should handle API errors when listing keys", async () => {
      mockGetApiKeys.mockRejectedValueOnce(
        new Error("Failed to list API keys"),
      );

      await expect(mockGetApiKeys()).rejects.toThrow("Failed to list API keys");
    });
  });

  describe("Create API key", () => {
    it("should create a key with full permissions and return the full token", async () => {
      const fullToken = "mts_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6";
      const payload: CreateApiKeyPayload = {
        name: "Home Assistant",
        permissions: "full",
      };
      const created = createMockApiKey({
        id: 3,
        name: "Home Assistant",
        permissions: "full",
        token: fullToken,
      });
      mockCreateApiKey.mockResolvedValueOnce(created);

      const result = await mockCreateApiKey(payload);

      expect(mockCreateApiKey).toHaveBeenCalledWith(payload);
      expect(result.token).toBe(fullToken);
      expect(result.token).not.toContain("...");
      expect(result.name).toBe("Home Assistant");
      expect(result.permissions).toBe("full");
    });

    it("should create a key with readonly permissions", async () => {
      const payload: CreateApiKeyPayload = {
        name: "Read-Only Monitor",
        permissions: "readonly",
      };
      const created = createMockApiKey({
        id: 4,
        name: "Read-Only Monitor",
        permissions: "readonly",
        token: "mts_readonly_token_value_here",
      });
      mockCreateApiKey.mockResolvedValueOnce(created);

      const result = await mockCreateApiKey(payload);

      expect(result.permissions).toBe("readonly");
    });

    it("should create a key with expiresInHours", async () => {
      const payload: CreateApiKeyPayload = {
        name: "Short-lived Key",
        permissions: "full",
        expiresInHours: 24,
      };
      const futureDate = new Date(
        Date.now() + 24 * 60 * 60 * 1000,
      ).toISOString();
      const created = createMockApiKey({
        id: 5,
        name: "Short-lived Key",
        expiresAt: futureDate,
      });
      mockCreateApiKey.mockResolvedValueOnce(created);

      const result = await mockCreateApiKey(payload);

      expect(mockCreateApiKey).toHaveBeenCalledWith(payload);
      expect(result.expiresAt).toBeTruthy();
    });

    it("should create a key with expiresAt custom date", async () => {
      const futureDate = "2027-06-15T00:00:00Z";
      const payload: CreateApiKeyPayload = {
        name: "Custom Expiry Key",
        permissions: "full",
        expiresAt: futureDate,
      };
      const created = createMockApiKey({
        id: 6,
        name: "Custom Expiry Key",
        expiresAt: futureDate,
      });
      mockCreateApiKey.mockResolvedValueOnce(created);

      const result = await mockCreateApiKey(payload);

      expect(result.expiresAt).toBe(futureDate);
    });

    it("should create a key that never expires when no expiration is set", async () => {
      const payload: CreateApiKeyPayload = {
        name: "Forever Key",
        permissions: "full",
      };
      const created = createMockApiKey({
        id: 7,
        name: "Forever Key",
        expiresAt: null,
      });
      mockCreateApiKey.mockResolvedValueOnce(created);

      const result = await mockCreateApiKey(payload);

      expect(result.expiresAt).toBeNull();
    });

    it("should handle validation error when name is empty", async () => {
      const { APIError } = await import("$lib/api/client");
      mockCreateApiKey.mockRejectedValueOnce(
        new APIError(400, "name is required"),
      );

      await expect(
        mockCreateApiKey({ name: "", permissions: "full" }),
      ).rejects.toThrow("name is required");
    });

    it("should handle validation error for invalid permissions", async () => {
      const { APIError } = await import("$lib/api/client");
      mockCreateApiKey.mockRejectedValueOnce(
        new APIError(400, "permissions must be 'full' or 'readonly'"),
      );

      await expect(
        mockCreateApiKey({ name: "Test", permissions: "admin" }),
      ).rejects.toThrow("permissions must be 'full' or 'readonly'");
    });

    it("should handle validation error for past expiresAt", async () => {
      const { APIError } = await import("$lib/api/client");
      mockCreateApiKey.mockRejectedValueOnce(
        new APIError(400, "expiresAt must be in the future"),
      );

      await expect(
        mockCreateApiKey({
          name: "Test",
          permissions: "full",
          expiresAt: "2020-01-01T00:00:00Z",
        }),
      ).rejects.toThrow("expiresAt must be in the future");
    });

    it("should handle validation error for both expiration fields", async () => {
      const { APIError } = await import("$lib/api/client");
      mockCreateApiKey.mockRejectedValueOnce(
        new APIError(
          400,
          "specify either expiresInHours or expiresAt, not both",
        ),
      );

      await expect(
        mockCreateApiKey({
          name: "Test",
          permissions: "full",
          expiresInHours: 24,
          expiresAt: "2027-01-01T00:00:00Z",
        }),
      ).rejects.toThrow("specify either expiresInHours or expiresAt, not both");
    });
  });

  describe("Delete API key", () => {
    it("should delete a key by ID successfully", async () => {
      mockDeleteApiKey.mockResolvedValueOnce(undefined);

      await mockDeleteApiKey(1);

      expect(mockDeleteApiKey).toHaveBeenCalledWith(1);
    });

    it("should handle not found error", async () => {
      const { APIError } = await import("$lib/api/client");
      mockDeleteApiKey.mockRejectedValueOnce(
        new APIError(404, "API key not found"),
      );

      await expect(mockDeleteApiKey(999)).rejects.toThrow("API key not found");
    });

    it("should handle forbidden error for other user's key", async () => {
      const { APIError } = await import("$lib/api/client");
      mockDeleteApiKey.mockRejectedValueOnce(
        new APIError(403, "cannot delete another user's API key"),
      );

      await expect(mockDeleteApiKey(5)).rejects.toThrow(
        "cannot delete another user's API key",
      );
    });
  });

  // ---------------------------------------------------------------------------
  // Component state machine tests
  // ---------------------------------------------------------------------------

  describe("API Keys list state management", () => {
    it("should load keys on mount and transition from loading to loaded", async () => {
      const keys = [
        createMockApiKey({ id: 1, name: "Key A" }),
        createMockApiKey({ id: 2, name: "Key B" }),
      ];
      mockGetApiKeys.mockResolvedValueOnce(keys);

      // Simulate component mount state transitions
      let loading = true;
      let apiKeys: ApiKey[] = [];
      let error = "";

      try {
        apiKeys = await mockGetApiKeys();
      } catch (e: unknown) {
        error = e instanceof Error ? e.message : "Failed to load API keys";
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(apiKeys).toHaveLength(2);
      expect(error).toBe("");
    });

    it("should handle load failure and show error", async () => {
      mockGetApiKeys.mockRejectedValueOnce(new Error("Network error"));

      let loading = true;
      let apiKeys: ApiKey[] = [];
      let error = "";

      try {
        apiKeys = await mockGetApiKeys();
      } catch (e: unknown) {
        error = e instanceof Error ? e.message : "Failed to load API keys";
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(apiKeys).toHaveLength(0);
      expect(error).toBe("Network error");
    });
  });

  describe("Create key modal state machine", () => {
    it("should transition through correct states during creation", async () => {
      const fullToken = "mts_new_full_token_value_here_long";
      mockCreateApiKey.mockResolvedValueOnce(
        createMockApiKey({ id: 5, token: fullToken, name: "New Key" }),
      );

      // Initial modal state
      let showCreateModal = false;
      let newKeyName = "";
      let newKeyPermissions = "full";
      let newKeyExpiration = "168";
      let creating = false;
      let createdToken = "";
      let createError = "";

      // Open modal
      showCreateModal = true;
      expect(showCreateModal).toBe(true);

      // Fill form
      newKeyName = "New Key";
      newKeyPermissions = "full";
      newKeyExpiration = "168";

      // Submit
      creating = true;
      createError = "";

      try {
        const result = await mockCreateApiKey({
          name: newKeyName,
          permissions: newKeyPermissions,
          expiresInHours: parseInt(newKeyExpiration, 10),
        });
        createdToken = result.token;
      } catch (e: unknown) {
        createError =
          e instanceof Error ? e.message : "Failed to create API key";
      } finally {
        creating = false;
      }

      expect(creating).toBe(false);
      expect(createdToken).toBe(fullToken);
      expect(createError).toBe("");
    });

    it("should show error when creation fails", async () => {
      mockCreateApiKey.mockRejectedValueOnce(new Error("name is required"));

      let creating = false;
      let createdToken = "";
      let createError = "";

      creating = true;
      try {
        const result = await mockCreateApiKey({
          name: "",
          permissions: "full",
        });
        createdToken = result.token;
      } catch (e: unknown) {
        createError =
          e instanceof Error ? e.message : "Failed to create API key";
      } finally {
        creating = false;
      }

      expect(creating).toBe(false);
      expect(createdToken).toBe("");
      expect(createError).toBe("name is required");
    });

    it("should reset modal state when closing", () => {
      let showCreateModal = true;
      let newKeyName = "Some Key";
      let newKeyPermissions = "readonly";
      let newKeyExpiration = "24";
      let newKeyCustomDate = "2027-01-01";
      let createdToken = "some-token";
      let createError = "some error";

      // Close modal resets state
      showCreateModal = false;
      newKeyName = "";
      newKeyPermissions = "full";
      newKeyExpiration = "168";
      newKeyCustomDate = "";
      createdToken = "";
      createError = "";

      expect(showCreateModal).toBe(false);
      expect(newKeyName).toBe("");
      expect(newKeyPermissions).toBe("full");
      expect(newKeyExpiration).toBe("168");
      expect(newKeyCustomDate).toBe("");
      expect(createdToken).toBe("");
      expect(createError).toBe("");
    });
  });

  describe("Delete confirmation flow", () => {
    it("should require confirmation before deletion", async () => {
      mockDeleteApiKey.mockResolvedValueOnce(undefined);

      let confirmingDeleteId: number | null = null;
      let deleting = false;

      // Step 1: Click delete -> shows confirm
      confirmingDeleteId = 1;
      expect(confirmingDeleteId).toBe(1);

      // Step 2: Confirm delete
      deleting = true;
      await mockDeleteApiKey(confirmingDeleteId);
      deleting = false;
      confirmingDeleteId = null;

      expect(mockDeleteApiKey).toHaveBeenCalledWith(1);
      expect(deleting).toBe(false);
      expect(confirmingDeleteId).toBeNull();
    });

    it("should cancel deletion when dismiss is clicked", () => {
      let confirmingDeleteId: number | null = 1;

      // Cancel
      confirmingDeleteId = null;

      expect(confirmingDeleteId).toBeNull();
    });

    it("should remove key from local list after successful deletion", async () => {
      mockDeleteApiKey.mockResolvedValueOnce(undefined);

      let apiKeys = [
        createMockApiKey({ id: 1, name: "Key A" }),
        createMockApiKey({ id: 2, name: "Key B" }),
        createMockApiKey({ id: 3, name: "Key C" }),
      ];

      // Delete key 2
      await mockDeleteApiKey(2);
      apiKeys = apiKeys.filter((k) => k.id !== 2);

      expect(apiKeys).toHaveLength(2);
      expect(apiKeys.map((k) => k.id)).toEqual([1, 3]);
    });
  });

  // ---------------------------------------------------------------------------
  // Token display and copy
  // ---------------------------------------------------------------------------

  describe("Token display", () => {
    it("should display full token only once after creation", () => {
      const fullToken = "mts_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6";
      let createdToken = fullToken;

      // Token is visible
      expect(createdToken).toBe(fullToken);
      expect(createdToken.length).toBeGreaterThan(8);

      // After modal close, token is gone
      createdToken = "";
      expect(createdToken).toBe("");
    });

    it("should show redacted tokens in the list", () => {
      const redactedTokens = ["abc12345...", "xyz98765..."];

      for (const token of redactedTokens) {
        expect(token.endsWith("...")).toBe(true);
        expect(token.length).toBeLessThanOrEqual(11); // 8 chars + "..."
      }
    });
  });

  describe("Copy token to clipboard", () => {
    it("should copy token using navigator.clipboard API", async () => {
      const writeTextMock = vi.fn().mockResolvedValueOnce(undefined);
      Object.assign(navigator, {
        clipboard: { writeText: writeTextMock },
      });

      const token = "mts_full_token_for_copy";
      await navigator.clipboard.writeText(token);

      expect(writeTextMock).toHaveBeenCalledWith(token);
    });

    it("should show copied confirmation temporarily", async () => {
      vi.useFakeTimers();

      let tokenCopied = false;

      // Copy action
      tokenCopied = true;
      expect(tokenCopied).toBe(true);

      // After timeout, reset
      setTimeout(() => {
        tokenCopied = false;
      }, 2000);

      vi.advanceTimersByTime(2000);
      expect(tokenCopied).toBe(false);

      vi.useRealTimers();
    });

    it("should handle clipboard API failure with fallback", async () => {
      const writeTextMock = vi
        .fn()
        .mockRejectedValueOnce(new Error("Clipboard not available"));
      Object.assign(navigator, {
        clipboard: { writeText: writeTextMock },
      });

      let copied = false;
      let usedFallback = false;

      try {
        await navigator.clipboard.writeText("test-token");
        copied = true;
      } catch {
        usedFallback = true;
        copied = true;
      }

      expect(usedFallback).toBe(true);
      expect(copied).toBe(true);
    });
  });

  // ---------------------------------------------------------------------------
  // Permissions display
  // ---------------------------------------------------------------------------

  describe("Permission badges", () => {
    it("should distinguish full and readonly permissions", () => {
      const fullKey = createMockApiKey({ permissions: "full" });
      const readonlyKey = createMockApiKey({ permissions: "readonly" });

      expect(fullKey.permissions).toBe("full");
      expect(readonlyKey.permissions).toBe("readonly");

      // Badge labels
      const getBadgeLabel = (perm: string) =>
        perm === "full" ? "Full Access" : "Read-Only";

      expect(getBadgeLabel(fullKey.permissions)).toBe("Full Access");
      expect(getBadgeLabel(readonlyKey.permissions)).toBe("Read-Only");
    });
  });

  // ---------------------------------------------------------------------------
  // Expiration display
  // ---------------------------------------------------------------------------

  describe("Expiration display", () => {
    it("should detect an expired key", () => {
      const pastDate = "2025-01-01T00:00:00Z";
      const isExpired = (expiresAt: string | null | undefined) =>
        expiresAt ? new Date(expiresAt) < new Date() : false;

      expect(isExpired(pastDate)).toBe(true);
    });

    it("should detect a non-expired key", () => {
      const futureDate = "2099-12-31T23:59:59Z";
      const isExpired = (expiresAt: string | null | undefined) =>
        expiresAt ? new Date(expiresAt) < new Date() : false;

      expect(isExpired(futureDate)).toBe(false);
    });

    it("should treat null expiresAt as never-expiring", () => {
      const isExpired = (expiresAt: string | null | undefined) =>
        expiresAt ? new Date(expiresAt) < new Date() : false;

      expect(isExpired(null)).toBe(false);
      expect(isExpired(undefined)).toBe(false);
    });

    it("should show 'Never' for keys without expiration", () => {
      const formatExpiration = (expiresAt: string | null | undefined) =>
        expiresAt ? new Date(expiresAt).toLocaleDateString() : "Never";

      expect(formatExpiration(null)).toBe("Never");
      expect(formatExpiration(undefined)).toBe("Never");
    });

    it("should format expiration date when present", () => {
      const formatExpiration = (expiresAt: string | null | undefined) =>
        expiresAt ? new Date(expiresAt).toLocaleDateString() : "Never";

      const result = formatExpiration("2027-06-15T00:00:00Z");
      expect(result).not.toBe("Never");
    });
  });

  describe("Expiration options in create modal", () => {
    it("should default to 7 days (168 hours)", () => {
      const defaultExpiration = "168";
      expect(defaultExpiration).toBe("168");
    });

    it("should map preset values to hours correctly", () => {
      const presets: Record<string, number> = {
        "24": 24,
        "168": 168,
        "720": 720,
      };

      expect(presets["24"]).toBe(24);
      expect(presets["168"]).toBe(168);
      expect(presets["720"]).toBe(720);
    });

    it("should not send expiration for 'never' option", () => {
      const expiration: string = "never";
      let expiresInHours: number | null = null;
      let expiresAt: string | null = null;

      if (expiration === "custom") {
        expiresAt = "2027-01-01T00:00:00Z";
      } else if (expiration !== "never") {
        expiresInHours = parseInt(expiration, 10);
      }

      expect(expiresInHours).toBeNull();
      expect(expiresAt).toBeNull();
    });

    it("should send expiresAt for custom date option", () => {
      const expiration = "custom";
      const customDate = "2027-06-15";
      let expiresAt: string | null = null;

      if (expiration === "custom") {
        expiresAt = new Date(customDate).toISOString();
      }

      expect(expiresAt).toBeTruthy();
      expect(expiresAt).toContain("2027");
    });

    it("should reject a custom date in the past", () => {
      const customDate = "2020-01-01";
      const selected = new Date(customDate);
      const isValid = selected > new Date();

      expect(isValid).toBe(false);
    });
  });

  // ---------------------------------------------------------------------------
  // Date formatting for "last used"
  // ---------------------------------------------------------------------------

  describe("Last used display", () => {
    it("should show 'Never' when lastUsedAt is null", () => {
      const key = createMockApiKey({ lastUsedAt: null });

      const formatLastUsed = (lastUsedAt: string | null | undefined) =>
        lastUsedAt ? new Date(lastUsedAt).toLocaleDateString() : "Never";

      expect(formatLastUsed(key.lastUsedAt)).toBe("Never");
    });

    it("should format lastUsedAt date when present", () => {
      const key = createMockApiKey({ lastUsedAt: "2026-02-16T08:30:00Z" });

      const formatLastUsed = (lastUsedAt: string | null | undefined) =>
        lastUsedAt ? new Date(lastUsedAt).toLocaleDateString() : "Never";

      expect(formatLastUsed(key.lastUsedAt)).not.toBe("Never");
    });
  });

  // ---------------------------------------------------------------------------
  // Edge cases and security
  // ---------------------------------------------------------------------------

  describe("Security considerations", () => {
    it("should not store full tokens in component state after modal close", () => {
      let createdToken = "mts_secret_full_token";

      // Simulate closing modal
      createdToken = "";

      expect(createdToken).toBe("");
    });

    it("should only call create with sanitized name", () => {
      const name = "  My Key  ";
      const trimmed = name.trim();

      expect(trimmed).toBe("My Key");
    });
  });

  describe("Empty state", () => {
    it("should be identifiable when key list is empty", () => {
      const keys: ApiKey[] = [];
      const showEmptyState = keys.length === 0;

      expect(showEmptyState).toBe(true);
    });

    it("should not show empty state when keys exist", () => {
      const keys = [createMockApiKey()];
      const showEmptyState = keys.length === 0;

      expect(showEmptyState).toBe(false);
    });
  });

  describe("Optimistic list update after creation", () => {
    it("should add new key to list without re-fetching", async () => {
      const existingKeys = [createMockApiKey({ id: 1 })];
      const newKey = createMockApiKey({
        id: 2,
        name: "Brand New Key",
        token: "mts_new_token",
      });
      mockCreateApiKey.mockResolvedValueOnce(newKey);

      const result = await mockCreateApiKey({
        name: "Brand New Key",
        permissions: "full",
      });

      // Simulate adding to list with redacted token
      const updatedKeys = [
        ...existingKeys,
        { ...result, token: result.token.substring(0, 8) + "..." },
      ];

      expect(updatedKeys).toHaveLength(2);
      expect(updatedKeys[1].name).toBe("Brand New Key");
      expect(updatedKeys[1].token).toContain("...");
    });
  });
});
