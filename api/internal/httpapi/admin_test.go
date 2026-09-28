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

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/store"
)

func TestAdminLoginFlow(t *testing.T) {
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
	secret, _ := auth.NewTOTPSecret()
	if err := st.UpsertAdmin(ctx, "admin@lumen.test", hash, secret); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), SiteOrigin: "https://lumen.test", CookieSecure: true}
	srv := httptest.NewServer(New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()

	post := func(path, body, origin string, cookie *http.Cookie) *http.Response {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	// Unauthenticated admin API is refused.
	if res, _ := http.Get(srv.URL + "/api/admin/films"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("films without login: %d", res.StatusCode)
	}
	// Cross-site login attempt is refused.
	if res := post("/api/admin/login", `{}`, "https://evil.test", nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin login: %d", res.StatusCode)
	}
	// Wrong password fails.
	if res := post("/api/admin/login", `{"email":"admin@lumen.test","password":"wrong password"}`, "https://lumen.test", nil); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad password: %d", res.StatusCode)
	}

	res := post("/api/admin/login", `{"email":"ADMIN@lumen.test","password":"correct horse battery"}`, "https://lumen.test", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", res.StatusCode)
	}
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie flags: %+v", session)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/me", nil)
	req.AddCookie(session)
	me, err := http.DefaultClient.Do(req)
	if err != nil || me.StatusCode != http.StatusOK {
		t.Fatalf("me: %v %v", me.StatusCode, err)
	}
	me.Body.Close()

	// The film list is paginated; out-of-range pages clamp to 1.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/admin/films?page=0", nil)
	req.AddCookie(session)
	list, err := http.DefaultClient.Do(req)
	if err != nil || list.StatusCode != http.StatusOK {
		t.Fatalf("films: %v %v", list.StatusCode, err)
	}
	var page Page[store.Film]
	err = json.NewDecoder(list.Body).Decode(&page)
	list.Body.Close()
	if err != nil || page.Page != 1 || page.Items == nil || len(page.Items) > adminPageSize {
		t.Fatalf("films page: %+v %v", page, err)
	}

	// A signed-in write from another origin is still refused (CSRF).
	if res := post("/api/admin/films/1/uploaded", `{}`, "https://evil.test", session); res.StatusCode != http.StatusForbidden {
		t.Fatalf("csrf write: %d", res.StatusCode)
	}

	// Brute force: 5 failures lock the IP, even for correct credentials afterwards.
	for i := 0; i < 5; i++ {
		post("/api/admin/login", `{"email":"x@y.z","password":"nope"}`, "https://lumen.test", nil)
	}
	if res := post("/api/admin/login", `{"email":"admin@lumen.test","password":"correct horse battery"}`, "https://lumen.test", nil); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d", res.StatusCode)
	}
}
