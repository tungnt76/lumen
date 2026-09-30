// Package httpapi exposes the public catalog API and the hidden admin API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tony/lumen/api/internal/audiosrc"
	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
	"github.com/tony/lumen/api/internal/tmdb"
)

// sessionCookie holds the random session token (HttpOnly; the database stores only its hash).
const sessionCookie = "lumen_session"

// authHint is a readable, non-secret cookie set alongside the session so the site knows to ask
// /api/auth/me (for the avatar menu) without doing so for every visitor. It grants nothing.
const authHint = "lumen_auth"

type Server struct {
	cfg     config.Config
	store   *store.Store
	tmdb    *tmdb.Client
	r2      *storage.R2
	limiter *auth.Limiter // failed sign-ins per IP
	invites *auth.Limiter // wrong invite codes per IP
	guesses *auth.Limiter // wrong invite codes from everyone: 6 digits need a global cap too
	signups *auth.Limiter // new accounts per IP, so one multi-use code can't mint accounts in bulk
	plays   *auth.Limiter // one counted play per viewer and film per window
	audio   *audiosrc.Client
	log     *slog.Logger
}

func New(cfg config.Config, st *store.Store, tc *tmdb.Client, r2 *storage.R2, log *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		store:   st,
		tmdb:    tc,
		r2:      r2,
		limiter: auth.NewLimiter(5, 15*time.Minute),
		invites: auth.NewLimiter(10, 15*time.Minute),
		guesses: auth.NewLimiter(200, 15*time.Minute),
		signups: auth.NewLimiter(5, time.Hour),
		plays:   auth.NewLimiter(1, 6*time.Hour),
		audio:   audiosrc.New("LumenStudio/1.0 (" + cfg.SiteOrigin + ")"),
		log:     log,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Public, no login.
	mux.HandleFunc("GET /api/home", s.home)
	mux.HandleFunc("GET /api/genres", s.genres)
	mux.HandleFunc("GET /api/movies/{id}", s.movie)
	mux.HandleFunc("POST /api/movies/{id}/play", s.play)
	mux.HandleFunc("GET /api/browse", s.browse)
	mux.HandleFunc("GET /api/search", s.search)
	mux.HandleFunc("GET /api/books", s.works(store.KindBook))
	mux.HandleFunc("GET /api/music", s.works(store.KindAlbum))
	mux.HandleFunc("GET /api/works/{id}", s.work)

	// Accounts.
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/signup", s.signup)
	mux.HandleFunc("POST /api/auth/invite", s.checkInvite)
	mux.Handle("GET /api/auth/me", s.signedIn(s.authMe))
	mux.Handle("PATCH /api/auth/me", s.signedIn(s.updateProfile))
	mux.Handle("POST /api/auth/password", s.signedIn(s.changePassword))
	mux.Handle("GET /api/auth/sessions", s.signedIn(s.listSessions))
	mux.Handle("DELETE /api/auth/sessions/{id}", s.signedIn(s.endSession))
	mux.Handle("POST /api/auth/sessions/others", s.signedIn(s.endOtherSessions))

	// Studio (admins). /api/admin/login is the studio sign-in: the same accounts, admins only.
	mux.HandleFunc("POST /api/admin/login", s.adminLogin)
	mux.HandleFunc("POST /api/admin/logout", s.logout)
	mux.Handle("GET /api/admin/users", s.admin(s.adminUsers))
	mux.Handle("PATCH /api/admin/users/{id}", s.admin(s.adminUpdateUser))
	mux.Handle("GET /api/admin/invites", s.admin(s.adminInvites))
	mux.Handle("POST /api/admin/invites", s.admin(s.adminCreateInvite))
	mux.Handle("PATCH /api/admin/invites/{code}", s.admin(s.adminUpdateInvite))
	mux.Handle("DELETE /api/admin/invites/{code}", s.admin(s.adminDeleteInvite))
	mux.Handle("GET /api/admin/me", s.admin(s.me))
	mux.Handle("GET /api/admin/tmdb/search", s.admin(s.adminTMDBSearch))
	mux.Handle("GET /api/admin/films", s.admin(s.adminFilms))
	mux.Handle("POST /api/admin/films", s.admin(s.adminCreateFilm))
	mux.Handle("POST /api/admin/films/{id}/uploaded", s.admin(s.adminUploaded))
	mux.Handle("POST /api/admin/films/{id}/subtitles", s.admin(s.adminSubtitleURL))
	mux.Handle("PATCH /api/admin/films/{id}", s.admin(s.adminUpdateFilm))
	mux.Handle("DELETE /api/admin/films/{id}", s.admin(s.adminDeleteFilm))
	mux.Handle("GET /api/admin/works", s.admin(s.adminWorks))
	mux.Handle("POST /api/admin/works", s.admin(s.adminCreateWork))
	mux.Handle("POST /api/admin/works/import", s.admin(s.adminImportWorks))
	mux.Handle("GET /api/admin/works/{id}", s.admin(s.adminWork))
	mux.Handle("PATCH /api/admin/works/{id}", s.admin(s.adminUpdateWork))
	mux.Handle("DELETE /api/admin/works/{id}", s.admin(s.adminDeleteWork))
	mux.Handle("POST /api/admin/works/{id}/uploads", s.admin(s.adminWorkUpload))
	mux.Handle("PUT /api/admin/works/{id}/tracks", s.admin(s.adminSetTracks))
	// Excalidraw: every signed-in user has their own drawings; the studio sees all of them.
	for _, rt := range []struct {
		prefix string
		wrap   func(http.HandlerFunc) http.Handler
		scope  drawingScope
	}{{"/api/drawings", s.signedIn, ownDrawings}, {"/api/admin/drawings", s.admin, allDrawings}} {
		mux.Handle("GET "+rt.prefix, rt.wrap(s.listDrawings(rt.scope)))
		mux.Handle("POST "+rt.prefix, rt.wrap(s.createDrawing))
		mux.Handle("GET "+rt.prefix+"/{id}", rt.wrap(s.getDrawing(rt.scope)))
		mux.Handle("PATCH "+rt.prefix+"/{id}", rt.wrap(s.renameDrawing(rt.scope)))
		mux.Handle("DELETE "+rt.prefix+"/{id}", rt.wrap(s.deleteDrawing(rt.scope)))
		mux.Handle("POST "+rt.prefix+"/{id}/uploads", rt.wrap(s.drawingUploads(rt.scope)))
		mux.Handle("POST "+rt.prefix+"/{id}/saved", rt.wrap(s.drawingSaved(rt.scope)))
	}

	// Code projects, with the same split: your own, or (studio) everyone's.
	for _, rt := range []struct {
		prefix string
		wrap   func(http.HandlerFunc) http.Handler
		scope  codeScope
	}{{"/api/code", s.signedIn, ownCode}, {"/api/admin/code", s.admin, allCode}} {
		mux.Handle("GET "+rt.prefix, rt.wrap(s.listProjects(rt.scope)))
		mux.Handle("POST "+rt.prefix, rt.wrap(s.createProject))
		mux.Handle("GET "+rt.prefix+"/{id}", rt.wrap(s.getProject(rt.scope)))
		mux.Handle("PATCH "+rt.prefix+"/{id}", rt.wrap(s.renameProject(rt.scope)))
		mux.Handle("DELETE "+rt.prefix+"/{id}", rt.wrap(s.deleteProject(rt.scope)))
		mux.Handle("POST "+rt.prefix+"/{id}/uploads", rt.wrap(s.projectUpload(rt.scope)))
		mux.Handle("POST "+rt.prefix+"/{id}/saved", rt.wrap(s.projectSaved(rt.scope)))
		mux.Handle("GET "+rt.prefix+"/{id}/commits", rt.wrap(s.listCommits(rt.scope)))
		mux.Handle("POST "+rt.prefix+"/{id}/commits", rt.wrap(s.createCommit(rt.scope)))
		mux.Handle("GET "+rt.prefix+"/{id}/commits/{cid}", rt.wrap(s.getCommit(rt.scope)))
	}

	return s.recoverer(s.logger(securityHeaders(mux)))
}

// ---- middleware ----

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "err", v, "path", r.URL.Path)
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

type authCtx struct {
	user      *store.User
	sessionID int64
}

// signedIn requires a valid session and, for writes, a same-site Origin (CSRF defence).
func (s *Server) signedIn(h http.HandlerFunc) http.Handler { return s.requireUser(h, false) }

// admin is signedIn for admins only.
func (s *Server) admin(h http.HandlerFunc) http.Handler { return s.requireUser(h, true) }

func (s *Server) requireUser(h http.HandlerFunc, adminOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		if r.Method != http.MethodGet && !s.sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "bad origin")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "not signed in")
			return
		}
		u, sid, err := s.store.SessionUser(r.Context(), c.Value)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "session expired, sign in again")
			return
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
		if adminOnly && u.Role != store.RoleAdmin {
			writeErr(w, http.StatusForbidden, "this needs an admin account")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, authCtx{user: u, sessionID: sid})))
	})
}

// currentUser is the signed-in user inside signedIn / admin handlers.
func currentUser(r *http.Request) (*store.User, int64) {
	a, _ := r.Context().Value(ctxKey{}).(authCtx)
	return a.user, a.sessionID
}

func (s *Server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	return o == "" || strings.EqualFold(strings.TrimRight(o, "/"), s.cfg.SiteOrigin)
}

// clientIP uses the first X-Forwarded-For hop set by the hosting proxy (Vercel / Render).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		writeErr(w, http.StatusBadGateway, "upstream error")
	}
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// pathUUID reads a {id} that is a UUID (books, albums, drawings), lowercased.
func pathUUID(r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("id"))
	return id, uuidRe.MatchString(id)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

// Page is one page of a list response. Pages are numbered from 1.
type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	TotalPages int `json:"totalPages"`
	Total      int `json:"total"`
}

const (
	browsePageSize = 48 // fills 2, 3, 4, 6 or 8 grid columns evenly
	adminPageSize  = 50
	maxPage        = 1000 // bounds OFFSET scans on database-backed lists
)

// pageParam reads ?page=, defaulting to 1 and clamping to [1, max] (maxPage when max is 0).
func pageParam(r *http.Request, max int) int {
	if max == 0 {
		max = maxPage
	}
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return min(p, max)
}

// pages is how many pages of size hold total items.
func pages(total, size int) int { return (total + size - 1) / size }

func cacheFor(w http.ResponseWriter, seconds int) {
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(seconds)+", stale-while-revalidate=600")
}
