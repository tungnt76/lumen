"use client";

import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useMe } from "@/lib/auth";
import { GUEST_CODE_KEY, MY_CODE, parseBundle, saveBundle, type Project } from "@/lib/code";
import { jsonFetch } from "@/lib/studio";
import { CodeList } from "./CodeList";

const CodeWorkspace = dynamic(async () => (await import("./CodeWorkspace")).CodeWorkspace, {
  ssr: false,
  loading: () => <div className="code-page embedded"><div className="draw-loading muted">Loading the editor…</div></div>,
});

/** The Code tab: scratch code for visitors, "My code" (saved to R2) once signed in. */
export function CodeHome() {
  const me = useMe();
  if (me === undefined) return <div className="code-page embedded"><div className="draw-loading muted">Loading…</div></div>;
  if (!me) return <CodeWorkspace mode="guest" />;
  return (
    <div className="wrap" style={{ paddingTop: 36, paddingBottom: 64 }}>
      <CodeList base={MY_CODE} hrefFor={(id) => `/code/${id}`} title="My code" signInPath="/login?next=/code" above={<GuestCode />}
        intro="Your projects, saved to R2 with a commit history. Only you can see them. Edit on a computer; phones can read." />
    </div>
  );
}

/** Offers to keep scratch code written before signing in (reusing the project if a save failed). */
function GuestCode() {
  const router = useRouter();
  const [files, setFiles] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const draftId = useRef<string | undefined>(undefined);

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(GUEST_CODE_KEY);
      if (raw) setFiles(parseBundle(JSON.parse(raw)).files.length);
    } catch { /* nothing to offer */ }
  }, []);
  if (!files) return null;

  function discard() {
    try { window.localStorage.removeItem(GUEST_CODE_KEY); } catch { /* ignore */ }
    setFiles(0);
  }

  async function keep() {
    if (busy) return;
    setBusy(true); setError("");
    try {
      const bundle = parseBundle(JSON.parse(window.localStorage.getItem(GUEST_CODE_KEY) || "{}"));
      if (!draftId.current) {
        const p = await jsonFetch<Project>(MY_CODE, { method: "POST", body: JSON.stringify({ title: "My scratch code" }) });
        draftId.current = p.id;
      }
      await saveBundle(MY_CODE, draftId.current, bundle);
      discard();
      router.push(`/code/${draftId.current}`);
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  }

  return (
    <div className="welcome" role="status" style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
      <span style={{ marginRight: "auto" }}>
        <strong>You have scratch code from before you signed in</strong> ({files} file{files === 1 ? "" : "s"}, kept only in this browser).
        {error && <span className="error" style={{ display: "block" }}>{error}</span>}
      </span>
      <button type="button" className="btn btn-primary" style={{ height: 40 }} onClick={keep} disabled={busy}>{busy ? "Saving…" : "Save to my code"}</button>
      <button type="button" className="btn" style={{ height: 40 }} onClick={discard} disabled={busy}>Discard</button>
    </div>
  );
}
