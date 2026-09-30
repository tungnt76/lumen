/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { formatDuration, toneFor, workHref, type WorkCard } from "@/lib/api";
import { Scroller } from "@/components/Scroller";

export function CoverArt({ work }: { work: Pick<WorkCard, "title" | "cover"> }) {
  return (
    <div className="poster-art cover-art" style={{ background: toneFor(work.title) }}>
      {work.cover ? <img src={work.cover} alt="" loading="lazy" /> : <span className="poster-fallback">{work.title}</span>}
    </div>
  );
}

/** "Jane Austen · 13h 6m" for hosted works, "Single · 2021" for link-out ones. */
export function workMeta(w: WorkCard) {
  if (!w.playable) return [w.genres[0], w.year].filter(Boolean).join(" · ");
  return [w.creator, formatDuration(w.durationSec)].filter(Boolean).join(" · ");
}

export function WorkTile({ work }: { work: WorkCard }) {
  return (
    <Link href={workHref(work)} className="poster work-tile">
      <div style={{ position: "relative" }}>
        <CoverArt work={work} />
        {work.playable ? <span className="poster-badge">Free</span> : <span className="poster-badge badge-out">Listen elsewhere</span>}
        {work.aiVoice && <span className="poster-badge badge-ai">AI voice</span>}
      </div>
      <span className="poster-title">{work.title}</span>
      <span className="poster-meta">{workMeta(work)}</span>
    </Link>
  );
}

export function WorkRow({ title, items, href }: { title: string; items: WorkCard[]; href?: string }) {
  if (!items?.length) return null;
  return (
    <section aria-label={title}>
      <div className="row-head">
        <h2>{title}</h2>
        {href && <Link href={href}>See all</Link>}
      </div>
      <Scroller className="row-scroll">
        {items.map((w) => <WorkTile key={w.id} work={w} />)}
      </Scroller>
    </section>
  );
}

export function WorkGrid({ items }: { items: WorkCard[] }) {
  return <div className="grid grid-square">{items.map((w) => <WorkTile key={w.id} work={w} />)}</div>;
}

/** The audio counterpart of the film hero: a large card each for the featured book and album. */
export function AudioSpotlight({ book, album }: { book?: WorkCard; album?: WorkCard }) {
  const items = [book && { w: book, label: "Featured audiobook" }, album && { w: album, label: "Featured album" }].filter(Boolean) as { w: WorkCard; label: string }[];
  if (!items.length) return null;
  return (
    <section aria-label="Featured audiobook and album">
      <div className="row-head"><h2>Listen on Lumen</h2></div>
      <div className="spotlight">
        {items.map(({ w, label }) => (
          <Link key={w.id} href={workHref(w)} className="spot" style={{ background: toneFor(w.title) }}>
            {w.cover && <img className="spot-bg" src={w.cover} alt="" loading="lazy" />}
            <div className="spot-shade" />
            <div className="spot-cover"><CoverArt work={w} /></div>
            <div className="spot-body">
              <span className="eyebrow">{label} · Free</span>
              <h3>{w.title}</h3>
              <span className="spot-meta">{[w.creator, w.trackCount > 1 ? `${w.trackCount} ${w.kind === "book" ? "chapters" : "tracks"}` : "", formatDuration(w.durationSec)].filter(Boolean).join(" · ")}</span>
              <span className="btn btn-primary spot-btn">Listen now</span>
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}
