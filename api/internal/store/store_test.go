package store

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Set TEST_DATABASE_URL to run, e.g. postgres://postgres@localhost:5432/postgres?sslmode=disable
func openTest(t *testing.T) *Store {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `TRUNCATE film_plays, films, admins RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestFilmLifecycle(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	yes, no := true, false

	f, err := s.CreateFilm(ctx, Film{TMDBID: 19, Title: "Metropolis", Year: 1927, GenreIDs: []int32{878, 18}, Rights: "public_domain", RightsNote: "https://archive.org/details/metropolis"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != StatusAwaitingUpload {
		t.Fatalf("status %s", f.Status)
	}
	if _, err := s.CreateFilm(ctx, Film{TMDBID: 19, Title: "dup", Rights: "own", RightsNote: "x"}); err == nil {
		t.Fatal("duplicate tmdb id accepted")
	}

	// Cannot publish before encoding.
	got, err := s.SetFlags(ctx, f.ID, &yes, nil)
	if err != nil || got.Published {
		t.Fatalf("published unencoded film: %v %v", got, err)
	}

	if _, err := s.ClaimJob(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claimed a film that was never uploaded: %v", err)
	}
	if err := s.MarkUploaded(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx)
	if err != nil || job.ID != f.ID || job.Status != StatusEncoding {
		t.Fatalf("claim: %+v %v", job, err)
	}
	if _, err := s.ClaimJob(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatal("job claimed twice")
	}
	if err := s.SetProgress(ctx, f.ID, 42); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishJob(ctx, f.ID, []string{"360p", "720p"}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.PublishedByTMDB(ctx, 19); !errors.Is(err, ErrNotFound) {
		t.Fatal("unpublished film visible")
	}
	got, err = s.SetFlags(ctx, f.ID, &yes, &yes)
	if err != nil || !got.Published || !got.Featured {
		t.Fatalf("publish: %+v %v", got, err)
	}
	if _, err := s.PublishedByTMDB(ctx, 19); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Published(ctx, 878, 10, 0); len(list) != 1 {
		t.Fatalf("genre filter sci-fi: %d", len(list))
	}
	if list, _ := s.Published(ctx, 35, 10, 0); len(list) != 0 {
		t.Fatalf("genre filter comedy: %d", len(list))
	}
	if list, _ := s.Published(ctx, 878, 10, 1); len(list) != 0 {
		t.Fatalf("offset past the end: %d", len(list))
	}
	if n, err := s.CountPublished(ctx, 878); err != nil || n != 1 {
		t.Fatalf("count sci-fi: %d %v", n, err)
	}
	if list, total, err := s.FilmsPage(ctx, 10, 0); err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("films page: %d of %d, %v", len(list), total, err)
	}

	if err := s.AddSubtitle(ctx, f.ID, "vi"); err != nil {
		t.Fatal(err)
	}
	_ = s.AddSubtitle(ctx, f.ID, "vi")
	g, _ := s.GetFilm(ctx, f.ID)
	if len(g.Subtitles) != 1 {
		t.Fatalf("subtitles duplicated: %v", g.Subtitles)
	}

	got, _ = s.SetFlags(ctx, f.ID, &no, nil)
	if got.Published || !got.Featured {
		t.Fatalf("unpublish changed featured: %+v", got)
	}
}

func TestAdmins(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.UpsertAdmin(ctx, "Tony@Example.com", "h1", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAdmin(ctx, "tony@example.com", "h2", "s2"); err != nil {
		t.Fatal(err)
	}
	a, err := s.AdminByEmail(ctx, "TONY@example.com")
	if err != nil || a.PasswordHash != "h2" {
		t.Fatalf("%+v %v", a, err)
	}
}
