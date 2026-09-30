"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import type { Page } from "@/lib/api";
import type { User } from "@/lib/auth";
import { adminFetch, ApiError } from "@/lib/studio";
import { Avatar } from "@/components/Avatar";
import { StudioShell } from "@/components/StudioShell";

type Invite = { code: string; role: "admin" | "member"; maxUses: number; uses: number; note: string; expiresAt: string | null; disabled: boolean; createdAt: string; usable: boolean };
type AdminUser = User & { disabled: boolean };

function when(iso: string | null) {
  return iso ? new Date(iso).toLocaleDateString() : "—";
}

export default function StudioUsers() {
  return (
    <StudioShell active="/studio/users">
      <h1 style={{ fontSize: 34 }}>Users</h1>
      <Invites />
      <UsersTable />
    </StudioShell>
  );
}

function copy(text: string, done: () => void) {
  navigator.clipboard?.writeText(text).then(done).catch(() => window.prompt("Copy this:", text));
}

function Invites() {
  const [list, setList] = useState<Invite[]>([]);
  const [role, setRole] = useState("member");
  const [uses, setUses] = useState(1);
  const [days, setDays] = useState(14);
  const [note, setNote] = useState("");
  const [fresh, setFresh] = useState<Invite | null>(null);
  const [copied, setCopied] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => { adminFetch<Invite[]>("/invites").then(setList).catch((e) => setError((e as Error).message)); }, []);
  useEffect(load, [load]);

  async function create() {
    setBusy(true); setError(""); setCopied("");
    try {
      const inv = await adminFetch<Invite>("/invites", { method: "POST", body: JSON.stringify({ role, maxUses: uses, expiresInDays: days, note }) });
      setFresh(inv);
      setNote("");
      load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function toggle(inv: Invite) {
    await adminFetch(`/invites/${inv.code}`, { method: "PATCH", body: JSON.stringify({ disabled: !inv.disabled }) }).catch((e) => setError((e as Error).message));
    load();
  }
  async function remove(inv: Invite) {
    if (!window.confirm(`Delete code ${inv.code}? Accounts made with it stay.`)) return;
    await adminFetch(`/invites/${inv.code}`, { method: "DELETE" }).catch((e) => setError((e as Error).message));
    if (fresh?.code === inv.code) setFresh(null);
    load();
  }
  const link = (c: string) => `${window.location.origin}/signup?code=${c}`;

  return (
    <section className="panel" style={{ gap: 18 }}>
      <h2>Invite codes</h2>
      <p className="muted" style={{ margin: 0, fontSize: 14 }}>People need a code to create an account. Share the code, or the link that fills it in for them.</p>
      <div className="invite-form">
        <div className="field">
          <label htmlFor="irole">Role</label>
          <select id="irole" className="input" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <div className="field">
          <label htmlFor="iuses">Sign-ups</label>
          <select id="iuses" className="input" value={uses} onChange={(e) => setUses(Number(e.target.value))}>
            {[1, 5, 10, 25, 100].map((n) => <option key={n} value={n}>{n === 1 ? "1 person" : `${n} people`}</option>)}
          </select>
        </div>
        <div className="field">
          <label htmlFor="idays">Expires</label>
          <select id="idays" className="input" value={days} onChange={(e) => setDays(Number(e.target.value))}>
            <option value={1}>in 1 day</option>
            <option value={7}>in 7 days</option>
            <option value={14}>in 14 days</option>
            <option value={30}>in 30 days</option>
            <option value={0}>never</option>
          </select>
        </div>
        <div className="field" style={{ flex: "2 1 200px" }}>
          <label htmlFor="inote">Note <span className="muted">(optional)</span></label>
          <input id="inote" className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Who it's for" maxLength={200} />
        </div>
        <button type="button" className="btn btn-primary" style={{ alignSelf: "flex-end", height: 46 }} onClick={create} disabled={busy}>{busy ? "Creating…" : "Create code"}</button>
      </div>

      {fresh && (
        <div className="fresh-code" role="status">
          <span className="muted" style={{ fontSize: 13 }}>New {fresh.role} code{fresh.maxUses > 1 ? ` for ${fresh.maxUses} people` : ""}</span>
          <code>{fresh.code}</code>
          <div className="actions">
            <button type="button" className="btn" onClick={() => copy(fresh.code, () => setCopied("code"))}>{copied === "code" ? "Copied!" : "Copy code"}</button>
            <button type="button" className="btn" onClick={() => copy(link(fresh.code), () => setCopied("link"))}>{copied === "link" ? "Copied!" : "Copy sign-up link"}</button>
          </div>
        </div>
      )}
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}

      {list.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead><tr><th>Code</th><th>Role</th><th>Used</th><th>Expires</th><th>Note</th><th>Status</th><th><span className="sr-only">Actions</span></th></tr></thead>
            <tbody>
              {list.map((inv) => (
                <tr key={inv.code}>
                  <td><code className="code-chip">{inv.code}</code></td>
                  <td><span className={`role-badge role-${inv.role}`}>{inv.role}</span></td>
                  <td>{inv.uses} / {inv.maxUses}</td>
                  <td className="muted">{inv.expiresAt ? when(inv.expiresAt) : "never"}</td>
                  <td className="muted" style={{ maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{inv.note}</td>
                  <td>{inv.usable ? <span className="badge badge-ready">Active</span> : <span className="badge badge-idle">{inv.disabled ? "Disabled" : inv.uses >= inv.maxUses ? "Used up" : "Expired"}</span>}</td>
                  <td style={{ whiteSpace: "nowrap", fontSize: 14 }}>
                    {inv.usable && <button type="button" className="link-btn" style={{ marginRight: 14 }} onClick={() => copy(link(inv.code), () => setCopied(inv.code))}>{copied === inv.code ? "Copied!" : "Copy link"}</button>}
                    <button type="button" className="link-btn" style={{ marginRight: 14 }} onClick={() => toggle(inv)}>{inv.disabled ? "Enable" : "Disable"}</button>
                    <button type="button" className="link-btn danger" onClick={() => remove(inv)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function UsersTable() {
  const router = useRouter();
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [q, setQ] = useState("");
  const [total, setTotal] = useState(0);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const res = await adminFetch<Page<AdminUser>>(`/users?q=${encodeURIComponent(q.trim())}`);
      setUsers(res.items);
      setTotal(res.total);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) router.replace("/studio");
      else setError((e as Error).message);
    }
  }, [q, router]);
  useEffect(() => { const t = window.setTimeout(() => void load(), 250); return () => window.clearTimeout(t); }, [load]);

  async function patch(u: AdminUser, body: Partial<Pick<AdminUser, "role" | "disabled">>) {
    setError("");
    try { await adminFetch(`/users/${u.id}`, { method: "PATCH", body: JSON.stringify(body) }); }
    catch (e) { setError(`${u.displayName}: ${(e as Error).message}`); }
    load();
  }

  return (
    <section style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <h2 style={{ fontSize: 20, marginRight: "auto" }}>Accounts <span className="muted" style={{ fontWeight: 400 }}>· {total}</span></h2>
        <label htmlFor="uq" className="sr-only">Search accounts</label>
        <input id="uq" className="input" style={{ width: 240, height: 40 }} type="search" placeholder="Search name or email" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {error && <p className="error" role="alert" style={{ margin: 0 }}>{error}</p>}
      <div className="table-wrap">
        <table className="table">
          <thead><tr><th>User</th><th>Role</th><th>Status</th><th>Joined</th><th>Last sign-in</th></tr></thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} style={u.disabled ? { opacity: 0.6 } : undefined}>
                <td>
                  <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                    <Avatar id={u.avatar} name={u.displayName} size={36} />
                    <span style={{ minWidth: 0 }}>
                      <span style={{ display: "block", fontWeight: 600 }}>{u.displayName}</span>
                      <span className="muted" style={{ fontSize: 13 }}>{u.email}</span>
                    </span>
                  </div>
                </td>
                <td>
                  <label className="sr-only" htmlFor={`role-${u.id}`}>Role for {u.displayName}</label>
                  <select id={`role-${u.id}`} className="input" style={{ height: 36, width: 120 }} value={u.role} onChange={(e) => patch(u, { role: e.target.value as AdminUser["role"] })}>
                    <option value="member">Member</option>
                    <option value="admin">Admin</option>
                  </select>
                </td>
                <td>
                  <button type="button" className={`badge ${u.disabled ? "badge-failed" : "badge-ready"}`} style={{ border: 0, cursor: "pointer" }}
                    onClick={() => { if (u.disabled || window.confirm(`Disable ${u.displayName}? They're signed out everywhere and can't sign in.`)) void patch(u, { disabled: !u.disabled }); }}
                    title={u.disabled ? "Click to enable" : "Click to disable"}>
                    {u.disabled ? "Disabled" : "Active"}
                  </button>
                </td>
                <td className="muted">{when(u.createdAt)}</td>
                <td className="muted">{when(u.lastLoginAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
