"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ReactNode, useEffect, useState } from "react";
import { adminFetch } from "@/lib/studio";
import { LogoMark } from "./Logo";
import { Avatar } from "./Avatar";
import type { User } from "@/lib/auth";

const tabs = [
  { href: "/studio/films", label: "Films" },
  { href: "/studio/works", label: "Books & music" },
  { href: "/studio/drawings", label: "Excalidraw" },
  { href: "/studio/code", label: "Code" },
  { href: "/studio/users", label: "Users" },
];

/** Studio chrome: checks the session, then shows the sidebar and the page. `bare` skips the
 * sidebar for full-screen tools such as the drawing editor. */
export function StudioShell({ active, bare, children }: { active: string; bare?: boolean; children: ReactNode }) {
  const router = useRouter();
  const [me, setMe] = useState<User | null>(null);

  useEffect(() => {
    adminFetch<{ user: User }>("/me").then((m) => setMe(m.user)).catch(() => router.replace("/studio"));
  }, [router]);

  async function logout() {
    await adminFetch("/logout", { method: "POST" }).catch(() => {});
    router.replace("/studio");
  }

  if (!me) return null;
  if (bare) return <>{children}</>;

  return (
    <div className="studio">
      <aside className="side">
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "0 10px 20px" }}>
          <LogoMark size={24} />
          <span className="logo-word" style={{ fontSize: 22 }}>Lumen</span>
          <span className="chip chip-accent" style={{ fontSize: 11, fontWeight: 600, padding: "2px 7px" }}>Studio</span>
        </div>
        {tabs.map((t) => (
          <Link key={t.href} href={t.href} aria-current={active === t.href ? "page" : undefined}>{t.label}</Link>
        ))}
        <Link href="/" target="_blank">View public site</Link>
        <Link href="/account" className="side-me" style={{ marginTop: "auto" }}>
          <Avatar id={me.avatar} name={me.displayName} size={32} />
          <span style={{ minWidth: 0 }}>
            <span style={{ display: "block", color: "var(--text)", fontWeight: 600 }}>{me.displayName}</span>
            <span className="muted" style={{ fontSize: 12 }}>{me.email}</span>
          </span>
        </Link>
        <button type="button" className="btn" style={{ height: 40, margin: "8px 12px 0" }} onClick={logout}>Sign out</button>
      </aside>
      <main className="studio-main">{children}</main>
    </div>
  );
}
