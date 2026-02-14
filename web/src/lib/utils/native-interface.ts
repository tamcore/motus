/**
 * Native Interface for Traccar Manager mobile app compatibility.
 *
 * The Traccar Manager app (Flutter) loads the web UI inside an InAppWebView
 * and injects a JavaScript bridge named `appInterface`. The web app uses
 * this bridge to communicate login/logout state to the native app.
 *
 * Message protocol (pipe-delimited):
 *   "login|<token>"       - Store a login token in the native app
 *   "logout"              - Delete the stored login token
 *   "authentication"      - Request the stored login token from the native app
 *   "server|<url>"        - Change the server URL in the native app
 *
 * Platform detection:
 *   iOS:     window.webkit.messageHandlers.appInterface.postMessage(msg)
 *   Android: window.appInterface.postMessage(msg)
 *
 * Reference: https://github.com/traccar/traccar-web/blob/master/src/common/components/NativeInterface.js
 */

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

/**
 * Returns true when running inside a native WebView that provides the
 * appInterface bridge (Traccar Manager on iOS or Android).
 */
export function isNativeEnvironment(): boolean {
  if (typeof window === "undefined") return false;
  return !!(
    window.webkit?.messageHandlers?.appInterface || window.appInterface
  );
}

/**
 * Sends a message to the native Traccar Manager app through the
 * JavaScript bridge. No-op when not running inside the app's WebView.
 */
export function nativePostMessage(message: string): void {
  if (typeof window === "undefined") return;

  if (window.webkit?.messageHandlers?.appInterface) {
    window.webkit.messageHandlers.appInterface.postMessage(message);
  } else if (window.appInterface) {
    window.appInterface.postMessage(message);
  }
}

/**
 * Listeners that are called when the native app provides a stored login
 * token in response to an "authentication" message.
 */
export const handleLoginTokenListeners = new Set<(token: string) => void>();

/**
 * Initialize the global callback that the native app calls when
 * providing a stored login token. This should be called once at app
 * startup.
 */
export function initNativeTokenHandler(): void {
  if (typeof window === "undefined") return;

  window.handleLoginToken = (token: string) => {
    handleLoginTokenListeners.forEach((listener) => listener(token));
  };
}

/**
 * Generates a login token via the API and sends it to the native app
 * for persistent storage. On subsequent app launches, the native app
 * will provide this token back to auto-login the user.
 */
export async function generateLoginToken(): Promise<void> {
  if (!isNativeEnvironment()) return;

  try {
    const response = await fetch("/api/session/token", { method: "POST" });
    if (response.ok) {
      const data = await response.json();
      const token = data.token || "";
      if (token) {
        nativePostMessage(`login|${token}`);
      }
    }
  } catch {
    // Silently ignore token generation failures in native context.
  }
}

/**
 * Notifies the native Traccar Manager app that the user has logged out.
 * This causes the native app to delete the stored login token, preventing
 * auto-login on the next app launch.
 */
export function notifyNativeLogout(): void {
  nativePostMessage("logout");
}

/**
 * Changes the server URL in the Traccar Manager native app.
 * This deletes the stored login token, saves the new URL to
 * SharedPreferences, and reloads the WebView with the new server.
 *
 * IMPORTANT: Never pass an empty string. The Traccar Manager app will
 * save "" as the server URL and crash when trying to parse it, leaving
 * the user with a white screen that requires reinstalling the app.
 * Always validate that `url` is a non-empty, well-formed URL before
 * calling this function.
 *
 * @param url - New server URL (e.g., "https://newserver.com"). Must be
 *              a valid, non-empty URL with http or https protocol.
 * @throws {Error} If url is empty (guards against the native app crash)
 */
export function changeServerUrl(url: string): void {
  if (!url || !url.trim()) {
    throw new Error(
      "changeServerUrl: refusing to send empty URL to native app (would crash Traccar Manager)",
    );
  }
  nativePostMessage(`server|${url}`);
}
