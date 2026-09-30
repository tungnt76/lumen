// Device-local state: "My list" and "Continue watching". No account needed.
// Every access is guarded: storage can be blocked (private mode, strict settings).

export type SavedFilm = {
  tmdbId: number;
  title: string;
  year: number;
  poster: string;
  backdrop: string;
};

export type Progress = SavedFilm & { position: number; duration: number; updatedAt: number };

const LIST_KEY = "lumen:list";
const PROGRESS_KEY = "lumen:progress";
const EVENT = "lumen:storage";

function read<T>(key: string, fallback: T): T {
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}

function write(key: string, value: unknown) {
  try {
    window.localStorage.setItem(key, JSON.stringify(value));
    window.dispatchEvent(new Event(EVENT));
  } catch {
    /* storage unavailable: keep working without persistence */
  }
}

export function onLocalChange(cb: () => void) {
  window.addEventListener(EVENT, cb);
  window.addEventListener("storage", cb);
  return () => {
    window.removeEventListener(EVENT, cb);
    window.removeEventListener("storage", cb);
  };
}

export function getList(): SavedFilm[] {
  return read<SavedFilm[]>(LIST_KEY, []);
}

export function inList(id: number) {
  return getList().some((f) => f.tmdbId === id);
}

export function toggleList(film: SavedFilm) {
  const list = getList();
  const next = list.some((f) => f.tmdbId === film.tmdbId)
    ? list.filter((f) => f.tmdbId !== film.tmdbId)
    : [film, ...list].slice(0, 200);
  write(LIST_KEY, next);
}

export function getProgress(): Progress[] {
  return read<Progress[]>(PROGRESS_KEY, []).sort((a, b) => b.updatedAt - a.updatedAt);
}

export function progressFor(id: number) {
  return getProgress().find((p) => p.tmdbId === id);
}

export function saveProgress(p: Omit<Progress, "updatedAt">) {
  let all = getProgress().filter((x) => x.tmdbId !== p.tmdbId);
  // Nearly finished: drop it from "Continue watching".
  if (p.duration > 0 && p.position / p.duration < 0.95 && p.position > 30) {
    all = [{ ...p, updatedAt: Date.now() }, ...all];
  }
  write(PROGRESS_KEY, all.slice(0, 30));
}

// ---- audiobooks and music: saved works and listening position ----

export type SavedWork = { id: string; kind: "book" | "album"; title: string; creator: string; cover: string };

// Entries saved before ids became UUIDs (numbers) point at pages that no longer exist.
const isCurrent = (w: { id: unknown }) => typeof w.id === "string" && /^[0-9a-f-]{36}$/.test(w.id);

/** Where the listener stopped: the track index within the work and the position in that track. */
export type Listening = SavedWork & { track: number; trackCount: number; position: number; duration: number; updatedAt: number };

const WORKS_KEY = "lumen:works";
const LISTENING_KEY = "lumen:listening";
const RATE_KEY = "lumen:rate";

export function getWorkList(): SavedWork[] {
  return read<SavedWork[]>(WORKS_KEY, []).filter(isCurrent);
}

export function inWorkList(id: string) {
  return getWorkList().some((w) => w.id === id);
}

export function toggleWorkList(work: SavedWork) {
  const list = getWorkList();
  const next = list.some((w) => w.id === work.id)
    ? list.filter((w) => w.id !== work.id)
    : [work, ...list].slice(0, 200);
  write(WORKS_KEY, next);
}

export function getListening(): Listening[] {
  return read<Listening[]>(LISTENING_KEY, []).filter(isCurrent).sort((a, b) => b.updatedAt - a.updatedAt);
}

export function listeningFor(id: string) {
  return getListening().find((l) => l.id === id);
}

export function saveListening(l: Omit<Listening, "updatedAt">) {
  let all = getListening().filter((x) => x.id !== l.id);
  // Finished the last track: drop it from "Continue listening".
  const done = l.track >= l.trackCount - 1 && l.duration > 0 && l.position / l.duration >= 0.98;
  if (!done && (l.track > 0 || l.position > 10)) {
    all = [{ ...l, updatedAt: Date.now() }, ...all];
  }
  write(LISTENING_KEY, all.slice(0, 30));
}

export function getRate(): number {
  const r = read<number>(RATE_KEY, 1);
  return typeof r === "number" && r >= 0.5 && r <= 3 ? r : 1;
}

export function saveRate(rate: number) {
  write(RATE_KEY, rate);
}
