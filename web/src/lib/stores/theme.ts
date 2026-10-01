import { writable } from "svelte/store";

type Theme = "dark" | "light" | "auto";

function getInitialTheme(): Theme {
  const stored = localStorage.getItem("motus_theme") as Theme;
  if (stored && ["dark", "light", "auto"].includes(stored)) {
    return stored;
  }

  return "auto";
}

function getEffectiveTheme(theme: Theme): "dark" | "light" {
  if (theme === "auto") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }
  return theme;
}

function createThemeStore() {
  const { subscribe, set } = writable<Theme>(getInitialTheme());

  return {
    subscribe,
    setTheme: (theme: Theme) => {
      localStorage.setItem("motus_theme", theme);
      document.documentElement.setAttribute(
        "data-theme",
        getEffectiveTheme(theme),
      );
      set(theme);
    },
    initialize: () => {
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
    },
  };
}

export const theme = createThemeStore();
