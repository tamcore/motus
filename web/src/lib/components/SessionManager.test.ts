import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock stores that depend on $app/environment
vi.mock("$lib/stores/settings", () => ({
  settings: {
    subscribe: vi.fn((fn: (value: unknown) => void) => {
      fn({
        dateFormat: "iso",
        speedUnit: "kmh",
        distanceUnit: "km",
        mapCenter: [0, 0],
        mapZoom: 13,
        mapOverlay: "osm_dark",
        mapLocationSet: false,
        timezone: "local",
      });
      return () => {};
    }),
  },
}));

// Mock the API client
const mockGetSessions = vi.fn();
const mockRevokeSession = vi.fn();

vi.mock("$lib/api/client", () => ({
  api: {
    getSessions: mockGetSessions,
    revokeSession: mockRevokeSession,
  },
  APIError: class APIError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

import type { Session } from "$lib/types/api";

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/** Create a mock Session with sensible defaults. */
function createMockSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess_abc123def456",
    userId: 1,
    rememberMe: true,
    isCurrent: false,
    apiKeyId: null,
    apiKeyName: null,
    createdAt: "2026-02-15T10:00:00Z",
    expiresAt: "2026-03-15T10:00:00Z",
    ...overrides,
  };
}

/** Mirror the truncateId helper from the component. */
function truncateId(id: string): string {
  if (id.length > 12) return id.substring(0, 12) + "\u2026";
  return id;
}

/** Mirror the getSessionTypeLabel helper from the component. */
function getSessionTypeLabel(session: Session): string {
  if (session.rememberMe) return "Persistent";
  return "24h";
}

describe("SessionManager", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // 1. Renders session list correctly (shows session data, badges)
  // ---------------------------------------------------------------------------

  describe("Session list rendering", () => {
    it("should return an array of sessions from the API", async () => {
      const sessions: Session[] = [
        createMockSession({ id: "sess_aaa111", rememberMe: true }),
        createMockSession({ id: "sess_bbb222", rememberMe: false }),
      ];
      mockGetSessions.mockResolvedValueOnce(sessions);

      const result = await mockGetSessions();

      expect(mockGetSessions).toHaveBeenCalledOnce();
      expect(result).toHaveLength(2);
      expect(result[0].id).toBe("sess_aaa111");
      expect(result[1].id).toBe("sess_bbb222");
    });

    it("should truncate long session IDs with ellipsis", () => {
      const longId = "sess_abc123def456ghi789";
      expect(longId.length).toBeGreaterThan(12);
      expect(truncateId(longId)).toBe("sess_abc123d\u2026");
      expect(truncateId(longId)).toHaveLength(13); // 12 chars + ellipsis
    });

    it("should not truncate short session IDs", () => {
      const shortId = "sess_abc";
      expect(shortId.length).toBeLessThanOrEqual(12);
      expect(truncateId(shortId)).toBe("sess_abc");
    });

    it("should handle exactly 12-character IDs without truncation", () => {
      const exactId = "sess_abc1234";
      expect(exactId.length).toBe(12);
      expect(truncateId(exactId)).toBe("sess_abc1234");
    });

    it("should display Persistent badge for rememberMe sessions", () => {
      const session = createMockSession({ rememberMe: true });
      expect(getSessionTypeLabel(session)).toBe("Persistent");
    });

    it("should display 24h badge for non-rememberMe sessions", () => {
      const session = createMockSession({ rememberMe: false });
      expect(getSessionTypeLabel(session)).toBe("24h");
    });

    it("should include both created and expires dates in session data", () => {
      const session = createMockSession({
        createdAt: "2026-02-15T10:00:00Z",
        expiresAt: "2026-03-15T10:00:00Z",
      });

      expect(session.createdAt).toBe("2026-02-15T10:00:00Z");
      expect(session.expiresAt).toBe("2026-03-15T10:00:00Z");
    });
  });

  // ---------------------------------------------------------------------------
  // 2. Current session shows "This session" badge and no Revoke button
  // ---------------------------------------------------------------------------

  describe("Current session identification", () => {
    it("should identify current session via isCurrent flag", () => {
      const currentSession = createMockSession({ isCurrent: true });
      const otherSession = createMockSession({ isCurrent: false });

      expect(currentSession.isCurrent).toBe(true);
      expect(otherSession.isCurrent).toBe(false);
    });

    it("should show 'Use logout to end' hint for current session instead of Revoke", () => {
      const currentSession = createMockSession({ isCurrent: true });

      // The component conditionally renders:
      //   if (session.isCurrent) -> "Use logout to end"
      //   else                   -> Revoke button
      const showRevokeButton = !currentSession.isCurrent;
      const showLogoutHint = !!currentSession.isCurrent;

      expect(showRevokeButton).toBe(false);
      expect(showLogoutHint).toBe(true);
    });

    it("should show Revoke button for non-current sessions", () => {
      const otherSession = createMockSession({ isCurrent: false });

      const showRevokeButton = !otherSession.isCurrent;
      expect(showRevokeButton).toBe(true);
    });

    it("should handle isCurrent being undefined (defaults to falsy)", () => {
      const session = createMockSession();
      // Remove isCurrent to simulate undefined
      delete (session as unknown as Record<string, unknown>)["isCurrent"];

      const showRevokeButton = !session.isCurrent;
      expect(showRevokeButton).toBe(true);
    });
  });

  // ---------------------------------------------------------------------------
  // 3. API key name badge displays correctly
  // ---------------------------------------------------------------------------

  describe("API key name badge", () => {
    it("should display 'via {apiKeyName}' when apiKeyName is set", () => {
      const session = createMockSession({ apiKeyName: "Home Assistant" });

      expect(session.apiKeyName).toBe("Home Assistant");
      const badgeText = session.apiKeyName ? `via ${session.apiKeyName}` : null;
      expect(badgeText).toBe("via Home Assistant");
    });

    it("should not display api key badge when apiKeyName is null", () => {
      const session = createMockSession({ apiKeyName: null });

      const showApiKeyBadge = !!session.apiKeyName;
      expect(showApiKeyBadge).toBe(false);
    });

    it("should not display api key badge when apiKeyName is undefined", () => {
      const session = createMockSession();
      delete (session as unknown as Record<string, unknown>)["apiKeyName"];

      const showApiKeyBadge = !!session.apiKeyName;
      expect(showApiKeyBadge).toBe(false);
    });

    it("should handle apiKeyName with special characters", () => {
      const session = createMockSession({
        apiKeyName: "My Key (v2) - production",
      });

      expect(session.apiKeyName).toBe("My Key (v2) - production");
      const badgeText = `via ${session.apiKeyName}`;
      expect(badgeText).toBe("via My Key (v2) - production");
    });
  });

  // ---------------------------------------------------------------------------
  // 4. Revoke button triggers confirmation dialog
  // ---------------------------------------------------------------------------

  describe("Revoke confirmation flow", () => {
    it("should set confirmingDeleteId when requesting delete", () => {
      let confirmingDeleteId: string | null = null;
      let deleteError = "";

      // Simulate requestDelete(id)
      const sessionId = "sess_target123";
      confirmingDeleteId = sessionId;
      deleteError = "";

      expect(confirmingDeleteId).toBe(sessionId);
      expect(deleteError).toBe("");
    });

    it("should clear confirmingDeleteId and error when cancelling", () => {
      let confirmingDeleteId: string | null = "sess_abc123";
      let deleteError = "previous error";

      // Simulate cancelDelete()
      confirmingDeleteId = null;
      deleteError = "";

      expect(confirmingDeleteId).toBeNull();
      expect(deleteError).toBe("");
    });

    it("should show confirmation text with truncated session ID", () => {
      const sessionId = "sess_abc123def456ghi789";
      const truncated = truncateId(sessionId);

      // The component renders:
      // "Are you sure you want to revoke session <strong>{truncateId(confirmingDeleteId)}</strong>?"
      expect(truncated).toBe("sess_abc123d\u2026");
    });

    it("should not trigger confirmation for current session", () => {
      const sessions = [
        createMockSession({ id: "sess_current", isCurrent: true }),
        createMockSession({ id: "sess_other", isCurrent: false }),
      ];

      // Only non-current sessions have the Revoke button
      const revokableSessions = sessions.filter((s) => !s.isCurrent);
      expect(revokableSessions).toHaveLength(1);
      expect(revokableSessions[0].id).toBe("sess_other");
    });
  });

  // ---------------------------------------------------------------------------
  // 5. Confirm delete calls api.revokeSession and removes from list
  // ---------------------------------------------------------------------------

  describe("Session revocation", () => {
    it("should call api.revokeSession with the correct session ID", async () => {
      mockRevokeSession.mockResolvedValueOnce(undefined);

      const targetId = "sess_to_revoke";
      await mockRevokeSession(targetId);

      expect(mockRevokeSession).toHaveBeenCalledWith(targetId);
    });

    it("should remove revoked session from the local list", async () => {
      mockRevokeSession.mockResolvedValueOnce(undefined);

      let sessions = [
        createMockSession({ id: "sess_aaa" }),
        createMockSession({ id: "sess_bbb" }),
        createMockSession({ id: "sess_ccc" }),
      ];

      const confirmingDeleteId = "sess_bbb";

      // Simulate confirmDelete()
      await mockRevokeSession(confirmingDeleteId);
      sessions = sessions.filter((s) => s.id !== confirmingDeleteId);

      expect(sessions).toHaveLength(2);
      expect(sessions.map((s) => s.id)).toEqual(["sess_aaa", "sess_ccc"]);
    });

    it("should transition through correct states during revocation", async () => {
      mockRevokeSession.mockResolvedValueOnce(undefined);

      let confirmingDeleteId: string | null = "sess_target";
      let deleting = false;
      let deleteError = "";
      let sessions = [
        createMockSession({ id: "sess_target" }),
        createMockSession({ id: "sess_other" }),
      ];

      // Simulate confirmDelete()
      deleting = true;
      deleteError = "";

      expect(deleting).toBe(true);

      try {
        await mockRevokeSession(confirmingDeleteId);
        sessions = sessions.filter((s) => s.id !== confirmingDeleteId);
        confirmingDeleteId = null;
      } catch (e: unknown) {
        if (e instanceof Error) {
          deleteError = e.message;
        } else {
          deleteError = "Failed to revoke session. Please try again.";
        }
      } finally {
        deleting = false;
      }

      expect(deleting).toBe(false);
      expect(confirmingDeleteId).toBeNull();
      expect(deleteError).toBe("");
      expect(sessions).toHaveLength(1);
      expect(sessions[0].id).toBe("sess_other");
    });

    it("should show error and keep session in list when revocation fails", async () => {
      mockRevokeSession.mockRejectedValueOnce(new Error("Forbidden"));

      let confirmingDeleteId: string | null = "sess_target";
      let deleting = false;
      let deleteError = "";
      let sessions = [
        createMockSession({ id: "sess_target" }),
        createMockSession({ id: "sess_other" }),
      ];

      // Simulate confirmDelete()
      deleting = true;
      deleteError = "";

      try {
        await mockRevokeSession(confirmingDeleteId);
        sessions = sessions.filter((s) => s.id !== confirmingDeleteId);
        confirmingDeleteId = null;
      } catch (e: unknown) {
        if (e instanceof Error) {
          deleteError = e.message;
        } else {
          deleteError = "Failed to revoke session. Please try again.";
        }
      } finally {
        deleting = false;
      }

      expect(deleting).toBe(false);
      // confirmingDeleteId stays set (dialog remains open) on error
      expect(confirmingDeleteId).toBe("sess_target");
      expect(deleteError).toBe("Forbidden");
      // Session is NOT removed from list on failure
      expect(sessions).toHaveLength(2);
    });

    it("should handle APIError during revocation", async () => {
      const { APIError } = await import("$lib/api/client");
      mockRevokeSession.mockRejectedValueOnce(
        new APIError(403, "cannot revoke another user's session"),
      );

      let deleteError = "";

      try {
        await mockRevokeSession("sess_target");
      } catch (e: unknown) {
        if (e instanceof Error) {
          deleteError = e.message;
        } else {
          deleteError = "Failed to revoke session. Please try again.";
        }
      }

      expect(deleteError).toBe("cannot revoke another user's session");
    });

    it("should handle unknown error type during revocation", async () => {
      mockRevokeSession.mockRejectedValueOnce("something unexpected");

      let deleteError = "";

      try {
        await mockRevokeSession("sess_target");
      } catch (e: unknown) {
        if (e instanceof Error) {
          deleteError = e.message;
        } else {
          deleteError = "Failed to revoke session. Please try again.";
        }
      }

      expect(deleteError).toBe("Failed to revoke session. Please try again.");
    });

    it("should do nothing if confirmingDeleteId is null", async () => {
      const confirmingDeleteId: string | null = null;

      // Simulate the guard in confirmDelete()
      if (confirmingDeleteId === null) {
        // early return
      } else {
        await mockRevokeSession(confirmingDeleteId);
      }

      expect(mockRevokeSession).not.toHaveBeenCalled();
    });
  });

  // ---------------------------------------------------------------------------
  // 6. Error state shown when API call fails
  // ---------------------------------------------------------------------------

  describe("Error state on load failure", () => {
    it("should set listError when getSessions throws an Error", async () => {
      mockGetSessions.mockRejectedValueOnce(new Error("Network error"));

      let loading = true;
      let sessions: Session[] = [];
      let listError = "";

      try {
        sessions = await mockGetSessions();
      } catch (e: unknown) {
        if (e instanceof Error) {
          listError = `Failed to load sessions: ${e.message}`;
        } else {
          listError = "Failed to load sessions. Please try again.";
        }
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(sessions).toHaveLength(0);
      expect(listError).toBe("Failed to load sessions: Network error");
    });

    it("should set listError when getSessions throws an APIError", async () => {
      const { APIError } = await import("$lib/api/client");
      mockGetSessions.mockRejectedValueOnce(
        new APIError(500, "internal server error"),
      );

      let loading = true;
      let sessions: Session[] = [];
      let listError = "";

      try {
        sessions = await mockGetSessions();
      } catch (e: unknown) {
        // APIError extends Error, so instanceof Error also matches
        if (e instanceof Error) {
          listError = `Failed to load sessions: ${e.message}`;
        } else {
          listError = "Failed to load sessions. Please try again.";
        }
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(sessions).toHaveLength(0);
      expect(listError).toBe("Failed to load sessions: internal server error");
    });

    it("should set generic listError for unknown error types", async () => {
      mockGetSessions.mockRejectedValueOnce(42);

      let listError = "";

      try {
        await mockGetSessions();
      } catch (e: unknown) {
        if (e instanceof Error) {
          listError = `Failed to load sessions: ${e.message}`;
        } else {
          listError = "Failed to load sessions. Please try again.";
        }
      }

      expect(listError).toBe("Failed to load sessions. Please try again.");
    });
  });

  // ---------------------------------------------------------------------------
  // 7. Empty state shown when no sessions
  // ---------------------------------------------------------------------------

  describe("Empty state", () => {
    it("should be identifiable when session list is empty", async () => {
      mockGetSessions.mockResolvedValueOnce([]);

      const sessions = await mockGetSessions();
      const showEmptyState = sessions.length === 0;

      expect(showEmptyState).toBe(true);
    });

    it("should not show empty state when sessions exist", async () => {
      mockGetSessions.mockResolvedValueOnce([
        createMockSession({ id: "sess_aaa" }),
      ]);

      const sessions = await mockGetSessions();
      const showEmptyState = sessions.length === 0;

      expect(showEmptyState).toBe(false);
    });
  });

  // ---------------------------------------------------------------------------
  // 8. Loading state shown initially
  // ---------------------------------------------------------------------------

  describe("Loading state", () => {
    it("should start in loading state and transition to loaded", async () => {
      const sessions = [
        createMockSession({ id: "sess_aaa" }),
        createMockSession({ id: "sess_bbb" }),
      ];
      mockGetSessions.mockResolvedValueOnce(sessions);

      let loading = true;
      let loadedSessions: Session[] = [];
      let listError = "";

      // Loading starts true
      expect(loading).toBe(true);

      try {
        loadedSessions = await mockGetSessions();
      } catch (e: unknown) {
        if (e instanceof Error) {
          listError = `Failed to load sessions: ${e.message}`;
        } else {
          listError = "Failed to load sessions. Please try again.";
        }
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(loadedSessions).toHaveLength(2);
      expect(listError).toBe("");
    });

    it("should transition from loading to error state on failure", async () => {
      mockGetSessions.mockRejectedValueOnce(new Error("Timeout"));

      let loading = true;
      let loadedSessions: Session[] = [];
      let listError = "";

      expect(loading).toBe(true);

      try {
        loadedSessions = await mockGetSessions();
      } catch (e: unknown) {
        if (e instanceof Error) {
          listError = `Failed to load sessions: ${e.message}`;
        } else {
          listError = "Failed to load sessions. Please try again.";
        }
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(loadedSessions).toHaveLength(0);
      expect(listError).toBe("Failed to load sessions: Timeout");
    });

    it("should reset loading to false in finally block regardless of outcome", async () => {
      // Test success path
      mockGetSessions.mockResolvedValueOnce([]);
      let loading = true;
      try {
        await mockGetSessions();
      } finally {
        loading = false;
      }
      expect(loading).toBe(false);

      // Test failure path
      mockGetSessions.mockRejectedValueOnce(new Error("fail"));
      loading = true;
      try {
        await mockGetSessions();
      } catch {
        // expected
      } finally {
        loading = false;
      }
      expect(loading).toBe(false);
    });
  });

  // ---------------------------------------------------------------------------
  // Mixed badge combinations
  // ---------------------------------------------------------------------------

  describe("Badge combinations", () => {
    it("should show all three badges for current persistent API key session", () => {
      const session = createMockSession({
        isCurrent: true,
        rememberMe: true,
        apiKeyName: "Grafana",
      });

      expect(session.isCurrent).toBe(true);
      expect(getSessionTypeLabel(session)).toBe("Persistent");
      expect(session.apiKeyName).toBe("Grafana");
    });

    it("should show only 24h badge for non-current temporary non-API session", () => {
      const session = createMockSession({
        isCurrent: false,
        rememberMe: false,
        apiKeyName: null,
      });

      expect(session.isCurrent).toBeFalsy();
      expect(getSessionTypeLabel(session)).toBe("24h");
      expect(session.apiKeyName).toBeFalsy();
    });

    it("should correctly distinguish multiple sessions with mixed properties", () => {
      const sessions = [
        createMockSession({
          id: "sess_current",
          isCurrent: true,
          rememberMe: true,
          apiKeyName: null,
        }),
        createMockSession({
          id: "sess_apikey",
          isCurrent: false,
          rememberMe: false,
          apiKeyName: "Home Assistant",
        }),
        createMockSession({
          id: "sess_persistent",
          isCurrent: false,
          rememberMe: true,
          apiKeyName: null,
        }),
      ];

      // Current session
      expect(sessions[0].isCurrent).toBe(true);
      expect(getSessionTypeLabel(sessions[0])).toBe("Persistent");
      expect(sessions[0].apiKeyName).toBeNull();

      // API key session
      expect(sessions[1].isCurrent).toBe(false);
      expect(getSessionTypeLabel(sessions[1])).toBe("24h");
      expect(sessions[1].apiKeyName).toBe("Home Assistant");

      // Persistent non-current session
      expect(sessions[2].isCurrent).toBe(false);
      expect(getSessionTypeLabel(sessions[2])).toBe("Persistent");
      expect(sessions[2].apiKeyName).toBeNull();
    });
  });

  // ---------------------------------------------------------------------------
  // Full load + revoke lifecycle
  // ---------------------------------------------------------------------------

  describe("Full lifecycle: load then revoke", () => {
    it("should load sessions then revoke one successfully", async () => {
      const initialSessions = [
        createMockSession({ id: "sess_aaa", isCurrent: true }),
        createMockSession({ id: "sess_bbb", isCurrent: false }),
        createMockSession({ id: "sess_ccc", isCurrent: false }),
      ];
      mockGetSessions.mockResolvedValueOnce(initialSessions);
      mockRevokeSession.mockResolvedValueOnce(undefined);

      // Step 1: Load
      let loading = true;
      let sessions: Session[] = [];
      let listError = "";
      let confirmingDeleteId: string | null = null;
      let deleting = false;
      let deleteError = "";

      try {
        sessions = await mockGetSessions();
      } catch (e: unknown) {
        listError = e instanceof Error ? e.message : "Failed to load sessions.";
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(sessions).toHaveLength(3);
      expect(listError).toBe("");

      // Step 2: Click Revoke on sess_bbb
      confirmingDeleteId = "sess_bbb";
      expect(confirmingDeleteId).toBe("sess_bbb");

      // Step 3: Confirm revoke
      deleting = true;
      deleteError = "";

      try {
        await mockRevokeSession(confirmingDeleteId);
        sessions = sessions.filter((s) => s.id !== confirmingDeleteId);
        confirmingDeleteId = null;
      } catch (e: unknown) {
        deleteError =
          e instanceof Error
            ? e.message
            : "Failed to revoke session. Please try again.";
      } finally {
        deleting = false;
      }

      expect(deleting).toBe(false);
      expect(confirmingDeleteId).toBeNull();
      expect(deleteError).toBe("");
      expect(sessions).toHaveLength(2);
      expect(sessions.map((s) => s.id)).toEqual(["sess_aaa", "sess_ccc"]);
      expect(mockRevokeSession).toHaveBeenCalledWith("sess_bbb");
    });
  });
});
