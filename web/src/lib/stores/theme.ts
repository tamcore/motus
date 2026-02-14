import { writable } from "svelte/store";
import { browser } from "$app/environment";

type Theme = "dark" | "light" | "auto";

function getInitialTheme(): Theme {
  if (!browser) return "auto";

  const stored = localStorage.getItem("motus_theme") as Theme;
  if (stored && ["dark", "light", "auto"].includes(stored)) {
    return stored;
  }

  return "auto";
}

function getEffectiveTheme(theme: Theme): "dark" | "light" {
  if (theme === "auto" && browser) {
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return theme === "auto" ? "dark" : theme;
}

function createThemeStore() {
  const { subscribe, set } = writable<Theme>(getInitialTheme());

  return {
    subscribe,
    setTheme: (theme: Theme) => {
      if (browser) {
        localStorage.setItem("motus_theme", theme);
        document.documentElement.setAttribute(
          "data-theme",
          getEffectiveTheme(theme),
        );
      }
      set(theme);
    },
    initialize: () => {
      if (browser) {
        const theme = getInitialTheme();
        document.documentElement.setAttribute(
          "data-theme",
          getEffectiveTheme(theme),
        );

        const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
        mediaQuery.addEventListener("change", () => {
          const currentTheme = localStorage.getItem("motus_theme") as Theme;
          if (currentTheme === "auto") {
            document.documentElement.setAttribute(
              "data-theme",
              getEffectiveTheme("auto"),
            );
          }
        });
      }
    },
  };
}

export const theme = createThemeStore();
