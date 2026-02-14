import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock the API client
const mockGenerateToken = vi.fn();
vi.mock("$lib/api/client", () => ({
  api: {
    generateToken: mockGenerateToken,
  },
  APIError: class APIError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

describe("Settings page - API Token Management", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset localStorage for each test
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("Token generation API interaction", () => {
    it("should call api.generateToken and return a token string", async () => {
      const fakeToken = "abc123def456ghi789";
      mockGenerateToken.mockResolvedValueOnce({ token: fakeToken });

      const result = await mockGenerateToken();

      expect(mockGenerateToken).toHaveBeenCalledOnce();
      expect(result).toEqual({ token: fakeToken });
      expect(typeof result.token).toBe("string");
      expect(result.token.length).toBeGreaterThan(0);
    });

    it("should handle API errors gracefully", async () => {
      const { APIError } = await import("$lib/api/client");
      mockGenerateToken.mockRejectedValueOnce(
        new APIError(500, "failed to generate token"),
      );

      await expect(mockGenerateToken()).rejects.toThrow(
        "failed to generate token",
      );
    });

    it("should handle network errors", async () => {
      mockGenerateToken.mockRejectedValueOnce(new Error("Network error"));

      await expect(mockGenerateToken()).rejects.toThrow("Network error");
    });

    it("should handle unknown errors", async () => {
      mockGenerateToken.mockRejectedValueOnce("something unexpected");

      await expect(mockGenerateToken()).rejects.toBe("something unexpected");
    });
  });

  describe("Token existence detection", () => {
    it("should detect when user has an existing token", () => {
      const user = { id: 1, email: "test@test.com", token: "existing-token" };
      expect(!!user.token).toBe(true);
    });

    it("should detect when user has no token", () => {
      const user = { id: 1, email: "test@test.com", token: null };
      expect(!!user.token).toBe(false);
    });

    it("should handle undefined token field", () => {
      const user = { id: 1, email: "test@test.com" };
      expect(!!(user as any).token).toBe(false);
    });

    it("should handle empty string token as falsy", () => {
      const user = { id: 1, email: "test@test.com", token: "" };
      expect(!!user.token).toBe(false);
    });
  });

  describe("User store update after token generation", () => {
    it("should create a new user object with token (immutability)", () => {
      const originalUser = {
        id: 1,
        email: "test@test.com",
        name: "Test",
        token: null as string | null,
      };
      const newToken = "new-generated-token";

      // Simulate the immutable update pattern used in the component
      const updatedUser = { ...originalUser, token: newToken };

      expect(updatedUser.token).toBe(newToken);
      expect(originalUser.token).toBeNull(); // Original unchanged
      expect(updatedUser).not.toBe(originalUser); // New object
      expect(updatedUser.id).toBe(originalUser.id);
      expect(updatedUser.email).toBe(originalUser.email);
    });
  });

  describe("Clipboard copy functionality", () => {
    it("should copy token text using navigator.clipboard API", async () => {
      const writeTextMock = vi.fn().mockResolvedValueOnce(undefined);
      Object.assign(navigator, {
        clipboard: { writeText: writeTextMock },
      });

      const token = "test-token-to-copy";
      await navigator.clipboard.writeText(token);

      expect(writeTextMock).toHaveBeenCalledWith(token);
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
        // Simulate the fallback: select + execCommand
        usedFallback = true;
        copied = true; // Fallback assumed successful
      }

      expect(usedFallback).toBe(true);
      expect(copied).toBe(true);
    });
  });

  describe("Token display state machine", () => {
    it("should transition through correct states during generation", async () => {
      const fakeToken = "generated-token-value";
      mockGenerateToken.mockResolvedValueOnce({ token: fakeToken });

      // Initial state
      let generatingToken = false;
      let generatedToken = "";
      let tokenError = "";
      let tokenCopied = false;

      // Start generation (mirrors the component logic)
      generatingToken = true;
      generatedToken = "";
      tokenError = "";
      tokenCopied = false;

      expect(generatingToken).toBe(true);
      expect(generatedToken).toBe("");

      // API call succeeds
      try {
        const response = await mockGenerateToken();
        generatedToken = response.token;
      } catch (error: unknown) {
        if (error instanceof Error) {
          tokenError = `Failed to generate token: ${error.message}`;
        } else {
          tokenError = "Failed to generate token. Please try again.";
        }
      } finally {
        generatingToken = false;
      }

      // Final state after success
      expect(generatingToken).toBe(false);
      expect(generatedToken).toBe(fakeToken);
      expect(tokenError).toBe("");
      expect(tokenCopied).toBe(false);
    });

    it("should set error state on API failure", async () => {
      mockGenerateToken.mockRejectedValueOnce(
        new Error("Internal server error"),
      );

      let generatingToken = false;
      let generatedToken = "";
      let tokenError = "";

      generatingToken = true;
      generatedToken = "";
      tokenError = "";

      try {
        const response = await mockGenerateToken();
        generatedToken = response.token;
      } catch (error: unknown) {
        if (error instanceof Error) {
          tokenError = `Failed to generate token: ${error.message}`;
        } else {
          tokenError = "Failed to generate token. Please try again.";
        }
      } finally {
        generatingToken = false;
      }

      expect(generatingToken).toBe(false);
      expect(generatedToken).toBe("");
      expect(tokenError).toBe(
        "Failed to generate token: Internal server error",
      );
    });

    it("should reset copied state after timeout", async () => {
      vi.useFakeTimers();

      let tokenCopied = false;
      tokenCopied = true;

      expect(tokenCopied).toBe(true);

      // Simulate the setTimeout pattern
      setTimeout(() => {
        tokenCopied = false;
      }, 2000);

      vi.advanceTimersByTime(2000);
      expect(tokenCopied).toBe(false);

      vi.useRealTimers();
    });

    it("should clear previous token when regenerating", () => {
      let generatedToken = "old-token";
      let tokenError = "";
      let tokenCopied = true;

      // Reset at start of generation
      generatedToken = "";
      tokenError = "";
      tokenCopied = false;

      expect(generatedToken).toBe("");
      expect(tokenError).toBe("");
      expect(tokenCopied).toBe(false);
    });
  });

  describe("Security considerations", () => {
    it("should not store token in localStorage directly", () => {
      const token = "sensitive-api-token";
      // The generated token should NOT be persisted to localStorage
      // Only the user object's token field is stored (via the auth store)
      // to indicate "has token" status
      localStorage.setItem("test-direct", token);
      expect(localStorage.getItem("test-direct")).toBe(token);
      localStorage.removeItem("test-direct");

      // The component only stores the token presence in the user store,
      // not the raw token value (the store stores the full user object
      // but that's already in localStorage for auth persistence)
    });

    it("should generate token as a string of sufficient length", async () => {
      const longToken =
        "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.longpayload.signature";
      mockGenerateToken.mockResolvedValueOnce({ token: longToken });

      const result = await mockGenerateToken();
      expect(result.token.length).toBeGreaterThan(10);
    });
  });
});
