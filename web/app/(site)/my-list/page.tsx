import type { Metadata } from "next";
import { MyListGrid } from "@/components/Local";

export const metadata: Metadata = { title: "My list" };

export default function MyListPage() {
  return (
    <div className="wrap" style={{ padding: "44px var(--gutter) 64px", display: "flex", flexDirection: "column", gap: 24 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <h1 style={{ fontSize: "clamp(34px, 4vw, 48px)" }}>My list</h1>
        <p className="muted" style={{ margin: 0 }}>Saved in this browser only. No account, nothing sent to us.</p>
      </div>
      <MyListGrid />
    </div>
  );
}
