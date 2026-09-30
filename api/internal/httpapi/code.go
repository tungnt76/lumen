package httpapi

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tony/lumen/api/internal/store"
)

// The Code tab: projects of text files edited in the browser, each saved as one JSON bundle in the
// media bucket. Signed-in users keep their own under /api/code; admins see all under
// /api/admin/code. Commits are server-side copies of the saved bundle (no second upload).

const (
	maxBundleSize     = 5 << 20
	maxProjectFiles   = 100
	maxMemberProjects = 100
)

// codeBundle is the saved working copy (and each commit snapshot).
type codeBundle struct {
	Version int `json:"version"`
	Files   []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
	Main string `json:"main"`
}

// languages maps file extensions to the names shown in the list (Monaco ids on the web side).
var languages = map[string]string{
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".ts": "typescript", ".tsx": "typescript",
	".py": "python", ".go": "go", ".java": "java", ".c": "c", ".h": "c", ".cpp": "cpp", ".cs": "csharp",
	".rs": "rust", ".rb": "ruby", ".php": "php", ".swift": "swift", ".kt": "kotlin", ".sql": "sql",
	".html": "html", ".css": "css", ".scss": "scss", ".json": "json", ".md": "markdown", ".yaml": "yaml",
	".yml": "yaml", ".xml": "xml", ".sh": "shell", ".txt": "plaintext",
}

func languageOf(p string) string {
	if l, ok := languages[strings.ToLower(path.Ext(p))]; ok {
		return l
	}
	return "plaintext"
}

// validPath allows relative paths like "src/app.js": no leading slash, no "..", no empty parts.
func validPath(p string) bool {
	if p == "" || len(p) > 300 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

type codeScope struct{ all bool }

var (
	ownCode = codeScope{}
	allCode = codeScope{all: true}
)

func (s *Server) loadProject(w http.ResponseWriter, r *http.Request, sc codeScope) (*store.CodeProject, bool) {
	if !s.needDrawings(w) { // same storage requirement as drawings
		return nil, false
	}
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return nil, false
	}
	p, err := s.store.GetProject(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	me, _ := currentUser(r)
	if !sc.all && p.OwnerID != me.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return p, true
}

// GET .../code?q=&page=
func (s *Server) listProjects(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.needDrawings(w) {
			return
		}
		me, _ := currentUser(r)
		owner := me.ID
		if sc.all {
			owner = 0
		}
		page := pageParam(r, 0)
		list, total, err := s.store.ProjectsPage(r.Context(), owner, r.URL.Query().Get("q"), adminPageSize, (page-1)*adminPageSize)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, Page[store.CodeProject]{Items: list, Page: page, TotalPages: pages(total, adminPageSize), Total: total})
	}
}

func projectTitle(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if t == "" {
		t = "Untitled project"
	}
	return t, utf8.RuneCountInString(t) <= 200
}

// POST .../code {title, language}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if !s.needDrawings(w) {
		return
	}
	var in struct {
		Title    string `json:"title"`
		Language string `json:"language"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	title, ok := projectTitle(in.Title)
	if !ok || len(in.Language) > 40 {
		writeErr(w, http.StatusBadRequest, "the title is too long (200 characters max)")
		return
	}
	me, _ := currentUser(r)
	if me.Role != store.RoleAdmin {
		n, err := s.store.CountProjects(r.Context(), me.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if n >= maxMemberProjects {
			writeErr(w, http.StatusConflict, "you have 100 projects, the most an account can keep; delete some to make room")
			return
		}
	}
	p, err := s.store.CreateProject(r.Context(), title, in.Language, me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// GET .../code/{id}: the project and, once saved, a URL to download its files.
func (s *Server) getProject(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		out := map[string]any{"project": p, "bundleUrl": ""}
		if p.SavedAt != nil {
			u, err := s.r2.PresignGet(r.Context(), p.BundleKey(), drawingURLTTL)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			out["bundleUrl"] = u
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// PATCH .../code/{id} {title}
func (s *Server) renameProject(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		var in struct {
			Title string `json:"title"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad request")
			return
		}
		title, ok := projectTitle(in.Title)
		if !ok {
			writeErr(w, http.StatusBadRequest, "the title is too long (200 characters max)")
			return
		}
		renamed, err := s.store.RenameProject(r.Context(), p.ID, title)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, renamed)
	}
}

// DELETE .../code/{id}: removes the project, its commits and all its files.
func (s *Server) deleteProject(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		if err := s.r2.DeletePrefix(r.Context(), p.Folder()); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.store.DeleteProject(r.Context(), p.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST .../code/{id}/uploads: a presigned PUT URL for the working copy.
func (s *Server) projectUpload(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		u, err := s.r2.PresignPut(r.Context(), p.BundleKey(), "application/json", 15*time.Minute)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"bundleUrl": u})
	}
}

// POST .../code/{id}/saved: the upload finished. The server reads the bundle back to check it and
// to take the file count, main language and preview for the list (not trusting the browser).
func (s *Server) projectSaved(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		size, found, err := s.r2.Size(r.Context(), p.BundleKey())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !found {
			writeErr(w, http.StatusConflict, "the project hasn't reached storage yet; save again")
			return
		}
		if size > maxBundleSize {
			writeErr(w, http.StatusRequestEntityTooLarge, "the project is over 5 MB; code projects are for text files")
			return
		}
		raw, err := s.r2.Get(r.Context(), p.BundleKey(), maxBundleSize)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var b codeBundle
		if err := json.Unmarshal(raw, &b); err != nil {
			writeErr(w, http.StatusBadRequest, "the saved project isn't valid")
			return
		}
		if len(b.Files) > maxProjectFiles {
			writeErr(w, http.StatusBadRequest, "a project can have at most 100 files")
			return
		}
		seen := map[string]bool{}
		mainIdx := 0
		for i, f := range b.Files {
			if !validPath(f.Path) || seen[f.Path] {
				writeErr(w, http.StatusBadRequest, "file names must be unique relative paths like src/app.js")
				return
			}
			seen[f.Path] = true
			if f.Path == b.Main {
				mainIdx = i
			}
		}
		language, preview := "", ""
		if len(b.Files) > 0 {
			main := b.Files[mainIdx]
			language = languageOf(main.Path)
			lines := strings.SplitN(main.Content, "\n", 13)
			preview = strings.Join(lines[:min(len(lines), 12)], "\n")
			if len(preview) > 800 {
				preview = preview[:800]
				for !utf8.ValidString(preview) { // don't cut a character in half
					preview = preview[:len(preview)-1]
				}
			}
		}
		saved, err := s.store.MarkProjectSaved(r.Context(), p.ID, size, len(b.Files), language, preview)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

// GET .../code/{id}/commits
func (s *Server) listCommits(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		list, err := s.store.Commits(r.Context(), p.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// POST .../code/{id}/commits {message, changes}: snapshots the saved working copy. The browser
// saves first; the server copies project.json to commits/{id}.json inside the bucket.
func (s *Server) createCommit(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		var in struct {
			Message string             `json:"message"`
			Changes []store.FileChange `json:"changes"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad request")
			return
		}
		in.Message = strings.TrimSpace(in.Message)
		switch {
		case p.SavedAt == nil:
			writeErr(w, http.StatusConflict, "save the project before committing")
			return
		case in.Message == "":
			writeErr(w, http.StatusBadRequest, "write a commit message")
			return
		case utf8.RuneCountInString(in.Message) > 500:
			writeErr(w, http.StatusBadRequest, "the commit message is too long (500 characters max)")
			return
		case len(in.Changes) > maxProjectFiles*2:
			writeErr(w, http.StatusBadRequest, "too many changes")
			return
		}
		for _, c := range in.Changes {
			if !validPath(c.Path) || (c.Status != "A" && c.Status != "M" && c.Status != "D") {
				writeErr(w, http.StatusBadRequest, "bad change list")
				return
			}
		}
		id, err := s.store.NewCommitID(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.r2.Copy(r.Context(), p.BundleKey(), p.CommitKey(id)); err != nil {
			s.fail(w, r, err)
			return
		}
		me, _ := currentUser(r)
		c, dropped, err := s.store.CreateCommit(r.Context(), id, p.ID, me.ID, in.Message, in.Changes, p.FileCount, p.SizeBytes)
		if err != nil {
			_ = s.r2.Delete(r.Context(), p.CommitKey(id))
			s.fail(w, r, err)
			return
		}
		for _, d := range dropped { // beyond the last MaxCommits
			if err := s.r2.Delete(r.Context(), p.CommitKey(d)); err != nil {
				s.log.Warn("delete old commit", "project", p.ID, "commit", d, "err", err)
			}
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

// GET .../code/{id}/commits/{cid}: the commit and a URL to download its snapshot.
func (s *Server) getCommit(sc codeScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadProject(w, r, sc)
		if !ok {
			return
		}
		cid := strings.ToLower(r.PathValue("cid"))
		if !uuidRe.MatchString(cid) {
			writeErr(w, http.StatusBadRequest, "bad commit id")
			return
		}
		c, err := s.store.GetCommit(r.Context(), p.ID, cid)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		u, err := s.r2.PresignGet(r.Context(), p.CommitKey(c.ID), drawingURLTTL)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"commit": c, "url": u})
	}
}
