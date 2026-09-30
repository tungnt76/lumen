package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/store"
)

func TestAdminWorks(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, _ := auth.HashPassword("correct horse battery")
	if _, err := st.UpsertUser(ctx, "works@lumen.test", hash, "Admin", "admin", "film"); err != nil {
		t.Fatal(err)
	}

	// A fake LibriVox for the import; book 8888 never answers in time.
	lv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "8888" {
			<-r.Context().Done()
			return
		}
		if r.URL.Query().Get("id") != "7777" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"books":[{"id":"7777","title":"Admin Test Book","language":"English","url_librivox":"https://librivox.org/x/",
			"authors":[{"first_name":"Ann","last_name":"Author"}],"sections":[
			{"section_number":"1","title":"One","listen_url":"https://archive.org/download/x/1.mp3","playtime":"60","readers":[]},
			{"section_number":"2","title":"Two","listen_url":"https://archive.org/download/x/2.mp3","playtime":"70","readers":[]}]}]}`))
	}))
	defer lv.Close()

	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), SiteOrigin: "https://lumen.test", MediaBaseURL: "https://media.lumen.test"}
	s := New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.audio.LibriVoxURL = lv.URL
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	var session *http.Cookie
	do := func(method, path, body string, out any) int {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://lumen.test")
		if session != nil {
			req.AddCookie(session)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		for _, c := range res.Cookies() {
			if c.Name == sessionCookie {
				session = c
			}
		}
		if out != nil && res.StatusCode < 300 {
			if err := json.NewDecoder(res.Body).Decode(out); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode
	}

	if code := do("GET", "/api/admin/works", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("works without login: %d", code)
	}
	if code := do("POST", "/api/admin/login", `{"email":"works@lumen.test","password":"correct horse battery"}`, nil); code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}

	// Create needs a rights confirmation.
	if code := do("POST", "/api/admin/works", `{"kind":"book","title":"Truyện Kiều","license":"public_domain","rightsNote":"https://vi.wikisource.org/wiki/Truyện_Kiều"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("create without confirm: %d", code)
	}
	if code := do("POST", "/api/admin/works", `{"kind":"book","title":"x","license":"external","rightsNote":"https://example.com/x","confirm":true}`, nil); code != http.StatusBadRequest {
		t.Fatalf("create with external licence: %d", code)
	}
	var draft store.Work
	if code := do("POST", "/api/admin/works", `{"kind":"book","title":"Truyện Kiều","creator":"Nguyễn Du","language":"vi","license":"public_domain",
		"rightsNote":"https://vi.wikisource.org/wiki/Truyện_Kiều","confirm":true,"aiVoice":true}`, &draft); code != http.StatusCreated || draft.ID == "" || draft.Source != "studio" {
		t.Fatalf("create: %d %+v", code, draft)
	}
	defer st.DeleteWork(ctx, draft.ID)
	did := draft.ID

	if code := do("PATCH", "/api/admin/works/"+did, `{"published":true}`, nil); code != http.StatusConflict {
		t.Fatalf("publish without tracks: %d", code)
	}
	if code := do("POST", "/api/admin/works/"+did+"/uploads", `{"contentType":"video/mp4"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("upload a video: %d", code)
	}
	// Tracks can't point at arbitrary URLs or another work's files.
	for _, audio := range []string{"https://evil.test/a.mp3", "audio/01900000-0000-7000-8000-000000000000/u0123456789abcdef.mp3", "hls/1/master.m3u8"} {
		body := `[{"title":"x","durationSec":1,"audio":"` + audio + `"}]`
		if code := do("PUT", "/api/admin/works/"+did+"/tracks", body, nil); code != http.StatusBadRequest {
			t.Fatalf("foreign audio %s: %d", audio, code)
		}
	}

	// Search ignores Vietnamese tones.
	var found Page[store.Work]
	if code := do("GET", "/api/admin/works?q=nguyen%20du&kind=book", "", &found); code != http.StatusOK || found.Total != 1 || found.Items[0].ID != draft.ID {
		t.Fatalf("search: %d %+v", code, found)
	}

	// Import from LibriVox, pasting the RSS link.
	var imported struct{ Works []store.Work }
	if code := do("POST", "/api/admin/works/import", `{"source":"librivox","id":"https://librivox.org/rss/7777","publish":true}`, &imported); code != http.StatusOK ||
		len(imported.Works) != 1 || !imported.Works[0].Published || imported.Works[0].TrackCount != 2 {
		t.Fatalf("import: %d %+v", code, imported)
	}
	book := imported.Works[0]
	defer st.DeleteWork(ctx, book.ID)
	bid := book.ID
	if code := do("POST", "/api/admin/works/import", `{"source":"librivox","id":"abc"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad librivox id: %d", code)
	}
	if code := do("POST", "/api/admin/works/import", `{"source":"librivox","id":"1"}`, nil); code != http.StatusBadGateway {
		t.Fatalf("missing librivox book: %d", code)
	}
	defer func(d time.Duration) { importTimeout = d }(importTimeout)
	importTimeout = 300 * time.Millisecond
	var slow struct{ Error string }
	req, _ := http.NewRequest("POST", srv.URL+"/api/admin/works/import", strings.NewReader(`{"source":"librivox","id":"8888"}`))
	req.Header.Set("Origin", "https://lumen.test")
	req.AddCookie(session)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(res.Body).Decode(&slow)
	res.Body.Close()
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(slow.Error, "didn't answer in time") {
		t.Fatalf("slow source: %d %q", res.StatusCode, slow.Error)
	}

	// Reorder and rename existing tracks.
	var detail struct {
		Work   store.Work
		Tracks []adminTrack
	}
	body := `[{"title":"Chapter Two","durationSec":70,"audio":"https://archive.org/download/x/2.mp3"},
		{"title":"Chapter One","durationSec":60,"audio":"https://archive.org/download/x/1.mp3"}]`
	if code := do("PUT", "/api/admin/works/"+bid+"/tracks", body, &detail); code != http.StatusOK ||
		len(detail.Tracks) != 2 || detail.Tracks[0].Title != "Chapter Two" || detail.Tracks[0].Position != 1 ||
		detail.Tracks[1].URL != "https://archive.org/download/x/1.mp3" {
		t.Fatalf("reorder: %d %+v", code, detail)
	}
	if code := do("PUT", "/api/admin/works/"+bid+"/tracks", `[{"title":" ","audio":"https://archive.org/download/x/1.mp3"}]`, nil); code != http.StatusBadRequest {
		t.Fatalf("blank title: %d", code)
	}

	if code := do("PATCH", "/api/admin/works/"+bid, `{"featured":true}`, &book); code != http.StatusOK || !book.Featured || !book.Published {
		t.Fatalf("feature: %d %+v", code, book)
	}
	if code := do("DELETE", "/api/admin/works/"+bid, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code := do("GET", "/api/admin/works/"+bid, "", nil); code != http.StatusNotFound {
		t.Fatalf("deleted work: %d", code)
	}
}

func TestSourceID(t *testing.T) {
	for _, c := range []struct{ source, in, want string }{
		{"archive", "https://archive.org/details/musopen-chopin?tab=about", "musopen-chopin"},
		{"archive", "musopen-chopin", "musopen-chopin"},
		{"musicbrainz", "https://musicbrainz.org/artist/797fbb26-5ba0-4e72-9dc9-2501bf88b5ea/releases", "797fbb26-5ba0-4e72-9dc9-2501bf88b5ea"},
		{"librivox", "https://librivox.org/rss/52", "52"},
		{"librivox", "52", "52"},
	} {
		if got := sourceID(c.source, c.in); got != c.want {
			t.Errorf("%s %q: got %q, want %q", c.source, c.in, got, c.want)
		}
	}
}
