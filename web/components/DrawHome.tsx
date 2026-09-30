"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useMe } from "@/lib/auth";
import { importLocalScene, MY_DRAWINGS } from "@/lib/drawings";
import { DrawingList } from "./DrawingList";
import { GuestDrawing, GUEST_KEY } from "./GuestDrawing";

/** The Excalidraw tab: a free canvas for visitors, "My drawings" (saved to R2) once signed in. */
export function DrawHome() {
  const me = useMe();
  if (me === undefined) return <div className="guest-draw"><div className="draw-loading muted">Loading…</div></div>;
  if (!me) return <GuestDrawing />;
  return (
    <div className="wrap" style={{ paddingTop: 36, paddingBottom: 64 }}>
      <DrawingList
        base={MY_DRAWINGS}
        hrefFor={(id) => `/draw/${id}`}
        title="My drawings"
        intro="Saved to your account: open them from any device. Only you can see them."
        signInPath="/login?next=/draw"
        above={<LocalSketch />}
      />
    </div>
  );
}

/** Offers to keep a sketch drawn before signing in. */
function LocalSketch() {
  const router = useRouter();
  const [count, setCount] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const draftId = useRef<string | undefined>(undefined); // drawing made by a failed attempt, reused on retry

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(GUEST_KEY);
      const els = raw ? (JSON.parse(raw).elements ?? []).filter((e: { isDeleted?: boolean }) => !e.isDeleted) : [];
      setCount(els.length);
    } catch { /* storage blocked or unreadable: nothing to offer */ }
  }, []);
  if (!count) return null;

  function discard() {
    try { window.localStorage.removeItem(GUEST_KEY); } catch { /* ignore */ }
    setCount(0);
  }

  async function keep() {
    if (busy) return;
    setBusy(true); setError("");
    try {
      const d = await importLocalScene(window.localStorage.getItem(GUEST_KEY) || "{}", "My sketch", draftId.current, (id) => { draftId.current = id; });
      discard();
      router.push(`/draw/${d.id}`);
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  }

  return (
    <div className="welcome" role="status" style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
      <span style={{ marginRight: "auto" }}>
        <strong>You have a sketch from before you signed in</strong> ({count} shape{count === 1 ? "" : "s"}, kept only in this browser).
        {error && <span className="error" style={{ display: "block" }}>{error}</span>}
      </span>
      <button type="button" className="btn btn-primary" style={{ height: 40 }} onClick={keep} disabled={busy}>{busy ? "Saving…" : "Save to my drawings"}</button>
      <button type="button" className="btn" style={{ height: 40 }} onClick={discard} disabled={busy}>Discard</button>
    </div>
  );
}
