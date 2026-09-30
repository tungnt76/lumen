"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { FormEvent, Suspense, useCallback, useEffect, useState } from "react";
import { authFetch, describeDevice, setMe, signOut, useMe, type User } from "@/lib/auth";
import { Avatar, AvatarPicker } from "./Avatar";

type Session = { id: number; userAgent: string; ip: string; createdAt: string; lastSeenAt: string; current: boolean };

function ago(iso: string) {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 90) return "active now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(iso).toLocaleDateString();
}

export function AccountSettings() {
  return <Suspense><Settings /></Suspense>;
}

function Settings() {
  const me = useMe();
  const router = useRouter();
  const welcome = useSearchParams().get("welcome") === "1";

  useEffect(() => { if (me === null) router.replace("/login?next=/account"); }, [me, router]);
  if (!me) return <div className="wrap" style={{ padding: "64px var(--gutter)" }}><p className="muted">Loading…</p></div>;

  return (
    <div className="wrap account-page">
      {welcome && (
        <div className="welcome" role="status">
          <strong>Welcome to Lumen, {me.displayName.split(" ")[0]}!</strong> Your account is ready.{" "}
          <Link href="/">Start exploring</Link>
        </div>
      )}
      <header className="account-hero">
        <Avatar id={me.avatar} name={me.displayName} size={88} />
        <div style={{ minWidth: 0 }}>
          <h1>{me.displayName}</h1>
          <p className="muted" style={{ margin: "6px 0 0" }}>{me.email} · member since {new Date(me.createdAt).toLocaleDateString()}</p>
          <span className={`role-badge role-${me.role}`} style={{ marginTop: 10 }}>{me.role === "admin" ? "Admin" : "Member"}</span>
        </div>
      </header>
      <div className="account-grid">
        <ProfileForm me={me} />
        <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
          <PasswordForm />
          <Devices />
          <section className="panel">
            <h2>Sign out</h2>
            <p className="muted" style={{ margin: 0, fontSize: 14 }}>Sign out of Lumen on this device.</p>
            <button type="button" className="btn" style={{ alignSelf: "flex-start" }} onClick={async () => { await signOut(); router.replace("/"); router.refresh(); }}>Sign out</button>
          </section>
        </div>
      </div>
    </div>
  );
}

function ProfileForm({ me }: { me: User }) {
  const [name, setName] = useState(me.displayName);
  const [bio, setBio] = useState(me.bio);
  const [avatar, setAvatar] = useState(me.avatar);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const dirty = name !== me.displayName || bio !== me.bio || avatar !== me.avatar;

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setMsg(null);
    try {
      const r = await authFetch<{ user: User }>("/me", { method: "PATCH", body: JSON.stringify({ displayName: name, bio, avatar }) });
      setMe(r.user);
      setName(r.user.displayName);
      setMsg({ ok: true, text: "Profile saved." });
    } catch (err) {
      setMsg({ ok: false, text: (err as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="panel" onSubmit={save} style={{ gap: 18 }}>
      <h2>Profile</h2>
      <div className="field">
        <span className="label">Avatar</span>
        <AvatarPicker value={avatar} onChange={setAvatar} />
      </div>
      <div className="field">
        <label htmlFor="pname">Display name</label>
        <input id="pname" className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} required />
      </div>
      <div className="field">
        <label htmlFor="pbio">About you <span className="muted">(optional)</span></label>
        <textarea id="pbio" className="input" rows={3} style={{ height: "auto", padding: 12 }} value={bio} onChange={(e) => setBio(e.target.value)} maxLength={300} placeholder="Favourite films, books you're listening to…" />
        <span className="muted field-hint">{bio.length}/300</span>
      </div>
      <div className="field">
        <label htmlFor="pemail">Email</label>
        <input id="pemail" className="input" value={me.email} disabled />
      </div>
      {msg && <p className={msg.ok ? "ok-text" : "error"} role="status" style={{ margin: 0 }}>{msg.text}</p>}
      <button type="submit" className="btn btn-primary" style={{ alignSelf: "flex-start" }} disabled={busy || !dirty}>{busy ? "Saving…" : "Save profile"}</button>
    </form>
  );
}

function PasswordForm() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setMsg(null);
    try {
      await authFetch("/password", { method: "POST", body: JSON.stringify({ current, new: next }) });
      setCurrent(""); setNext("");
      setMsg({ ok: true, text: "Password changed. Other devices have been signed out." });
    } catch (err) {
      setMsg({ ok: false, text: (err as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="panel" onSubmit={save}>
      <h2>Password</h2>
      <div className="field">
        <label htmlFor="cur">Current password</label>
        <input id="cur" className="input" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
      </div>
      <div className="field">
        <label htmlFor="npw">New password</label>
        <input id="npw" className="input" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} minLength={8} required />
        <span className="muted field-hint">At least 8 characters. Changing it signs out your other devices.</span>
      </div>
      {msg && <p className={msg.ok ? "ok-text" : "error"} role="status" style={{ margin: 0 }}>{msg.text}</p>}
      <button type="submit" className="btn" style={{ alignSelf: "flex-start" }} disabled={busy}>{busy ? "Changing…" : "Change password"}</button>
    </form>
  );
}

function Devices() {
  const [list, setList] = useState<Session[] | null>(null);
  const [error, setError] = useState("");
  const load = useCallback(() => { authFetch<Session[]>("/sessions").then(setList).catch((e) => setError((e as Error).message)); }, []);
  useEffect(load, [load]);

  async function end(id: number) {
    await authFetch(`/sessions/${id}`, { method: "DELETE" }).catch((e) => setError((e as Error).message));
    load();
  }
  async function endOthers() {
    await authFetch("/sessions/others", { method: "POST" }).catch((e) => setError((e as Error).message));
    load();
  }
  const others = list?.filter((s) => !s.current).length ?? 0;

  return (
    <section className="panel">
      <h2>Signed-in devices</h2>
      {error && <p className="error" style={{ margin: 0 }}>{error}</p>}
      {!list ? <p className="muted" style={{ margin: 0 }}>Loading…</p> : (
        <ul className="devices">
          {list.map((s) => (
            <li key={s.id}>
              <div style={{ minWidth: 0 }}>
                <div style={{ fontWeight: 600 }}>{describeDevice(s.userAgent)} {s.current && <span className="chip chip-accent" style={{ fontSize: 11, padding: "1px 7px", marginLeft: 6 }}>This device</span>}</div>
                <div className="muted" style={{ fontSize: 13 }}>{s.current ? "active now" : ago(s.lastSeenAt)}{s.ip ? ` · ${s.ip}` : ""}</div>
              </div>
              {!s.current && <button type="button" className="link-btn danger" onClick={() => end(s.id)}>Sign out</button>}
            </li>
          ))}
        </ul>
      )}
      {others > 0 && <button type="button" className="btn" style={{ alignSelf: "flex-start" }} onClick={endOthers}>Sign out of {others} other device{others === 1 ? "" : "s"}</button>}
    </section>
  );
}
