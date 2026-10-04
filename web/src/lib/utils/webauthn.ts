import {
  browserSupportsWebAuthn,
  startAuthentication,
  startRegistration,
  WebAuthnError,
} from "@simplewebauthn/browser";
import { api } from "$lib/api/client";
import type { PasskeyCredentialInfo, User } from "$lib/types/api";

export function isPasskeySupported(): boolean {
  return browserSupportsWebAuthn();
}

/**
 * Detects a user-cancelled / dismissed ceremony: the browser raises a
 * `NotAllowedError`, which @simplewebauthn/browser may wrap in a WebAuthnError
 * (ERROR_CEREMONY_ABORTED or ERROR_PASSTHROUGH_SEE_CAUSE_PROPERTY).
 */
export function isPasskeyCancellation(error: unknown): boolean {
  if (error instanceof WebAuthnError) {
    if (error.code === "ERROR_CEREMONY_ABORTED") return true;
    const cause = error.cause;
    if (cause instanceof Error && cause.name === "NotAllowedError") return true;
  }
  return error instanceof Error && error.name === "NotAllowedError";
}

export async function registerPasskey(name: string): Promise<PasskeyCredentialInfo> {
  const optionsJSON = await api.passkeyRegisterBegin();
  return api.passkeyRegisterFinish(await startRegistration({ optionsJSON }), name);
}

export async function loginWithPasskey(): Promise<User> {
  const optionsJSON = await api.passkeyLoginBegin();
  return api.passkeyLoginFinish(await startAuthentication({ optionsJSON }));
}
