// The signed-in user, shared by the nav, account pages and anything that differs for
// members or admins. Sessions are HttpOnly cookies; this only mirrors /api/auth/me.

import { useEffect, useState } from "react";

export type User = {
  id: number;
  email: string;
  displayName: string;
  role: "admin" | "member";
  avatar: string;
  bio: string;
  createdAt: string;
  lastLoginAt: string | null;
};

export class AuthError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function authFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api/auth${path}`, {
    ...init,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new AuthError(res.status, body.error || `Request failed (${res.status})`);
  return body as T;
}

const EVENT = "lumen:auth";
let cached: User | null | undefined; // undefined = not loaded yet
let inflight: Promise<User | null> | null = null;

function hinted() {
  return document.cookie.split("; ").includes("lumen_auth=1");
}

/** Loads the current user once per page load (skipped entirely for visitors without the hint cookie). */
export function loadMe(force = false): Promise<User | null> {
  if (!force && cached !== undefined) return Promise.resolve(cached);
  if (!force && inflight) return inflight;
  if (!hinted()) { cached = null; return Promise.resolve(null); }
  inflight = authFetch<{ user: User }>("/me")
    .then((r) => r.user)
    .catch(() => null)
    .then((u) => { cached = u; inflight = null; return u; });
  return inflight;
}

/** Call after sign-in, sign-out or a profile change so every useMe() updates. */
export function setMe(u: User | null) {
  cached = u;
  window.dispatchEvent(new Event(EVENT));
}

/** undefined while loading, then the user or null. */
export function useMe() {
  const [me, setState] = useState<User | null | undefined>(cached);
  useEffect(() => {
    let alive = true;
    loadMe().then((u) => { if (alive) setState(u); });
    const on = () => setState(cached);
    window.addEventListener(EVENT, on);
    return () => { alive = false; window.removeEventListener(EVENT, on); };
  }, []);
  return me;
}

export async function signOut() {
  await authFetch("/logout", { method: "POST" }).catch(() => {});
  setMe(null);
}

/** A same-site path to return to after signing in (never another site). */
export function safeNext(next: string | null | undefined, fallback = "/") {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : fallback;
}

/** "Chrome on macOS" from a user-agent string. */
export function describeDevice(ua: string) {
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
  const os = /iPhone/.test(ua) ? "iPhone" : /iPad/.test(ua) ? "iPad" : /Android/.test(ua) ? "Android" : /Mac OS X|Macintosh/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  return os ? `${browser} on ${os}` : browser;
}
