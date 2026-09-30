-- 0002: the Code tab. A project is a small set of text files edited in the browser (Monaco).
-- Its working copy is one JSON bundle in the media bucket at code/{id}-{token}/project.json;
-- each commit is a copy of that bundle at code/{id}-{token}/commits/{commit id}.json.

CREATE TABLE code_projects (
    id         uuid PRIMARY KEY DEFAULT uuid_v7(),
    owner_id   BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token      TEXT NOT NULL,              -- random part of the storage folder (the bucket is public)
    title      TEXT NOT NULL,
    language   TEXT NOT NULL DEFAULT '',   -- main language, for the list
    preview    TEXT NOT NULL DEFAULT '',   -- first lines of the main file, for the list
    file_count INT NOT NULL DEFAULT 0,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    saved_at   TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX code_projects_owner_idx ON code_projects (owner_id, updated_at DESC);

-- changes: what the commit changed compared with the one before, e.g.
-- [{"path":"src/app.js","status":"M"},{"path":"README.md","status":"A"}]
CREATE TABLE code_commits (
    id         uuid PRIMARY KEY DEFAULT uuid_v7(),
    project_id uuid NOT NULL REFERENCES code_projects (id) ON DELETE CASCADE,
    author_id  BIGINT REFERENCES users (id) ON DELETE SET NULL,
    message    TEXT NOT NULL,
    changes    JSONB NOT NULL DEFAULT '[]',
    file_count INT NOT NULL DEFAULT 0,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX code_commits_project_idx ON code_commits (project_id, created_at DESC);
