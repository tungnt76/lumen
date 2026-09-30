"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import type { Page } from "@/lib/api";
import { LANGUAGE_NAMES, TEMPLATES, type Project } from "@/lib/code";
import { ApiError, jsonFetch } from "@/lib/studio";
import { PlusIcon } from "./icons";

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1 << 20)).toFixed(1)} MB`;
}

function ago(iso: string) {
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(iso).toLocaleDateString();
}

type Props = { base: string; hrefFor: (id: string) => string; title: string; intro: ReactNode; showOwner?: boolean; signInPath: string; above?: ReactNode };

/** Cards of code projects (first lines, language, files, size), with new, rename and delete. */
export function CodeList({ base, hrefFor, title, intro, showOwner, signInPath, above }: Props) {
  const router = useRouter();
  const [items, setItems] = useState<Project[]>([]);
  const [q, setQ] = useState("");
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [showNew, setShowNew] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [template, setTemplate] = useState(TEMPLATES[0].id);
  const creating = useRef(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await jsonFetch<Page<Project>>(`${base}?q=${encodeURIComponent(q.trim())}`);
      setItems(res.items);
      setTotal(res.total);
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) router.replace(signInPath);
      else setError((err as Error).message);
    } finally {
      setLoaded(true);
    }
  }, [base, q, router, signInPath]);

  useEffect(() => { const t = window.setTimeout(() => void load(), 250); return () => window.clearTimeout(t); }, [load]);

  async function create() {
    if (creating.current) return;
    creating.current = true;
    setBusy(true);
    try {
      const tpl = TEMPLATES.find((t) => t.id === template) ?? TEMPLATES[0];
      const p = await jsonFetch<Project>(base, { method: "POST", body: JSON.stringify({ title: newTitle.trim() || `${tpl.label} project`, language: tpl.id }) });
      // The editor starts from this template until the first save.
      try { window.localStorage.setItem(`lumen:code-template:${p.id}`, JSON.stringify(tpl.id)); } catch { /* ignore */ }
      router.push(hrefFor(p.id));
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
      creating.current = false;
    }
  }

  async function rename(p: Project) {
    const t = window.prompt("Rename project", p.title);
    if (t === null || t.trim() === p.title) return;
    try { await jsonFetch(`${base}/${p.id}`, { method: "PATCH", body: JSON.stringify({ title: t }) }); } catch (err) { setError((err as Error).message); }
    void load();
  }

  async function remove(p: Project) {
    if (!window.confirm(`Delete “${p.title}” with all its files and commits? This can't be undone.`)) return;
    try { await jsonFetch(`${base}/${p.id}`, { method: "DELETE" }); } catch (err) { setError((err as Error).message); }
    void load();
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <h1 style={{ fontSize: "clamp(30px, 4vw, 40px)", marginRight: "auto" }}>{title}{total > 0 && <span className="muted" style={{ fontWeight: 400, fontSize: 20 }}> · {total}</span>}</h1>
        <label htmlFor="cq" className="sr-only">Search projects</label>
        <input id="cq" className="input" style={{ width: 220, height: 44 }} type="search" placeholder="Search projects" value={q} onChange={(e) => setQ(e.target.value)} />
        <button type="button" className="btn btn-primary" onClick={() => setShowNew(!showNew)}><PlusIcon size={18} /> New project</button>
      </div>
      <p className="muted" style={{ margin: 0, fontSize: 14 }}>{intro}</p>
      {showNew && (
        <div className="panel new-project">
          <div className="field" style={{ flex: "2 1 240px" }}>
            <label htmlFor="ntitle">Project name</label>
            <input id="ntitle" className="input" value={newTitle} onChange={(e) => setNewTitle(e.target.value)} placeholder="My project" maxLength={200} autoFocus
              onKeyDown={(e) => { if (e.key === "Enter") void create(); }} />
          </div>
          <div className="field" style={{ flex: "1 1 180px" }}>
            <label htmlFor="ntpl">Start from</label>
            <select id="ntpl" className="input" value={template} onChange={(e) => setTemplate(e.target.value)}>
              {TEMPLATES.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
            </select>
          </div>
          <button type="button" className="btn btn-primary" style={{ alignSelf: "flex-end", height: 46 }} onClick={create} disabled={busy}>{busy ? "Creating…" : "Create"}</button>
        </div>
      )}
      {above}
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      {loaded && !items.length && !error && (
        <div className="drop" style={{ minHeight: 200 }} role="button" tabIndex={0} onClick={() => setShowNew(true)} onKeyDown={(e) => { if (e.key === "Enter") setShowNew(true); }}>
          <PlusIcon size={30} />
          <span style={{ fontWeight: 600 }}>{q ? "No projects match." : "No projects yet. Start your first one."}</span>
        </div>
      )}
      <div className="drawing-grid">
        {items.map((p) => (
          <div key={p.id} className="drawing-card">
            <Link href={hrefFor(p.id)} className="code-thumb" aria-label={`Open ${p.title}`}>
              {p.preview ? <pre>{p.preview}</pre> : <span className="muted">Not saved yet</span>}
            </Link>
            <div className="drawing-meta">
              <Link href={hrefFor(p.id)} className="drawing-title">{p.title}</Link>
              <span className="muted">
                {showOwner && <>{p.ownerName} · </>}
                {p.language && <span className="lang-chip">{LANGUAGE_NAMES[p.language] ?? p.language}</span>}
                {p.savedAt ? ` ${p.fileCount} file${p.fileCount === 1 ? "" : "s"} · ${formatBytes(p.sizeBytes)} · ${ago(p.updatedAt)}` : " Empty"}
              </span>
              <span className="drawing-actions">
                <button type="button" className="link-btn" onClick={() => rename(p)}>Rename</button>
                <button type="button" className="link-btn danger" onClick={() => remove(p)}>Delete</button>
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
