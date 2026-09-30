// Saving Excalidraw scenes to R2 through the drawings API. `base` is "/api/drawings" (your own)
// or "/api/admin/drawings" (studio). Files go straight from the browser to storage.

import type { Drawing } from "./api";
import { jsonFetch } from "./studio";

export const MY_DRAWINGS = "/api/drawings";
export const ALL_DRAWINGS = "/api/admin/drawings";

/** PUTs a body to a presigned R2 URL. */
async function put(url: string, body: Blob | string, type: string) {
  let res: Response;
  try {
    res = await fetch(url, { method: "PUT", headers: { "Content-Type": type }, body });
  } catch {
    // The browser blocks the request before it starts when the bucket's CORS rules don't allow this site.
    throw new Error("Couldn't upload to storage: the R2 bucket's CORS rules don't allow this site yet (run make r2-cors).");
  }
  if (!res.ok) throw new Error(`Upload to storage failed (${res.status}); check the bucket's CORS rules (make r2-cors).`);
}

/** Uploads a serialized scene (and an optional PNG preview) and records the save. */
export async function saveScene(base: string, id: string, json: string, preview: Blob | null): Promise<Drawing> {
  const urls = await jsonFetch<{ sceneUrl: string; previewUrl: string }>(`${base}/${id}/uploads`, { method: "POST" });
  await Promise.all([
    put(urls.sceneUrl, json, "application/json"),
    preview ? put(urls.previewUrl, preview, "image/png") : Promise.resolve(),
  ]);
  return jsonFetch<Drawing>(`${base}/${id}/saved`, { method: "POST" });
}

/** A small PNG of a scene for the drawings list, or null for an empty canvas. */
export async function previewOf(scene: { elements: readonly unknown[]; appState: object; files: object }): Promise<Blob | null> {
  if (!scene.elements.length) return null;
  const { exportToBlob } = await import("@excalidraw/excalidraw");
  return exportToBlob({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    elements: scene.elements as any, files: scene.files as any, mimeType: "image/png", maxWidthOrHeight: 720,
    appState: { ...scene.appState, exportBackground: true, exportWithDarkMode: false },
  });
}

/**
 * Saves a scene kept in this browser (a sketch made before signing in) as one of your drawings.
 * Pass the id from an earlier attempt so retrying after a failed upload reuses that drawing
 * instead of creating another; onCreated reports the new id before the upload starts.
 */
export async function importLocalScene(raw: string, title: string, existingId?: string, onCreated?: (id: string) => void): Promise<Drawing> {
  const { restore, serializeAsJSON } = await import("@excalidraw/excalidraw");
  const r = restore(JSON.parse(raw), null, null);
  let d: Pick<Drawing, "id">;
  if (existingId) d = { id: existingId };
  else {
    d = await jsonFetch<Drawing>(MY_DRAWINGS, { method: "POST", body: JSON.stringify({ title }) });
    onCreated?.(d.id);
  }
  const json = serializeAsJSON(r.elements, r.appState, r.files, "local");
  return saveScene(MY_DRAWINGS, d.id, json, await previewOf({ elements: r.elements, appState: r.appState, files: r.files }));
}
