CREATE TABLE IF NOT EXISTS admins (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS films (
    id            BIGSERIAL PRIMARY KEY,
    tmdb_id       INT UNIQUE NOT NULL,
    title         TEXT NOT NULL,
    year          INT NOT NULL DEFAULT 0,
    overview      TEXT NOT NULL DEFAULT '',
    poster_path   TEXT NOT NULL DEFAULT '',
    backdrop_path TEXT NOT NULL DEFAULT '',
    genre_ids     INT[] NOT NULL DEFAULT '{}',
    runtime       INT NOT NULL DEFAULT 0,
    rights        TEXT NOT NULL CHECK (rights IN ('public_domain', 'licensed', 'own')),
    rights_note   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'awaiting_upload'
                  CHECK (status IN ('awaiting_upload', 'queued', 'encoding', 'ready', 'failed')),
    progress      INT NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT '',
    renditions    TEXT[] NOT NULL DEFAULT '{}',
    subtitles     TEXT[] NOT NULL DEFAULT '{}',
    published     BOOLEAN NOT NULL DEFAULT false,
    featured      BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS films_published_idx ON films (published, created_at DESC);
CREATE INDEX IF NOT EXISTS films_status_idx ON films (status, updated_at);

-- Daily play counts per film, for the "Top 5 on Lumen today" row.
CREATE TABLE IF NOT EXISTS film_plays (
    film_id BIGINT NOT NULL REFERENCES films (id) ON DELETE CASCADE,
    day     DATE NOT NULL DEFAULT current_date,
    plays   INT NOT NULL DEFAULT 0,
    PRIMARY KEY (film_id, day)
);

-- Full release date from TMDB, for newest-first ordering (NULL for films added before it existed).
ALTER TABLE films ADD COLUMN IF NOT EXISTS release_date DATE;

-- Audiobooks and music albums. A work is a book or an album; its tracks are chapters or songs.
-- (source, source_id) identifies where a work was imported from, so re-imports update in place.
CREATE TABLE IF NOT EXISTS works (
    id          BIGSERIAL PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN ('book', 'album')),
    source      TEXT NOT NULL,
    source_id   TEXT NOT NULL,
    title       TEXT NOT NULL,
    creator     TEXT NOT NULL DEFAULT '',
    narrator    TEXT NOT NULL DEFAULT '',
    language    TEXT NOT NULL DEFAULT '',
    year        INT NOT NULL DEFAULT 0,
    description TEXT NOT NULL DEFAULT '',
    cover_url   TEXT NOT NULL DEFAULT '',
    genres      TEXT[] NOT NULL DEFAULT '{}',
    license     TEXT NOT NULL CHECK (license IN ('public_domain', 'cc0', 'cc_by', 'cc_by_sa', 'cc_by_nd',
                    'cc_by_nc', 'cc_by_nc_sa', 'cc_by_nc_nd', 'licensed', 'own')),
    rights_note TEXT NOT NULL,
    ai_voice    BOOLEAN NOT NULL DEFAULT false,
    published   BOOLEAN NOT NULL DEFAULT false,
    featured    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, source_id)
);

CREATE INDEX IF NOT EXISTS works_published_idx ON works (kind, published, created_at DESC);

-- audio is either an absolute URL (e.g. a LibriVox MP3 on archive.org) or a key in the R2 bucket.
CREATE TABLE IF NOT EXISTS tracks (
    id           BIGSERIAL PRIMARY KEY,
    work_id      BIGINT NOT NULL REFERENCES works (id) ON DELETE CASCADE,
    position     INT NOT NULL,
    title        TEXT NOT NULL,
    duration_sec INT NOT NULL DEFAULT 0,
    audio        TEXT NOT NULL,
    UNIQUE (work_id, position)
);

-- Official places to listen, for works Lumen doesn't host (e.g. a current artist's albums):
-- [{"name":"Spotify","url":"https://open.spotify.com/..."}]. Such works have no tracks.
ALTER TABLE works ADD COLUMN IF NOT EXISTS links JSONB NOT NULL DEFAULT '[]';
ALTER TABLE works DROP CONSTRAINT IF EXISTS works_license_check;
ALTER TABLE works ADD CONSTRAINT works_license_check CHECK (license IN ('public_domain', 'cc0', 'cc_by', 'cc_by_sa',
    'cc_by_nd', 'cc_by_nc', 'cc_by_nc_sa', 'cc_by_nc_nd', 'licensed', 'own', 'external'));

-- Title, creator and narrator folded to lowercase ASCII (no Vietnamese tones, đ as d), so
-- "nguyen du" finds "Nguyễn Du". Filled by the app; Open backfills rows that predate it.
ALTER TABLE works ADD COLUMN IF NOT EXISTS search TEXT NOT NULL DEFAULT '';

-- Excalidraw drawings made in the studio. The scene (.excalidraw JSON) and a PNG preview live in
-- the media bucket under drawings/{id}-{token}/ (token: see store.CreateDrawing); saved_at is
-- NULL until the first save.
CREATE TABLE IF NOT EXISTS drawings (
    id         BIGSERIAL PRIMARY KEY,
    title      TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    saved_at   TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS drawings_updated_idx ON drawings (updated_at DESC);

-- Unguessable per-drawing folder token (the media bucket is public).
ALTER TABLE drawings ADD COLUMN IF NOT EXISTS token TEXT NOT NULL DEFAULT '';

-- ---- accounts ----
-- Users sign in with email and password. Roles: admin (studio, users, invites) and member.
-- avatar is the id of one of the preset avatars in web/components/Avatar.tsx.
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    avatar        TEXT NOT NULL DEFAULT '',
    bio           TEXT NOT NULL DEFAULT '',
    disabled      BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

-- Existing studio admins become admin users (same email and password).
INSERT INTO users (email, password_hash, display_name, role, avatar)
SELECT email, password_hash, split_part(email, '@', 1), 'admin', 'film' FROM admins
ON CONFLICT (email) DO NOTHING;

-- Server-side sessions: the cookie holds a random token, the database only its SHA-256.
CREATE TABLE IF NOT EXISTS sessions (
    id           BIGSERIAL PRIMARY KEY,
    token_hash   BYTEA UNIQUE NOT NULL,
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_agent   TEXT NOT NULL DEFAULT '',
    ip           TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions (user_id);

-- Sign-up needs an invite code: 6 digits (codes made before that may be 12 characters).
CREATE TABLE IF NOT EXISTS invite_codes (
    code       TEXT PRIMARY KEY,
    role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    max_uses   INT NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    uses       INT NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    disabled   BOOLEAN NOT NULL DEFAULT false,
    created_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Who made each drawing (members save their own; older studio drawings have no owner).
ALTER TABLE drawings ADD COLUMN IF NOT EXISTS owner_id BIGINT REFERENCES users (id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS drawings_owner_idx ON drawings (owner_id, updated_at DESC);
