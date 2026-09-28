/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { Card, formatRuntime, toneFor } from "@/lib/api";
import { Scroller } from "@/components/Scroller";

export function PosterArt({ card, size }: { card: Pick<Card, "title" | "poster">; size?: "wide" }) {
  return (
    <div className="poster-art" style={{ background: toneFor(card.title) }}>
      {card.poster ? (
        <img src={card.poster} alt="" loading="lazy" />
      ) : (
        <span className="poster-fallback" style={size === "wide" ? { fontSize: 22 } : undefined}>{card.title}</span>
      )}
    </div>
  );
}

export function Poster({ card, showBadge = true }: { card: Card; showBadge?: boolean }) {
  const meta = [card.year || "", card.genres?.[0] || "", formatRuntime(card.runtime)].filter(Boolean).join(" · ");
  return (
    <Link href={`/movie/${card.tmdbId}`} className="poster">
      <div style={{ position: "relative" }}>
        <PosterArt card={card} />
        {showBadge && card.playable && <span className="poster-badge">Free</span>}
      </div>
      <span className="poster-title">{card.title}</span>
      {meta && <span className="poster-meta">{meta}</span>}
    </Link>
  );
}

/** Ranked row: big outlined numbers beside each poster, as in the design's "Top 5". */
export function TopRow({ title, items }: { title: string; items: Card[] }) {
  if (!items?.length) return null;
  return (
    <section aria-label={title}>
      <div className="row-head">
        <h2>{title}</h2>
      </div>
      <ol className="top-row">
        {items.map((c, i) => (
          <li key={c.tmdbId}>
            <Link href={`/movie/${c.tmdbId}`} className="top" aria-label={`${i + 1}. ${c.title}`}>
              <span className="top-num" aria-hidden="true">{i + 1}</span>
              <PosterArt card={c} />
            </Link>
          </li>
        ))}
      </ol>
    </section>
  );
}

export function PosterRow({ title, items, href }: { title: string; items: Card[]; href?: string }) {
  if (!items?.length) return null;
  return (
    <section aria-label={title}>
      <div className="row-head">
        <h2>{title}</h2>
        {href && <Link href={href}>See all</Link>}
      </div>
      <Scroller className="row-scroll">
        {items.map((c) => <Poster key={c.tmdbId} card={c} />)}
      </Scroller>
    </section>
  );
}
