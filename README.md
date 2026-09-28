# Lumen: a free film website

Netflix-style catalog for **public-domain or licensed films**, built on free tiers.

- **Public site** (no login): home rows, movie page with adaptive HLS player, browse and search, "My list" and "Continue watching" saved in the browser.
- **Hidden admin** at `/studio`: email + password. Upload a film, match it on TMDB, record the rights basis, add subtitles, publish.
- Films not hosted here still get a page with a trailer and **legal where-to-watch** links (TMDB watch providers, data by JustWatch).

```
Browser ──> Next.js (Vercel) ──/api rewrite──> Go API (Render/Fly) ──> Postgres (Neon)
   │                                               └──> TMDB (metadata, cached)
   └── HLS video + subtitles <── Cloudflare R2 (public bucket, no egress fees)
                                     ▲
Admin browser ── presigned PUT ──────┘   Worker (your PC, ffmpeg) ── source → HLS 360/720/1080p
```

| Folder | What |
|---|---|
| `api/cmd/api` | HTTP API (public + admin) |
| `api/cmd/worker` | Encoder: claims uploads, runs ffmpeg, pushes HLS to R2 |
| `api/cmd/admin` | CLI to create/reset an admin and its password |
| `web/` | Next.js 15 site and studio |
| `design/` | Source of the design prototype canvas, mapped to the web pages |

## 1. Accounts you need (all free)

1. **TMDB**: themoviedb.org → Settings → API → copy the *API Read Access Token*.
2. **Cloudflare R2**: create bucket `lumen-media`.
   - Settings → Public access: enable the `r2.dev` URL (or connect a custom domain, better for production caching). This is `MEDIA_BASE_URL`.
   - Settings → CORS policy:
     ```json
     [{"AllowedOrigins":["https://YOUR-SITE.vercel.app","http://localhost:3000"],
       "AllowedMethods":["GET","HEAD","PUT"],
       "AllowedHeaders":["Content-Type","Range"],
       "ExposeHeaders":["ETag","Content-Length","Content-Range"],
       "MaxAgeSeconds":3600}]
     ```
   - R2 → Manage API tokens → *Object Read & Write*, scoped to this bucket.
3. **Neon** (or Supabase): create a Postgres database, copy the connection string. Tables are created automatically on start.

## 2. Run locally

With the Makefile (run `make` to list every command):

```bash
make setup                        # creates api/.env (with SESSION_SECRET), web/.env.local, starts Postgres, installs deps
# edit api/.env: add TMDB_TOKEN and the R2 keys
make admin EMAIL=you@example.com  # asks for the admin password
make dev                          # API :8080 + worker + web :3000 together
```

Other useful targets: `make api`, `make worker`, `make web`, `make test`, `make test-db`, `make build`, `make db-reset`.

Or by hand:
```bash
docker compose up -d                        # local Postgres
cd api && cp .env.example .env              # fill in TMDB, R2, SESSION_SECRET (openssl rand -hex 32)
set -a; source .env; set +a

go run ./cmd/admin -email you@example.com   # asks for the admin password
go run ./cmd/api                            # :8080
go run ./cmd/worker                         # needs ffmpeg installed

cd ../web && cp .env.example .env.local && npm install && npm run dev   # http://localhost:3000
```

Open `http://localhost:3000/studio`, sign in, upload a short test clip, wait for the worker, tick **Published**.

## 3. Deploy

Step-by-step go-live checklist (Vietnamese): [DEPLOY.md](DEPLOY.md).

**API → Render** (or Fly.io): new Web Service from this repo, root `api`, Dockerfile `api/Dockerfile`. Set the env vars from `.env.example` with `SITE_ORIGIN=https://YOUR-SITE.vercel.app` and `COOKIE_SECURE=true`. Free instances may sleep when idle, so the first request after a pause is slow.

**Web → Vercel**: import the repo, root directory `web`, env `API_URL=https://your-api.onrender.com`. The `/api/*` rewrite is set at build time, so redeploy after changing `API_URL`.

**Worker**: run it on your own computer (`go run ./cmd/worker`) or `docker build -f Dockerfile.worker`. It only needs to be on while there are uploads to encode; queued films wait for it.

Check each provider's current free-tier limits before launch. They change often.

## 4. How a film goes live

1. Studio: search TMDB, pick the film, choose rights basis, paste the source link (e.g. the archive.org page), tick the confirmation, drop the file.
2. The browser uploads the source **directly to R2** with a presigned URL (max 5 GB per file), so it never passes through the free API server.
3. The worker encodes an HLS ladder (no upscaling; silent films work), uploads it to `hls/{id}/`, and deletes the source (`DELETE_SOURCES=true`) to save storage.
4. Tick **Published**. Only encoded films can be published. **Featured** puts it in the home hero.
5. Add `.vtt` subtitles per language at any time.

## Security notes

- `/studio` is unlisted and `noindex`, but that is not the protection. The protection is bcrypt passwords (min 6 chars), HttpOnly + Secure + SameSite=Strict session cookies (12 h), an Origin check on every admin write, and a lockout after 5 failed logins per IP for 15 minutes.
- For extra safety put `/studio*` and `/api/admin*` behind **Cloudflare Access** (free for small teams) or an IP allowlist.
- The rate limiter is in memory, so it resets when the API restarts. That's fine for one instance; use Redis if you scale out.
- Keep `.env` out of git. Rotate `SESSION_SECRET` to sign everyone out.

## Legal checklist

- Publish only films that are public domain **where your viewers are** (e.g. check Vietnam as well as the US) or that you have written permission for. The studio records the basis and source link for every upload.
- TMDB: free for non-commercial use with attribution. Before launch, replace the "Powered by TMDB" text in `web/components/Nav.tsx` with an approved logo from themoviedb.org. Adding ads or paid features makes it commercial use, which needs a TMDB commercial licence.
- Where-to-watch data comes from JustWatch through TMDB; keep the credit on movie pages.

## Tests

```bash
cd api
go test ./...                                              # unit + ffmpeg encoder tests
TEST_DATABASE_URL=postgres://lumen:lumen@localhost:5432/lumen?sslmode=disable go test ./...   # + DB and admin-auth tests
cd ../web && npm run build
```

`TMDB_BASE_URL` and `R2_ENDPOINT` can point at local mocks for end-to-end testing.
