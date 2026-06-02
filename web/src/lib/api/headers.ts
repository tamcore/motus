import { getStoredAuthToken } from "$lib/auth-token-store";

let csrfToken: string | null = null;

export function getCsrfToken(): string | null {
  return csrfToken;
}

export function setCsrfToken(v: string | null): void {
  csrfToken = v;
}

/** Returns request headers including CSRF and auth tokens as needed. */
export async function getAuthHeaders(
  method: string,
): Promise<Record<string, string>> {
  const headers: Record<string, string> = {};
  const upper = method.toUpperCase();
  if (csrfToken && ["POST", "PUT", "DELETE", "PATCH"].includes(upper)) {
    headers["X-CSRF-Token"] = csrfToken;
  }
  const authToken = await getStoredAuthToken();
  if (authToken) {
    headers["X-Auth-Token"] = authToken;
  }
  return headers;
}
