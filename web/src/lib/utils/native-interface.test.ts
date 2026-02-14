import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  isNativeEnvironment,
  nativePostMessage,
  handleLoginTokenListeners,
  initNativeTokenHandler,
  notifyNativeLogout,
  generateLoginToken,
  changeServerUrl,
} from "./native-interface";

describe("native-interface", () => {
  beforeEach(() => {
    // Clean up any previous mocks.
    if (typeof window !== "undefined") {
      delete (window as unknown as Record<string, unknown>).webkit;
      delete (window as unknown as Record<string, unknown>).appInterface;
      delete (window as unknown as Record<string, unknown>).handleLoginToken;
    }
    handleLoginTokenListeners.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("isNativeEnvironment", () => {
    it("returns false when no bridge is present", () => {
      expect(isNativeEnvironment()).toBe(false);
    });

    it("returns true when Android bridge is present", () => {
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: vi.fn(),
      };
      expect(isNativeEnvironment()).toBe(true);
    });

    it("returns true when iOS bridge is present", () => {
      (window as unknown as Record<string, unknown>).webkit = {
        messageHandlers: {
          appInterface: {
            postMessage: vi.fn(),
          },
        },
      };
      expect(isNativeEnvironment()).toBe(true);
    });
  });

  describe("nativePostMessage", () => {
    it("calls Android bridge when available", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      nativePostMessage("logout");
      expect(mockPostMessage).toHaveBeenCalledWith("logout");
    });

    it("calls iOS bridge when available", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).webkit = {
        messageHandlers: {
          appInterface: {
            postMessage: mockPostMessage,
          },
        },
      };

      nativePostMessage("logout");
      expect(mockPostMessage).toHaveBeenCalledWith("logout");
    });

    it("prefers iOS bridge over Android bridge", () => {
      const mockiOS = vi.fn();
      const mockAndroid = vi.fn();
      (window as unknown as Record<string, unknown>).webkit = {
        messageHandlers: {
          appInterface: {
            postMessage: mockiOS,
          },
        },
      };
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockAndroid,
      };

      nativePostMessage("test");
      expect(mockiOS).toHaveBeenCalledWith("test");
      expect(mockAndroid).not.toHaveBeenCalled();
    });

    it("does nothing when no bridge is present", () => {
      // Should not throw.
      expect(() => nativePostMessage("logout")).not.toThrow();
    });

    it("sends login message with token", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      nativePostMessage("login|abc123");
      expect(mockPostMessage).toHaveBeenCalledWith("login|abc123");
    });
  });

  describe("notifyNativeLogout", () => {
    it("sends logout message via bridge", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      notifyNativeLogout();
      expect(mockPostMessage).toHaveBeenCalledWith("logout");
    });
  });

  describe("initNativeTokenHandler", () => {
    it("registers window.handleLoginToken", () => {
      expect(window.handleLoginToken).toBeUndefined();
      initNativeTokenHandler();
      expect(typeof window.handleLoginToken).toBe("function");
    });

    it("calls registered listeners when token is received", () => {
      initNativeTokenHandler();

      const listener = vi.fn();
      handleLoginTokenListeners.add(listener);

      window.handleLoginToken!("mytoken");
      expect(listener).toHaveBeenCalledWith("mytoken");
    });

    it("calls multiple listeners", () => {
      initNativeTokenHandler();

      const listener1 = vi.fn();
      const listener2 = vi.fn();
      handleLoginTokenListeners.add(listener1);
      handleLoginTokenListeners.add(listener2);

      window.handleLoginToken!("token");
      expect(listener1).toHaveBeenCalledWith("token");
      expect(listener2).toHaveBeenCalledWith("token");
    });
  });

  describe("generateLoginToken", () => {
    it("does nothing when not in native environment", async () => {
      const fetchSpy = vi.spyOn(globalThis, "fetch");
      await generateLoginToken();
      expect(fetchSpy).not.toHaveBeenCalled();
    });

    it("calls API and sends token to native app", async () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      vi.spyOn(globalThis, "fetch").mockResolvedValue({
        ok: true,
        json: async () => ({ token: "generated-token" }),
      } as Response);

      await generateLoginToken();

      expect(globalThis.fetch).toHaveBeenCalledWith("/api/session/token", {
        method: "POST",
      });
      expect(mockPostMessage).toHaveBeenCalledWith("login|generated-token");
    });

    it("handles API failure gracefully", async () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      vi.spyOn(globalThis, "fetch").mockResolvedValue({
        ok: false,
      } as Response);

      // Should not throw.
      await generateLoginToken();
      expect(mockPostMessage).not.toHaveBeenCalled();
    });

    it("handles network error gracefully", async () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      vi.spyOn(globalThis, "fetch").mockRejectedValue(
        new Error("network error"),
      );

      // Should not throw.
      await generateLoginToken();
      expect(mockPostMessage).not.toHaveBeenCalled();
    });

    it("does not send empty token", async () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      vi.spyOn(globalThis, "fetch").mockResolvedValue({
        ok: true,
        json: async () => ({ token: "" }),
      } as Response);

      await generateLoginToken();
      expect(mockPostMessage).not.toHaveBeenCalled();
    });
  });

  describe("changeServerUrl", () => {
    it("sends server|<url> message via Android bridge", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      changeServerUrl("https://newserver.example.com");
      expect(mockPostMessage).toHaveBeenCalledWith(
        "server|https://newserver.example.com",
      );
    });

    it("sends server|<url> message via iOS bridge", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).webkit = {
        messageHandlers: {
          appInterface: {
            postMessage: mockPostMessage,
          },
        },
      };

      changeServerUrl("https://ios-server.example.com");
      expect(mockPostMessage).toHaveBeenCalledWith(
        "server|https://ios-server.example.com",
      );
    });

    it("throws when called with an empty string to prevent native app crash", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      expect(() => changeServerUrl("")).toThrow(
        "changeServerUrl: refusing to send empty URL to native app",
      );
      expect(mockPostMessage).not.toHaveBeenCalled();
    });

    it("throws when called with whitespace-only string", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      expect(() => changeServerUrl("   ")).toThrow(
        "changeServerUrl: refusing to send empty URL to native app",
      );
      expect(mockPostMessage).not.toHaveBeenCalled();
    });

    it("does nothing when no bridge is present", () => {
      // Should not throw when no native bridge exists (valid URL, no bridge).
      expect(() => changeServerUrl("https://example.com")).not.toThrow();
    });

    it("includes port and path in URL", () => {
      const mockPostMessage = vi.fn();
      (window as unknown as Record<string, unknown>).appInterface = {
        postMessage: mockPostMessage,
      };

      changeServerUrl("https://example.com:8082/tracking");
      expect(mockPostMessage).toHaveBeenCalledWith(
        "server|https://example.com:8082/tracking",
      );
    });
  });
});
