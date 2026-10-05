import { api } from "$lib/api/client";
import type { PasskeyCredentialInfo, User } from "$lib/types/api";

export function isPasskeySupported(): boolean {
  return (
    typeof PublicKeyCredential === "function" &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === "function"
  );
}

export function isPasskeyCancellation(error: unknown): boolean {
  return error instanceof DOMException && error.name === "NotAllowedError";
}

export async function registerPasskey(name: string): Promise<PasskeyCredentialInfo> {
  const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(await api.passkeyRegisterBegin());
  const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential;
  return api.passkeyRegisterFinish(cred.toJSON() as RegistrationResponseJSON, name);
}

export async function loginWithPasskey(): Promise<User> {
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(await api.passkeyLoginBegin());
  const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential;
  return api.passkeyLoginFinish(cred.toJSON() as AuthenticationResponseJSON);
}
