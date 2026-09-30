// Code projects: a set of text files saved as one JSON bundle in R2 (see api/internal/httpapi/code.go).
// `base` is "/api/code" (your own) or "/api/admin/code" (studio).

import { jsonFetch } from "./studio";

export const MY_CODE = "/api/code";
export const ALL_CODE = "/api/admin/code";
export const GUEST_CODE_KEY = "lumen:code";
export const MAX_BUNDLE = 5 * 1024 * 1024;
export const MAX_FILES = 100;

export type CodeFile = { path: string; content: string };
export type Bundle = { version: 1; main: string; files: CodeFile[] };
export type FileChange = { path: string; status: "A" | "M" | "D" };

export type Project = {
  id: string;
  ownerId: number;
  ownerName?: string;
  title: string;
  language: string;
  preview: string;
  fileCount: number;
  sizeBytes: number;
  savedAt: string | null;
  createdAt: string;
  updatedAt: string;
};

export type Commit = {
  id: string;
  authorName: string;
  message: string;
  changes: FileChange[];
  fileCount: number;
  sizeBytes: number;
  createdAt: string;
};

/** Monaco language ids by extension (mirrors the API's list). */
const LANGS: Record<string, string> = {
  js: "javascript", jsx: "javascript", mjs: "javascript", ts: "typescript", tsx: "typescript",
  py: "python", go: "go", java: "java", c: "c", h: "c", cpp: "cpp", cs: "csharp", rs: "rust", rb: "ruby",
  php: "php", swift: "swift", kt: "kotlin", sql: "sql", html: "html", css: "css", scss: "scss", json: "json",
  md: "markdown", yaml: "yaml", yml: "yaml", xml: "xml", sh: "shell", txt: "plaintext",
};

export function languageOf(path: string) {
  return LANGS[path.split(".").pop()?.toLowerCase() ?? ""] ?? "plaintext";
}

export const LANGUAGE_NAMES: Record<string, string> = {
  javascript: "JavaScript", typescript: "TypeScript", python: "Python", go: "Go", java: "Java", c: "C", cpp: "C++",
  csharp: "C#", rust: "Rust", ruby: "Ruby", php: "PHP", swift: "Swift", kotlin: "Kotlin", sql: "SQL", html: "HTML",
  css: "CSS", scss: "SCSS", json: "JSON", markdown: "Markdown", yaml: "YAML", xml: "XML", shell: "Shell", plaintext: "Text",
};

/** Starter projects for "New project". */
export const TEMPLATES: { id: string; label: string; bundle: Bundle }[] = [
  { id: "javascript", label: "JavaScript", bundle: { version: 1, main: "index.js", files: [{ path: "index.js", content: "// Hello from Lumen\nconst greet = (name) => `Hello, ${name}!`;\n\nconsole.log(greet(\"world\"));\n" }] } },
  { id: "typescript", label: "TypeScript", bundle: { version: 1, main: "src/index.ts", files: [{ path: "src/index.ts", content: "export function greet(name: string): string {\n  return `Hello, ${name}!`;\n}\n\nconsole.log(greet(\"world\"));\n" }] } },
  { id: "python", label: "Python", bundle: { version: 1, main: "main.py", files: [{ path: "main.py", content: "def greet(name: str) -> str:\n    return f\"Hello, {name}!\"\n\n\nif __name__ == \"__main__\":\n    print(greet(\"world\"))\n" }] } },
  { id: "web", label: "HTML, CSS & JS", bundle: { version: 1, main: "index.html", files: [
    { path: "index.html", content: "<!doctype html>\n<html lang=\"en\">\n  <head>\n    <meta charset=\"utf-8\" />\n    <title>My page</title>\n    <link rel=\"stylesheet\" href=\"style.css\" />\n  </head>\n  <body>\n    <h1>Hello, world!</h1>\n    <script src=\"script.js\"></script>\n  </body>\n</html>\n" },
    { path: "style.css", content: "body {\n  font-family: system-ui, sans-serif;\n  margin: 2rem;\n}\n" },
    { path: "script.js", content: "document.querySelector(\"h1\").addEventListener(\"click\", () => alert(\"Hi!\"));\n" },
  ] } },
  { id: "go", label: "Go", bundle: { version: 1, main: "main.go", files: [{ path: "main.go", content: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}\n" }] } },
  { id: "blank", label: "Empty", bundle: { version: 1, main: "README.md", files: [{ path: "README.md", content: "# My project\n" }] } },
];

export function cloneBundle(b: Bundle): Bundle {
  return { version: 1, main: b.main, files: b.files.map((f) => ({ ...f })) };
}

export function sameBundle(a: Bundle | null, b: Bundle | null) {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** What changed from `from` (the last commit, or nothing) to `to`, as A/M/D per file. */
export function changesBetween(from: Bundle | null, to: Bundle): FileChange[] {
  const before = new Map((from?.files ?? []).map((f) => [f.path, f.content]));
  const out: FileChange[] = [];
  for (const f of to.files) {
    if (!before.has(f.path)) out.push({ path: f.path, status: "A" });
    else if (before.get(f.path) !== f.content) out.push({ path: f.path, status: "M" });
    before.delete(f.path);
  }
  for (const path of before.keys()) out.push({ path, status: "D" });
  return out.sort((a, b) => a.path.localeCompare(b.path));
}

/** Checks a new file path: relative, no "..", unique. Returns an error message or "". */
export function checkPath(path: string, taken: string[]) {
  if (!path) return "Enter a file name, like src/app.js.";
  if (path.startsWith("/") || path.includes("\\") || path.split("/").some((p) => !p || p === "." || p === ".."))
    return "Use a relative path like src/app.js (no leading slash or ..).";
  if (path.length > 300) return "That path is too long.";
  if (taken.includes(path)) return "A file with that name already exists.";
  return "";
}

export function parseBundle(raw: unknown): Bundle {
  const b = raw as Partial<Bundle>;
  const files = Array.isArray(b?.files) ? b.files.filter((f) => f && typeof f.path === "string").map((f) => ({ path: f.path, content: String(f.content ?? "") })) : [];
  return { version: 1, main: typeof b?.main === "string" ? b.main : files[0]?.path ?? "", files };
}

export async function fetchBundle(url: string): Promise<Bundle> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Couldn't load the files from storage (${res.status})`);
  return parseBundle(await res.json());
}

/** Uploads the working copy straight to R2, then asks the API to record the save. */
export async function saveBundle(base: string, id: string, bundle: Bundle): Promise<Project> {
  const body = JSON.stringify(bundle);
  if (body.length > MAX_BUNDLE) throw new Error("The project is over 5 MB. Code projects are for text files.");
  const { bundleUrl } = await jsonFetch<{ bundleUrl: string }>(`${base}/${id}/uploads`, { method: "POST" });
  let res: Response;
  try {
    res = await fetch(bundleUrl, { method: "PUT", headers: { "Content-Type": "application/json" }, body });
  } catch {
    throw new Error("Couldn't upload to storage: the R2 bucket's CORS rules don't allow this site yet (run make r2-cors).");
  }
  if (!res.ok) throw new Error(`Upload to storage failed (${res.status}); check the bucket's CORS rules (make r2-cors).`);
  return jsonFetch<Project>(`${base}/${id}/saved`, { method: "POST" });
}
