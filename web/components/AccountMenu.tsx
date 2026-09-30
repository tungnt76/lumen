"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { signOut, useMe } from "@/lib/auth";
import { Avatar } from "./Avatar";

/** Header: "Sign in" for visitors; the avatar with a small menu once signed in. */
export function AccountMenu() {
  const me = useMe();
  const path = usePathname();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => setOpen(false), [path]);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDown); document.removeEventListener("keydown", onKey); };
  }, [open]);

  if (me === undefined) return <span className="account-slot" aria-hidden="true" />;
  if (!me) {
    const next = path && path !== "/login" && path !== "/signup" ? `?next=${encodeURIComponent(path)}` : "";
    return <Link href={`/login${next}`} className="btn btn-signin">Sign in</Link>;
  }

  return (
    <div className="account" ref={box}>
      <button type="button" className="account-btn" aria-haspopup="menu" aria-expanded={open} aria-label={`Account: ${me.displayName}`} onClick={() => setOpen(!open)}>
        <Avatar id={me.avatar} name={me.displayName} size={38} />
      </button>
      {open && (
        <div className="account-menu" role="menu">
          <div className="account-head">
            <Avatar id={me.avatar} name={me.displayName} size={44} />
            <div style={{ minWidth: 0 }}>
              <div className="account-name">{me.displayName}</div>
              <div className="account-email">{me.email}</div>
              <span className={`role-badge role-${me.role}`}>{me.role === "admin" ? "Admin" : "Member"}</span>
            </div>
          </div>
          <Link href="/account" role="menuitem">Profile &amp; settings</Link>
          <Link href="/my-list" role="menuitem">My list</Link>
          {me.role === "admin" && <Link href="/studio/films" role="menuitem">Studio</Link>}
          <button type="button" role="menuitem" onClick={async () => { await signOut(); router.refresh(); }}>Sign out</button>
        </div>
      )}
    </div>
  );
}
