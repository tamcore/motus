import { get } from "svelte/store";
import { persisted } from "./persisted";

type Theme = "dark" | "light" | "auto";

const THEMES: readonly string[] = ["dark", "light", "auto"];

const stored = persisted<Theme>("motus_theme", "auto", (value) =>
  THEMES.includes(value as string) ? (value as Theme) : null,
);

function applyTheme(theme: Theme): void {
  const effective =
    theme === "auto"
      ? window.matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : theme;
  document.documentElement.setAttribute("data-theme", effective);
}

export const theme = {
  subscribe: stored.subscribe,
  setTheme: (value: Theme) => {
    stored.set(value);
    applyTheme(value);
  },
  initialize: () => {
    applyTheme(get(stored));
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
      if (get(stored) === "auto") applyTheme("auto");
    });
  },
};
