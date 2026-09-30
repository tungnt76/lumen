"use client";

/* eslint-disable @next/next/no-img-element */
import Link from "next/link";
import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { toneFor, workHref, type WorkDetail } from "@/lib/api";
import { getRate, listeningFor, saveListening, saveRate } from "@/lib/local";
import { Back15Icon, CloseIcon, Fwd30Icon, MoonIcon, NextIcon, PauseIcon, PlayIcon, PrevIcon } from "./icons";

/**
 * One <audio> element for the whole site, so books and music keep playing while the listener
 * moves between pages. The layout mounts the provider; pages call useAudio() to start playback.
 */

type Sleep = "off" | "15" | "30" | "60" | "track";

type AudioState = {
  work: WorkDetail | null;
  index: number;
  playing: boolean;
  /** Play a work from a track (default: where the listener stopped, or the start). */
  play: (work: WorkDetail, index?: number) => void;
  toggle: () => void;
};

const AudioContext = createContext<AudioState | null>(null);

export function useAudio() {
  const ctx = useContext(AudioContext);
  if (!ctx) throw new Error("useAudio needs <AudioProvider>");
  return ctx;
}

const RATES = [0.75, 1, 1.25, 1.5, 1.75, 2];

export function clock(s: number) {
  if (!isFinite(s) || s < 0) s = 0;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = Math.floor(s % 60);
  return (h ? `${h}:${String(m).padStart(2, "0")}` : `${m}`) + `:${String(sec).padStart(2, "0")}`;
}

export function AudioProvider({ children }: { children: ReactNode }) {
  const audio = useRef<HTMLAudioElement>(null);
  const [work, setWork] = useState<WorkDetail | null>(null);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [rate, setRate] = useState(1);
  const [sleep, setSleep] = useState<Sleep>("off");
  const [error, setError] = useState("");
  const sleepTimer = useRef<number | undefined>(undefined);
  // Latest values for event handlers registered once.
  const live = useRef({ work, index, sleep });
  live.current = { work, index, sleep };

  useEffect(() => setRate(getRate()), []);

  const persist = useCallback(() => {
    const a = audio.current;
    const { work: w, index: i } = live.current;
    if (!a || !w || !a.duration || !isFinite(a.duration)) return;
    saveListening({
      id: w.id, kind: w.kind, title: w.title, creator: w.creator, cover: w.cover,
      track: i, trackCount: w.tracks.length, position: a.currentTime, duration: a.duration,
    });
  }, []);

  /** Loads track i of w and starts it at `at` seconds. */
  const load = useCallback((w: WorkDetail, i: number, at = 0) => {
    const a = audio.current;
    const t = w.tracks[i];
    if (!a || !t) return;
    setWork(w);
    setIndex(i);
    setTime(at);
    setDuration(t.durationSec);
    setError("");
    a.src = t.url;
    a.playbackRate = getRate();
    if (at > 0) a.addEventListener("loadedmetadata", () => { a.currentTime = at; }, { once: true });
    a.play().catch(() => setPlaying(false)); // autoplay blocked: the listener presses play
  }, []);

  const play = useCallback((w: WorkDetail, i?: number) => {
    if (!w.tracks.length) return;
    if (i === undefined) {
      const saved = listeningFor(w.id);
      if (saved && saved.track < w.tracks.length) {
        load(w, saved.track, saved.position > 5 ? saved.position - 3 : 0); // replay a few seconds for context
        return;
      }
      i = 0;
    }
    load(w, Math.min(Math.max(i, 0), w.tracks.length - 1));
  }, [load]);

  const toggle = useCallback(() => {
    const a = audio.current;
    if (!a || !live.current.work) return;
    if (a.paused) a.play().catch(() => {});
    else a.pause();
  }, []);

  const skipTrack = useCallback((d: 1 | -1) => {
    const a = audio.current;
    const { work: w, index: i } = live.current;
    if (!a || !w) return;
    // "Previous" restarts the current track unless it has only just begun.
    if (d === -1 && a.currentTime > 5) { a.currentTime = 0; return; }
    const next = i + d;
    if (next >= 0 && next < w.tracks.length) load(w, next);
  }, [load]);

  const seekBy = useCallback((sec: number) => {
    const a = audio.current;
    if (a && isFinite(a.duration)) a.currentTime = Math.min(Math.max(0, a.currentTime + sec), a.duration - 0.5);
  }, []);

  const close = useCallback(() => {
    const a = audio.current;
    persist();
    if (a) { a.pause(); a.removeAttribute("src"); a.load(); }
    setWork(null);
    setPlaying(false);
    setSleep("off");
  }, [persist]);

  // Element events: progress, track end, errors, saving the position.
  useEffect(() => {
    const a = audio.current;
    if (!a) return;
    const onTime = () => setTime(a.currentTime);
    const onMeta = () => { if (isFinite(a.duration)) setDuration(a.duration); };
    const onPlay = () => setPlaying(true);
    const onPause = () => { setPlaying(false); persist(); };
    const onEnded = () => {
      persist();
      const { work: w, index: i, sleep: s } = live.current;
      if (s === "track") { setSleep("off"); return; }
      if (w && i + 1 < w.tracks.length) load(w, i + 1);
    };
    const onError = () => { if (a.getAttribute("src")) setError("This track can't be played right now."); setPlaying(false); };
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("loadedmetadata", onMeta);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("ended", onEnded);
    a.addEventListener("error", onError);
    const tick = window.setInterval(() => { if (!a.paused) persist(); }, 5000);
    window.addEventListener("pagehide", persist);
    // A film starting pauses the audio.
    const onMediaPlay = (e: Event) => { if (e.target instanceof HTMLVideoElement) a.pause(); };
    document.addEventListener("play", onMediaPlay, true);
    return () => {
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("loadedmetadata", onMeta);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("ended", onEnded);
      a.removeEventListener("error", onError);
      window.clearInterval(tick);
      window.removeEventListener("pagehide", persist);
      document.removeEventListener("play", onMediaPlay, true);
    };
  }, [load, persist]);

  // Lock screen, headphone buttons and car controls.
  useEffect(() => {
    if (!("mediaSession" in navigator) || !work) return;
    const t = work.tracks[index];
    navigator.mediaSession.metadata = new MediaMetadata({
      title: t?.title || work.title,
      artist: work.creator,
      album: work.title,
      artwork: work.cover ? [{ src: work.cover, sizes: "500x500" }] : [],
    });
    const ms = navigator.mediaSession;
    const handlers: [MediaSessionAction, MediaSessionActionHandler][] = [
      ["play", () => { void audio.current?.play(); }],
      ["pause", () => audio.current?.pause()],
      ["previoustrack", () => skipTrack(-1)],
      ["nexttrack", () => skipTrack(1)],
      ["seekbackward", (d) => seekBy(-(d.seekOffset || 15))],
      ["seekforward", (d) => seekBy(d.seekOffset || 30)],
      ["seekto", (d) => { if (audio.current && d.seekTime !== undefined) audio.current.currentTime = d.seekTime; }],
    ];
    for (const [action, h] of handlers) {
      try { ms.setActionHandler(action, h); } catch { /* unsupported action */ }
    }
  }, [work, index, skipTrack, seekBy]);

  useEffect(() => {
    if (!("mediaSession" in navigator) || !work || !duration || !isFinite(duration)) return;
    try {
      navigator.mediaSession.setPositionState({ duration, position: Math.min(time, duration), playbackRate: rate });
    } catch { /* ignore invalid states during track changes */ }
  }, [work, time, duration, rate]);

  // Sleep timer.
  useEffect(() => {
    window.clearTimeout(sleepTimer.current);
    if (sleep === "off" || sleep === "track") return;
    sleepTimer.current = window.setTimeout(() => { audio.current?.pause(); setSleep("off"); }, Number(sleep) * 60_000);
    return () => window.clearTimeout(sleepTimer.current);
  }, [sleep]);

  const cycleRate = () => {
    const next = RATES[(RATES.indexOf(rate) + 1) % RATES.length] ?? 1;
    setRate(next);
    saveRate(next);
    if (audio.current) audio.current.playbackRate = next;
  };

  const value = useMemo(() => ({ work, index, playing, play, toggle }), [work, index, playing, play, toggle]);
  const track = work?.tracks[index];
  const multi = (work?.tracks.length ?? 0) > 1;

  return (
    <AudioContext.Provider value={value}>
      {children}
      <audio ref={audio} preload="metadata" hidden />
      {work && track && (
        <>
          <div className="player-spacer" aria-hidden="true" />
          <section className="audio-bar" aria-label="Audio player">
            <input
              className="audio-seek"
              type="range"
              min={0}
              max={Math.max(1, Math.floor(duration))}
              step={1}
              value={Math.min(Math.floor(time), Math.floor(duration) || 0)}
              onChange={(e) => { if (audio.current) audio.current.currentTime = Number(e.target.value); }}
              aria-label="Seek"
              aria-valuetext={`${clock(time)} of ${clock(duration)}`}
              style={{ "--pct": `${duration ? (time / duration) * 100 : 0}%` } as React.CSSProperties}
            />
            <Link href={workHref(work)} className="audio-now">
              <span className="audio-cover" style={{ background: toneFor(work.title) }}>
                {work.cover && <img src={work.cover} alt="" />}
              </span>
              <span className="audio-titles">
                <span className="audio-track">{track.title}</span>
                <span className="audio-work">{multi ? `${work.title} · ${index + 1}/${work.tracks.length}` : work.creator}</span>
              </span>
            </Link>
            <div className="audio-controls">
              {multi && <button type="button" className="audio-btn hide-sm" onClick={() => skipTrack(-1)} aria-label="Previous track"><PrevIcon size={18} /></button>}
              <button type="button" className="audio-btn" onClick={() => seekBy(-15)} aria-label="Back 15 seconds"><Back15Icon size={24} /></button>
              <button type="button" className="audio-btn audio-play" onClick={toggle} aria-label={playing ? "Pause" : "Play"}>
                {playing ? <PauseIcon size={20} /> : <PlayIcon size={20} />}
              </button>
              <button type="button" className="audio-btn" onClick={() => seekBy(30)} aria-label="Forward 30 seconds"><Fwd30Icon size={24} /></button>
              {multi && <button type="button" className="audio-btn hide-sm" onClick={() => skipTrack(1)} aria-label="Next track" disabled={index + 1 >= work.tracks.length}><NextIcon size={18} /></button>}
            </div>
            <div className="audio-extra">
              <span className="audio-time muted hide-sm">{clock(time)} / {clock(duration)}</span>
              <button type="button" className="audio-chip" onClick={cycleRate} aria-label={`Playback speed ${rate}x`}>{rate}×</button>
              <label className={`audio-chip audio-sleep hide-sm${sleep !== "off" ? " on" : ""}`} title="Sleep timer">
                <MoonIcon size={15} />
                <span className="sr-only">Sleep timer</span>
                <select value={sleep} onChange={(e) => setSleep(e.target.value as Sleep)}>
                  <option value="off">Off</option>
                  <option value="15">15 min</option>
                  <option value="30">30 min</option>
                  <option value="60">1 hour</option>
                  <option value="track">{work.kind === "book" ? "End of chapter" : "End of track"}</option>
                </select>
              </label>
              <button type="button" className="audio-btn" onClick={close} aria-label="Close player"><CloseIcon size={18} /></button>
            </div>
            {error && <span className="audio-error error" role="alert">{error}</span>}
          </section>
        </>
      )}
    </AudioContext.Provider>
  );
}
