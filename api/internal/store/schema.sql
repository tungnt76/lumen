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
