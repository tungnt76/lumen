"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Film, Page } from "@/lib/api";
import { adminFetch, ApiError, putFile, videoType } from "@/lib/studio";
import { UploadIcon } from "@/components/icons";
import { StudioShell } from "@/components/StudioShell";

type Match = { tmdbId: number; title: string; year: number; poster: string };

const statusLabel: Record<Film["status"], [string, string]> = {
  awaiting_upload: ["Awaiting upload", "badge-idle"],
  queued: ["Queued", "badge-idle"],
  encoding: ["Encoding", "badge-encoding"],
  ready: ["Ready", "badge-ready"],
  failed: ["Failed", "badge-failed"],
};

export default function StudioFilms() {
  const router = useRouter();
  const [films, setFilms] = useState<Film[]>([]);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [total, setTotal] = useState(0);
  const [loadError, setLoadError] = useState("");

  const load = useCallback(async () => {
    try {
      const res = await adminFetch<Page<Film>>(`/films?page=${page}`);
      // The last film on this page was deleted: step back to the new last page.
      if (!res.items.length && page > 1 && res.totalPages > 0) { setPage(res.totalPages); return; }
      setFilms(res.items);
      setTotalPages(res.totalPages);
      setTotal(res.total);
      setLoadError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) router.replace("/studio");
      else setLoadError((err as Error).message);
    }
  }, [page, router]);

  useEffect(() => { void load(); }, [load]);

  // New films are listed first, so show page 1 after an upload.
  const showNewest = useCallback(() => { if (page === 1) void load(); else setPage(1); }, [page, load]);

  // Poll while anything is being processed.
  const busy = films.some((f) => f.status === "queued" || f.status === "encoding");
  useEffect(() => {
    if (!busy) return;
    const t = window.setInterval(load, 5000);
    return () => window.clearInterval(t);
  }, [busy, load]);

  return (
    <StudioShell active="/studio/films">
      <h1 style={{ fontSize: 34 }}>Films</h1>
      <UploadForm onCreated={showNewest} />
      <section style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <h2 style={{ fontSize: 20 }}>Library{total > 0 && <span className="muted" style={{ fontWeight: 400 }}> · {total}</span>}</h2>
        {loadError && <p className="error" role="alert">{loadError}</p>}
        <FilmsTable films={films} reload={load} />
        {totalPages > 1 && (
          <nav aria-label="Pages" className="pager">
            <button type="button" className="btn" disabled={page <= 1} onClick={() => setPage(page - 1)}>Previous</button>
            <span className="muted">Page {page} of {totalPages}</span>
            <button type="button" className="btn" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>Next</button>
          </nav>
        )}
      </section>
    </StudioShell>
  );
}

function UploadForm({ onCreated }: { onCreated: () => void }) {
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<Match[]>([]);
  const [picked, setPicked] = useState<Match | null>(null);
  const [rights, setRights] = useState("public_domain");
  const [note, setNote] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [over, setOver] = useState(false);
  const [stage, setStage] = useState<"" | "creating" | "uploading" | "done">("");
  const [pct, setPct] = useState(0);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (query.trim().length < 2) { setMatches([]); return; }
    const t = window.setTimeout(() => {
      adminFetch<Page<Match>>(`/tmdb/search?q=${encodeURIComponent(query.trim())}`).then((m) => setMatches(m.items.slice(0, 6))).catch(() => setMatches([]));
    }, 300);
    return () => window.clearTimeout(t);
  }, [query]);

  function choose(f: File | undefined) {
    if (!f) return;
    if (!videoType(f)) { setError("Choose an .mp4, .mkv, .mov or .webm file."); return; }
    setError("");
    setFile(f);
  }

  async function submit() {
    if (!picked) return setError("Pick the film on TMDB first.");
    if (!file) return setError("Choose the video file.");
    if (note.trim().length < 8) return setError("Add the source link or licence reference.");
    if (!confirm) return setError("Confirm you have the right to publish this film.");
    setError("");
    try {
      setStage("creating");
      const ct = videoType(file);
      const res = await adminFetch<{ film: Film; uploadUrl: string }>("/films", {
        method: "POST",
        body: JSON.stringify({ tmdbId: picked.tmdbId, rights, rightsNote: note.trim(), confirm, contentType: ct }),
      });
      setStage("uploading");
      await putFile(res.uploadUrl, file, ct, setPct);
      await adminFetch(`/films/${res.film.id}/uploaded`, { method: "POST" });
      setStage("done");
      setPicked(null); setQuery(""); setFile(null); setNote(""); setConfirm(false); setPct(0);
      onCreated();
    } catch (err) {
      setError((err as Error).message);
      setStage("");
      onCreated();
    }
  }

  const working = stage === "creating" || stage === "uploading";

  return (
    <section className="panel" style={{ gap: 18, maxWidth: 760 }}>
      <h2>Upload a film</h2>

      <div className="field">
        <label htmlFor="tmdb">Find the film on TMDB</label>
        <input id="tmdb" className="input" type="search" value={query} placeholder="e.g. Metropolis 1927" onChange={(e) => { setQuery(e.target.value); setPicked(null); }} autoComplete="off" />
      </div>
      {matches.length > 0 && !picked && (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: 8 }}>
          {matches.map((m) => (
            <button key={m.tmdbId} type="button" className="match" aria-pressed={false} onClick={() => setPicked(m)}>
              {m.poster ? <img src={m.poster} alt="" /> : <span className="thumb" />}
              <span style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <span style={{ fontWeight: 600 }}>{m.title}</span>
                <span className="muted" style={{ fontSize: 13 }}>{m.year || "Unknown year"} · TMDB {m.tmdbId}</span>
              </span>
            </button>
          ))}
        </div>
      )}
      {picked && (
        <button type="button" className="match" aria-pressed={true} onClick={() => setPicked(null)}>
          {picked.poster ? <img src={picked.poster} alt="" /> : <span className="thumb" />}
          <span style={{ display: "flex", flexDirection: "column", gap: 2, flex: 1 }}>
            <span style={{ fontWeight: 600 }}>{picked.title} ({picked.year || "?"})</span>
            <span className="muted" style={{ fontSize: 13 }}>TMDB {picked.tmdbId} · click to change</span>
          </span>
          <span style={{ color: "var(--accent-text)", fontSize: 13, fontWeight: 600 }}>Selected</span>
        </button>
      )}

      <div
        className={`drop${over ? " over" : ""}`}
        role="button"
        tabIndex={0}
        onClick={() => input.current?.click()}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); input.current?.click(); } }}
        onDragOver={(e) => { e.preventDefault(); setOver(true); }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => { e.preventDefault(); setOver(false); choose(e.dataTransfer.files[0]); }}
      >
        <UploadIcon size={30} />
        <span style={{ fontWeight: 600 }}>{file ? file.name : "Drop the video file here, or click to browse"}</span>
        <span className="muted" style={{ fontSize: 13 }}>
          {file ? `${(file.size / 1e9).toFixed(2)} GB` : ".mp4, .mkv, .mov or .webm, up to 5 GB. Encoded to HLS 360p/720p/1080p."}
        </span>
        <input ref={input} type="file" accept="video/mp4,video/x-matroska,video/quicktime,video/webm,.mkv" hidden onChange={(e) => choose(e.target.files?.[0])} />
      </div>

      <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
        <div className="field" style={{ flex: "1 1 200px" }}>
          <label htmlFor="rights">Rights basis</label>
          <select id="rights" className="input" value={rights} onChange={(e) => setRights(e.target.value)}>
            <option value="public_domain">Public domain</option>
            <option value="licensed">Licensed by the rights holder</option>
            <option value="own">Our own production</option>
          </select>
        </div>
        <div className="field" style={{ flex: "2 1 300px" }}>
          <label htmlFor="note">Source link or licence reference</label>
          <input id="note" className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="https://archive.org/details/…" />
        </div>
      </div>

      <label style={{ display: "flex", gap: 12, alignItems: "flex-start", fontSize: 14, lineHeight: 1.5, color: "var(--text-2)" }}>
        <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} style={{ width: 20, height: 20, margin: "1px 0 0", accentColor: "var(--accent)" }} />
        I confirm this film is in the public domain in the countries we serve, or we have written permission to publish it.
      </label>

      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      {stage === "uploading" && (
        <div aria-live="polite" style={{ fontSize: 14 }}>
          Uploading {pct}%<div className="bar"><span style={{ width: `${pct}%` }} /></div>
        </div>
      )}
      {stage === "done" && <p style={{ margin: 0, color: "var(--ok)", fontSize: 14 }} aria-live="polite">Uploaded. The worker will encode it; publish it from the library once it&apos;s ready.</p>}

      <div className="actions">
        <button type="button" className="btn btn-primary" disabled={working} onClick={submit}>
          {stage === "creating" ? "Preparing…" : stage === "uploading" ? "Uploading…" : "Upload and encode"}
        </button>
      </div>
    </section>
  );
}

function FilmsTable({ films, reload }: { films: Film[]; reload: () => void }) {
  const [error, setError] = useState("");

  async function patch(f: Film, body: Partial<Pick<Film, "published" | "featured">>) {
    setError("");
    try { await adminFetch(`/films/${f.id}`, { method: "PATCH", body: JSON.stringify(body) }); }
    catch (err) { setError(`${f.title}: ${(err as Error).message}`); }
    reload();
  }

  async function remove(f: Film) {
    if (!window.confirm(`Delete “${f.title}” and all its video files? This can't be undone.`)) return;
    setError("");
    try { await adminFetch(`/films/${f.id}`, { method: "DELETE" }); }
    catch (err) { setError(`${f.title}: ${(err as Error).message}`); }
    reload();
  }

  async function retry(f: Film) {
    setError("");
    try { await adminFetch(`/films/${f.id}/uploaded`, { method: "POST" }); }
    catch (err) { setError(`${f.title}: ${(err as Error).message}`); }
    reload();
  }

  if (!films.length) return <p className="muted">No films yet. Upload the first one above.</p>;

  return (
    <>
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr><th>Title</th><th>Rights</th><th>Status</th><th>Subtitles</th><th>Published</th><th>Featured</th><th><span className="sr-only">Actions</span></th></tr>
          </thead>
          <tbody>
            {films.map((f) => {
              const [label, cls] = statusLabel[f.status];
              return (
                <tr key={f.id}>
                  <td>
                    <div style={{ fontWeight: 600 }}>{f.title}</div>
                    <div className="muted" style={{ fontSize: 13 }}>{f.year || ""} · TMDB {f.tmdbId}</div>
                  </td>
                  <td>
                    <div>{f.rights === "public_domain" ? "Public domain" : f.rights === "licensed" ? "Licensed" : "Own"}</div>
                    <div className="muted" style={{ fontSize: 12, maxWidth: 220, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={f.rightsNote}>{f.rightsNote}</div>
                  </td>
                  <td style={{ minWidth: 160 }}>
                    <span className={`badge ${cls}`}>{label}{f.status === "encoding" ? ` ${f.progress}%` : ""}</span>
                    {f.status === "encoding" && <div className="bar"><span style={{ width: `${f.progress}%` }} /></div>}
                    {f.status === "ready" && <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>{f.renditions.join(" · ")}</div>}
                    {f.status === "failed" && (
                      <div style={{ fontSize: 12, marginTop: 4 }}>
                        <span className="error" style={{ fontSize: 12 }}>{f.error}</span>{" "}
                        <button type="button" onClick={() => retry(f)} style={{ background: "none", border: 0, color: "var(--accent-text)", cursor: "pointer", padding: 0 }}>Retry</button>
                      </div>
                    )}
                  </td>
                  <td><SubtitleCell film={f} onDone={reload} /></td>
                  <td>
                    <input type="checkbox" aria-label={`Publish ${f.title}`} checked={f.published} disabled={f.status !== "ready"}
                      onChange={(e) => patch(f, { published: e.target.checked })} style={{ width: 20, height: 20, accentColor: "var(--accent)" }} />
                  </td>
                  <td>
                    <input type="checkbox" aria-label={`Feature ${f.title} on the home page`} checked={f.featured}
                      onChange={(e) => patch(f, { featured: e.target.checked })} style={{ width: 20, height: 20, accentColor: "var(--accent)" }} />
                  </td>
                  <td style={{ whiteSpace: "nowrap" }}>
                    {f.published && <Link href={`/movie/${f.tmdbId}`} target="_blank" style={{ marginRight: 14, fontSize: 14 }}>View</Link>}
                    <button type="button" onClick={() => remove(f)} style={{ background: "none", border: 0, color: "var(--danger)", cursor: "pointer", fontSize: 14 }}>Delete</button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </>
  );
}

function SubtitleCell({ film, onDone }: { film: Film; onDone: () => void }) {
  const [lang, setLang] = useState("vi");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);

  async function upload(file?: File) {
    if (!file) return;
    if (!file.name.toLowerCase().endsWith(".vtt")) { setError("Use a .vtt file"); return; }
    setBusy(true);
    setError("");
    try {
      const { uploadUrl } = await adminFetch<{ uploadUrl: string }>(`/films/${film.id}/subtitles`, { method: "POST", body: JSON.stringify({ lang }) });
      await putFile(uploadUrl, file, "text/vtt", () => {});
      onDone();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
      if (input.current) input.current.value = "";
    }
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 13 }}>
      <span>{film.subtitles.length ? film.subtitles.join(", ") : <span className="muted">None</span>}</span>
      <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
        <label htmlFor={`lang-${film.id}`} className="sr-only">Subtitle language</label>
        <select id={`lang-${film.id}`} className="input" style={{ height: 32, width: 64, fontSize: 13 }} value={lang} onChange={(e) => setLang(e.target.value)}>
          {["vi", "en", "fr", "de", "ja", "ko", "zh"].map((l) => <option key={l}>{l}</option>)}
        </select>
        <button type="button" className="btn" style={{ height: 32, padding: "0 10px", fontSize: 13 }} disabled={busy} onClick={() => input.current?.click()}>
          {busy ? "…" : "Add .vtt"}
        </button>
        <input ref={input} type="file" accept=".vtt,text/vtt" hidden onChange={(e) => upload(e.target.files?.[0])} />
      </div>
      {error && <span className="error" style={{ fontSize: 12 }}>{error}</span>}
    </div>
  );
}
