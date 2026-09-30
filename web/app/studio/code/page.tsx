"use client";

import { CodeList } from "@/components/CodeList";
import { StudioShell } from "@/components/StudioShell";
import { ALL_CODE } from "@/lib/code";

export default function StudioCode() {
  return (
    <StudioShell active="/studio/code">
      <CodeList base={ALL_CODE} hrefFor={(id) => `/studio/code/${id}`} title="Code" showOwner signInPath="/studio"
        intro="Everyone's code projects, with who made them. Files are JSON bundles in the media bucket's code/ folder." />
    </StudioShell>
  );
}
