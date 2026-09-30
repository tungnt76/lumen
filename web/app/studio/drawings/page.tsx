"use client";

import { DrawingList } from "@/components/DrawingList";
import { StudioShell } from "@/components/StudioShell";
import { ALL_DRAWINGS } from "@/lib/drawings";

export default function StudioDrawings() {
  return (
    <StudioShell active="/studio/drawings">
      <DrawingList
        base={ALL_DRAWINGS}
        hrefFor={(id) => `/studio/drawings/${id}`}
        title="Excalidraw"
        intro="Everyone's drawings, with who made them. They're saved to R2 in the media bucket's drawings/ folder under unguessable names."
        showOwner
        signInPath="/studio"
      />
    </StudioShell>
  );
}
