package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

func TestDrawingsAPI(t *testing.T) {
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
	for _, u := range []struct{ email, role string }{{"draw-admin@lumen.test", "admin"}, {"ann@lumen.test", "member"}, {"ben@lumen.test", "member"}} {
		if _, err := st.UpsertUser(ctx, u.email, hash, strings.Split(u.email, "@")[0], u.role, "film"); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), SiteOrigin: "https://lumen.test"}
	s := New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	signIn := func(email string) func(method, path, body string, out any) int {
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar}
		do := func(method, path, body string, out any) int {
			t.Helper()
			req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
			req.Header.Set("Origin", "https://lumen.test")
			res, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if out != nil && res.StatusCode < 300 {
				if err := json.NewDecoder(res.Body).Decode(out); err != nil {
					t.Fatal(err)
				}
			}
			return res.StatusCode
		}
		if code := do("POST", "/api/auth/login", `{"email":"`+email+`","password":"correct horse battery"}`, nil); code != http.StatusOK {
			t.Fatalf("sign in %s: %d", email, code)
		}
		return do
	}
	ann, ben, admin := signIn("ann@lumen.test"), signIn("ben@lumen.test"), signIn("draw-admin@lumen.test")

	// Without storage configured the feature says so.
	if code := ann("GET", "/api/drawings", "", nil); code != http.StatusServiceUnavailable {
		t.Fatalf("no storage: %d", code)
	}
	// Drawings use the media bucket. Presigning works offline, so an unreachable endpoint is enough.
	s.r2 = storage.NewR2("acct", "http://127.0.0.1:1", "key", "secret", "lumen-media")

	var d drawingOut
	if code := ann("POST", "/api/drawings", `{"title":"  "}`, &d); code != http.StatusCreated || d.Title != "Untitled drawing" || d.OwnerName != "ann" {
		t.Fatalf("member creates: %d %+v", code, d)
	}
	defer st.DeleteDrawing(ctx, d.ID)
	id := d.ID
	if code := ann("POST", "/api/drawings", `{"title":"`+strings.Repeat("x", 201)+`"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("long title: %d", code)
	}
	if code := ann("PATCH", "/api/drawings/"+id, `{"title":"Sơ đồ của Ann"}`, &d); code != http.StatusOK || d.Title != "Sơ đồ của Ann" {
		t.Fatalf("rename: %d %+v", code, d)
	}

	// Files go to drawings/{id}-{token}/ in the media bucket; the token is never sent as a field.
	var up struct{ SceneURL, PreviewURL string }
	folder := regexp.MustCompile(`/lumen-media/drawings/` + regexp.QuoteMeta(id) + `-[0-9a-f]{32}/`)
	if code := ann("POST", "/api/drawings/"+id+"/uploads", "", &up); code != http.StatusOK ||
		!folder.MatchString(up.SceneURL) || !strings.Contains(up.SceneURL, "/scene.excalidraw?") ||
		!folder.MatchString(up.PreviewURL) || !strings.Contains(up.PreviewURL, "X-Amz-Signature") {
		t.Fatalf("upload urls: %d %+v", code, up)
	}
	var raw map[string]any
	ann("GET", "/api/drawings/"+id, "", &raw)
	if _, leaked := raw["drawing"].(map[string]any)["token"]; leaked {
		t.Fatalf("token sent to the browser: %+v", raw)
	}

	// Another member can't see, change or delete it (404, not 403, so ids can't be probed).
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/drawings/" + id, ""}, {"PATCH", "/api/drawings/" + id, `{"title":"mine"}`},
		{"POST", "/api/drawings/" + id + "/uploads", ""}, {"POST", "/api/drawings/" + id + "/saved", ""}, {"DELETE", "/api/drawings/" + id, ""},
	} {
		if code := ben(c.method, c.path, c.body, nil); code != http.StatusNotFound {
			t.Fatalf("ben %s %s: %d", c.method, c.path, code)
		}
	}
	var list Page[drawingOut]
	if code := ben("GET", "/api/drawings", "", &list); code != http.StatusOK || list.Total != 0 {
		t.Fatalf("ben's list: %d %+v", code, list)
	}
	if code := ann("GET", "/api/drawings?q=cua%20ann", "", &list); code != http.StatusOK || list.Total != 1 || list.Items[0].ID != d.ID {
		t.Fatalf("ann's search: %d %+v", code, list)
	}
	// Members can't use the studio list; admins see everyone's there.
	if code := ann("GET", "/api/admin/drawings", "", nil); code != http.StatusForbidden {
		t.Fatalf("member in studio: %d", code)
	}
	if code := admin("GET", "/api/admin/drawings?q=ann", "", &list); code != http.StatusOK || list.Total < 1 || list.Items[0].OwnerName != "ann" {
		t.Fatalf("studio list: %d %+v", code, list)
	}
	if code := admin("GET", "/api/admin/drawings/"+id, "", nil); code != http.StatusOK {
		t.Fatalf("admin opens a member's drawing: %d", code)
	}
	if code := admin("GET", "/api/drawings/"+id, "", nil); code != http.StatusNotFound {
		t.Fatalf("admin's own list includes others: %d", code)
	}
	if code := ann("POST", "/api/drawings/01900000-0000-7000-8000-000000000000/uploads", "", nil); code != http.StatusNotFound {
		t.Fatalf("uploads for missing drawing: %d", code)
	}
}
