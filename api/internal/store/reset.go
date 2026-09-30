package store

import "context"

// ResetAll drops every Lumen table (and the uuid_v7 function), so the next Open recreates an
// empty database from schema.sql and the migrations. It can't be undone.
func (s *Store) ResetAll(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `DROP TABLE IF EXISTS
		code_commits, code_projects, sessions, invite_codes, drawings, tracks, works, film_plays, films, users, admins, schema_migrations CASCADE;
		DROP FUNCTION IF EXISTS uuid_v7(timestamptz);`)
	return err
}
