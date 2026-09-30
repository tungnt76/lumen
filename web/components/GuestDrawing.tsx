"use client";

import "@excalidraw/excalidraw/index.css";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ExcalidrawImperativeAPI, ExcalidrawInitialDataState } from "@excalidraw/excalidraw/types";
import { getTheme, onThemeChange, type Theme } from "@/lib/theme";

const Excalidraw = dynamic(async () => (await import("@excalidraw/excalidraw")).Excalidraw, {
  ssr: false,
  loading: () => <div className="draw-loading muted">Loading the canvas…</div>,
});

/** Where a visitor's canvas is kept in their browser (offered for saving after sign-in). */
export const GUEST_KEY = "lumen:drawing";
const KEY = GUEST_KEY;

/**
 * The Excalidraw tab for visitors: a free canvas saved only in their own browser (nothing
 * reaches the server). Signing in turns the tab into "My drawings", saved to R2.
 */
export function GuestDrawing() {
  const api = useRef<ExcalidrawImperativeAPI | null>(null);
  const [initial, setInitial] = useState<ExcalidrawInitialDataState | null>(null);
  const [theme, setTheme] = useState<Theme>("dark");
  const [note, setNote] = useState("");
  const timer = useRef<number | undefined>(undefined);
  const lastVersion = useRef(-1);

  useEffect(() => {
    const read = () => setTheme(getTheme());
    read();
    return onThemeChange(read);
  }, []);

  // Restore the visitor's last drawing from this browser.
  useEffect(() => {
    (async () => {
      let data: ExcalidrawInitialDataState = {};
      try {
        const raw = window.localStorage.getItem(KEY);
        if (raw) {
          const { restore } = await import("@excalidraw/excalidraw");
          const r = restore(JSON.parse(raw), null, null);
          data = { elements: r.elements, appState: r.appState, files: r.files, scrollToContent: true };
        }
      } catch {
        setNote("Your previous drawing couldn't be restored, so this is a fresh canvas.");
      }
      setInitial(data);
    })();
  }, []);

  const persist = useCallback(async () => {
    const ex = api.current;
    if (!ex) return;
    const { serializeAsJSON } = await import("@excalidraw/excalidraw");
    try {
      window.localStorage.setItem(KEY, serializeAsJSON(ex.getSceneElementsIncludingDeleted(), ex.getAppState(), ex.getFiles(), "local"));
      setNote("");
    } catch {
      // Usually the browser's storage quota, when large images are pasted in.
      setNote("This drawing is too big to keep in your browser. Export it from the menu (☰ → Save to…) to keep a copy.");
    }
  }, []);

  // Save shortly after the drawing stops changing, and when leaving the page.
  const onChange = useCallback(async (elements: readonly { version: number }[]) => {
    const { getSceneVersion } = await import("@excalidraw/excalidraw");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const v = getSceneVersion(elements as any);
    if (v === lastVersion.current) return; // pointer moves and zooming don't change the drawing
    lastVersion.current = v;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => void persist(), 800);
  }, [persist]);

  useEffect(() => {
    const flush = () => void persist();
    window.addEventListener("pagehide", flush);
    return () => { window.removeEventListener("pagehide", flush); window.clearTimeout(timer.current); };
  }, [persist]);

  return (
    <div className="guest-draw">
      <div className="guest-draw-bar">
        <span style={{ marginRight: "auto" }}>Draw anything. It&apos;s kept in this browser only; use the menu (☰) to export PNG, SVG or an .excalidraw file.</span>
        {note && <span className="error" role="status">{note}</span>}
        <Link href="/login?next=/draw" className="btn btn-primary guest-save">Sign in to save</Link>
      </div>
      <div className="draw-canvas">
        {initial && (
          <Excalidraw
            excalidrawAPI={(a) => { api.current = a; }}
            initialData={initial}
            theme={theme}
            onChange={(els) => void onChange(els)}
          />
        )}
        {!initial && <div className="draw-loading muted">Loading the canvas…</div>}
      </div>
    </div>
  );
}
