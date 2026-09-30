"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { useEffect, useState } from "react";
import { formatDuration, workHref, type WorkDetail } from "@/lib/api";
import {
  getListening, getWorkList, inWorkList, Listening, listeningFor, onLocalChange, SavedWork, toggleWorkList,
} from "@/lib/local";
import { clock, useAudio } from "./AudioPlayer";
import { CheckIcon, PauseIcon, PlayIcon, PlusIcon } from "./icons";
import { Scroller } from "./Scroller";
import { CoverArt } from "./Works";

function useLocal<T>(read: () => T, initial: T): T {
  const [v, setV] = useState<T>(initial);
  useEffect(() => {
    setV(read());
    return onLocalChange(() => setV(read()));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return v;
}

function saved(w: WorkDetail): SavedWork {
  return { id: w.id, kind: w.kind, title: w.title, creator: w.creator, cover: w.cover };
}

/** Play / Resume / Pause for a whole work, plus the My list toggle. */
export function WorkActions({ work }: { work: WorkDetail }) {
  const audio = useAudio();
  const resume = useLocal(() => listeningFor(work.id), undefined);
  const inList = useLocal(() => inWorkList(work.id), false);
  const current = audio.work?.id === work.id;

  let label = "Play";
  if (current && audio.playing) label = "Pause";
  else if (current) label = "Continue";
  else if (resume) label = work.tracks.length > 1 ? `Resume chapter ${resume.track + 1}` : `Resume at ${clock(resume.position)}`;

  return (
    <div className="actions">
      {work.tracks.length > 0 && (
        <button type="button" className="btn btn-primary" onClick={() => (current ? audio.toggle() : audio.play(work))}>
          {current && audio.playing ? <PauseIcon /> : <PlayIcon />} {label}
        </button>
      )}
      <button type="button" className="btn" aria-pressed={inList} onClick={() => toggleWorkList(saved(work))}>
        {inList ? <CheckIcon size={16} /> : <PlusIcon size={16} />}
        My list
      </button>
    </div>
  );
}

/** Chapters or songs; the one playing is highlighted, and any row starts playback there. */
export function TrackList({ work }: { work: WorkDetail }) {
  const audio = useAudio();
  const resume = useLocal(() => listeningFor(work.id), undefined);
  const current = audio.work?.id === work.id ? audio.index : -1;
  const noun = work.kind === "book" ? "chapters" : "tracks";
  return (
    <section className="tracks" aria-label={noun}>
      <h2>{work.tracks.length} {noun} · {formatDuration(work.durationSec)}</h2>
      <ol>
        {work.tracks.map((t, i) => {
          const active = i === current;
          return (
            <li key={t.position}>
              <button
                type="button"
                className="track"
                aria-current={active ? "true" : undefined}
                onClick={() => (active ? audio.toggle() : audio.play(work, i))}
                aria-label={`${active && audio.playing ? "Pause" : "Play"} ${t.title}`}
              >
                <span className="track-num">{active && audio.playing ? <PauseIcon size={14} /> : active ? <PlayIcon size={14} /> : i + 1}</span>
                <span className="track-title">{t.title}</span>
                {!active && resume?.track === i && <span className="chip chip-accent track-resume">Stopped here</span>}
                <span className="track-dur muted">{t.durationSec ? clock(t.durationSec) : ""}</span>
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

export function ContinueListening() {
  const items = useLocal<Listening[]>(getListening, []);
  if (!items.length) return null;
  return (
    <section aria-label="Continue listening">
      <div className="row-head" style={{ justifyContent: "flex-start" }}>
        <h2>Continue listening</h2>
        <span className="muted" style={{ fontSize: 13 }}>Saved on this device</span>
      </div>
      <Scroller className="row-scroll">
        {items.map((l) => (
          <Link key={l.id} href={workHref(l)} className="poster work-tile">
            <CoverArt work={l} />
            <div className="progress"><span style={{ width: `${Math.min(100, ((l.track + (l.duration ? l.position / l.duration : 0)) / Math.max(1, l.trackCount)) * 100)}%` }} /></div>
            <span className="poster-title">{l.title}</span>
            <span className="poster-meta">{l.trackCount > 1 ? `${l.kind === "book" ? "Chapter" : "Track"} ${l.track + 1} of ${l.trackCount}` : `${clock(l.position)} in`}</span>
          </Link>
        ))}
      </Scroller>
    </section>
  );
}

export function SavedWorksGrid() {
  const items = useLocal<SavedWork[]>(getWorkList, []);
  if (!items.length) return <p className="muted">No books or music saved yet. Tap “My list” on any book or album.</p>;
  return (
    <div className="grid grid-square">
      {items.map((w) => (
        <Link key={w.id} href={workHref(w)} className="poster work-tile">
          <CoverArt work={w} />
          <span className="poster-title">{w.title}</span>
          <span className="poster-meta">{w.creator}</span>
        </Link>
      ))}
    </div>
  );
}
