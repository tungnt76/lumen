"use client";

import Editor, { DiffEditor, loader, type OnMount } from "@monaco-editor/react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  changesBetween, checkPath, cloneBundle, fetchBundle, GUEST_CODE_KEY, languageOf, LANGUAGE_NAMES, MAX_FILES,
  parseBundle, sameBundle, saveBundle, TEMPLATES, type Bundle, type Commit, type Project,
} from "@/lib/code";
import { ApiError, jsonFetch } from "@/lib/studio";
import { getTheme, onThemeChange } from "@/lib/theme";
import { BackIcon, CloseIcon, PlusIcon } from "./icons";

// The editor loads from jsDelivr on first use (a few MB), not from the site bundle.
loader.config({ paths: { vs: "https://cdn.jsdelivr.net/npm/monaco-editor@0.57.0/min/vs" } });

type Props =
  | { mode: "guest" }
  | { mode: "saved"; id: string; base: string; backHref: string; signInPath: string; embedded?: boolean };

type Status = { kind: "idle" | "busy" | "ok" | "error"; text: string };
type Diff = { commit: Commit; snap: Bundle; path: string };

const draftKey = (id: string) => `lumen:code-draft:${id}`;

function readJSON<T>(key: string): T | null {
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

function writeJSON(key: string, value: unknown) {
  try { window.localStorage.setItem(key, JSON.stringify(value)); } catch { /* storage full or blocked */ }
}

function ago(iso: string) {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(iso).toLocaleDateString();
}

type Tree = { name: string; path: string; children: Tree[]; file: boolean };

/** Folders first, then files, alphabetically, from flat paths like "src/app.js". */
function buildTree(paths: string[]): Tree[] {
  const root: Tree = { name: "", path: "", children: [], file: false };
  for (const p of paths) {
    const parts = p.split("/");
    let node = root;
    parts.forEach((part, i) => {
      const path = parts.slice(0, i + 1).join("/");
      let child = node.children.find((c) => c.name === part && c.file === (i === parts.length - 1));
      if (!child) {
        child = { name: part, path, children: [], file: i === parts.length - 1 };
        node.children.push(child);
      }
      node = child;
    });
  }
  const sort = (n: Tree) => {
    n.children.sort((a, b) => (a.file === b.file ? a.name.localeCompare(b.name) : a.file ? 1 : -1));
    n.children.forEach(sort);
  };
  sort(root);
  return root.children;
}

export function CodeWorkspace(props: Props) {
  const router = useRouter();
  const guest = props.mode === "guest";
  const base = props.mode === "saved" ? props.base : "";
  const id = props.mode === "saved" ? props.id : "";

  const [project, setProject] = useState<Project | null>(null);
  const [title, setTitle] = useState("");
  const [bundle, setBundle] = useState<Bundle | null>(null);
  const [saved, setSaved] = useState<Bundle | null>(null); // last saved working copy
  const [head, setHead] = useState<Bundle | null>(null); // last commit's files
  const [draft, setDraft] = useState<Bundle | null>(null); // unsaved changes found in this browser
  const [active, setActive] = useState("");
  const [tabs, setTabs] = useState<string[]>([]);
  const [panel, setPanel] = useState<"files" | "git">("files");
  const [sideOpen, setSideOpen] = useState(false); // phones: the side panel slides over the editor
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [commits, setCommits] = useState<Commit[]>([]);
  const [message, setMessage] = useState("");
  const [diff, setDiff] = useState<Diff | null>(null);
  const [status, setStatus] = useState<Status>({ kind: "idle", text: "" });
  const [cursor, setCursor] = useState({ line: 1, col: 1 });
  const [theme, setTheme] = useState<"vs-dark" | "light">("vs-dark");
  const [phone, setPhone] = useState(false);
  const busy = useRef(false);
  const snaps = useRef(new Map<string, Bundle>());

  useEffect(() => {
    const read = () => setTheme(getTheme() === "light" ? "light" : "vs-dark");
    read();
    setPhone(window.matchMedia("(max-width: 900px)").matches);
    return onThemeChange(read);
  }, []);

  const openFile = useCallback((path: string) => {
    setSideOpen(false);
    setActive(path);
    setTabs((t) => (t.includes(path) ? t : [...t, path]));
    setDiff(null);
  }, []);

  const start = useCallback((b: Bundle) => {
    setBundle(b);
    const first = b.main && b.files.some((f) => f.path === b.main) ? b.main : b.files[0]?.path ?? "";
    setActive(first);
    setTabs(first ? [first] : []);
  }, []);

  // ---- load ----
  useEffect(() => {
    if (guest) {
      const stored = readJSON<Bundle>(GUEST_CODE_KEY);
      start(stored ? parseBundle(stored) : cloneBundle(TEMPLATES[0].bundle));
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await jsonFetch<{ project: Project; bundleUrl: string }>(`${base}/${id}`);
        let files: Bundle;
        let savedCopy: Bundle | null = null;
        if (res.bundleUrl) {
          files = await fetchBundle(res.bundleUrl);
          savedCopy = cloneBundle(files);
        } else {
          // Never saved: start from the template picked when it was created.
          const tpl = readJSON<string>(`lumen:code-template:${id}`);
          files = cloneBundle((TEMPLATES.find((t) => t.id === tpl) ?? TEMPLATES[0]).bundle);
        }
        const list = await jsonFetch<Commit[]>(`${base}/${id}/commits`);
        let headCopy: Bundle | null = null;
        if (list[0]) {
          const c = await jsonFetch<{ url: string }>(`${base}/${id}/commits/${list[0].id}`);
          headCopy = await fetchBundle(c.url);
          snaps.current.set(list[0].id, headCopy);
        }
        if (cancelled) return;
        setProject(res.project);
        setTitle(res.project.title);
        setSaved(savedCopy);
        setHead(headCopy);
        setCommits(list);
        start(files);
        const d = readJSON<{ bundle: Bundle }>(draftKey(id));
        if (d && !sameBundle(parseBundle(d.bundle), files)) setDraft(parseBundle(d.bundle));
      } catch (err) {
        if (err instanceof ApiError && err.status === 401 && props.mode === "saved") router.replace(props.signInPath);
        else if (err instanceof ApiError && err.status === 404 && props.mode === "saved") router.replace(props.backHref);
        else if (!cancelled) setStatus({ kind: "error", text: (err as Error).message });
      }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [guest, base, id]);

  const dirty = !guest && bundle !== null && !sameBundle(bundle, saved);
  const changes = useMemo(() => (bundle ? changesBetween(head, bundle) : []), [bundle, head]);

  // Keep a draft in this browser (guests: that's their only copy).
  useEffect(() => {
    if (!bundle) return;
    const t = window.setTimeout(() => {
      if (guest) writeJSON(GUEST_CODE_KEY, bundle);
      else if (dirty) writeJSON(draftKey(id), { bundle, at: Date.now() });
    }, 600);
    return () => window.clearTimeout(t);
  }, [bundle, dirty, guest, id]);

  useEffect(() => {
    if (!dirty) return;
    const onLeave = (e: BeforeUnloadEvent) => { e.preventDefault(); };
    window.addEventListener("beforeunload", onLeave);
    return () => window.removeEventListener("beforeunload", onLeave);
  }, [dirty]);

  // ---- save and commit ----
  const save = useCallback(async (): Promise<boolean> => {
    if (guest || !bundle || busy.current) return false;
    busy.current = true;
    setStatus({ kind: "busy", text: "Saving…" });
    try {
      const snapshot = cloneBundle(bundle);
      const p = await saveBundle(base, id, snapshot);
      setProject(p);
      setSaved(snapshot);
      try { window.localStorage.removeItem(draftKey(id)); } catch { /* ignore */ }
      setStatus({ kind: "ok", text: "Saved to R2" });
      return true;
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
      return false;
    } finally {
      busy.current = false;
    }
  }, [guest, bundle, base, id]);

  const saveRef = useRef(save);
  saveRef.current = save;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") { e.preventDefault(); void saveRef.current(); }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  async function commit() {
    if (!bundle) return;
    if (!message.trim()) return setStatus({ kind: "error", text: "Write a commit message first." });
    if (!changes.length) return setStatus({ kind: "error", text: "Nothing to commit: no changes since the last commit." });
    if (dirty && !(await save())) return;
    busy.current = true;
    setStatus({ kind: "busy", text: "Committing…" });
    try {
      const c = await jsonFetch<Commit>(`${base}/${id}/commits`, { method: "POST", body: JSON.stringify({ message: message.trim(), changes }) });
      const snap = cloneBundle(bundle);
      snaps.current.set(c.id, snap);
      setHead(snap);
      setCommits((list) => [c, ...list]);
      setMessage("");
      setStatus({ kind: "ok", text: `Committed “${c.message}”` });
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
    } finally {
      busy.current = false;
    }
  }

  async function openCommit(c: Commit) {
    try {
      let snap = snaps.current.get(c.id);
      if (!snap) {
        const res = await jsonFetch<{ url: string }>(`${base}/${id}/commits/${c.id}`);
        snap = await fetchBundle(res.url);
        snaps.current.set(c.id, snap);
      }
      const path = c.changes.find((x) => x.status !== "D")?.path ?? c.changes[0]?.path ?? snap.files[0]?.path ?? "";
      setDiff({ commit: c, snap, path });
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
    }
  }

  function restore(snap: Bundle, label: string) {
    if (!window.confirm(`Replace your files with ${label}? You can still undo by restoring another commit or not saving.`)) return;
    start(cloneBundle(snap));
    setDiff(null);
    setStatus({ kind: "ok", text: `Restored ${label}. Save to keep it.` });
  }

  // ---- files ----
  const paths = bundle?.files.map((f) => f.path) ?? [];
  const file = bundle?.files.find((f) => f.path === active);
  const readOnly = phone;

  function update(fn: (b: Bundle) => Bundle) {
    setBundle((b) => (b ? fn(b) : b));
  }

  function newFile(folder = "") {
    if (paths.length >= MAX_FILES) return setStatus({ kind: "error", text: `A project can have at most ${MAX_FILES} files.` });
    const path = window.prompt("New file (use / for folders, e.g. src/utils.js)", folder ? `${folder}/` : "")?.trim();
    if (path === undefined || path === "") return;
    const err = checkPath(path, paths);
    if (err) return setStatus({ kind: "error", text: err });
    update((b) => ({ ...b, main: b.main || path, files: [...b.files, { path, content: "" }] }));
    openFile(path);
  }

  function renameFile(path: string) {
    const next = window.prompt("Rename file", path)?.trim();
    if (!next || next === path) return;
    const err = checkPath(next, paths.filter((p) => p !== path));
    if (err) return setStatus({ kind: "error", text: err });
    update((b) => ({ ...b, main: b.main === path ? next : b.main, files: b.files.map((f) => (f.path === path ? { ...f, path: next } : f)) }));
    setTabs((t) => t.map((x) => (x === path ? next : x)));
    if (active === path) setActive(next);
  }

  function deleteFile(path: string) {
    if (!window.confirm(`Delete ${path}?`)) return;
    update((b) => {
      const files = b.files.filter((f) => f.path !== path);
      return { ...b, files, main: b.main === path ? files[0]?.path ?? "" : b.main };
    });
    closeTab(path);
  }

  function closeTab(path: string) {
    setTabs((t) => {
      const rest = t.filter((x) => x !== path);
      if (active === path) setActive(rest[rest.length - 1] ?? "");
      return rest;
    });
  }

  async function rename() {
    const t = title.trim() || "Untitled project";
    if (guest || !project || t === project.title) { setTitle(t); return; }
    try {
      const p = await jsonFetch<Project>(`${base}/${id}`, { method: "PATCH", body: JSON.stringify({ title: t }) });
      setProject(p);
      setTitle(p.title);
    } catch (err) {
      setStatus({ kind: "error", text: (err as Error).message });
    }
  }

  const onMount: OnMount = (editor, monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => void saveRef.current());
    editor.onDidChangeCursorPosition((e) => setCursor({ line: e.position.lineNumber, col: e.position.column }));
  };

  const editorOptions = {
    readOnly, fontSize: 14, minimap: { enabled: !phone }, automaticLayout: true, scrollBeyondLastLine: false,
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace", tabSize: 2, renderWhitespace: "selection" as const,
    smoothScrolling: true, padding: { top: 10 },
  };

  function renderTree(nodes: Tree[], depth = 0): React.ReactNode {
    return nodes.map((n) =>
      n.file ? (
        <div key={n.path} className="tree-row" data-active={n.path === active && !diff ? "true" : undefined} style={{ paddingLeft: 12 + depth * 14 }}>
          <button type="button" className="tree-name" onClick={() => openFile(n.path)} title={n.path}>
            <span className={`file-dot lang-${languageOf(n.path)}`} />{n.name}
            {bundle?.main === n.path && <span className="tree-main" title="Main file">main</span>}
          </button>
          {!readOnly && (
            <span className="tree-actions">
              <button type="button" onClick={() => renameFile(n.path)} title="Rename">✎</button>
              <button type="button" onClick={() => deleteFile(n.path)} title="Delete">✕</button>
            </span>
          )}
        </div>
      ) : (
        <div key={n.path}>
          <div className="tree-row folder" style={{ paddingLeft: 12 + depth * 14 }}>
            <button type="button" className="tree-name" onClick={() => setCollapsed((c) => { const s = new Set(c); if (s.has(n.path)) s.delete(n.path); else s.add(n.path); return s; })}>
              <span className="tree-caret">{collapsed.has(n.path) ? "▸" : "▾"}</span>{n.name}
            </button>
            {!readOnly && <span className="tree-actions"><button type="button" onClick={() => newFile(n.path)} title="New file here">+</button></span>}
          </div>
          {!collapsed.has(n.path) && renderTree(n.children, depth + 1)}
        </div>
      ),
    );
  }

  const embedded = props.mode === "guest" || props.embedded;
  const statusText = status.kind === "error" || status.kind === "busy" ? status.text : dirty ? "Unsaved changes" : status.text;

  if (!bundle) {
    return <div className={embedded ? "code-page embedded" : "code-page"}><div className="draw-loading muted">{status.kind === "error" ? status.text : "Loading…"}</div></div>;
  }

  const diffOld = diff?.snap.files.find((f) => f.path === diff.path)?.content ?? "";
  const diffNew = bundle.files.find((f) => f.path === diff?.path)?.content ?? "";

  return (
    <div className={embedded ? "code-page embedded" : "code-page"}>
      <header className="draw-bar">
        {props.mode === "saved" && (
          <Link href={props.backHref} className="icon-btn" aria-label="Back to projects"
            onClick={(e) => { if (dirty && !window.confirm("You have unsaved changes. Leave without saving?")) e.preventDefault(); }}>
            <BackIcon size={20} />
          </Link>
        )}
        {guest ? (
          <span className="code-guest-note">Scratch code, kept in this browser only.</span>
        ) : (
          <>
            <label htmlFor="ptitle" className="sr-only">Project title</label>
            <input id="ptitle" className="draw-title" value={title} onChange={(e) => setTitle(e.target.value)} onBlur={rename}
              onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }} maxLength={200} disabled={readOnly} />
          </>
        )}
        <span className={`draw-status ${status.kind === "ok" ? "saved" : status.kind}`} role="status" aria-live="polite">{statusText}</span>
        {guest ? (
          <Link href="/login?next=/code" className="btn btn-primary guest-save">Sign in to save</Link>
        ) : (
          <button type="button" className="btn btn-primary" style={{ height: 40 }} onClick={() => void save()} disabled={readOnly || status.kind === "busy"}>
            {status.kind === "busy" ? "Saving…" : "Save"}
          </button>
        )}
      </header>

      {phone && <div className="code-note">You can read code here. Editing works on a computer.</div>}
      {draft && (
        <div className="code-note warn">
          <span>You have unsaved changes to this project from this browser.</span>
          <button type="button" className="link-btn" onClick={() => { start(draft); setDraft(null); setStatus({ kind: "ok", text: "Unsaved changes restored. Save to keep them." }); }}>Restore</button>
          <button type="button" className="link-btn danger" onClick={() => { setDraft(null); try { window.localStorage.removeItem(draftKey(id)); } catch { /* ignore */ } }}>Discard</button>
        </div>
      )}

      <div className={sideOpen ? "code-body side-open" : "code-body"}>
        <nav className="code-activity" aria-label="Side panels">
          <button type="button" aria-pressed={panel === "files"} onClick={() => { setSideOpen(!(sideOpen && panel === "files")); setPanel("files"); }} title="Explorer">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d="M13 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V10z" /><path d="M13 3v7h7" /></svg>
          </button>
          <button type="button" aria-pressed={panel === "git"} onClick={() => { setSideOpen(!(sideOpen && panel === "git")); setPanel("git"); }} title="Source control">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><circle cx="6" cy="5" r="2.5" /><circle cx="6" cy="19" r="2.5" /><circle cx="18" cy="8" r="2.5" /><path d="M6 7.5v9M18 10.5c0 4-6 3.5-11 7" /></svg>
            {changes.length > 0 && !guest && <span className="activity-badge">{changes.length}</span>}
          </button>
        </nav>

        <aside className="code-side">
          {panel === "files" ? (
            <>
              <div className="side-head">
                <span>Explorer</span>
                {!readOnly && <button type="button" className="icon-btn sm" onClick={() => newFile()} title="New file" aria-label="New file"><PlusIcon size={16} /></button>}
              </div>
              <div className="tree">{renderTree(buildTree(paths))}</div>
              {!readOnly && file && bundle.main !== file.path && (
                <button type="button" className="link-btn" style={{ margin: "10px 12px", fontSize: 13 }} onClick={() => update((b) => ({ ...b, main: file.path }))}>Make {file.path} the main file</button>
              )}
            </>
          ) : guest ? (
            <div className="side-empty">
              <strong>Source control</strong>
              <p className="muted">Sign in to save your code and keep a history of commits.</p>
              <Link href="/login?next=/code" className="btn btn-primary" style={{ height: 38 }}>Sign in</Link>
            </div>
          ) : (
            <>
              <div className="side-head"><span>Source control</span></div>
              <div className="git-panel">
                <textarea className="input" rows={2} placeholder={`Message (${navigator.platform.includes("Mac") ? "⌘" : "Ctrl"}+Enter to commit)`} value={message}
                  onChange={(e) => setMessage(e.target.value)} disabled={readOnly} maxLength={500}
                  onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void commit(); } }} />
                <button type="button" className="btn btn-primary git-commit" onClick={() => void commit()} disabled={readOnly || status.kind === "busy" || !changes.length}>
                  ✓ Commit{changes.length ? ` ${changes.length} change${changes.length === 1 ? "" : "s"}` : ""}
                </button>
                <div className="git-section">Changes</div>
                {changes.length === 0 ? <p className="muted git-empty">{head ? "No changes since the last commit." : "Everything is new: make your first commit."}</p> : (
                  <ul className="git-changes">
                    {changes.map((c) => (
                      <li key={c.path}>
                        <button type="button" onClick={() => c.status !== "D" && openFile(c.path)} title={c.path}>{c.path}</button>
                        <span className={`git-status s-${c.status}`}>{c.status}</span>
                      </li>
                    ))}
                  </ul>
                )}
                <div className="git-section">History</div>
                {commits.length === 0 ? <p className="muted git-empty">No commits yet.</p> : (
                  <ul className="git-history">
                    {commits.map((c) => (
                      <li key={c.id} data-active={diff?.commit.id === c.id ? "true" : undefined}>
                        <button type="button" onClick={() => void openCommit(c)}>
                          <span className="git-msg">{c.message}</span>
                          <span className="muted">{c.authorName} · {ago(c.createdAt)} · {c.changes.length} file{c.changes.length === 1 ? "" : "s"}</span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}
        </aside>

        <section className="code-main">
          {diff ? (
            <>
              <div className="code-tabs diff-head">
                <span className="diff-title">“{diff.commit.message}” ↔ your files</span>
                <select className="input" value={diff.path} onChange={(e) => setDiff({ ...diff, path: e.target.value })} aria-label="File to compare">
                  {Array.from(new Set([...diff.snap.files.map((f) => f.path), ...paths])).sort().map((p) => <option key={p} value={p}>{p}</option>)}
                </select>
                {!readOnly && <button type="button" className="btn" style={{ height: 32 }} onClick={() => restore(diff.snap, `“${diff.commit.message}”`)}>Restore this commit</button>}
                <button type="button" className="icon-btn sm" onClick={() => setDiff(null)} aria-label="Close comparison"><CloseIcon size={16} /></button>
              </div>
              <div className="code-editor">
                {/* keepCurrent…Model: let the wrapper leave model disposal to Monaco, avoiding
                    "TextModel got disposed before DiffEditorWidget model got reset" on close. */}
                <DiffEditor key={diff.commit.id + diff.path} original={diffOld} modified={diffNew} language={languageOf(diff.path)} theme={theme}
                  keepCurrentOriginalModel keepCurrentModifiedModel
                  options={{ ...editorOptions, readOnly: true, renderSideBySide: !phone }} />
              </div>
            </>
          ) : (
            <>
              <div className="code-tabs" role="tablist">
                {tabs.map((t) => (
                  <div key={t} className="code-tab" role="tab" aria-selected={t === active}>
                    <button type="button" onClick={() => setActive(t)} title={t}>{t.split("/").pop()}</button>
                    <button type="button" className="tab-close" onClick={() => closeTab(t)} aria-label={`Close ${t}`}>×</button>
                  </div>
                ))}
              </div>
              <div className="code-editor">
                {file ? (
                  <Editor path={file.path} language={languageOf(file.path)} value={file.content} theme={theme} options={editorOptions} onMount={onMount}
                    onChange={(v) => update((b) => ({ ...b, files: b.files.map((f) => (f.path === file.path ? { ...f, content: v ?? "" } : f)) }))}
                    loading={<div className="draw-loading muted">Loading the editor…</div>} />
                ) : (
                  <div className="draw-loading muted">{paths.length ? "Open a file from the Explorer." : "No files yet. Add one with + in the Explorer."}</div>
                )}
              </div>
            </>
          )}
          <footer className="code-status">
            <span>{file ? LANGUAGE_NAMES[languageOf(file.path)] ?? "Text" : ""}</span>
            {file && !diff && <span>Ln {cursor.line}, Col {cursor.col}</span>}
            <span>{paths.length} file{paths.length === 1 ? "" : "s"}</span>
            {!guest && <span>{commits.length} commit{commits.length === 1 ? "" : "s"}</span>}
            <span style={{ marginLeft: "auto" }}>{guest ? "Browser only" : dirty ? "● Unsaved" : saved ? "Saved" : "Not saved yet"}</span>
          </footer>
        </section>
      </div>
    </div>
  );
}
