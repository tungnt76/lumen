// Colour theme: dark by default; light when the viewer picks it with the header button.
// The choice is saved on this device; <html data-theme="light"> drives the CSS in globals.css.

export type Theme = "light" | "dark";

const KEY = "lumen:theme";
const EVENT = "lumen:theme";

/** Runs inline in <head> before first paint, so a saved light choice never flashes dark first. */
export const themeScript = `try{if(localStorage.getItem("${KEY}")==="light")document.documentElement.dataset.theme="light"}catch(e){}`;

export function getTheme(): Theme {
  try {
    return window.localStorage.getItem(KEY) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

export function setTheme(t: Theme) {
  try {
    if (t === "light") window.localStorage.setItem(KEY, "light");
    else window.localStorage.removeItem(KEY);
  } catch {
    /* storage blocked: the choice lasts until the page is closed */
  }
  const root = document.documentElement;
  if (t === "light") root.dataset.theme = "light";
  else delete root.dataset.theme;
  // Keep the browser UI (address bar on phones) in step with the page.
  document.querySelectorAll('meta[name="theme-color"]').forEach((m) => m.setAttribute("content", t === "light" ? "#efeae2" : "#0e0e10"));
  window.dispatchEvent(new Event(EVENT));
}

/** Calls cb when the theme changes here or in another tab. */
export function onThemeChange(cb: () => void) {
  const onStorage = (e: StorageEvent) => { if (e.key === KEY) { setTheme(getTheme()); cb(); } };
  window.addEventListener(EVENT, cb);
  window.addEventListener("storage", onStorage);
  return () => {
    window.removeEventListener(EVENT, cb);
    window.removeEventListener("storage", onStorage);
  };
}
