"use client";

import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";
import { adminFetch } from "@/lib/studio";
import { LogoMark } from "@/components/Logo";

export default function StudioLogin() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    // Already signed in: go straight to the dashboard.
    adminFetch("/me").then(() => router.replace("/studio/films")).catch(() => {});
  }, [router]);

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const email = String(data.get("email") || "").trim();
    const password = String(data.get("password") || "");
    if (!email || !password) return setError("Enter your email and password.");
    setBusy(true);
    setError("");
    try {
      await adminFetch("/login", { method: "POST", body: JSON.stringify({ email, password }) });
      router.replace("/studio/films");
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  }

  return (
    <div className="login-wrap">
      <form className="login" onSubmit={submit} noValidate>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <LogoMark size={28} />
          <span className="logo-word" style={{ fontSize: 24 }}>Lumen</span>
          <span className="chip chip-accent" style={{ fontSize: 12, fontWeight: 600 }}>Studio</span>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          <h1 style={{ fontSize: 28 }}>Admin sign in</h1>
          <p className="muted" style={{ margin: 0, fontSize: 14 }}>Staff only.</p>
        </div>
        <div className="field">
          <label htmlFor="email">Email</label>
          <input id="email" name="email" type="email" className="input" autoComplete="username" required />
        </div>
        <div className="field">
          <label htmlFor="password">Password</label>
          <input id="password" name="password" type="password" className="input" autoComplete="current-password" required />
        </div>
        {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
        <button type="submit" className="btn btn-primary" disabled={busy} style={{ justifyContent: "center" }}>
          {busy ? "Signing in…" : "Sign in"}
        </button>
        <span className="muted" style={{ fontSize: 12, textAlign: "center" }}>5 failed attempts locks sign-in for 15 minutes.</span>
      </form>
    </div>
  );
}
