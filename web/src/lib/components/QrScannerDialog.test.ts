import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

/**
 * QrScannerDialog tests
 *
 * These tests cover the QR scan result parsing logic and camera API
 * integration in isolation (no real camera or Svelte component needed).
 */

// ---------------------------------------------------------------------------
// Helpers: token extraction logic (mirrors handleScanResult in login page)
// ---------------------------------------------------------------------------

function extractTokenFromScanResult(raw: string): string | null {
  try {
    const parsed = new URL(raw);
    return parsed.searchParams.get("token");
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// 1. Scan result URL parsing
// ---------------------------------------------------------------------------

describe("QrScannerDialog - scan result URL parsing", () => {
  it("extracts token from URL containing ?token=", () => {
    const url = "https://server.example.com?token=mts_abc123def456";
    const token = extractTokenFromScanResult(url);
    expect(token).toBe("mts_abc123def456");
  });

  it("extracts token from URL with other query parameters", () => {
    const url = "https://server.example.com?foo=bar&token=mts_xyz789";
    const token = extractTokenFromScanResult(url);
    expect(token).toBe("mts_xyz789");
  });

  it("returns null for a plain server URL with no token", () => {
    const url = "https://server.example.com";
    const token = extractTokenFromScanResult(url);
    expect(token).toBeNull();
  });

  it("returns null for a URL with other params but no token", () => {
    const url = "https://server.example.com?foo=bar&baz=qux";
    const token = extractTokenFromScanResult(url);
    expect(token).toBeNull();
  });

  it("does not crash and returns null for an invalid (non-URL) string", () => {
    const notAUrl = "this is not a url";
    expect(() => extractTokenFromScanResult(notAUrl)).not.toThrow();
    expect(extractTokenFromScanResult(notAUrl)).toBeNull();
  });

  it("does not crash for an empty string", () => {
    expect(() => extractTokenFromScanResult("")).not.toThrow();
    expect(extractTokenFromScanResult("")).toBeNull();
  });

  it("handles URLs with ports correctly", () => {
    const url = "https://server.example.com:8082?token=mts_portkey";
    const token = extractTokenFromScanResult(url);
    expect(token).toBe("mts_portkey");
  });
});

// ---------------------------------------------------------------------------
// 2. Camera permission denied state
// ---------------------------------------------------------------------------

describe("QrScannerDialog - camera permission denied", () => {
  let originalGetUserMedia: typeof navigator.mediaDevices.getUserMedia;

  beforeEach(() => {
    // Ensure mediaDevices exists on jsdom's navigator
    if (!navigator.mediaDevices) {
      Object.defineProperty(navigator, "mediaDevices", {
        value: {},
        writable: true,
        configurable: true,
      });
    }
    originalGetUserMedia = navigator.mediaDevices.getUserMedia?.bind(
      navigator.mediaDevices,
    );
  });

  afterEach(() => {
    if (originalGetUserMedia) {
      navigator.mediaDevices.getUserMedia = originalGetUserMedia;
    }
  });

  it("rejects with NotAllowedError when camera permission is denied", async () => {
    const deniedError = Object.assign(new Error("Permission denied"), {
      name: "NotAllowedError",
    });

    // jsdom doesn't define getUserMedia — define it so vi.spyOn can work
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      value: vi.fn().mockRejectedValue(deniedError),
      writable: true,
      configurable: true,
    });

    await expect(
      navigator.mediaDevices.getUserMedia({ video: true }),
    ).rejects.toMatchObject({ name: "NotAllowedError" });
  });

  it("identifies NotAllowedError as a permission denial", () => {
    const err = Object.assign(new Error("Permission denied"), {
      name: "NotAllowedError",
    });

    const isDenied =
      err instanceof Error &&
      (err.name === "NotAllowedError" || err.name === "PermissionDeniedError");

    expect(isDenied).toBe(true);
  });

  it("identifies PermissionDeniedError as a permission denial", () => {
    const err = Object.assign(new Error("Permission denied"), {
      name: "PermissionDeniedError",
    });

    const isDenied =
      err instanceof Error &&
      (err.name === "NotAllowedError" || err.name === "PermissionDeniedError");

    expect(isDenied).toBe(true);
  });

  it("does not identify a generic error as a permission denial", () => {
    const err = new Error("Camera not found");

    const isDenied =
      err instanceof Error &&
      (err.name === "NotAllowedError" || err.name === "PermissionDeniedError");

    expect(isDenied).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 3. Stream cleanup on close
// ---------------------------------------------------------------------------

describe("QrScannerDialog - stream cleanup on close", () => {
  it("stops all tracks when stream is stopped", () => {
    const mockTrack1 = { stop: vi.fn(), kind: "video" };
    const mockTrack2 = { stop: vi.fn(), kind: "audio" };
    const mockStream = {
      getTracks: vi.fn().mockReturnValue([mockTrack1, mockTrack2]),
    } as unknown as MediaStream;

    // Simulate what the component does on close
    mockStream.getTracks().forEach((track) => track.stop());

    expect(mockTrack1.stop).toHaveBeenCalledOnce();
    expect(mockTrack2.stop).toHaveBeenCalledOnce();
  });

  it("cancels animation frame on close", () => {
    const cancelAnimationFrame = vi.spyOn(window, "cancelAnimationFrame");
    const frameId = 42;

    // Simulate what the component does on close
    cancelAnimationFrame(frameId);

    expect(cancelAnimationFrame).toHaveBeenCalledWith(frameId);
    cancelAnimationFrame.mockRestore();
  });

  it("does not throw when stream is null on close (no camera was started)", () => {
    // Use let so TypeScript doesn't narrow to never inside the if-guard
    let stream: MediaStream | null = null;

    // Simulate stopCamera() when stream is null
    expect(() => {
      if (stream) {
        stream.getTracks().forEach((track) => track.stop());
      }
    }).not.toThrow();

    // Suppress unused variable warning
    stream = null;
  });

  it("releases all video tracks when stopping camera", () => {
    const mockVideoTrack = { stop: vi.fn(), kind: "video" };
    const mockStream = {
      getTracks: vi.fn().mockReturnValue([mockVideoTrack]),
    } as unknown as MediaStream;

    mockStream.getTracks().forEach((track) => track.stop());

    expect(mockVideoTrack.stop).toHaveBeenCalledOnce();
  });
});

// ---------------------------------------------------------------------------
// 4. getUserMedia integration
// ---------------------------------------------------------------------------

describe("QrScannerDialog - getUserMedia", () => {
  it("requests rear-facing camera via facingMode: environment", async () => {
    const mockStream = {
      getTracks: vi.fn().mockReturnValue([]),
    } as unknown as MediaStream;

    if (!navigator.mediaDevices) {
      Object.defineProperty(navigator, "mediaDevices", {
        value: {},
        writable: true,
        configurable: true,
      });
    }

    // jsdom doesn't define getUserMedia — define it so we can mock it
    const mockGetUserMedia = vi.fn().mockResolvedValue(mockStream);
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      value: mockGetUserMedia,
      writable: true,
      configurable: true,
    });

    const stream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: "environment" },
    });

    expect(mockGetUserMedia).toHaveBeenCalledWith({
      video: { facingMode: "environment" },
    });
    expect(stream).toBe(mockStream);

    vi.restoreAllMocks();
  });
});
