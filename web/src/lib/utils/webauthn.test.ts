import { describe, it, expect, vi } from "vitest";

// The module under test transitively imports the API client, which pulls in
// $lib/stores/auth. That store reads localStorage at module-load time. The
// vitest jsdom env in this repo runs without a localStorage-file (a known
// pre-existing limitation), so provide a minimal in-memory shim before the
// module graph is evaluated. Only affects this isolated test file.
vi.stubGlobal("localStorage", {
  getItem: () => null,
  setItem: () => {},
  removeItem: () => {},
  clear: () => {},
  key: () => null,
  length: 0,
});

const { isPasskeyCancellation } = await import("./webauthn");

describe("isPasskeyCancellation", () => {
  it("returns true for a NotAllowedError DOMException", () => {
    // Arrange
    const error = new DOMException("dismissed", "NotAllowedError");

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(true);
  });

  it("returns false for another DOMException", () => {
    // Arrange
    const error = new DOMException("bad rp", "SecurityError");

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(false);
  });

  it("returns false for a generic error", () => {
    // Arrange
    const error = new Error("network down");

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(false);
  });

  it("returns false for non-error values", () => {
    // Act & Assert
    expect(isPasskeyCancellation(null)).toBe(false);
    expect(isPasskeyCancellation(undefined)).toBe(false);
    expect(isPasskeyCancellation("NotAllowedError")).toBe(false);
  });
});
