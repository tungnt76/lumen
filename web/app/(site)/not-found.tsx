import Link from "next/link";

export default function NotFound() {
  return (
    <div className="wrap" style={{ padding: "96px var(--gutter)", display: "flex", flexDirection: "column", gap: 16, alignItems: "flex-start" }}>
      <h1 style={{ fontSize: 48 }}>Film not found</h1>
      <p className="muted" style={{ margin: 0 }}>It may have been removed, or the link is wrong.</p>
      <Link href="/browse" className="btn btn-primary">Browse films</Link>
    </div>
  );
}
