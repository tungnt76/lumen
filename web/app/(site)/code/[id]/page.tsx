"use client";

import dynamic from "next/dynamic";
import { notFound, useRouter } from "next/navigation";
import { use, useEffect } from "react";
import { UUID_RE } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { MY_CODE } from "@/lib/code";

const CodeWorkspace = dynamic(async () => (await import("@/components/CodeWorkspace")).CodeWorkspace, {
  ssr: false,
  loading: () => <div className="code-page embedded"><div className="draw-loading muted">Loading the editor…</div></div>,
});

export default function ProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const me = useMe();
  if (!UUID_RE.test(id)) notFound();
  useEffect(() => { if (me === null) router.replace(`/login?next=/code/${id}`); }, [me, id, router]);
  if (!me) return <div className="code-page embedded"><div className="draw-loading muted">Loading…</div></div>;
  return <CodeWorkspace mode="saved" id={id} base={MY_CODE} backHref="/code" signInPath={`/login?next=/code/${id}`} embedded />;
}
