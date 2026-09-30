"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { FormEvent, useEffect, useRef, useState } from "react";
import { authFetch, safeNext, setMe, useMe, type User } from "@/lib/auth";
import { AVATARS, Avatar, AvatarPicker } from "./Avatar";
import { LogoMark } from "./Logo";

function PasswordField({ id, label, value, onChange, autoComplete, hint }: { id: string; label: string; value: string; onChange: (v: string) => void; autoComplete: string; hint?: React.ReactNode }) {
  const [show, setShow] = useState(false);
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <div className="pw-wrap">
        <input id={id} className="input" type={show ? "text" : "password"} value={value} onChange={(e) => onChange(e.target.value)} autoComplete={autoComplete} required />
        <button type="button" className="pw-toggle" onClick={() => setShow(!show)} aria-label={show ? "Hide password" : "Show password"}>{show ? "Hide" : "Show"}</button>
      </div>
      {hint}
    </div>
  );
}

/** 0-4 from length and variety; enough to nudge people away from weak passwords. */
function strength(pw: string) {
  let s = 0;
  if (pw.length >= 8) s++;
  if (pw.length >= 12) s++;
  if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) s++;
  if (/\d/.test(pw) && /[^A-Za-z0-9]/.test(pw)) s++;
  return pw.length < 8 ? Math.min(s, 1) : s;
}

function Strength({ pw }: { pw: string }) {
  if (!pw) return <span className="muted field-hint">At least 8 characters.</span>;
  const s = strength(pw);
  const label = ["Too short", "Weak", "Okay", "Good", "Strong"][s];
  return (
    <span className="pw-strength" data-level={s}>
      <span className="pw-bars">{[1, 2, 3, 4].map((i) => <i key={i} className={i <= s ? "on" : ""} />)}</span>
      {label}
    </span>
  );
}

function AuthCard({ title, subtitle, children }: { title: string; subtitle?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="auth-page">
      <div className="auth-card">
        <Link href="/" className="auth-brand"><LogoMark size={30} /><span className="logo-word" style={{ fontSize: 22 }}>Lumen</span></Link>
        <div>
          <h1>{title}</h1>
          {subtitle && <p className="muted" style={{ margin: "8px 0 0", fontSize: 15 }}>{subtitle}</p>}
        </div>
        {children}
      </div>
    </div>
  );
}

export function SignInForm() {
  const router = useRouter();
  const params = useSearchParams();
  const me = useMe();
  const next = safeNext(params.get("next"));
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => { if (me) router.replace(next); }, [me, next, router]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError("");
    try {
      const r = await authFetch<{ user: User }>("/login", { method: "POST", body: JSON.stringify({ email: email.trim(), password }) });
      setMe(r.user);
      router.replace(next);
      router.refresh();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  }

  return (
    <AuthCard title="Welcome back" subtitle="Sign in to your Lumen account.">
      <form onSubmit={submit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div className="field">
          <label htmlFor="email">Email</label>
          <input id="email" className="input" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
        </div>
        <PasswordField id="password" label="Password" value={password} onChange={setPassword} autoComplete="current-password" />
        {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
        <button type="submit" className="btn btn-primary auth-submit" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
      </form>
      <p className="auth-foot">New here? <Link href={`/signup${params.get("next") ? `?next=${encodeURIComponent(next)}` : ""}`}>Create an account with an invite code</Link></p>
    </AuthCard>
  );
}

const CODE_LEN = 6;

/** Digits only, at most 6. */
function formatCode(raw: string) {
  return raw.replace(/\D/g, "").slice(0, CODE_LEN);
}

export function SignUpForm() {
  const router = useRouter();
  const params = useSearchParams();
  const me = useMe();
  const next = safeNext(params.get("next"));
  const [step, setStep] = useState<1 | 2>(1);
  const [code, setCode] = useState(formatCode(params.get("code") ?? ""));
  const [role, setRole] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [avatar, setAvatar] = useState(() => AVATARS[Math.floor(Math.random() * AVATARS.length)].id);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const autoChecked = useRef(false);

  useEffect(() => { if (me && !busy) router.replace(next); }, [me, next, router, busy]);

  async function checkCode(e?: FormEvent, typed?: string) {
    e?.preventDefault();
    const value = typed ?? code;
    if (value.length !== CODE_LEN) return setError(`Invite codes have ${CODE_LEN} digits.`);
    setBusy(true); setError("");
    try {
      const r = await authFetch<{ role: string; code: string }>("/invite", { method: "POST", body: JSON.stringify({ code: value }) });
      setRole(r.role);
      setCode(r.code);
      setStep(2);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  }

  // A shared link (/signup?code=…) skips straight to the details.
  useEffect(() => {
    if (!autoChecked.current && params.get("code") && code.length === CODE_LEN) {
      autoChecked.current = true;
      void checkCode();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (password.length < 8) return setError("Use at least 8 characters for the password.");
    setBusy(true); setError("");
    try {
      const r = await authFetch<{ user: User }>("/signup", { method: "POST", body: JSON.stringify({ code, email: email.trim(), password, displayName: name, avatar }) });
      setMe(r.user);
      router.replace(next === "/" ? "/account?welcome=1" : next);
      router.refresh();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  }

  if (step === 1) {
    return (
      <AuthCard title="Create your account" subtitle="Lumen is invite-only for now. Enter the 6-digit code you were given.">
        <ol className="steps" aria-label="Progress"><li aria-current="step">Invite code</li><li>Your details</li></ol>
        <form onSubmit={checkCode} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <div className="field">
            <label htmlFor="code">Invite code</label>
            <input id="code" className="input code-input" value={code} placeholder="000000" maxLength={CODE_LEN} autoComplete="one-time-code"
              inputMode="numeric" pattern="[0-9]*" spellCheck={false} autoFocus
              onChange={(e) => {
                const c = formatCode(e.target.value);
                setCode(c);
                setError("");
                if (c.length === CODE_LEN && !busy) void checkCode(undefined, c); // check as soon as the 6th digit is in
              }} />
          </div>
          {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
          <button type="submit" className="btn btn-primary auth-submit" disabled={busy}>{busy ? "Checking…" : "Continue"}</button>
        </form>
        <p className="auth-foot">Already have an account? <Link href={`/login${params.get("next") ? `?next=${encodeURIComponent(next)}` : ""}`}>Sign in</Link></p>
      </AuthCard>
    );
  }

  return (
    <AuthCard title="Almost there" subtitle={<>Code <strong className="code-chip">{code}</strong> accepted{role === "admin" ? " · admin account" : ""}.</>}>
      <ol className="steps" aria-label="Progress"><li className="done">Invite code</li><li aria-current="step">Your details</li></ol>
      <form onSubmit={submit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div className="field">
          <span className="label">Pick an avatar</span>
          <div className="avatar-preview">
            <Avatar id={avatar} size={64} />
            <span className="muted" style={{ fontSize: 13 }}>You can change it any time in your profile.</span>
          </div>
          <AvatarPicker value={avatar} onChange={setAvatar} />
        </div>
        <div className="field">
          <label htmlFor="name">Your name</label>
          <input id="name" className="input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" maxLength={60} required autoFocus />
        </div>
        <div className="field">
          <label htmlFor="semail">Email</label>
          <input id="semail" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" required />
        </div>
        <PasswordField id="spw" label="Password" value={password} onChange={setPassword} autoComplete="new-password" hint={<Strength pw={password} />} />
        {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
        <button type="submit" className="btn btn-primary auth-submit" disabled={busy}>{busy ? "Creating your account…" : "Create account"}</button>
        <button type="button" className="link-btn" style={{ alignSelf: "center" }} onClick={() => { setStep(1); setError(""); }}>Use a different code</button>
      </form>
    </AuthCard>
  );
}
