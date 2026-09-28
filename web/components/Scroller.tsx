"use client";

import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeftIcon, ChevronRightIcon } from "@/components/icons";

/**
 * Horizontal row without a visible scrollbar. Touch and trackpads scroll natively;
 * mouse users get arrow buttons on hover, shown only when there is more to see.
 */
export function Scroller({ className, inset, children }: { className: string; inset?: boolean; children: ReactNode }) {
  const track = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({ prev: false, next: false });

  const update = useCallback(() => {
    const t = track.current;
    if (!t) return;
    setEdges({ prev: t.scrollLeft > 4, next: t.scrollLeft + t.clientWidth < t.scrollWidth - 4 });
  }, []);

  useEffect(() => {
    const t = track.current;
    if (!t) return;
    update();
    const ro = new ResizeObserver(update);
    ro.observe(t);
    t.addEventListener("scroll", update, { passive: true });
    return () => {
      ro.disconnect();
      t.removeEventListener("scroll", update);
    };
  }, [update]);

  const page = (dir: 1 | -1) => {
    const t = track.current;
    if (t) t.scrollBy({ left: dir * t.clientWidth * 0.85, behavior: "smooth" });
  };

  return (
    <div className={inset ? "scroller scroller-inset" : "scroller"}>
      <div ref={track} className={className}>{children}</div>
      <button type="button" className="scroll-btn prev" hidden={!edges.prev} onClick={() => page(-1)} aria-label="Scroll left">
        <ChevronLeftIcon />
      </button>
      <button type="button" className="scroll-btn next" hidden={!edges.next} onClick={() => page(1)} aria-label="Scroll right">
        <ChevronRightIcon />
      </button>
    </div>
  );
}
