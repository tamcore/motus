import { describe, it, expect, vi } from "vitest";
import { WebAuthnError } from "@simplewebauthn/browser";

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

const { isPasskeyCancellation, PasskeyCancelledError } = await import(
  "./webauthn"
);

/** Build a WebAuthnError with the given code and optional cause. */
function makeWebAuthnError(
  code: WebAuthnError["code"],
  cause: Error = new Error("cause"),
): WebAuthnError {
  return new WebAuthnError({ message: "test", code, cause });
}

/** Build a DOMException-like error with a specific name. */
function makeNamedError(name: string): Error {
  const err = new Error(name);
  err.name = name;
  return err;
}

describe("isPasskeyCancellation", () => {
  it("returns true for PasskeyCancelledError", () => {
    // Arrange
    const error = new PasskeyCancelledError();

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(true);
  });

  it("returns true for a WebAuthnError with ERROR_CEREMONY_ABORTED", () => {
    // Arrange
    const error = makeWebAuthnError("ERROR_CEREMONY_ABORTED");

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(true);
  });

  it("returns true when a WebAuthnError wraps a NotAllowedError cause", () => {
    // Arrange
    const error = makeWebAuthnError(
      "ERROR_PASSTHROUGH_SEE_CAUSE_PROPERTY",
      makeNamedError("NotAllowedError"),
    );

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(true);
  });

  it("returns true for a bare NotAllowedError DOMException", () => {
    // Arrange
    const error = makeNamedError("NotAllowedError");

    // Act & Assert
    expect(isPasskeyCancellation(error)).toBe(true);
  });

  it("returns false for an unrelated WebAuthnError code", () => {
    // Arrange
    const error = makeWebAuthnError("ERROR_INVALID_RP_ID");

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

describe("PasskeyCancelledError", () => {
  it("is an Error subclass with a stable name and default message", () => {
    // Arrange
    const error = new PasskeyCancelledError();

    // Act & Assert
    expect(error).toBeInstanceOf(Error);
    expect(error.name).toBe("PasskeyCancelledError");
    expect(error.message).toBe("Passkey operation was cancelled");
  });

  it("accepts a custom message", () => {
    // Arrange
    const error = new PasskeyCancelledError("nope");

    // Act & Assert
    expect(error.message).toBe("nope");
  });
});
