"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { useEffect, useState } from "react";
import { getList, getProgress, inList, onLocalChange, Progress, SavedFilm, toggleList } from "@/lib/local";
import { CheckIcon, PlusIcon } from "./icons";
import { Poster } from "./Poster";
import { Scroller } from "./Scroller";

function useLocal<T>(read: () => T, initial: T): T {
  const [v, setV] = useState<T>(initial);
  useEffect(() => {
    setV(read());
    return onLocalChange(() => setV(read()));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return v;
}

export function ContinueWatching() {
  const items = useLocal<Progress[]>(getProgress, []);
  if (!items.length) return null;
  return (
    <section aria-label="Continue watching">
      <div className="row-head" style={{ justifyContent: "flex-start" }}>
        <h2>Continue watching</h2>
        <span className="muted" style={{ fontSize: 13 }}>Saved on this device, no account needed</span>
      </div>
      <Scroller className="row-scroll">
        {items.map((p) => {
          const left = Math.max(0, Math.round((p.duration - p.position) / 60));
          return (
            <Link key={p.tmdbId} href={`/movie/${p.tmdbId}?play=1`} className="poster wide">
              <div className="poster-art" style={{ display: "flex", flexDirection: "column", justifyContent: "flex-end" }}>
                {p.backdrop && <img src={p.backdrop} alt="" loading="lazy" style={{ position: "absolute", inset: 0 }} />}
                <div className="progress" style={{ position: "relative", margin: 12 }}>
                  <span style={{ width: `${Math.min(100, (p.position / p.duration) * 100)}%` }} />
                </div>
              </div>
              <span className="poster-title">{p.title}</span>
              <span className="poster-meta">{left} min left</span>
            </Link>
          );
        })}
      </Scroller>
    </section>
  );
}

export function ListButton({ film, compact }: { film: SavedFilm; compact?: boolean }) {
  const saved = useLocal(() => inList(film.tmdbId), false);
  const label = saved ? "Remove from my list" : "Add to my list";
  if (compact) {
    return (
      <button type="button" className="icon-btn" style={{ width: 48, height: 48, borderRadius: 24 }} aria-label={label} aria-pressed={saved} onClick={() => toggleList(film)}>
        {saved ? <CheckIcon /> : <PlusIcon />}
      </button>
    );
  }
  return (
    <button type="button" className="btn" aria-pressed={saved} onClick={() => toggleList(film)}>
      {saved ? <CheckIcon size={16} /> : <PlusIcon size={16} />}
      My list
    </button>
  );
}

export function MyListGrid() {
  const items = useLocal<SavedFilm[]>(getList, []);
  if (!items.length) {
    return <p className="muted">Nothing here yet. Tap “My list” on any film to save it on this device.</p>;
  }
  return (
    <div className="grid">
      {items.map((f) => (
        <Poster key={f.tmdbId} card={{ ...f, overview: "", genres: [], playable: false }} showBadge={false} />
      ))}
    </div>
  );
}
