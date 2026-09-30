/* eslint-disable @next/next/no-img-element */
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { api, formatDuration, languageNames, UUID_RE, type WorkDetail, type WorkKind } from "@/lib/api";
import { ExternalIcon } from "@/components/icons";
import { TrackList, WorkActions } from "@/components/WorkControls";
import { CoverArt } from "@/components/Works";

export async function loadWork(kind: WorkKind, id: string) {
  if (!UUID_RE.test(id)) return null;
  const w = await api<WorkDetail>(`/api/works/${id}`, 300);
  return w && w.kind === kind ? w : null;
}

export function workMetadata(w: WorkDetail | null): Metadata {
  if (!w) return { title: "Not found" };
  return {
    title: w.creator ? `${w.title} · ${w.creator}` : w.title,
    description: (w.description || `${w.title} by ${w.creator}`).slice(0, 160),
    openGraph: { images: w.cover ? [w.cover] : [] },
  };
}

/** A book or album: cover, details, play controls and chapters, or official links when not hosted. */
export function WorkPage({ work: w }: { work: WorkDetail | null }) {
  if (!w) notFound();
  const book = w.kind === "book";

  return (
    <div className="wrap" style={{ paddingTop: 36, paddingBottom: 64, display: "flex", flexDirection: "column", gap: 40 }}>
      <div className="work-head">
        <div className="work-cover"><CoverArt work={w} /></div>
        <div style={{ display: "flex", flexDirection: "column", gap: 18, minWidth: 0 }}>
          <span className="eyebrow">{book ? "Audiobook" : w.genres[0] || "Album"}</span>
          <h1>{w.title}</h1>
          {w.creator && <p className="work-creator">{book ? "by " : ""}{w.creator}</p>}
          <div className="chips">
            {w.playable ? <span className="chip chip-accent">Free on Lumen</span> : <span className="chip">Not hosted on Lumen</span>}
            {w.aiVoice && <span className="chip">Giọng đọc AI · AI voice</span>}
            {w.language && <span className="chip">{languageNames[w.language] ?? w.language}</span>}
            {w.year ? <span className="chip">{w.year}</span> : null}
            {w.durationSec > 0 && <span className="chip">{formatDuration(w.durationSec)}</span>}
          </div>
          {w.playable && <WorkActions work={w} />}
          {!w.playable && w.links.length > 0 && (
            <div className="actions">
              {w.links.map((l, i) => (
                <a key={l.url} href={l.url} target="_blank" rel="noopener noreferrer" className={i === 0 ? "btn btn-primary" : "btn"}>
                  {l.name} <ExternalIcon size={15} />
                </a>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="detail">
        <div style={{ display: "flex", flexDirection: "column", gap: 28, minWidth: 0 }}>
          {w.description && <p className="overview">{w.description}</p>}
          {!w.playable && (
            <p className="overview muted" style={{ fontSize: 15 }}>
              This music is copyrighted by the artist and their label, so Lumen doesn&apos;t host it. Listen on the
              artist&apos;s official channels above; streaming there supports the artist.
            </p>
          )}
          {w.tracks.length > 0 && <TrackList work={w} />}
        </div>

        <aside className="panel">
          <h2>Details</h2>
          {w.creator && <div className="facts"><span className="muted">{book ? "Author" : "Artist"}</span><span>{w.creator}</span></div>}
          {w.narrator && <div className="facts"><span className="muted">Read by</span><span>{w.narrator}</span></div>}
          {w.genres.length > 0 && <div className="facts"><span className="muted">Genre</span><span>{w.genres.slice(0, 3).join(", ")}</span></div>}
          {w.durationSec > 0 && <div className="facts"><span className="muted">Length</span><span>{formatDuration(w.durationSec)}</span></div>}
          <div className="facts"><span className="muted">Rights</span><span>{w.license}</span></div>
          {w.sourceUrl && (
            <a href={w.sourceUrl} target="_blank" rel="noopener noreferrer" style={{ fontSize: 14 }}>
              {w.playable ? "Source and licence" : "Release info on MusicBrainz"}
            </a>
          )}
          {w.aiVoice && (
            <span className="muted" style={{ fontSize: 12 }}>
              Read by a text-to-speech model from a public-domain text. Pronunciation may be imperfect.
            </span>
          )}
        </aside>
      </div>
    </div>
  );
}
