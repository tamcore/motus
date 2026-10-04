// Traccar Manager WebView bridge; see https://github.com/traccar/traccar-web/blob/master/src/common/components/NativeInterface.js
import { api } from "$lib/api/client";

declare global {
  interface Window {
    webkit?: {
      messageHandlers?: {
        appInterface?: {
          postMessage: (message: string) => void;
        };
      };
    };
    appInterface?: {
      postMessage: (message: string) => void;
    };
    handleLoginToken?: (token: string) => void;
  }
}

export function isNativeEnvironment(): boolean {
  return !!(
    window.webkit?.messageHandlers?.appInterface || window.appInterface
  );
}

/** No-op outside the app's WebView. */
export function nativePostMessage(message: string): void {
  if (window.webkit?.messageHandlers?.appInterface) {
    window.webkit.messageHandlers.appInterface.postMessage(message);
  } else if (window.appInterface) {
    window.appInterface.postMessage(message);
  }
}

/** Called when the native app answers an "authentication" message with its stored token. */
export const handleLoginTokenListeners = new Set<(token: string) => void>();

export function initNativeTokenHandler(): void {
  window.handleLoginToken = (token: string) => {
    handleLoginTokenListeners.forEach((listener) => listener(token));
  };
}

/** Hands the native app a login token for auto-login on its next launch. */
export async function generateLoginToken(): Promise<void> {
  if (!isNativeEnvironment()) return;

  try {
    const { token } = await api.generateToken();
    if (token) nativePostMessage(`login|${token}`);
  } catch {
    // Silently ignore token generation failures in native context.
  }
}

export function notifyNativeLogout(): void {
  nativePostMessage("logout");
}

export function changeServerUrl(url: string): void {
  if (!url || !url.trim()) {
    throw new Error(
      "changeServerUrl: refusing to send empty URL to native app (would crash Traccar Manager)",
    );
  }
  nativePostMessage(`server|${url}`);
}
