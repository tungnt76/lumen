package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
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
	if _, err := s.db.Exec(ctx, `TRUNCATE sessions, invite_codes, users, drawings, tracks, works, film_plays, films, admins RESTART IDENTITY CASCADE`); err != nil {
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

func TestWorks(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	yes := true

	w, err := s.UpsertWork(ctx, Work{Kind: KindBook, Source: "wikisource", SourceID: "Truyện_Kiều", Title: "Truyện Kiều",
		Creator: "Nguyễn Du", Language: "vi", License: "public_domain", RightsNote: "https://vi.wikisource.org/wiki/Truyện_Kiều", AIVoice: true})
	if err != nil {
		t.Fatal(err)
	}
	if w.TrackCount != 0 || w.Genres == nil || len(w.ID) != 36 || w.ID[14] != '7' || w.AudioFolder() != "audio/"+w.ID+"/" {
		t.Fatalf("new work: %+v", w)
	}

	// Cannot publish a work with no tracks.
	if got, err := s.SetWorkFlags(ctx, w.ID, &yes, nil); err != nil || got.Published {
		t.Fatalf("published empty work: %+v %v", got, err)
	}

	tracks := []Track{{1, "Phần 1", 600, "audio/1/1.m4a"}, {2, "Phần 2", 540, "audio/1/2.m4a"}}
	if err := s.ReplaceTracks(ctx, w.ID, tracks); err != nil {
		t.Fatal(err)
	}
	got, err := s.SetWorkFlags(ctx, w.ID, &yes, nil)
	if err != nil || !got.Published || got.TrackCount != 2 || got.DurationSec != 1140 {
		t.Fatalf("publish: %+v %v", got, err)
	}

	// Re-import updates metadata in place and keeps the published flag.
	again, err := s.UpsertWork(ctx, Work{Kind: KindBook, Source: "wikisource", SourceID: "Truyện_Kiều", Title: "Truyện Kiều (Đoạn trường tân thanh)",
		Creator: "Nguyễn Du", Language: "vi", License: "public_domain", RightsNote: "x"})
	if err != nil || again.ID != w.ID || !again.Published || again.TrackCount != 2 {
		t.Fatalf("re-import: %+v %v", again, err)
	}
	if err := s.ReplaceTracks(ctx, w.ID, tracks[:1]); err != nil {
		t.Fatal(err)
	}
	if ts, _ := s.Tracks(ctx, w.ID); len(ts) != 1 || ts[0].Title != "Phần 1" {
		t.Fatalf("replace tracks: %+v", ts)
	}

	// An unpublished album in the same catalog.
	if _, err := s.UpsertWork(ctx, Work{Kind: KindAlbum, Source: "musopen", SourceID: "1", Title: "Nocturnes", License: "public_domain", RightsNote: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertWork(ctx, Work{Kind: "podcast", Source: "x", SourceID: "1", Title: "x", License: "own", RightsNote: "x"}); err == nil {
		t.Fatal("unknown kind accepted")
	}

	for _, c := range []struct {
		f    WorkFilter
		want int
	}{
		{WorkFilter{Kind: KindBook}, 1},
		{WorkFilter{Kind: KindBook, Language: "vi"}, 1},
		{WorkFilter{Kind: KindBook, Language: "en"}, 0},
		{WorkFilter{Kind: KindBook, Query: "nguyễn"}, 1},
		{WorkFilter{Kind: KindBook, Query: "tân thanh"}, 1},
		{WorkFilter{Kind: KindBook, Query: "nguyen du"}, 1}, // no tones typed
		{WorkFilter{Kind: KindBook, Query: "DOAN TRUONG"}, 1},
		{WorkFilter{Kind: KindBook, Query: "truyen kieu x"}, 0},
		{WorkFilter{Kind: KindBook, Query: "%"}, 0},
		{WorkFilter{Kind: KindAlbum}, 0}, // the album above is unpublished
	} {
		list, err := s.PublishedWorks(ctx, c.f, 10, 0)
		n, err2 := s.CountPublishedWorks(ctx, c.f)
		if err != nil || err2 != nil || len(list) != c.want || n != c.want {
			t.Errorf("%+v: got %d/%d, want %d (%v %v)", c.f, len(list), n, c.want, err, err2)
		}
	}

	// A work Lumen doesn't host can be published with official links and no tracks.
	ext, err := s.UpsertWork(ctx, Work{Kind: KindAlbum, Source: "musicbrainz", SourceID: "rg-1", Title: "Show Của Đen", Creator: "Đen",
		License: "external", RightsNote: "https://musicbrainz.org/release-group/rg-1",
		Links: []Link{{"Spotify", "https://open.spotify.com/artist/x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.SetWorkFlags(ctx, ext.ID, &yes, nil); err != nil || !got.Published || got.TrackCount != 0 || len(got.Links) != 1 || got.Links[0].Name != "Spotify" {
		t.Fatalf("publish external: %+v %v", got, err)
	}
	if n, _ := s.CountPublishedWorks(ctx, WorkFilter{Kind: KindAlbum}); n != 1 {
		t.Fatalf("external album not listed: %d", n)
	}

	// The studio sees drafts too.
	if list, total, err := s.WorksPage(ctx, "", "", 10, 0); err != nil || total != 3 || len(list) != 3 {
		t.Fatalf("works page: %d of %d, %v", len(list), total, err)
	}
	if list, total, _ := s.WorksPage(ctx, KindAlbum, "den", 10, 0); total != 1 || list[0].ID != ext.ID {
		t.Fatalf("studio search for Đen: %d %+v", total, list)
	}

	if _, err := s.PublishedWork(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWork(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PublishedWork(ctx, w.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted work still visible: %v", err)
	}
}

func TestDrawings(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	alice, _ := s.UpsertUser(ctx, "alice@draw.test", "h", "Alice", RoleMember, "star")
	bob, _ := s.UpsertUser(ctx, "bob@draw.test", "h", "Bob", RoleMember, "moon")

	a, err := s.CreateDrawing(ctx, "Sơ đồ kiến trúc", alice.ID)
	if err != nil || a.SavedAt != nil || a.SizeBytes != 0 || len(a.Token) != 32 || !a.OwnedBy(alice.ID) || a.OwnedBy(bob.ID) || a.OwnerName != "Alice" {
		t.Fatalf("create: %+v %v", a, err)
	}
	if len(a.ID) != 36 || a.ID[14] != '7' {
		t.Fatalf("drawing id isn't a UUID v7: %q", a.ID)
	}
	if want := fmt.Sprintf("drawings/%s-%s/", a.ID, a.Token); a.Folder() != want {
		t.Fatalf("folder %q, want %q", a.Folder(), want)
	}
	b, _ := s.CreateDrawing(ctx, "Wireframe home", alice.ID)
	c, _ := s.CreateDrawing(ctx, "Bob's plan", bob.ID)
	if _, err := s.MarkDrawingSaved(ctx, a.ID, 1234); err != nil {
		t.Fatal(err)
	}
	// Each user lists only their own, most recently changed first; the studio sees all.
	list, total, err := s.DrawingsPage(ctx, alice.ID, "", 10, 0)
	if err != nil || total != 2 || list[0].ID != a.ID || list[0].SavedAt == nil || list[0].SizeBytes != 1234 {
		t.Fatalf("alice's list: %d %+v %v", total, list, err)
	}
	if _, total, _ := s.DrawingsPage(ctx, bob.ID, "", 10, 0); total != 1 {
		t.Fatalf("bob's list: %d", total)
	}
	if _, total, _ := s.DrawingsPage(ctx, 0, "", 10, 0); total != 3 {
		t.Fatalf("studio list: %d", total)
	}
	// Search ignores tones and also matches the owner's name (studio).
	if list, total, _ := s.DrawingsPage(ctx, alice.ID, "so do", 10, 0); total != 1 || list[0].ID != a.ID {
		t.Fatalf("search: %d %+v", total, list)
	}
	if list, total, _ := s.DrawingsPage(ctx, 0, "bob", 10, 0); total != 1 || list[0].ID != c.ID {
		t.Fatalf("owner search: %d %+v", total, list)
	}
	if list, total, _ := s.DrawingsPage(ctx, alice.ID, "", 1, 5); total != 2 || len(list) != 0 {
		t.Fatalf("offset past end: %d %+v", total, list)
	}
	if n, _ := s.CountDrawings(ctx, alice.ID); n != 2 {
		t.Fatalf("count: %d", n)
	}
	if r, err := s.RenameDrawing(ctx, b.ID, "Home wireframe"); err != nil || r.Title != "Home wireframe" {
		t.Fatalf("rename: %+v %v", r, err)
	}
	if err := s.DeleteDrawing(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDrawing(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted drawing: %v", err)
	}
}

func TestAccounts(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	exp := time.Now().Add(time.Hour)
	inv, err := s.CreateInvite(ctx, RoleMember, 1, &exp, "one friend", 0)
	if err != nil || len(inv.Code) != InviteCodeLen || strings.Trim(inv.Code, "0123456789") != "" || !inv.Usable {
		t.Fatalf("invite: %+v %v", inv, err)
	}
	// Typed with a space is fine: "482 915".
	typed := inv.Code[:3] + " " + inv.Code[3:]
	if got, err := s.InviteByCode(ctx, typed); err != nil || got.Code != inv.Code {
		t.Fatalf("typed code: %+v %v", got, err)
	}

	u, err := s.SignUp(ctx, typed, "An@Example.com", "hash", "An Nguyễn", "moon")
	if err != nil || u.Role != RoleMember || u.Email != "an@example.com" {
		t.Fatalf("sign up: %+v %v", u, err)
	}
	// Single-use code is spent; the email is taken.
	if _, err := s.SignUp(ctx, inv.Code, "b@example.com", "hash", "B", "sun"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("reused code: %v", err)
	}
	inv2, _ := s.CreateInvite(ctx, RoleAdmin, 5, nil, "", 0)
	if _, err := s.SignUp(ctx, inv2.Code, "AN@example.com", "hash", "Dup", "sun"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	// A failed sign-up doesn't use up the code.
	if got, _ := s.InviteByCode(ctx, inv2.Code); got.Uses != 0 {
		t.Fatalf("code used by a failed sign-up: %+v", got)
	}
	if a, err := s.SignUp(ctx, inv2.Code, "boss@example.com", "hash", "Boss", "star"); err != nil || a.Role != RoleAdmin {
		t.Fatalf("admin invite: %+v %v", a, err)
	}

	// Sessions: the token works, is stored hashed, and ends on sign-out or disable.
	tok, err := s.CreateSession(ctx, u.ID, "Safari on iPhone", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var stored int
	s.db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = convert_to($1, 'UTF8')`, tok).Scan(&stored)
	if stored != 0 {
		t.Fatal("raw token stored in the database")
	}
	got, sid, err := s.SessionUser(ctx, tok)
	if err != nil || got.ID != u.ID || sid == 0 {
		t.Fatalf("session user: %+v %v", got, err)
	}
	tok2, _ := s.CreateSession(ctx, u.ID, "Chrome on Mac", "5.6.7.8")
	if list, _ := s.Sessions(ctx, u.ID, sid); len(list) != 2 || !(list[0].Current || list[1].Current) {
		t.Fatalf("sessions: %+v", list)
	}
	if err := s.DeleteOtherSessions(ctx, u.ID, sid); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, tok2); !errors.Is(err, ErrNotFound) {
		t.Fatal("other session survived")
	}
	yes := true
	if _, err := s.SetUserAccess(ctx, u.ID, nil, &yes); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, tok); !errors.Is(err, ErrNotFound) {
		t.Fatal("disabled user still signed in")
	}

	if p, err := s.UpdateProfile(ctx, u.ID, "An", "rocket", "Hi"); err != nil || p.Avatar != "rocket" || p.Bio != "Hi" {
		t.Fatalf("profile: %+v %v", p, err)
	}
	if list, total, _ := s.UsersPage(ctx, "boss", 10, 0); total != 1 || list[0].Role != RoleAdmin {
		t.Fatalf("users search: %d %+v", total, list)
	}
	// Many codes: always 6 digits and unique.
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := s.CreateInvite(ctx, RoleMember, 1, nil, "", 0)
		if err != nil || len(c.Code) != InviteCodeLen || seen[c.Code] {
			t.Fatalf("code %d: %+v %v", i, c, err)
		}
		seen[c.Code] = true
	}
	if n, _ := s.CountAdmins(ctx); n != 1 {
		t.Fatalf("admins: %d", n)
	}
}
