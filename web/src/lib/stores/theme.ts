import { derived } from "svelte/store";
import { persisted } from "./persisted";

type Theme = "dark" | "light" | "auto";

const THEMES: readonly string[] = ["dark", "light", "auto"];

const stored = persisted<Theme>("motus_theme", "auto", (value) =>
  THEMES.includes(value as string) ? (value as Theme) : null,
);

export const isDark = derived<typeof stored, boolean>(stored, (t, set) => {
  if (t !== "auto") {
    set(t === "dark");
    return;
  }
  const query = window.matchMedia("(prefers-color-scheme: dark)");
  const update = () => set(query.matches);
  update();
  query.addEventListener("change", update);
  return () => query.removeEventListener("change", update);
});

export const theme = {
  subscribe: stored.subscribe,
  setTheme: (value: Theme) => stored.set(value),
  initialize: () => {
    isDark.subscribe((dark) =>
      document.documentElement.setAttribute("data-theme", dark ? "dark" : "light"),
    );
  },
};
