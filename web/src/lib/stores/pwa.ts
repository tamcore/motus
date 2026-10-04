import { writable, derived } from "svelte/store";

interface BeforeInstallPromptEvent extends Event {
  readonly platforms: string[];
  readonly userChoice: Promise<{ outcome: "accepted" | "dismissed" }>;
  prompt(): Promise<void>;
}

interface PwaState {
  installable: boolean;
  installed: boolean;
  /** A new service worker version is waiting to activate. */
  updateAvailable: boolean;
  /** Remembered for this session. */
  installDismissed: boolean;
}

const initialState: PwaState = {
  installable: false,
  installed: false,
  updateAvailable: false,
  installDismissed: false,
};

function createPwaStore() {
  const { subscribe, update } = writable<PwaState>(initialState);

  let deferredPrompt: BeforeInstallPromptEvent | null = null;
  let updateWorker: ServiceWorker | null = null;

  function initialize(): void {
    if (!("serviceWorker" in navigator)) return;

    // Check if already installed as standalone
    const isStandalone =
      window.matchMedia("(display-mode: standalone)").matches ||
      (navigator as unknown as Record<string, unknown>).standalone === true;

    if (isStandalone) {
      update((s) => ({ ...s, installed: true }));
    }

    // Listen for the beforeinstallprompt event
    window.addEventListener("beforeinstallprompt", (e) => {
      e.preventDefault();
      deferredPrompt = e as BeforeInstallPromptEvent;

      // Check if user previously dismissed in this session
      const dismissed =
        sessionStorage.getItem("motus_pwa_install_dismissed") === "true";

      update((s) => ({
        ...s,
        installable: true,
        installDismissed: dismissed,
      }));
    });

    // Listen for app installed event
    window.addEventListener("appinstalled", () => {
      deferredPrompt = null;
      update((s) => ({
        ...s,
        installable: false,
        installed: true,
      }));
    });

    // Listen for display-mode changes (user might install through browser menu)
    window
      .matchMedia("(display-mode: standalone)")
      .addEventListener("change", (e) => {
        update((s) => ({ ...s, installed: e.matches }));
      });

    // Register the service worker
    registerServiceWorker();
  }

  async function registerServiceWorker(): Promise<void> {
    try {
      const registration = await navigator.serviceWorker.register("/sw.js", {
        scope: "/",
      });

      // Check for updates on registration
      registration.addEventListener("updatefound", () => {
        const newWorker = registration.installing;
        if (!newWorker) return;

        newWorker.addEventListener("statechange", () => {
          if (
            newWorker.state === "installed" &&
            navigator.serviceWorker.controller
          ) {
            // New version available
            updateWorker = newWorker;
            update((s) => ({ ...s, updateAvailable: true }));
          }
        });
      });

      // Listen for controller change (new SW activated)
      navigator.serviceWorker.addEventListener("controllerchange", () => {
        update((s) => ({ ...s, updateAvailable: false }));
      });

      // Periodically check for updates (every 60 minutes)
      setInterval(
        () => {
          registration.update();
        },
        60 * 60 * 1000,
      );
    } catch (err) {
      // The app still works without a service worker.
      console.warn("Service worker registration failed:", err);
    }
  }

  async function promptInstall(): Promise<boolean> {
    if (!deferredPrompt) return false;

    try {
      await deferredPrompt.prompt();
      const { outcome } = await deferredPrompt.userChoice;

      if (outcome === "accepted") {
        deferredPrompt = null;
        update((s) => ({ ...s, installable: false }));
        return true;
      }

      return false;
    } catch {
      return false;
    }
  }

  function dismissInstall(): void {
    sessionStorage.setItem("motus_pwa_install_dismissed", "true");
    update((s) => ({ ...s, installDismissed: true }));
  }

  function applyUpdate(): void {
    if (updateWorker) {
      updateWorker.postMessage({ type: "SKIP_WAITING" });
      updateWorker = null;
    }
    // Reload to activate the new service worker
    window.location.reload();
  }

  return {
    subscribe,
    initialize,
    promptInstall,
    dismissInstall,
    applyUpdate,
  };
}

export const pwa = createPwaStore();

export const showInstallBanner = derived(
  pwa,
  ($pwa) => $pwa.installable && !$pwa.installDismissed && !$pwa.installed,
);
