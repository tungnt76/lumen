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
	"strings"
	"testing"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

func TestCodeProjects(t *testing.T) {
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
	for _, u := range []struct{ email, role string }{{"code-admin@lumen.test", "admin"}, {"cara@lumen.test", "member"}, {"dan@lumen.test", "member"}} {
		if _, err := st.UpsertUser(ctx, u.email, hash, strings.Split(u.email, "@")[0], u.role, "film"); err != nil {
			t.Fatal(err)
		}
	}
	s3 := newFakeS3(t)
	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), SiteOrigin: "https://lumen.test"}
	s := New(cfg, st, nil, storage.NewR2("acct", s3.srv.URL, "key", "secret", "lumen-media"), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	cara, dan, admin := signIn("cara@lumen.test"), signIn("dan@lumen.test"), signIn("code-admin@lumen.test")
	put := func(u, body string) {
		t.Helper()
		req, _ := http.NewRequest("PUT", u, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("PUT bundle: %v %v", res, err)
		}
		res.Body.Close()
	}

	var p store.CodeProject
	if code := cara("POST", "/api/code", `{"title":"","language":"javascript"}`, &p); code != http.StatusCreated || p.Title != "Untitled project" || len(p.ID) != 36 || p.ID[14] != '7' {
		t.Fatalf("create: %d %+v", code, p)
	}
	defer st.DeleteProject(ctx, p.ID)
	base := "/api/code/" + p.ID

	// Committing before the first save is refused.
	if code := cara("POST", base+"/commits", `{"message":"init"}`, nil); code != http.StatusConflict {
		t.Fatalf("commit unsaved: %d", code)
	}

	save := func(bundle string) (int, store.CodeProject) {
		var up struct{ BundleURL string }
		if code := cara("POST", base+"/uploads", "", &up); code != http.StatusOK || !strings.Contains(up.BundleURL, "/lumen-media/code/"+p.ID+"-") {
			t.Fatalf("upload url: %d %+v", code, up)
		}
		put(up.BundleURL, bundle)
		var saved store.CodeProject
		return cara("POST", base+"/saved", "", &saved), saved
	}
	code, saved := save(`{"version":1,"main":"src/app.js","files":[{"path":"README.md","content":"# Hi"},{"path":"src/app.js","content":"const a = 1;\nconsole.log(a);"}]}`)
	if code != http.StatusOK || saved.FileCount != 2 || saved.Language != "javascript" || !strings.HasPrefix(saved.Preview, "const a = 1;") || saved.SavedAt == nil {
		t.Fatalf("saved: %d %+v", code, saved)
	}
	// Bad bundles are rejected by the server's check.
	for _, bad := range []string{`not json`, `{"files":[{"path":"../etc/passwd","content":""}]}`, `{"files":[{"path":"a.js","content":""},{"path":"a.js","content":""}]}`} {
		if code, _ := save(bad); code != http.StatusBadRequest {
			t.Fatalf("bad bundle %q: %d", bad, code)
		}
	}
	save(`{"version":1,"main":"src/app.js","files":[{"path":"README.md","content":"# Hi"},{"path":"src/app.js","content":"const a = 1;\nconsole.log(a);"}]}`)

	// Commit, then change and commit again.
	var c1 store.CodeCommit
	if code := cara("POST", base+"/commits", `{"message":"First version","changes":[{"path":"README.md","status":"A"},{"path":"src/app.js","status":"A"}]}`, &c1); code != http.StatusCreated ||
		c1.Message != "First version" || c1.FileCount != 2 || len(c1.Changes) != 2 || c1.AuthorName != "cara" {
		t.Fatalf("commit 1: %d %+v", code, c1)
	}
	save(`{"version":1,"main":"src/app.js","files":[{"path":"src/app.js","content":"const a = 2;"}]}`)
	var c2 store.CodeCommit
	cara("POST", base+"/commits", `{"message":"Change a","changes":[{"path":"src/app.js","status":"M"},{"path":"README.md","status":"D"}]}`, &c2)
	for _, bad := range []string{`{"message":"  "}`, `{"message":"x","changes":[{"path":"/abs","status":"M"}]}`, `{"message":"x","changes":[{"path":"a","status":"Z"}]}`} {
		if code := cara("POST", base+"/commits", bad, nil); code != http.StatusBadRequest {
			t.Fatalf("bad commit %s: %d", bad, code)
		}
	}
	var commits []store.CodeCommit
	if code := cara("GET", base+"/commits", "", &commits); code != http.StatusOK || len(commits) != 2 || commits[0].ID != c2.ID {
		t.Fatalf("history: %d %+v", code, commits)
	}
	// Each commit is its own snapshot: the first still has two files.
	var got struct {
		Commit store.CodeCommit
		URL    string
	}
	if code := cara("GET", base+"/commits/"+c1.ID, "", &got); code != http.StatusOK || got.URL == "" {
		t.Fatalf("get commit: %d %+v", code, got)
	}
	res, err := http.Get(got.URL)
	if err != nil {
		t.Fatal(err)
	}
	var snap codeBundle
	json.NewDecoder(res.Body).Decode(&snap)
	res.Body.Close()
	if len(snap.Files) != 2 {
		t.Fatalf("snapshot of commit 1 has %d files", len(snap.Files))
	}
	if n := len(s3.keys("lumen-media/code/" + p.ID + "-")); n != 3 { // project.json + 2 commits
		t.Fatalf("objects in storage: %d %v", n, s3.keys("lumen-media/code/"))
	}

	// Someone else can't reach it; the studio can.
	for _, path := range []string{base, base + "/commits", base + "/commits/" + c1.ID} {
		if code := dan("GET", path, "", nil); code != http.StatusNotFound {
			t.Fatalf("dan GET %s: %d", path, code)
		}
	}
	var list Page[store.CodeProject]
	if code := dan("GET", "/api/code", "", &list); code != http.StatusOK || list.Total != 0 {
		t.Fatalf("dan's list: %d %+v", code, list)
	}
	if code := admin("GET", "/api/admin/code?q=cara", "", &list); code != http.StatusOK || list.Total < 1 || list.Items[0].OwnerName != "cara" {
		t.Fatalf("studio list: %d %+v", code, list)
	}

	// Delete removes the files and the commits.
	if code := cara("DELETE", base, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if left := s3.keys("lumen-media/code/" + p.ID + "-"); len(left) != 0 {
		t.Fatalf("files left: %v", left)
	}
	if n, _ := st.Commits(ctx, p.ID); len(n) != 0 {
		t.Fatalf("commits left: %d", len(n))
	}
}
