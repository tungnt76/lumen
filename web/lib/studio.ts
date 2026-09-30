// Browser-side calls to the admin API (same origin through the /api rewrite).

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export async function adminFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  return jsonFetch<T>(`/api/admin${path}`, init);
}

/** JSON request to the API (same origin through the /api rewrite); throws ApiError with the API's message. */
export async function jsonFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(url, {
    ...init,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, body.error || `Request failed (${res.status})`);
  return body as T;
}

/** PUT a file to a presigned R2 URL with upload progress. */
export function putFile(url: string, file: File, contentType: string, onProgress: (pct: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", url);
    xhr.setRequestHeader("Content-Type", contentType);
    xhr.upload.onprogress = (e) => { if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100)); };
    xhr.onload = () => (xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(`Upload failed (${xhr.status})`)));
    xhr.onerror = () => reject(new Error("Upload failed: check the bucket's CORS settings"));
    xhr.send(file);
  });
}

export function videoType(file: File) {
  if (file.type) return file.type;
  const ext = file.name.split(".").pop()?.toLowerCase();
  return ({ mp4: "video/mp4", mkv: "video/x-matroska", mov: "video/quicktime", webm: "video/webm" } as Record<string, string>)[ext || ""] || "";
}

/** The upload type for an audio file, or "" if browsers can't all play it. */
export function audioType(file: File) {
  const ext = file.name.split(".").pop()?.toLowerCase();
  const byExt: Record<string, string> = { mp3: "audio/mpeg", m4a: "audio/mp4", aac: "audio/aac" };
  if (ext && byExt[ext]) return byExt[ext];
  return ["audio/mpeg", "audio/mp4", "audio/x-m4a", "audio/aac"].includes(file.type) ? file.type : "";
}

/** Reads an audio file's length in whole seconds in the browser (0 if it can't tell). */
export function audioDuration(file: File): Promise<number> {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const a = new Audio();
    const done = (d: number) => { URL.revokeObjectURL(url); resolve(isFinite(d) ? Math.round(d) : 0); };
    a.preload = "metadata";
    a.onloadedmetadata = () => done(a.duration);
    a.onerror = () => done(0);
    a.src = url;
  });
}
