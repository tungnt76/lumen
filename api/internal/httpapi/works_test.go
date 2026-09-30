package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/store"
)

func TestPublicWorks(t *testing.T) {
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

	yes := true
	book, err := st.UpsertWork(ctx, store.Work{Kind: store.KindBook, Source: "httpapi-test", SourceID: "chi-pheo",
		Title: "Chí Phèo httpapi-test", Creator: "Nam Cao", Language: "vi", License: "public_domain",
		RightsNote: "https://vi.wikisource.org/wiki/Chí_Phèo", AIVoice: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.DeleteWork(ctx, book.ID)
	if err := st.ReplaceTracks(ctx, book.ID, []store.Track{
		{Position: 1, Title: "Chương 1", DurationSec: 900, Audio: "audio/chi-pheo/1.m4a"},
		{Position: 2, Title: "Chương 2", DurationSec: 800, Audio: "https://archive.org/download/x/2.mp3"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetWorkFlags(ctx, book.ID, &yes, nil); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), MediaBaseURL: "https://media.lumen.test"}
	srv := httptest.NewServer(New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()

	get := func(path string, v any) int {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if v != nil && res.StatusCode == http.StatusOK {
			if err := json.NewDecoder(res.Body).Decode(v); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode
	}

	var books Page[WorkCard]
	if code := get("/api/books?q=httpapi-test&lang=vi", &books); code != http.StatusOK || books.Total != 1 || len(books.Items) != 1 {
		t.Fatalf("books: %d %+v", code, books)
	}
	if c := books.Items[0]; c.ID != book.ID || c.TrackCount != 2 || c.DurationSec != 1700 || !c.AIVoice {
		t.Fatalf("book card: %+v", c)
	}
	var music Page[WorkCard]
	if code := get("/api/music?q=httpapi-test", &music); code != http.StatusOK || music.Total != 0 || music.Items == nil {
		t.Fatalf("music: %d %+v", code, music)
	}

	var detail struct {
		WorkCard
		License   string `json:"license"`
		SourceURL string `json:"sourceUrl"`
		Tracks    []struct {
			Position int    `json:"position"`
			URL      string `json:"url"`
		} `json:"tracks"`
	}
	if code := get("/api/works/"+book.ID, &detail); code != http.StatusOK {
		t.Fatalf("work: %d", code)
	}
	if detail.License != "Public domain" || detail.SourceURL == "" || len(detail.Tracks) != 2 ||
		detail.Tracks[0].URL != "https://media.lumen.test/audio/chi-pheo/1.m4a" ||
		detail.Tracks[1].URL != "https://archive.org/download/x/2.mp3" {
		t.Fatalf("work detail: %+v", detail)
	}

	if code := get("/api/works/0", nil); code != http.StatusBadRequest {
		t.Fatalf("numeric id: %d", code)
	}
	if code := get("/api/works/01900000-0000-7000-8000-000000000000", nil); code != http.StatusNotFound {
		t.Fatalf("bad id: %d", code)
	}
	if _, err := st.SetWorkFlags(ctx, book.ID, new(bool), nil); err != nil {
		t.Fatal(err)
	}
	if code := get("/api/works/"+book.ID, nil); code != http.StatusNotFound {
		t.Fatalf("unpublished work: %d", code)
	}
}
