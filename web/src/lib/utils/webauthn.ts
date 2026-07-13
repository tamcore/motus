/**
 * Thin WebAuthn / passkey helper module.
 *
 * Wraps @simplewebauthn/browser and the Motus passkey API endpoints. The
 * begin/finish options are passed through unchanged: the browser library
 * consumes the begin response directly and produces the finish request body
 * directly, so we never reshape the WebAuthn JSON.
 */
import {
  browserSupportsWebAuthn,
  startAuthentication,
  startRegistration,
  WebAuthnError,
} from "@simplewebauthn/browser";
import { api } from "$lib/api/client";
import type { PasskeyCredentialInfo, User } from "$lib/types/api";

/** Error thrown when the user cancels or dismisses the passkey ceremony. */
export class PasskeyCancelledError extends Error {
  constructor(message = "Passkey operation was cancelled") {
    super(message);
    this.name = "PasskeyCancelledError";
  }
}

/**
 * Returns true when the current browser exposes the WebAuthn API. Safe to call
 * during SSR — returns false when `window` / `navigator` are unavailable.
 */
export function isPasskeySupported(): boolean {
  if (typeof window === "undefined") return false;
  return browserSupportsWebAuthn();
}

/**
 * Detects whether an error represents a user-cancelled / dismissed ceremony.
 *
 * The browser raises a `NotAllowedError` DOMException when the user aborts,
 * which @simplewebauthn/browser passes through (wrapped in a WebAuthnError with
 * code ERROR_CEREMONY_ABORTED or ERROR_PASSTHROUGH_SEE_CAUSE_PROPERTY).
 */
export function isPasskeyCancellation(error: unknown): boolean {
  if (error instanceof PasskeyCancelledError) return true;
  if (error instanceof WebAuthnError) {
    if (error.code === "ERROR_CEREMONY_ABORTED") return true;
    const cause = error.cause;
    if (cause instanceof Error && cause.name === "NotAllowedError") return true;
  }
  if (error instanceof Error && error.name === "NotAllowedError") return true;
  return false;
}

/**
 * Registers a new passkey for the authenticated user.
 *
 * Runs begin → startRegistration → finish and returns the created credential
 * metadata. Throws {@link PasskeyCancelledError} when the user dismisses the
 * browser prompt; re-throws any other error unchanged.
 */
export async function registerPasskey(
  name: string,
): Promise<PasskeyCredentialInfo> {
  const optionsJSON = await api.passkeyRegisterBegin();

  let attestation;
  try {
    attestation = await startRegistration({ optionsJSON });
  } catch (error: unknown) {
    if (isPasskeyCancellation(error)) {
      throw new PasskeyCancelledError();
    }
    throw error;
  }

  return api.passkeyRegisterFinish(attestation, name);
}

/**
 * Authenticates the user with an existing passkey.
 *
 * Runs begin → startAuthentication → finish and returns the logged-in User.
 * Throws {@link PasskeyCancelledError} when the user dismisses the browser
 * prompt; re-throws any other error unchanged.
 */
export async function loginWithPasskey(): Promise<User> {
  const optionsJSON = await api.passkeyLoginBegin();

  let assertion;
  try {
    assertion = await startAuthentication({ optionsJSON });
  } catch (error: unknown) {
    if (isPasskeyCancellation(error)) {
      throw new PasskeyCancelledError();
    }
    throw error;
  }

  return api.passkeyLoginFinish(assertion);
}
