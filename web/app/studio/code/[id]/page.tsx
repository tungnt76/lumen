"use client";

import dynamic from "next/dynamic";
import { notFound } from "next/navigation";
import { use } from "react";
import { UUID_RE } from "@/lib/api";
import { ALL_CODE } from "@/lib/code";
import { StudioShell } from "@/components/StudioShell";

const CodeWorkspace = dynamic(async () => (await import("@/components/CodeWorkspace")).CodeWorkspace, { ssr: false });

export default function StudioProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  if (!UUID_RE.test(id)) notFound();
  return (
    <StudioShell active="/studio/code" bare>
      <CodeWorkspace mode="saved" id={id} base={ALL_CODE} backHref="/studio/code" signInPath="/studio" />
    </StudioShell>
  );
}
