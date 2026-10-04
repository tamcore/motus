import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

// jsdom lacks the modal dialog API.
HTMLDialogElement.prototype.showModal ??= function (this: HTMLDialogElement) {
  this.setAttribute("open", "");
};
// jsdom lacks Web Animations, which Svelte transitions use; finish them at once.
Element.prototype.animate ??= function () {
  const animation = { onfinish: null as null | (() => void), cancel() {}, playState: "finished", effect: null };
  queueMicrotask(() => animation.onfinish?.());
  return animation as unknown as Animation;
};
// jsdom lacks ResizeObserver, which Svelte `bind:clientHeight` uses.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
HTMLDialogElement.prototype.close ??= function (this: HTMLDialogElement) {
  if (!this.hasAttribute("open")) return;
  this.removeAttribute("open");
  this.dispatchEvent(new Event("close"));
};
