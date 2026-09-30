"use client";

import { useEffect, useState } from "react";
import { getTheme, onThemeChange, setTheme, type Theme } from "@/lib/theme";
import { MoonIcon, SunIcon } from "./icons";

/** Header button: one tap switches between dark (default) and light. */
export function ThemeToggle({ className = "icon-btn" }: { className?: string }) {
  const [theme, setT] = useState<Theme>("dark");
  useEffect(() => {
    const read = () => setT(getTheme());
    read();
    return onThemeChange(read);
  }, []);
  const next = theme === "dark" ? "light" : "dark";
  return (
    <button type="button" className={className} onClick={() => setTheme(next)} aria-label={`Switch to ${next} theme`} title={`Switch to ${next} theme`}>
      {theme === "dark" ? <SunIcon size={19} /> : <MoonIcon size={18} />}
    </button>
  );
}
