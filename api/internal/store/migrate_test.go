package store

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrateUUIDv7 builds the pre-UUID schema in a scratch Postgres schema, fills it with
// numbered rows, then runs the migrations and checks ids, links and storage folders.
func TestMigrateUUIDv7(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS mig_test CASCADE; CREATE SCHEMA mig_test`); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, `DROP SCHEMA IF EXISTS mig_test CASCADE`)

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := pgxpool.New(ctx, url+sep+"search_path=mig_test")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The schema as it was before migration 0001 (schema.sql only), with old-style rows.
	if _, err := db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO works (kind, source, source_id, title, license, rights_note, created_at) VALUES
			('book', 'librivox', '52', 'Old book', 'public_domain', 'x', now() - interval '2 days'),
			('album', 'studio', 'a', 'Old album', 'own', 'x', now() - interval '1 day');
		INSERT INTO tracks (work_id, position, title, audio) SELECT id, 1, 'One', 'audio/' || id || '/u0123456789abcdef.mp3' FROM works;
		INSERT INTO drawings (title, token, created_at) VALUES ('Sketch', 'abcdabcdabcdabcdabcdabcdabcdabcd', now() - interval '3 days'), ('Older', '', now() - interval '4 days');`); err != nil {
		t.Fatal(err)
	}

	s := &Store{db: db}
	for i := 0; i < 2; i++ { // the second run must be a no-op
		if err := s.migrate(ctx); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if v, _ := s.AppliedMigrations(ctx); len(v) == 0 || v[0] != "0001_uuid_v7" {
		t.Fatalf("recorded migrations: %v", v)
	}

	works, _, err := s.WorksPage(ctx, "", "", 10, 0)
	if err != nil || len(works) != 2 {
		t.Fatalf("works: %+v %v", works, err)
	}
	for _, w := range works {
		if len(w.ID) != 36 || w.ID[14] != '7' {
			t.Errorf("%s: id %q isn't a UUID v7", w.Title, w.ID)
		}
		tracks, err := s.Tracks(ctx, w.ID)
		if err != nil || len(tracks) != 1 || !strings.HasPrefix(tracks[0].Audio, w.AudioFolder()) {
			t.Errorf("%s: tracks %+v (folder %s) %v", w.Title, tracks, w.AudioFolder(), err)
		}
		if w.AudioFolder() == "audio/"+w.ID+"/" {
			t.Errorf("%s: lost its old audio folder", w.Title)
		}
	}
	// Newest first, as before: the ids carry the original creation time.
	if works[0].Title != "Old album" || works[0].ID < works[1].ID {
		t.Errorf("order: %s (%s) then %s (%s)", works[0].Title, works[0].ID, works[1].Title, works[1].ID)
	}

	drawings, _, err := s.DrawingsPage(ctx, 0, "", 10, 0)
	if err != nil || len(drawings) != 2 {
		t.Fatalf("drawings: %+v %v", drawings, err)
	}
	for _, d := range drawings {
		want := map[string]string{"Sketch": "drawings/1-abcdabcdabcdabcdabcdabcdabcdabcd/", "Older": "drawings/2/"}[d.Title]
		if d.Folder() != want || d.ID[14] != '7' {
			t.Errorf("%s: id %s folder %q, want %q", d.Title, d.ID, d.Folder(), want)
		}
	}

	// New rows get fresh v7 ids and the new folder layout.
	nw, err := s.UpsertWork(ctx, Work{Kind: KindBook, Source: "x", SourceID: "new", Title: "New", License: "own", RightsNote: "x"})
	if err != nil || nw.ID[14] != '7' || nw.AudioFolder() != "audio/"+nw.ID+"/" || nw.ID < works[0].ID {
		t.Fatalf("new work: %+v %v", nw, err)
	}
	// Deleting a work still removes its tracks (the foreign key survived).
	if err := s.DeleteWork(ctx, works[1].ID); err != nil {
		t.Fatal(err)
	}
	var left int
	db.QueryRow(ctx, `SELECT count(*) FROM tracks`).Scan(&left)
	if left != 1 {
		t.Fatalf("tracks after delete: %d", left)
	}
}
