# Handoff: "Lumen" free film website (Tony)

## Goal

Free, legal, Netflix-style film site on free tiers. Public site needs no login.
Hidden admin at /studio uploads and manages films.
Only public-domain or licensed films are hosted. No pirated streams.

## Stack

- Web: Next.js 15 (App Router, TS) on Vercel. `/api/*` is rewritten to the Go API, so the admin cookie is first-party.
- API: Go 1.26 (net/http, pgx) on Vercel (Go framework preset, `api/vercel.json`); `api/Dockerfile` for container hosts.
- DB: Postgres on Supabase (Session pooler, port 5432). Schema is created automatically on start.
- Video storage: Cloudflare R2, public bucket, S3 API, zero egress fees.
- Metadata: TMDB API (free for non-commercial use, attribution required). Adding ads means commercial use and needs a TMDB licence.
- Encoding: worker with ffmpeg, run on Tony's PC. Produces HLS 360/720/1080p (no upscaling). The player uses hls.js.

## Repo layout (delivered as lumen.zip)

- `api/cmd/api`: public and admin HTTP API.
- `api/cmd/worker`: claims queued films (FOR UPDATE SKIP LOCKED), encodes, uploads `hls/{id}/`, deletes the source.
- `api/cmd/admin`: CLI that creates or resets an admin and prints a TOTP secret.
- `api/internal/`:
  - `auth`: bcrypt, TOTP (RFC 6238), HMAC session tokens, login limiter.
  - `tmdb`: cached client, one detail call via `append_to_response`.
  - `store`, `storage` (R2 presign, upload), `encode`, `httpapi`, `cache`, `config`.
- `web/app/(site)`: `/`, `/movie/[id]`, `/browse` (genre and search), `/my-list`.
- `web/app/studio`: login page and `films` dashboard (upload, TMDB match, rights, subtitles, publish/feature, delete).
- Also: `docker-compose.yml` (local Postgres), `.github/workflows/ci.yml`, `README.md` with the deploy guide.

## API endpoints

Public:

- `GET /api/home`, `/api/genres`, `/api/movies/{tmdbId}`, `/api/browse?genre=&page=`, `/api/search?q=&page=`. List endpoints return `{items, page, totalPages, total}`
- Audio: `GET /api/books?q=&lang=&page=`, `/api/music?q=&page=`, `/api/works/{id}` (work + tracks). Works are books or albums in `works`/`tracks`; a track's `audio` is an absolute URL (e.g. archive.org) or an R2 key. A work can be published only once it has tracks, or (license `external`) official `links`: current artists such as Đen Vâu are link-out cards imported from MusicBrainz, never hosted. Web: `/books`, `/music`, `/books/[id]`, `/music/[id]`, a site-wide `AudioProvider` player (`web/components/AudioPlayer.tsx`) with resume position in localStorage, "Continue listening" on Home, works in My list. Demo data: `make seed-audio`. Studio `/studio/works` (admin API `/api/admin/works*`): import from LibriVox / Internet Archive / MusicBrainz, create a work, upload .mp3/.m4a via presigned PUT to `audio/{id}/u{hex}.ext`, replace the track list (audio must be an existing track or an uploaded key for that work), publish/feature/delete. Search uses the `works.search` column (Fold: lowercase, no diacritics, đ→d). Also: content comes from `api/cmd/import-audio` (`make import ARGS=...`): LibriVox and Internet Archive (metadata only, audio stays on archive.org), Wikisource text + local TTS (Piper default, VieNeu via `scripts/tts_vieneu.py`) or local files, encoded to AAC and uploaded to R2 `audio/{id}/{stamp}/`. See README section 5.

Admin (cookie auth + Origin check):

- `POST /api/admin/login` with `{email, password, code}`, and `POST /api/admin/logout`
- `GET /api/admin/me`, `GET /api/admin/tmdb/search`
- `GET /api/admin/films?page=` and `POST /api/admin/films` (the POST returns a presigned PUT URL)
- `POST /api/admin/films/{id}/uploaded`, `POST /api/admin/films/{id}/subtitles`
- `PATCH /api/admin/films/{id}`, `DELETE /api/admin/films/{id}`

## Key decisions

- "My list" and "Continue watching" live in the browser's localStorage. No user accounts.
- Uploads go from the browser straight to R2 via presigned PUT (5 GB limit per file).
- Film status flow: awaiting_upload → queued → encoding → ready | failed. Only "ready" films can be published.
- Every upload stores a rights basis plus a source link, with a required confirmation checkbox.
- Films not hosted on the site get a trailer plus legal where-to-watch links (TMDB watch providers, region VN, JustWatch credit).
- Security:
  - Session cookie: HttpOnly, Secure, SameSite=Strict, 12 h.
  - Login lockout: 5 failures per IP locks for 15 min (in memory).
  - `/studio` is noindex.
  - Suggested extra: Cloudflare Access in front of `/studio*`.
- Removed the "Top 5" row: there's no view tracking, so the ranking would be fake.

## Excalidraw

- Public tab "Excalidraw" beside My list → `/draw` (`web/components/DrawHome.tsx`): visitors get `GuestDrawing` (canvas saved in localStorage `lumen:drawing`, "Sign in to save"); signed-in users get "My drawings" (`DrawingList`, base `/api/drawings`) and the editor `/draw/[id]` (`DrawingEditor` embedded), plus an offer to import the pre-sign-in sketch (`lib/drawings.ts importLocalScene`).
- Studio → Excalidraw (`/studio/drawings`, base `/api/admin/drawings`) lists everyone's drawings with owner names.
- Table `drawings` (owner_id → users, title, token, size, saved_at); files in the media bucket at `drawings/{id}-{token}/scene.excalidraw` + `preview.png`, uploaded/downloaded by the browser with presigned URLs. API handlers in `httpapi/drawings.go` with a scope: own (404 for others' ids) or all (admin). Limits: 50 MB each, 100 per member.

## Accounts

- Tables `users` (email, bcrypt hash, display_name, role admin|member, avatar preset id, bio, disabled), `sessions` (sha256 of a random token, user, UA, IP, sliding 30-day expiry), `invite_codes` (6-digit code, role, max_uses/uses, expiry, disabled; wrong codes rate-limited per IP and site-wide). Old `admins` rows are copied into `users` as admins on startup.
- Cookie `lumen_session` (HttpOnly, Lax) + readable hint `lumen_auth=1` so the web only calls `/api/auth/me` for signed-in browsers. Middleware `signedIn` / `admin` in `httpapi/server.go`; handlers in `httpapi/auth.go` and `admin_users.go`.
- Web: `/login`, `/signup` (2 steps, `?code=` prefill), `/account`, header `AccountMenu`, `Avatar` presets (`web/components/Avatar.tsx`, ids mirrored in `httpapi/auth.go`), Studio → Users. CLI: `cmd/users seed|invite|list`, `cmd/admin`.

## Code tab (phase 1 done)

- `/code` (`CodeHome`: guest scratch in localStorage `lumen:code`, or `CodeList` "My code"), `/code/[id]` and `/studio/code[/id]` use `CodeWorkspace` (Monaco via `@monaco-editor/react`, CDN monaco 0.57.0): explorer, tabs, status bar, source control (changes vs last commit, commit, history, DiffEditor, restore), drafts in localStorage `lumen:code-draft:{id}`, phones read-only.
- API `httpapi/code.go` (scopes own/all like drawings): list/create/get/rename/delete, uploads + saved (server reads bundle back), commits list/create (R2 CopyObject of project.json → commits/{id}.json, keeps 200)/get. Store `store/code.go`, migration 0002. Tests use an in-memory fake S3 (`httpapi/fakes3_test.go`).
- Next: phase 2 run code (sandboxed iframe for HTML/JS, Pyodide for Python), phase 3 GitHub import/push via OAuth.

## Migrations and ids

- `schema.sql` (idempotent, every start) + `store/migrations/NNNN_*.sql` (once each, tracked in `schema_migrations`, advisory lock; runner `store/migrate.go`; `make migrate`).
- 0001: works and drawings ids are UUID v7 (Postgres function `uuid_v7(ts)`, column default); Go/TS use string ids; routes validate UUIDs. Old storage folders kept in `works.media_prefix` / `drawings.folder` (`Work.AudioFolder()`, `Drawing.Folder()`). Films, users, tracks keep bigint ids.
- `make reset-data [STORAGE=1]` (`cmd/reset`, `store.ResetAll`): drops all Lumen tables after typing "delete", recreates the empty schema; optional R2 cleanup of audio/ and drawings/.

## Env vars (api/.env.example)

- Database and TMDB: `DATABASE_URL`, `TMDB_TOKEN`, `TMDB_LANGUAGE=vi-VN`, `TMDB_REGION=VN`
- Sessions: `SESSION_SECRET` (32+ chars), `SITE_ORIGIN`, `COOKIE_SECURE`
- R2: `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`, `MEDIA_BASE_URL`
- Worker: `WORK_DIR`, `DELETE_SOURCES`
- Test-only overrides: `TMDB_BASE_URL`, `R2_ENDPOINT`
- Web: `API_URL`. It's used at build time for the rewrite, so redeploy after changing it.

## Status

- Done and tested locally:
  - Go unit tests, ffmpeg encode tests, DB tests, admin-auth tests.
  - Full end-to-end run with a fake TMDB and an S3 mock (moto): login → create → upload → encode → publish → subtitles → public pages render.
  - `next build` and `tsc` pass.
- Not done yet: deployment with real TMDB, R2 and Supabase accounts.
- TODO: replace the "Powered by TMDB" text with an approved TMDB logo.

## Design

Prototype canvas (Artifact "Film Site Prototype"), 6 screens: Home, Movie + player, Browse, Mobile home, Admin sign-in, Admin upload.
Look: dark theme by default (#0E0E10); optional light theme (page #EFEAE2 with white nav, tab bar, search, cards and lists, shadows on covers; amber text #8F5400), amber accent #E8A33D. Light is chosen only with the header sun/moon button (saved in localStorage `lumen:theme`, applied before paint by an inline script; no system/auto mode; tokens in `web/app/globals.css`, logic in `web/lib/theme.ts`). Logo: `web/components/Logo.tsx`, favicon files in `web/app/`. Originally: dark background #0E0E10, amber accent #E8A33D, fonts Bricolage Grotesque (headings) and DM Sans (body). Placeholder brand name "Lumen".

## Next steps

1. Create accounts: TMDB token, R2 bucket (public access + CORS from README), Supabase DB (Session pooler URL).
2. Run `go run ./cmd/admin -email ...` and add the TOTP secret to an authenticator app.
3. Deploy the API and the web as two Vercel projects (roots: `api` and `web`). See DEPLOY.md.
4. Run the worker locally and upload the first public-domain film (e.g. from archive.org).
5. Possible next features: real view counts for a Top row, R2 multipart upload for files over 5 GB, Redis-backed rate limiter, a Vietnamese UI.
