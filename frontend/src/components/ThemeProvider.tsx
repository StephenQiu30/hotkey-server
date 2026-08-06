"use client";

import { createContext, useContext } from "react";

type Theme = "light" | "dark";

const ThemeContext = createContext({
  theme: "light" as Theme,
  toggleTheme: () => {},
  setTheme: (_theme: Theme) => {},
});

export function useTheme() { return useContext(ThemeContext); }

export function ThemeScript() {
  return <script dangerouslySetInnerHTML={{ __html: "document.documentElement.dataset.theme='light'" }} />;
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return <ThemeContext.Provider value={{ theme: "light", toggleTheme: () => {}, setTheme: () => {} }}>{children}</ThemeContext.Provider>;
}
