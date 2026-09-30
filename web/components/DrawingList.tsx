"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import type { Drawing, Page } from "@/lib/api";
import { ApiError, jsonFetch } from "@/lib/studio";
import { PlusIcon } from "./icons";

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1 << 20)).toFixed(1)} MB`;
}

function timeAgo(iso: string) {
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(iso).toLocaleDateString();
}

type Props = {
  base: string; // /api/drawings or /api/admin/drawings
  hrefFor: (id: string) => string;
  title: string;
  intro: ReactNode;
  showOwner?: boolean;
  signInPath: string; // where to go when the session has ended
  above?: ReactNode;
};

/** Grid of drawings with previews, search, new, rename and delete. */
export function DrawingList({ base, hrefFor, title, intro, showOwner, signInPath, above }: Props) {
  const router = useRouter();
  const [items, setItems] = useState<Drawing[]>([]);
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [creating, setCreating] = useState(false);
  const creatingRef = useRef(false); // a double click fires twice before the button re-renders disabled

  const load = useCallback(async () => {
    try {
      const qs = new URLSearchParams({ page: String(page) });
      if (q.trim()) qs.set("q", q.trim());
      const res = await jsonFetch<Page<Drawing>>(`${base}?${qs}`);
      if (!res.items.length && page > 1 && res.totalPages > 0) { setPage(res.totalPages); return; }
      setItems(res.items);
      setTotalPages(res.totalPages);
      setTotal(res.total);
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) router.replace(signInPath);
      else setError((err as Error).message);
    } finally {
      setLoaded(true);
    }
  }, [base, page, q, router, signInPath]);

  useEffect(() => {
    const t = window.setTimeout(() => void load(), 250);
    return () => window.clearTimeout(t);
  }, [load]);

  async function create() {
    if (creatingRef.current) return;
    creatingRef.current = true;
    setCreating(true);
    try {
      const d = await jsonFetch<Drawing>(base, { method: "POST", body: JSON.stringify({ title: "Untitled drawing" }) });
      router.push(hrefFor(d.id));
    } catch (err) {
      setError((err as Error).message);
      setCreating(false);
      creatingRef.current = false;
    }
  }

  async function rename(d: Drawing) {
    const t = window.prompt("Rename drawing", d.title);
    if (t === null || t.trim() === d.title) return;
    try { await jsonFetch(`${base}/${d.id}`, { method: "PATCH", body: JSON.stringify({ title: t }) }); }
    catch (err) { setError((err as Error).message); }
    void load();
  }

  async function remove(d: Drawing) {
    if (!window.confirm(`Delete “${d.title}”? Its file is removed from storage and can't be recovered.`)) return;
    try { await jsonFetch(`${base}/${d.id}`, { method: "DELETE" }); }
    catch (err) { setError((err as Error).message); }
    void load();
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <h1 style={{ fontSize: "clamp(30px, 4vw, 40px)", marginRight: "auto" }}>{title}{total > 0 && <span className="muted" style={{ fontWeight: 400, fontSize: 20 }}> · {total}</span>}</h1>
        <label htmlFor="dq" className="sr-only">Search drawings</label>
        <input id="dq" className="input" style={{ width: 220, height: 44 }} type="search" placeholder="Search drawings" value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} />
        <button type="button" className="btn btn-primary" onClick={create} disabled={creating}><PlusIcon size={18} /> {creating ? "Creating…" : "New drawing"}</button>
      </div>
      <p className="muted" style={{ margin: 0, fontSize: 14 }}>{intro}</p>
      {above}
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}

      {loaded && !items.length && !error && (
        <div className="drop" style={{ minHeight: 220 }} role="button" tabIndex={0} onClick={create} onKeyDown={(e) => { if (e.key === "Enter") void create(); }}>
          <PlusIcon size={30} />
          <span style={{ fontWeight: 600 }}>{q ? "No drawings match." : "No drawings yet. Start your first one."}</span>
        </div>
      )}

      <div className="drawing-grid">
        {items.map((d) => (
          <div key={d.id} className="drawing-card">
            <Link href={hrefFor(d.id)} className="drawing-thumb" aria-label={`Open ${d.title}`}>
              {d.previewUrl ? <img src={d.previewUrl} alt="" loading="lazy" onError={(e) => { e.currentTarget.style.display = "none"; }} /> : <span className="muted">Not saved yet</span>}
            </Link>
            <div className="drawing-meta">
              <Link href={hrefFor(d.id)} className="drawing-title">{d.title}</Link>
              <span className="muted">
                {showOwner && <>{d.ownerName || "Studio"} · </>}
                {d.savedAt ? `${timeAgo(d.updatedAt)} · ${formatBytes(d.sizeBytes)}` : "Empty"}
              </span>
              <span className="drawing-actions">
                <button type="button" className="link-btn" onClick={() => rename(d)}>Rename</button>
                <button type="button" className="link-btn danger" onClick={() => remove(d)}>Delete</button>
              </span>
            </div>
          </div>
        ))}
      </div>

      {totalPages > 1 && (
        <nav aria-label="Pages" className="pager">
          <button type="button" className="btn" disabled={page <= 1} onClick={() => setPage(page - 1)}>Previous</button>
          <span className="muted">Page {page} of {totalPages}</span>
          <button type="button" className="btn" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Next</button>
        </nav>
      )}
    </div>
  );
}
