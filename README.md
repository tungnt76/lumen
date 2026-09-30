# Lumen: a free film website

Netflix-style catalog for **public-domain or licensed films**, built on free tiers.

- **Public site** (no login): home rows, movie page with adaptive HLS player, browse and search, "My list" and "Continue watching" saved in the browser.
- **Hidden admin** at `/studio`: email + password. Upload a film, match it on TMDB, record the rights basis, add subtitles, publish.
- Films not hosted here still get a page with a trailer and **legal where-to-watch** links (TMDB watch providers, data by JustWatch).

```
Browser ──> Next.js (Vercel) ──/api rewrite──> Go API (Vercel) ──> Postgres (Supabase)
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
   - Settings → CORS policy (or run `make r2-cors` once your API token may edit bucket settings; `ARGS="-origin https://your-site.vercel.app"` adds your live site). **Without it the browser blocks every upload** (films, audio, drawings):
     ```json
     [{"AllowedOrigins":["https://YOUR-SITE.vercel.app","http://localhost:3000"],
       "AllowedMethods":["GET","HEAD","PUT"],
       "AllowedHeaders":["Content-Type","Range"],
       "ExposeHeaders":["ETag","Content-Length","Content-Range"],
       "MaxAgeSeconds":3600}]
     ```
   - R2 → Manage API tokens → *Object Read & Write*, scoped to this bucket.
3. **Supabase** (or Neon): create a Postgres database and copy the **Session pooler** connection string (port 5432; the transaction pooler breaks pgx prepared statements). Tables are created automatically on start.

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

**API → Vercel**: a second Vercel project with root directory `api`; the Go framework preset (`api/vercel.json`) builds `cmd/api`. Set the env vars from `.env.example` with `SITE_ORIGIN=https://YOUR-SITE.vercel.app` and `COOKIE_SECURE=true`. A daily Vercel cron hits `/api/browse` so a free Supabase project isn't paused. `api/Dockerfile` still works for Render or any container host.

**Web → Vercel**: import the repo, root directory `web`, env `API_URL=https://your-api.vercel.app`. The `/api/*` rewrite is set at build time, so redeploy after changing `API_URL`.

**Worker**: run it on your own computer (`go run ./cmd/worker`) or `docker build -f Dockerfile.worker`. It only needs to be on while there are uploads to encode; queued films wait for it.

Check each provider's current free-tier limits before launch. They change often.

## 4. How a film goes live

1. Studio: search TMDB, pick the film, choose rights basis, paste the source link (e.g. the archive.org page), tick the confirmation, drop the file.
2. The browser uploads the source **directly to R2** with a presigned URL (max 5 GB per file), so it never passes through the free API server.
3. The worker encodes an HLS ladder (no upscaling; silent films work), uploads it to `hls/{id}/`, and deletes the source (`DELETE_SOURCES=true`) to save storage.
4. Tick **Published**. Only encoded films can be published. **Featured** puts it in the home hero.
5. Add `.vtt` subtitles per language at any time.

## 5. Audiobooks and music

The site has **Books** and **Music** tabs, book and album pages with chapter lists, and a player that keeps playing while you browse (speed, sleep timer, lock-screen controls, resume position saved on the device).

For a demo catalog, run `make seed-audio`: six LibriVox audiobooks, five public-domain classical albums (Musopen, Beethoven, Mozart) and Đen Vâu's discography as link-out cards. Remove it with `make seed-audio-remove`.

**Studio → Books & music** (`/studio/works`) manages them in the browser:

- **Import free content**: paste a LibriVox id or RSS link, an Internet Archive identifier or link, or a MusicBrainz artist link (link-out cards). LibriVox's API often takes 15–20 s; if an import times out, try again or use the command below.
- **Add your own book or album**: fill in the details and rights, then drop `.mp3` or `.m4a` files. They upload straight to R2 (same CORS setup as films) under `audio/{id}/`; rename, reorder and remove tracks, then save and publish. Other formats, or audio you want loudness-normalised, go through `import-audio local`.
- Search ignores Vietnamese tones everywhere: "nguyen du" finds "Nguyễn Du", "den" finds "Đen".
- Public pages are cached for up to 5 minutes, so a change can take that long to show.

The same imports work from the command line (`make import ARGS="..."` loads `api/.env`; or run `go run ./cmd/import-audio` from `api/`). Everything is free:

```bash
make import ARGS="librivox -publish 52"                       # LibriVox audiobook by id (librivox.org/rss/52)
make import ARGS="archive -kind album -publish musopen-chopin" # Internet Archive item: Musopen, netlabels, 78s
make import ARGS='tts -lang vi -publish "Chí Phèo"'           # Vietnamese Wikisource page read by a local AI voice
make import ARGS='tts -out ./preview "Chí Phèo"'              # listen first: writes .m4a files only
make import ARGS="local -kind album -title T -license cc_by -source-url URL a.mp3 b.mp3"
make import ARGS="musicbrainz -publish 797fbb26-5ba0-4e72-9dc9-2501bf88b5ea"  # an artist's releases as link-out cards
make import ARGS="list"                                        # then publish / unpublish / feature / delete ID
```

- **Current artists** (for example Đen Vâu) are copyrighted, so their audio is never hosted. `musicbrainz` lists an artist's albums, EPs and singles from MusicBrainz with cover art from the Cover Art Archive, a YouTube search per release and the artist's official Spotify, Apple Music, YouTube, SoundCloud, Deezer and Tidal pages. Find the artist id on musicbrainz.org.

- **LibriVox and Internet Archive** imports store metadata only; the MP3s play straight from archive.org, so they use no R2 space. Archive items are refused unless their licence is public domain, CC0 or Creative Commons (override with `-license` only after checking the rights yourself).
- **`tts`** fetches each page (one chapter per argument, or `.txt` files with `-source-url`), speaks it with a local model, normalises loudness, encodes AAC 48 kbps mono (about 22 MB per hour) and uploads to `audio/{id}/`. Works are marked "Giọng đọc AI".
  - **Piper** (default, fast on CPU; *Chí Phèo* takes about 2 minutes on an M-series Mac): `pip install piper-tts`, download `vi_VN-vais1000-medium.onnx` and `.onnx.json` from huggingface.co/rhasspy/piper-voices, set `PIPER_MODEL=/path/to/vi_VN-vais1000-medium.onnx`. Install it in a short path such as `~/.venvs/piper`: espeak-ng fails on data paths over 160 characters.
  - **VieNeu-TTS** (more natural, Apache 2.0): `pip install vieneu numpy`, then `TTS_CMD='python3 scripts/tts_vieneu.py'`.
  - Any other engine works: `TTS_CMD` is a shell command that reads the text file `$IN` and writes the WAV `$OUT`.
- Public-domain in Vietnam means the author (and, for translations, the translator) died 50+ years ago, by 31 December. In 2026 that is 1975 or earlier.
- Credits: the Piper voice is trained on the VAIS-1000 corpus (CC BY 4.0); keep a credit on the site. LibriVox recordings are public domain; credit is appreciated.

## 6. Excalidraw drawings

The **Excalidraw** tab (next to My list, `/draw`) is a whiteboard built on [Excalidraw](https://github.com/excalidraw/excalidraw):

- **Visitors** get a free canvas that saves automatically in their own browser and exports PNG, SVG or `.excalidraw` from Excalidraw's menu. A **Sign in to save** button sits above the canvas; nothing is sent to the server.
- **Signed-in members and admins** see **My drawings** instead: a grid of their own drawings (preview, size, last save; search, rename, delete) and **New drawing**. The editor (`/draw/{id}`) has a title, an unsaved-changes indicator and **Save** (or ⌘S / Ctrl+S). Each person only sees and opens their own drawings (anyone else gets "not found"). A sketch drawn before signing in is offered for saving into the account.
- **Studio → Excalidraw** (`/studio/drawings`, admins) lists everyone's drawings with who made them.
- Saving uploads the scene as a `.excalidraw` file plus a PNG preview straight from the browser to the media bucket, in a `drawings/` folder that appears on the first save (nothing to set up). Each drawing gets its own folder, `drawings/{id}-{random token}/`: the media bucket is public, so the 128-bit token keeps drawings from being found by guessing ids, and the site loads them with short-lived signed links. Large drawings with pasted images don't hit the API's request size limit because files never pass through it.
- Limits: 50 MB per drawing, 100 drawings per member (admins unlimited). Deleting an account deletes its drawings from the database; their files stay in the bucket until removed.

## 7. Accounts

People can create an account with a **6-digit invite code** (e.g. `601381`); there are two roles, **admin** (studio, users, invite codes) and **member**.

```bash
make seed-users EMAIL=you@example.com   # admin account for you + demo member + a 10-use invite code (passwords printed once)
make invite ARGS="-uses 5 -days 30 -note team"   # another code from the command line
make admin EMAIL=you@example.com        # create an admin, or reset its password
```

- **Sign-up** (`/signup`) has two steps: the 6-digit invite code (number keypad on phones, checked as soon as the 6th digit is in; `/signup?code=…` fills it in), then name, email, password (with a strength hint) and one of 16 preset avatars.
- **Sign-in** (`/login`) returns you to the page you came from. The header shows **Sign in**, or your avatar with a menu: Profile & settings, My list, Studio (admins), Sign out.
- **Profile** (`/account`): avatar, name, short bio, password change (signs out your other devices), and the list of signed-in devices with per-device sign-out.
- **Studio → Users**: create invite codes (role, number of sign-ups, expiry, note) and copy the code or a ready-made sign-up link; disable or delete codes; change roles and disable accounts (they're signed out at once). You can't remove your own admin access, and at least one active admin always remains.
- **How sign-in works**: server-side sessions. The `lumen_session` cookie (HttpOnly, Secure, SameSite=Lax, 30 days, extended while you use the site) holds a random 256-bit token; the database stores only its SHA-256. Unlike a stateless JWT, signing out, disabling a user or changing a password takes effect immediately. Writes also require a same-site Origin, sign-in is rate-limited per IP, and wrong invite codes are limited to 10 per 15 minutes per IP and 200 per 15 minutes across the whole site (6 digits are only a million combinations, so guessing from many IPs is capped too). Keep codes short-lived and single-use when you can.
- Existing studio admins were moved into accounts automatically (same email and password), and the studio sign-in uses them.

## 8. Code editor (phase 1)

The **Code** tab (`/code`) is a VS Code-style editor built on [Monaco](https://microsoft.github.io/monaco-editor/), the editor inside VS Code (loaded from jsDelivr on first use, not part of the site bundle):

- **Visitors** get a scratch project kept in their browser, with **Sign in to save**; after signing in it's offered for saving into the account.
- **Signed-in users** see **My code**: cards with the first lines, language, file count and size; **New project** from a template (JavaScript, TypeScript, Python, HTML/CSS/JS, Go, empty); rename, delete, search.
- **The editor** (`/code/{id}`): Explorer with folders (new, rename, delete, main file), tabs, syntax highlighting and IntelliSense, status bar (language, line/column), theme following the site, **Save** or ⌘S / Ctrl+S, unsaved changes kept in the browser and offered back if the tab closed.
- **Source control** (built-in history): changed files marked **A / M / D** against the last commit, a message box, **Commit** (⌘Enter), a **History** list; click a commit for a side-by-side **diff** with your files and **Restore this commit**. The last 200 commits are kept.
- **Storage:** each project is one JSON bundle in the media bucket at `code/{id}-{token}/project.json` (uploaded straight from the browser; needs `make r2-cors`); a commit is a server-side copy of it at `commits/{commit id}.json`, so committing never uploads twice. The API reads the saved bundle back to check paths and take the file count and preview.
- **Studio → Code** lists everyone's projects. Limits: 5 MB and 100 files per project, 100 projects per member.
- **Phones** can read projects (read-only, side panels slide over); editing needs a computer.
- Next phases: run HTML/JS and Python in a sandbox; GitHub import/push.

## 9. Database migrations

`api/internal/store/schema.sql` holds idempotent statements (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`) and runs on every start. Changes that can't be written that way are numbered files in `api/internal/store/migrations/` (`0001_uuid_v7.sql`, …): each runs **once**, in its own transaction, in file-name order, and is recorded in the `schema_migrations` table. A Postgres advisory lock stops two API instances from applying the same one twice. They run automatically when the API (or any CLI) starts; `make migrate` applies them on demand and lists what's applied.

- **0002_code_projects**: tables `code_projects` and `code_commits` for the Code tab.
- **0001_uuid_v7**: books, albums and drawings use **UUID v7** ids (time-ordered, not guessable or countable) instead of 1, 2, 3… Existing rows get ids derived from their `created_at`, so the order is kept; `tracks.work_id` follows. Files don't move in storage: migrated works keep their audio folder (`works.media_prefix`, e.g. `audio/45/`) and drawings their folder (`drawings.folder`); new ones use `audio/{uuid}/` and `drawings/{uuid}-{token}/`. Old numeric links (`/books/45`) stop working, and numeric entries in visitors' saved lists are dropped. Films and users keep numeric ids.

**Starting over:** `make reset-data` deletes every Lumen table (films, books, music, drawings, users, sessions, invite codes) after you type `delete`, then recreates an empty schema with all migrations applied. Add `STORAGE=1` to also remove `audio/` and `drawings/` from the R2 bucket (film videos in `hls/` stay). Stop the API first, then reseed with `make seed-all EMAIL=you@example.com`.

## Security notes

- `/studio` is unlisted and `noindex`, but that is not the protection. The protection is admin-only accounts with bcrypt passwords, server-side sessions in HttpOnly + Secure cookies (revocable at any time), an Origin check on every write, and a lockout after 5 failed sign-ins per IP for 15 minutes.
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
