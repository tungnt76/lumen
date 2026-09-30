"use client";

import { use, useEffect } from "react";
import { notFound, useRouter } from "next/navigation";
import { DrawingEditor } from "@/components/DrawingEditor";
import { UUID_RE } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { MY_DRAWINGS } from "@/lib/drawings";

export default function MyDrawingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const me = useMe();
  if (!UUID_RE.test(id)) notFound();
  useEffect(() => { if (me === null) router.replace(`/login?next=/draw/${id}`); }, [me, id, router]);
  if (!me) return <div className="guest-draw"><div className="draw-loading muted">Loading…</div></div>;
  return <DrawingEditor id={id} base={MY_DRAWINGS} backHref="/draw" signInPath={`/login?next=/draw/${id}`} embedded />;
}
