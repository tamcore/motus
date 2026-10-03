import { describe, it, expect, beforeEach, vi } from "vitest";
import { get } from "svelte/store";
import { persisted } from "$lib/stores/persisted";

describe("persisted store", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("reads JSON and legacy plain-string values and writes JSON", () => {
    localStorage.setItem("test_key", "dark");
    const store = persisted("test_key", "auto");
    expect(get(store)).toBe("dark");
    store.set("light");
    expect(localStorage.getItem("test_key")).toBe('"light"');
  });

  it("falls back to the initial value when parse rejects the stored value", () => {
    localStorage.setItem("test_key", JSON.stringify("bogus"));
    const store = persisted("test_key", "auto", (v) => (v === "dark" ? v : null));
    expect(get(store)).toBe("auto");
  });

  it("keeps working when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const store = persisted("test_key", 1);
    store.set(2);
    expect(get(store)).toBe(2);
  });
});
