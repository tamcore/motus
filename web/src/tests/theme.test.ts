import { describe, it, expect, vi } from "vitest";
import { get } from "svelte/store";

const query = {
  matches: false,
  listeners: new Set<() => void>(),
  addEventListener: (_: string, fn: () => void) => query.listeners.add(fn),
  removeEventListener: (_: string, fn: () => void) => query.listeners.delete(fn),
};
vi.stubGlobal("matchMedia", () => query);

const { isDark, theme } = await import("$lib/stores/theme");

describe("isDark", () => {
  it("follows the OS preference in auto mode and the explicit choice otherwise", () => {
    const seen: boolean[] = [];
    const unsubscribe = isDark.subscribe((v) => seen.push(v));

    query.matches = true;
    query.listeners.forEach((fn) => fn());
    theme.setTheme("light");
    theme.setTheme("dark");

    expect(seen).toEqual([false, true, false, true]);
    expect(query.listeners.size).toBe(0);
    unsubscribe();
  });

  it("sets data-theme on the document", () => {
    theme.setTheme("light");
    theme.initialize();
    expect(document.documentElement.dataset.theme).toBe("light");
    theme.setTheme("dark");
    expect(get(isDark)).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("dark");
  });
});
