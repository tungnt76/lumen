"use client";

import { use } from "react";
import { notFound } from "next/navigation";
import { DrawingEditor } from "@/components/DrawingEditor";
import { UUID_RE } from "@/lib/api";
import { StudioShell } from "@/components/StudioShell";
import { ALL_DRAWINGS } from "@/lib/drawings";

export default function StudioDrawingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  if (!UUID_RE.test(id)) notFound();
  return (
    <StudioShell active="/studio/drawings" bare>
      <DrawingEditor id={id} base={ALL_DRAWINGS} backHref="/studio/drawings" signInPath="/studio" />
    </StudioShell>
  );
}
