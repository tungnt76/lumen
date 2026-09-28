# Handoff: "Lumen" free film website (Tony)

## Goal

Free, legal, Netflix-style film site on free tiers. Public site needs no login.
Hidden admin at /studio uploads and manages films.
Only public-domain or licensed films are hosted. No pirated streams.

## Stack

- Web: Next.js 15 (App Router, TS) on Vercel. `/api/*` is rewritten to the Go API, so the admin cookie is first-party.
- API: Go 1.26 (net/http, pgx) on Render or Fly.
- DB: Postgres on Neon or Supabase. Schema is created automatically on start.
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
- Not done yet: deployment with real TMDB, R2 and Neon accounts.
- TODO: replace the "Powered by TMDB" text with an approved TMDB logo.

## Design

Prototype canvas (Artifact "Film Site Prototype"), 6 screens: Home, Movie + player, Browse, Mobile home, Admin sign-in, Admin upload.
Look: dark background #0E0E10, amber accent #E8A33D, fonts Bricolage Grotesque (headings) and DM Sans (body). Placeholder brand name "Lumen".

## Next steps

1. Create accounts: TMDB token, R2 bucket (public access + CORS from README), Neon DB.
2. Run `go run ./cmd/admin -email ...` and add the TOTP secret to an authenticator app.
3. Deploy the API to Render (api/Dockerfile) and the web to Vercel (root: web).
4. Run the worker locally and upload the first public-domain film (e.g. from archive.org).
5. Possible next features: real view counts for a Top row, R2 multipart upload for files over 5 GB, Redis-backed rate limiter, a Vietnamese UI.
