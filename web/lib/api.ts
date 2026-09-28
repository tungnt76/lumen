// Types shared with the Go API and a server-side fetch helper.

export type Card = {
  tmdbId: number;
  title: string;
  year: number;
  overview: string;
  poster: string;
  backdrop: string;
  genres: string[];
  runtime?: number;
  playable: boolean;
};

export type Row = { title: string; genreId?: number; kind?: "top"; items: Card[] };

export type Home = { featured: Card | null; rows: Row[] };

export type Provider = { name: string; logo: string };

export type MovieDetail = Card & {
  director: string;
  cast: { name: string; character?: string; photo: string }[];
  trailerKey: string;
  similar: Card[];
  providers: { link: string; stream: Provider[]; rent: Provider[]; buy: Provider[] } | null;
  playback: { hls: string; subtitles: { lang: string; label: string; url: string }[] } | null;
  source?: string;
};

export type Genre = { id: number; name: string };

/** One page of a list response from the Go API. Pages are numbered from 1. */
export type Page<T> = { items: T[]; page: number; totalPages: number; total: number };

export type Film = {
  id: number;
  tmdbId: number;
  title: string;
  year: number;
  posterPath: string;
  rights: "public_domain" | "licensed" | "own";
  rightsNote: string;
  status: "awaiting_upload" | "queued" | "encoding" | "ready" | "failed";
  progress: number;
  error: string;
  renditions: string[];
  subtitles: string[];
  published: boolean;
  featured: boolean;
  createdAt: string;
};

const API_URL = process.env.API_URL || "http://localhost:8080";

/** Server-side GET against the Go API, cached by Next for `revalidate` seconds. */
export async function api<T>(path: string, revalidate = 60): Promise<T | null> {
  try {
    const res = await fetch(`${API_URL}${path}`, { next: { revalidate } });
    if (res.status === 404) return null;
    if (!res.ok) throw new Error(`${path}: ${res.status}`);
    return (await res.json()) as T;
  } catch (err) {
    console.error("api error", err);
    return null;
  }
}

export function formatRuntime(min?: number) {
  if (!min) return "";
  const h = Math.floor(min / 60);
  const m = min % 60;
  return h ? `${h}h ${m}m` : `${m}m`;
}

/** Deterministic dark tone for poster placeholders when TMDB has no image. */
export function toneFor(title: string) {
  const tones = ["#3A2E1E", "#2B2F3A", "#3D1F22", "#1E2B2A", "#33262B", "#2F2A3A", "#262A1E", "#1E2638"];
  let h = 0;
  for (const c of title) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return tones[h % tones.length];
}
