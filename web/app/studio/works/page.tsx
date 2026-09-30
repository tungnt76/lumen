"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { formatDuration, languageNames, toneFor, workHref, type Page, type StudioTrack, type StudioWork } from "@/lib/api";
import { adminFetch, ApiError, audioDuration, audioType, putFile } from "@/lib/studio";
import { clock } from "@/components/AudioPlayer";
import { UploadIcon } from "@/components/icons";
import { StudioShell } from "@/components/StudioShell";

const licenseNames: Record<string, string> = {
  public_domain: "Public domain", cc0: "CC0", cc_by: "CC BY", cc_by_sa: "CC BY-SA", cc_by_nd: "CC BY-ND",
  cc_by_nc: "CC BY-NC", cc_by_nc_sa: "CC BY-NC-SA", cc_by_nc_nd: "CC BY-NC-ND", licensed: "Licensed", own: "Our own", external: "Link-out only",
};

export default function StudioWorks() {
  const router = useRouter();
  const [works, setWorks] = useState<StudioWork[]>([]);
  const [kind, setKind] = useState("");
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [total, setTotal] = useState(0);
  const [editing, setEditing] = useState<string | null>(null);
  const [loadError, setLoadError] = useState("");

  const load = useCallback(async () => {
    try {
      const qs = new URLSearchParams({ page: String(page) });
      if (kind) qs.set("kind", kind);
      if (q.trim()) qs.set("q", q.trim());
      const res = await adminFetch<Page<StudioWork>>(`/works?${qs}`);
      if (!res.items.length && page > 1 && res.totalPages > 0) { setPage(res.totalPages); return; }
      setWorks(res.items);
      setTotalPages(res.totalPages);
      setTotal(res.total);
      setLoadError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) router.replace("/studio");
      else setLoadError((err as Error).message);
    }
  }, [page, kind, q, router]);

  useEffect(() => {
    const t = window.setTimeout(() => void load(), 250); // debounce typing in search
    return () => window.clearTimeout(t);
  }, [load]);

  const showNewest = useCallback(() => { if (page === 1) void load(); else setPage(1); }, [page, load]);

  return (
    <StudioShell active="/studio/works">
      <h1 style={{ fontSize: 34 }}>Books &amp; music</h1>

      {editing !== null && (
        <TrackEditor id={editing} onClose={() => { setEditing(null); void load(); }} onChanged={load} />
      )}

      <div className="studio-cols">
        <ImportPanel onImported={showNewest} />
        <CreatePanel onCreated={(id) => { showNewest(); setEditing(id); }} />
      </div>

      <section style={{ display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <h2 style={{ fontSize: 20, marginRight: "auto" }}>Library{total > 0 && <span className="muted" style={{ fontWeight: 400 }}> · {total}</span>}</h2>
          <label htmlFor="wq" className="sr-only">Search the library</label>
          <input id="wq" className="input" style={{ width: 240, height: 40 }} type="search" placeholder="Search (no accents needed)" value={q}
            onChange={(e) => { setQ(e.target.value); setPage(1); }} />
          <div className="pills" role="group" aria-label="Kind">
            {[["", "All"], ["book", "Books"], ["album", "Albums"]].map(([k, label]) => (
              <button key={k} type="button" className="pill" style={{ height: 40 }} aria-current={kind === k ? "true" : undefined} onClick={() => { setKind(k); setPage(1); }}>{label}</button>
            ))}
          </div>
        </div>
        {loadError && <p className="error" role="alert">{loadError}</p>}
        <WorksTable works={works} reload={load} onEdit={setEditing} />
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

const sources = {
  librivox: { label: "LibriVox audiobook", hint: "Book id or RSS link, e.g. 52 or librivox.org/rss/52", note: "Public-domain recordings. Audio stays on archive.org." },
  archive: { label: "Internet Archive item", hint: "Identifier or link, e.g. musopen-chopin", note: "Refused unless the item is public domain or Creative Commons." },
  musicbrainz: { label: "MusicBrainz artist (link-out)", hint: "Artist id or link from musicbrainz.org/artist/…", note: "For copyrighted artists: lists releases with links to official platforms. No audio hosted." },
};

function ImportPanel({ onImported }: { onImported: () => void }) {
  const [source, setSource] = useState<keyof typeof sources>("librivox");
  const [id, setId] = useState("");
  const [kind, setKind] = useState("album");
  const [publish, setPublish] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState("");

  async function submit() {
    if (!id.trim()) return setError("Paste an id or link.");
    setBusy(true); setError(""); setResult("");
    try {
      const res = await adminFetch<{ works: StudioWork[] }>("/works/import", { method: "POST", body: JSON.stringify({ source, id: id.trim(), kind, publish }) });
      const first = res.works[0];
      setResult(res.works.length === 1
        ? `Imported “${first.title}”: ${first.trackCount} tracks, ${formatDuration(first.durationSec) || "no audio"}${first.published ? ", published" : ", draft"}.`
        : `Imported ${res.works.length} releases by ${first?.creator ?? "the artist"}.`);
      setId("");
      onImported();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const s = sources[source];
  return (
    <section className="panel" style={{ gap: 16 }}>
      <h2>Import free content</h2>
      <div className="field">
        <label htmlFor="src">Source</label>
        <select id="src" className="input" value={source} onChange={(e) => { setSource(e.target.value as keyof typeof sources); setError(""); setResult(""); }}>
          {Object.entries(sources).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
        </select>
        <span className="muted" style={{ fontSize: 13 }}>{s.note}</span>
      </div>
      <div className="field">
        <label htmlFor="srcid">Id or link</label>
        <input id="srcid" className="input" value={id} placeholder={s.hint} onChange={(e) => setId(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") void submit(); }} />
      </div>
      {source === "archive" && (
        <div className="field">
          <label htmlFor="ikind">Import as</label>
          <select id="ikind" className="input" value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="album">Album</option>
            <option value="book">Audiobook</option>
          </select>
        </div>
      )}
      <label className="check"><input type="checkbox" checked={publish} onChange={(e) => setPublish(e.target.checked)} /> Publish right away</label>
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      {result && <p style={{ margin: 0, color: "var(--ok)", fontSize: 14 }} aria-live="polite">{result}</p>}
      <div className="actions"><button type="button" className="btn btn-primary" disabled={busy} onClick={submit}>{busy ? "Importing…" : "Import"}</button></div>
    </section>
  );
}

function CreatePanel({ onCreated }: { onCreated: (id: string) => void }) {
  const empty = { kind: "book", title: "", creator: "", narrator: "", language: "vi", year: "", coverUrl: "", description: "", license: "public_domain", rightsNote: "", aiVoice: false };
  const [f, setF] = useState(empty);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const set = <K extends keyof typeof empty>(k: K, v: (typeof empty)[K]) => setF((x) => ({ ...x, [k]: v }));

  async function submit() {
    if (!f.title.trim()) return setError("Add a title.");
    if (f.rightsNote.trim().length < 8) return setError("Add the source link or licence reference.");
    if (!confirm) return setError("Confirm you have the right to publish this.");
    setBusy(true); setError("");
    try {
      const w = await adminFetch<StudioWork>("/works", {
        method: "POST",
        body: JSON.stringify({ ...f, year: Number(f.year) || 0, confirm }),
      });
      setF(empty); setConfirm(false);
      onCreated(w.id);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel" style={{ gap: 16 }}>
      <h2>Add your own book or album</h2>
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        <div className="field" style={{ flex: "1 1 120px" }}>
          <label htmlFor="ckind">Type</label>
          <select id="ckind" className="input" value={f.kind} onChange={(e) => set("kind", e.target.value)}>
            <option value="book">Audiobook</option>
            <option value="album">Album</option>
          </select>
        </div>
        <div className="field" style={{ flex: "1 1 120px" }}>
          <label htmlFor="clang">Language</label>
          <select id="clang" className="input" value={f.language} onChange={(e) => set("language", e.target.value)}>
            {Object.entries(languageNames).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            <option value="">Other / none</option>
          </select>
        </div>
        <div className="field" style={{ flex: "1 1 90px" }}>
          <label htmlFor="cyear">Year</label>
          <input id="cyear" className="input" inputMode="numeric" value={f.year} onChange={(e) => set("year", e.target.value.replace(/\D/g, "").slice(0, 4))} />
        </div>
      </div>
      <div className="field"><label htmlFor="ctitle">Title</label><input id="ctitle" className="input" value={f.title} onChange={(e) => set("title", e.target.value)} placeholder="e.g. Chí Phèo" /></div>
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        <div className="field" style={{ flex: "1 1 180px" }}><label htmlFor="ccreator">{f.kind === "book" ? "Author" : "Artist"}</label><input id="ccreator" className="input" value={f.creator} onChange={(e) => set("creator", e.target.value)} /></div>
        {f.kind === "book" && <div className="field" style={{ flex: "1 1 180px" }}><label htmlFor="cnarr">Read by</label><input id="cnarr" className="input" value={f.narrator} onChange={(e) => set("narrator", e.target.value)} /></div>}
      </div>
      <div className="field"><label htmlFor="ccover">Cover image URL <span className="muted">(optional, https)</span></label><input id="ccover" className="input" value={f.coverUrl} onChange={(e) => set("coverUrl", e.target.value)} /></div>
      <div className="field"><label htmlFor="cdesc">Description <span className="muted">(optional)</span></label><textarea id="cdesc" className="input" rows={3} style={{ height: "auto", padding: 12 }} value={f.description} onChange={(e) => set("description", e.target.value)} /></div>
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        <div className="field" style={{ flex: "1 1 160px" }}>
          <label htmlFor="clic">Licence</label>
          <select id="clic" className="input" value={f.license} onChange={(e) => set("license", e.target.value)}>
            {Object.entries(licenseNames).filter(([k]) => k !== "external").map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
        </div>
        <div className="field" style={{ flex: "2 1 240px" }}><label htmlFor="cnote">Source link or licence reference</label><input id="cnote" className="input" value={f.rightsNote} onChange={(e) => set("rightsNote", e.target.value)} placeholder="https://vi.wikisource.org/wiki/…" /></div>
      </div>
      <label className="check"><input type="checkbox" checked={f.aiVoice} onChange={(e) => set("aiVoice", e.target.checked)} /> Read by an AI voice (shown as “Giọng đọc AI”)</label>
      <label className="check" style={{ alignItems: "flex-start" }}>
        <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />
        I confirm the text and the recording are public domain or openly licensed, or we have permission to publish them.
      </label>
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      <div className="actions"><button type="button" className="btn btn-primary" disabled={busy} onClick={submit}>{busy ? "Creating…" : "Create and add audio"}</button></div>
    </section>
  );
}

function WorksTable({ works, reload, onEdit }: { works: StudioWork[]; reload: () => void; onEdit: (id: string) => void }) {
  const [error, setError] = useState("");

  async function patch(w: StudioWork, body: Partial<Pick<StudioWork, "published" | "featured">>) {
    setError("");
    try { await adminFetch(`/works/${w.id}`, { method: "PATCH", body: JSON.stringify(body) }); }
    catch (err) { setError(`${w.title}: ${(err as Error).message}`); }
    reload();
  }

  async function remove(w: StudioWork) {
    if (!window.confirm(`Delete “${w.title}”${w.source === "studio" ? " and its uploaded audio" : ""}? This can't be undone.`)) return;
    setError("");
    try { await adminFetch(`/works/${w.id}`, { method: "DELETE" }); }
    catch (err) { setError(`${w.title}: ${(err as Error).message}`); }
    reload();
  }

  if (!works.length) return <p className="muted">Nothing here yet. Import something above, or add your own.</p>;

  return (
    <>
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr><th>Title</th><th>Source</th><th>Audio</th><th>Rights</th><th>Published</th><th>Featured</th><th><span className="sr-only">Actions</span></th></tr>
          </thead>
          <tbody>
            {works.map((w) => (
              <tr key={w.id}>
                <td>
                  <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
                    <span className="studio-thumb" style={{ background: toneFor(w.title) }}>{w.coverUrl && <img src={w.coverUrl} alt="" loading="lazy" />}</span>
                    <span style={{ minWidth: 0 }}>
                      <span style={{ display: "block", fontWeight: 600 }}>{w.title}</span>
                      <span className="muted" style={{ fontSize: 13 }}>{[w.kind === "book" ? "Book" : "Album", w.creator, w.aiVoice ? "AI voice" : ""].filter(Boolean).join(" · ")}</span>
                    </span>
                  </div>
                </td>
                <td className="muted" style={{ fontSize: 13 }}>{w.source}</td>
                <td style={{ fontSize: 13, whiteSpace: "nowrap" }}>
                  {w.license === "external" ? <span className="muted">{w.links.length} links</span>
                    : w.trackCount ? <>{w.trackCount} {w.trackCount === 1 ? "track" : "tracks"}<div className="muted">{formatDuration(w.durationSec)}</div></>
                    : <span className="badge badge-idle">No audio yet</span>}
                </td>
                <td style={{ fontSize: 13 }}>
                  <div>{licenseNames[w.license] ?? w.license}</div>
                  <div className="muted" style={{ fontSize: 12, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={w.rightsNote}>{w.rightsNote}</div>
                </td>
                <td>
                  <input type="checkbox" aria-label={`Publish ${w.title}`} checked={w.published} disabled={!w.published && !w.trackCount && !w.links.length}
                    onChange={(e) => patch(w, { published: e.target.checked })} style={{ width: 20, height: 20, accentColor: "var(--accent)" }} />
                </td>
                <td>
                  <input type="checkbox" aria-label={`Feature ${w.title}`} checked={w.featured}
                    onChange={(e) => patch(w, { featured: e.target.checked })} style={{ width: 20, height: 20, accentColor: "var(--accent)" }} />
                </td>
                <td style={{ whiteSpace: "nowrap", fontSize: 14 }}>
                  {w.license !== "external" && <button type="button" className="link-btn" onClick={() => onEdit(w.id)}>Tracks</button>}
                  {w.published && <Link href={workHref(w)} target="_blank" style={{ margin: "0 14px" }}>View</Link>}
                  <button type="button" className="link-btn danger" onClick={() => remove(w)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

type Row = { title: string; durationSec: number; audio: string; url?: string; key: string };
type Upload = { name: string; pct: number; error?: string };

/** Upload audio files, rename, reorder and remove tracks, then save the list. */
function TrackEditor({ id, onClose, onChanged }: { id: string; onClose: () => void; onChanged: () => void }) {
  const [work, setWork] = useState<StudioWork | null>(null);
  const [rows, setRows] = useState<Row[]>([]);
  const [dirty, setDirty] = useState(false);
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [over, setOver] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const panel = useRef<HTMLElement>(null);

  const apply = (res: { work: StudioWork; tracks: StudioTrack[] }) => {
    setWork(res.work);
    setRows(res.tracks.map((t) => ({ title: t.title, durationSec: t.durationSec, audio: t.audio, url: t.url, key: t.audio })));
    setDirty(false);
  };

  useEffect(() => {
    adminFetch<{ work: StudioWork; tracks: StudioTrack[] }>(`/works/${id}`).then(apply).catch((err) => setError((err as Error).message));
    panel.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [id]);

  const edit = (next: Row[]) => { setRows(next); setDirty(true); setSaved(""); };
  const move = (i: number, d: -1 | 1) => {
    const j = i + d;
    if (j < 0 || j >= rows.length) return;
    const next = [...rows];
    [next[i], next[j]] = [next[j], next[i]];
    edit(next);
  };

  async function addFiles(files: FileList | null) {
    if (!files?.length) return;
    setError("");
    const list = Array.from(files).sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    const bad = list.filter((f) => !audioType(f));
    if (bad.length) setError(`Skipped ${bad.map((f) => f.name).join(", ")}: use .mp3 or .m4a (encode other formats with the import-audio local command).`);
    const good = list.filter((f) => audioType(f));
    setUploads(good.map((f) => ({ name: f.name, pct: 0 })));
    // Upload one at a time so the progress is readable and a failure stops cleanly.
    for (const [i, file] of good.entries()) {
      const update = (u: Partial<Upload>) => setUploads((all) => all.map((x, j) => (j === i ? { ...x, ...u } : x)));
      try {
        const ct = audioType(file);
        const [duration, { key, uploadUrl }] = await Promise.all([
          audioDuration(file),
          adminFetch<{ key: string; uploadUrl: string }>(`/works/${id}/uploads`, { method: "POST", body: JSON.stringify({ contentType: ct }) }),
        ]);
        await putFile(uploadUrl, file, ct, (pct) => update({ pct }));
        const title = file.name.replace(/\.[^.]+$/, "").replace(/[_]+/g, " ").trim();
        setRows((r) => [...r, { title, durationSec: duration, audio: key, key }]);
        setDirty(true);
        setSaved("");
        update({ pct: 100 });
      } catch (err) {
        update({ error: (err as Error).message });
      }
    }
  }

  async function save() {
    setSaving(true); setError("");
    try {
      const res = await adminFetch<{ work: StudioWork; tracks: StudioTrack[] }>(`/works/${id}/tracks`, {
        method: "PUT",
        body: JSON.stringify(rows.map((r) => ({ title: r.title, durationSec: r.durationSec, audio: r.audio }))),
      });
      apply(res);
      setUploads([]);
      setSaved(res.work.published ? "Saved. Changes are live." : "Saved. Publish it from the library when you're ready.");
      onChanged();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  }

  function close() {
    if (dirty && !window.confirm("Discard unsaved track changes? Uploaded files that aren't saved will stay unused.")) return;
    onClose();
  }

  const total = rows.reduce((s, r) => s + r.durationSec, 0);
  const uploading = uploads.some((u) => u.pct < 100 && !u.error);

  return (
    <section ref={panel} className="panel" style={{ gap: 16 }} aria-label="Edit tracks">
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <h2 style={{ marginRight: "auto" }}>Tracks{work ? `: ${work.title}` : ""}</h2>
        <button type="button" className="btn" style={{ height: 40 }} onClick={close}>Close</button>
      </div>

      {rows.length > 0 ? (
        <ol className="edit-tracks">
          {rows.map((r, i) => (
            <li key={r.key}>
              <span className="track-num">{i + 1}</span>
              <label className="sr-only" htmlFor={`t-${i}`}>Track {i + 1} title</label>
              <input id={`t-${i}`} className="input" style={{ height: 38 }} value={r.title} onChange={(e) => edit(rows.map((x, j) => (j === i ? { ...x, title: e.target.value } : x)))} />
              <span className="muted track-dur">{r.durationSec ? clock(r.durationSec) : "?"}</span>
              <button type="button" className="icon-btn sm" aria-label={`Move track ${i + 1} up`} disabled={i === 0} onClick={() => move(i, -1)}>↑</button>
              <button type="button" className="icon-btn sm" aria-label={`Move track ${i + 1} down`} disabled={i === rows.length - 1} onClick={() => move(i, 1)}>↓</button>
              <button type="button" className="link-btn danger" onClick={() => edit(rows.filter((_, j) => j !== i))}>Remove</button>
            </li>
          ))}
        </ol>
      ) : (
        <p className="muted" style={{ margin: 0 }}>No tracks yet. Add audio files below.</p>
      )}

      <div
        className={`drop${over ? " over" : ""}`}
        style={{ minHeight: 120 }}
        role="button"
        tabIndex={0}
        onClick={() => input.current?.click()}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); input.current?.click(); } }}
        onDragOver={(e) => { e.preventDefault(); setOver(true); }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => { e.preventDefault(); setOver(false); void addFiles(e.dataTransfer.files); }}
      >
        <UploadIcon size={26} />
        <span style={{ fontWeight: 600 }}>Drop .mp3 or .m4a files here, or click to browse</span>
        <span className="muted" style={{ fontSize: 13 }}>Several at once is fine; they&apos;re added in file-name order. Files go straight to storage.</span>
        <input ref={input} type="file" accept=".mp3,.m4a,.aac,audio/mpeg,audio/mp4" multiple hidden onChange={(e) => { void addFiles(e.target.files); e.target.value = ""; }} />
      </div>

      {uploads.length > 0 && (
        <ul className="upload-list" aria-live="polite">
          {uploads.map((u) => (
            <li key={u.name}>
              <span>{u.name}</span>
              {u.error ? <span className="error">{u.error}</span> : <span className="muted">{u.pct}%</span>}
              {!u.error && <div className="bar"><span style={{ width: `${u.pct}%` }} /></div>}
            </li>
          ))}
        </ul>
      )}

      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      {saved && <p style={{ margin: 0, color: "var(--ok)", fontSize: 14 }} aria-live="polite">{saved}</p>}
      <div className="actions" style={{ alignItems: "center" }}>
        <button type="button" className="btn btn-primary" disabled={saving || uploading || !dirty} onClick={save}>{saving ? "Saving…" : "Save tracks"}</button>
        <span className="muted" style={{ fontSize: 14 }}>{rows.length} tracks · {formatDuration(total) || "0m"}</span>
      </div>
    </section>
  );
}
