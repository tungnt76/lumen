"use client";

/* eslint-disable @next/next/no-img-element */
import { useCallback, useEffect, useRef, useState } from "react";
import type Hls from "hls.js";
import { progressFor, saveProgress, SavedFilm } from "@/lib/local";
import { PlayIcon } from "./icons";

type Props = {
  film: SavedFilm;
  hls: string;
  subtitles: { lang: string; label: string; url: string }[];
  autoPlay?: boolean;
};

function clock(s: number) {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = Math.floor(s % 60);
  return (h ? `${h}:${String(m).padStart(2, "0")}` : `${m}`) + `:${String(sec).padStart(2, "0")}`;
}

export function Player({ film, hls: src, subtitles, autoPlay }: Props) {
  const video = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [started, setStarted] = useState(false);
  const [resumeAt, setResumeAt] = useState(0);
  const [levels, setLevels] = useState<{ index: number; label: string }[]>([]);
  const [level, setLevel] = useState(-1);
  const [error, setError] = useState("");

  useEffect(() => {
    const p = progressFor(film.tmdbId);
    if (p) setResumeAt(p.position);
  }, [film.tmdbId]);

  const persist = useCallback(() => {
    const v = video.current;
    if (!v || !v.duration || !isFinite(v.duration)) return;
    saveProgress({ ...film, position: v.currentTime, duration: v.duration });
  }, [film]);

  const start = useCallback(async () => {
    const v = video.current;
    if (!v) return;
    setStarted(true);
    setError("");
    const resume = () => { if (resumeAt > 5) v.currentTime = resumeAt; };
    // Counts toward "Top 5 on Lumen today" once the video really plays; the API dedupes per viewer.
    v.addEventListener("playing", () => { void fetch(`/api/movies/${film.tmdbId}/play`, { method: "POST" }).catch(() => {}); }, { once: true });

    // Safari and iOS play HLS natively.
    if (v.canPlayType("application/vnd.apple.mpegurl")) {
      v.src = src;
      v.addEventListener("loadedmetadata", resume, { once: true });
    } else {
      const { default: HlsLib } = await import("hls.js");
      if (!HlsLib.isSupported()) {
        setError("This browser can't play HLS video.");
        return;
      }
      const h = new HlsLib({ capLevelToPlayerSize: true, startLevel: -1 });
      hlsRef.current = h;
      h.on(HlsLib.Events.MANIFEST_PARSED, (_e, data) => {
        setLevels(data.levels.map((l, i) => ({ index: i, label: `${l.height}p` })));
        resume();
      });
      h.on(HlsLib.Events.ERROR, (_e, data) => {
        if (!data.fatal) return;
        if (data.type === HlsLib.ErrorTypes.NETWORK_ERROR) h.startLoad();
        else if (data.type === HlsLib.ErrorTypes.MEDIA_ERROR) h.recoverMediaError();
        else setError("Playback failed. Please reload the page.");
      });
      h.loadSource(src);
      h.attachMedia(v);
    }
    try {
      await v.play();
    } catch {
      /* autoplay blocked: native controls let the viewer press play */
    }
  }, [src, resumeAt, film.tmdbId]);

  useEffect(() => {
    if (autoPlay && !started) void start();
  }, [autoPlay, started, start]);

  useEffect(() => {
    const v = video.current;
    if (!v) return;
    const tick = window.setInterval(() => { if (!v.paused) persist(); }, 5000);
    v.addEventListener("pause", persist);
    v.addEventListener("ended", persist);
    window.addEventListener("pagehide", persist);
    return () => {
      window.clearInterval(tick);
      v.removeEventListener("pause", persist);
      v.removeEventListener("ended", persist);
      window.removeEventListener("pagehide", persist);
      persist();
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [persist]);

  const pickLevel = (i: number) => {
    setLevel(i);
    if (hlsRef.current) hlsRef.current.currentLevel = i;
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      <div className="player">
        <video ref={video} controls={started} playsInline crossOrigin="anonymous" preload="none" aria-label={`${film.title} video`}>
          {subtitles.map((s, i) => (
            <track key={s.lang} kind="subtitles" src={s.url} srcLang={s.lang} label={s.label} default={i === 0 && s.lang === "vi"} />
          ))}
        </video>
        {!started && (
          <button type="button" className="player-cover" onClick={() => void start()} aria-label={resumeAt > 5 ? `Resume ${film.title}` : `Play ${film.title}`}>
            {film.backdrop && <img src={film.backdrop} alt="" />}
            <span className="play-big"><PlayIcon size={34} /></span>
            <span className="player-note">{resumeAt > 5 ? `Resume from ${clock(resumeAt)}` : "Free to watch, no sign-up"}</span>
          </button>
        )}
      </div>
      {(levels.length > 1 || error) && (
        <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
          {error && <span className="error" role="alert">{error}</span>}
          {levels.length > 1 && (
            <>
              <label htmlFor="quality" className="muted" style={{ marginLeft: "auto" }}>Quality</label>
              <select id="quality" className="input" style={{ width: 120, height: 36 }} value={level} onChange={(e) => pickLevel(Number(e.target.value))}>
                <option value={-1}>Auto</option>
                {levels.map((l) => <option key={l.index} value={l.index}>{l.label}</option>)}
              </select>
            </>
          )}
        </div>
      )}
    </div>
  );
}
