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
