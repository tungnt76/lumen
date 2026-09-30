"use client";

import "@excalidraw/excalidraw/index.css";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ExcalidrawImperativeAPI, ExcalidrawInitialDataState } from "@excalidraw/excalidraw/types";
import type { Drawing } from "@/lib/api";
import { previewOf, saveScene } from "@/lib/drawings";
import { ApiError, jsonFetch } from "@/lib/studio";
import { getTheme } from "@/lib/theme";
import { BackIcon } from "./icons";

// Excalidraw touches window at import time, so it only loads in the browser.
const Excalidraw = dynamic(async () => (await import("@excalidraw/excalidraw")).Excalidraw, {
  ssr: false,
  loading: () => <div className="draw-loading muted">Loading the canvas…</div>,
});

type Status = { kind: "idle" | "saving" | "saved" | "error"; text: string };

type EditorProps = {
  id: string;
  base: string; // /api/drawings (your own) or /api/admin/drawings (studio)
  backHref: string;
  signInPath: string;
  embedded?: boolean; // inside the site layout (below the nav) rather than full screen
};

export function DrawingEditor({ id, base, backHref, signInPath, embedded }: EditorProps) {
  const router = useRouter();
  const api = useRef<ExcalidrawImperativeAPI | null>(null);
  const [drawing, setDrawing] = useState<Drawing | null>(null);
  const [initial, setInitial] = useState<ExcalidrawInitialDataState | null>(null);
  const [title, setTitle] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle", text: "" });
  const [dirty, setDirty] = useState(false);
  const savedVersion = useRef(-1); // scene version at the last save or load
  const saving = useRef(false);

  // Load the drawing and, if it has been saved before, its scene from storage.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await jsonFetch<{ drawing: Drawing; sceneUrl: string }>(`${base}/${id}`);
        let data: ExcalidrawInitialDataState = { appState: { viewBackgroundColor: "#ffffff" } };
        if (res.sceneUrl) {
          const scene = await fetch(res.sceneUrl);
          if (!scene.ok) throw new Error(`Couldn't load the drawing from storage (${scene.status})`);
          const { restore } = await import("@excalidraw/excalidraw");
          const restored = restore(await scene.json(), null, null);
          data = { elements: restored.elements, appState: restored.appState, files: restored.files, scrollToContent: true };
        }
        if (cancelled) return;
        setDrawing(res.drawing);
        setTitle(res.drawing.title);
        setInitial(data);
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) router.replace(signInPath);
        else if (err instanceof ApiError && err.status === 404) router.replace(backHref);
        else if (!cancelled) setStatus({ kind: "error", text: (err as Error).message });
      }
    })();
    return () => { cancelled = true; };
  }, [id, base, backHref, signInPath, router]);

  const save = useCallback(async () => {
    const ex = api.current;
    if (!ex || saving.current) return;
    saving.current = true;
    setStatus({ kind: "saving", text: "Saving…" });
    try {
      const { serializeAsJSON, getSceneVersion } = await import("@excalidraw/excalidraw");
      const elements = ex.getSceneElements();
      const appState = ex.getAppState();
      const files = ex.getFiles();
      // Same basis as onChange, which receives deleted elements too.
      const version = getSceneVersion(ex.getSceneElementsIncludingDeleted());
      const json = serializeAsJSON(elements, appState, files, "local");
      const d = await saveScene(base, id, json, await previewOf({ elements, appState, files }));
      setDrawing(d);
      savedVersion.current = version;
      setDirty(getSceneVersion(ex.getSceneElementsIncludingDeleted()) !== version);
      setStatus({ kind: "saved", text: "Saved to R2" });
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
    } finally {
      saving.current = false;
    }
  }, [id, base]);

  // Ctrl/Cmd+S saves to R2 instead of downloading a file.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        e.stopPropagation();
        void save();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [save]);

  // Warn before leaving with unsaved changes.
  useEffect(() => {
    if (!dirty) return;
    const onLeave = (e: BeforeUnloadEvent) => { e.preventDefault(); };
    window.addEventListener("beforeunload", onLeave);
    return () => window.removeEventListener("beforeunload", onLeave);
  }, [dirty]);

  async function rename() {
    const t = title.trim() || "Untitled drawing";
    if (!drawing || t === drawing.title) { setTitle(t); return; }
    try {
      const d = await jsonFetch<Drawing>(`${base}/${id}`, { method: "PATCH", body: JSON.stringify({ title: t }) });
      setDrawing(d);
      setTitle(d.title);
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
    }
  }

  const onChange = useCallback(async (elements: readonly { version: number }[]) => {
    const { getSceneVersion } = await import("@excalidraw/excalidraw");
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const v = getSceneVersion(elements as any);
    if (savedVersion.current === -1) { savedVersion.current = v; return; } // first render after load
    setDirty(v !== savedVersion.current);
  }, []);

  function back(e: React.MouseEvent) {
    if (dirty && !window.confirm("You have unsaved changes. Leave without saving?")) e.preventDefault();
  }

  return (
    <div className={embedded ? "draw-page embedded" : "draw-page"}>
      <header className="draw-bar">
        <Link href={backHref} className="icon-btn" aria-label="Back to drawings" onClick={back}><BackIcon size={20} /></Link>
        <label htmlFor="dtitle" className="sr-only">Drawing title</label>
        <input id="dtitle" className="draw-title" value={title} onChange={(e) => setTitle(e.target.value)} onBlur={rename}
          onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} maxLength={200} disabled={!drawing} />
        <span className={`draw-status ${status.kind}`} role="status" aria-live="polite">
          {status.kind === "error" ? status.text : dirty ? "Unsaved changes" : status.text}
        </span>
        <button type="button" className="btn btn-primary" style={{ height: 40 }} onClick={() => void save()} disabled={!drawing || status.kind === "saving"}>
          {status.kind === "saving" ? "Saving…" : "Save"}
        </button>
      </header>
      <div className="draw-canvas">
        {initial && (
          <Excalidraw
            excalidrawAPI={(a) => { api.current = a; }}
            initialData={initial}
            theme={getTheme()}
            name={title}
            onChange={(els) => void onChange(els)}
            UIOptions={{ canvasActions: { saveToActiveFile: false, loadScene: true, export: { saveFileToDisk: true } } }}
          />
        )}
      </div>
    </div>
  );
}
