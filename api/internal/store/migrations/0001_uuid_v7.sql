-- 0001: books, albums (works) and drawings get UUID v7 ids instead of sequential numbers.
--
-- UUID v7 starts with a millisecond timestamp, so ids still sort by creation time and index
-- well, but they don't reveal how many items exist or let anyone step through them.
-- Existing rows get ids built from their created_at, keeping their order. Files in storage
-- don't move: each row remembers its old folder (works.media_prefix, drawings.folder).

-- uuid_v7(ts): a version-7 UUID for the given time (default: now). Postgres before 18 has no
-- built-in, so take a random v4 UUID, write the 48-bit Unix-millisecond time over its first
-- 6 bytes and flip the version nibble from 4 (0100) to 7 (0111). The variant bits stay RFC 4122.
CREATE OR REPLACE FUNCTION uuid_v7(ts timestamptz DEFAULT clock_timestamp()) RETURNS uuid
LANGUAGE sql VOLATILE AS $$
    SELECT encode(
        set_bit(set_bit(
            overlay(uuid_send(gen_random_uuid())
                    PLACING substring(int8send(floor(extract(epoch FROM ts) * 1000)::bigint) FROM 3)
                    FROM 1 FOR 6),
            52, 1), 53, 1),
        'hex')::uuid
$$;

-- ---- works (books and albums) and their tracks ----

ALTER TABLE works ADD COLUMN new_id uuid;
UPDATE works SET new_id = uuid_v7(created_at);
-- Uploaded audio of existing works stays under audio/{old id}/.
ALTER TABLE works ADD COLUMN IF NOT EXISTS media_prefix TEXT NOT NULL DEFAULT '';
UPDATE works SET media_prefix = 'audio/' || id || '/';

ALTER TABLE tracks ADD COLUMN work_uuid uuid;
UPDATE tracks t SET work_uuid = w.new_id FROM works w WHERE w.id = t.work_id;
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS tracks_work_id_fkey;
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS tracks_work_id_position_key;
ALTER TABLE tracks DROP COLUMN work_id;
ALTER TABLE tracks RENAME COLUMN work_uuid TO work_id;
ALTER TABLE tracks ALTER COLUMN work_id SET NOT NULL;

ALTER TABLE works DROP CONSTRAINT works_pkey;
ALTER TABLE works DROP COLUMN id; -- drops its sequence too
ALTER TABLE works RENAME COLUMN new_id TO id;
ALTER TABLE works ALTER COLUMN id SET NOT NULL;
ALTER TABLE works ALTER COLUMN id SET DEFAULT uuid_v7();
ALTER TABLE works ADD PRIMARY KEY (id);

ALTER TABLE tracks ADD CONSTRAINT tracks_work_id_fkey FOREIGN KEY (work_id) REFERENCES works (id) ON DELETE CASCADE;
ALTER TABLE tracks ADD CONSTRAINT tracks_work_id_position_key UNIQUE (work_id, position);

-- ---- drawings ----

ALTER TABLE drawings ADD COLUMN new_id uuid;
UPDATE drawings SET new_id = uuid_v7(created_at);
-- Files of existing drawings stay in drawings/{old id}-{token}/ (or drawings/{old id}/).
ALTER TABLE drawings ADD COLUMN IF NOT EXISTS folder TEXT NOT NULL DEFAULT '';
UPDATE drawings SET folder = CASE WHEN token = '' THEN 'drawings/' || id || '/' ELSE 'drawings/' || id || '-' || token || '/' END;

ALTER TABLE drawings DROP CONSTRAINT drawings_pkey;
ALTER TABLE drawings DROP COLUMN id;
ALTER TABLE drawings RENAME COLUMN new_id TO id;
ALTER TABLE drawings ALTER COLUMN id SET NOT NULL;
ALTER TABLE drawings ALTER COLUMN id SET DEFAULT uuid_v7();
ALTER TABLE drawings ADD PRIMARY KEY (id);
