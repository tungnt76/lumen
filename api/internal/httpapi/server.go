// Package httpapi exposes the public catalog API and the hidden admin API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
	"github.com/tony/lumen/api/internal/tmdb"
)

const sessionCookie = "lumen_studio"

type Server struct {
	cfg      config.Config
	store    *store.Store
	tmdb     *tmdb.Client
	r2       *storage.R2
	sessions *auth.Sessions
	limiter  *auth.Limiter
	plays    *auth.Limiter // one counted play per viewer and film per window
	log      *slog.Logger
}

func New(cfg config.Config, st *store.Store, tc *tmdb.Client, r2 *storage.R2, log *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		store:    st,
		tmdb:     tc,
		r2:       r2,
		sessions: auth.NewSessions(cfg.SessionSecret),
		limiter:  auth.NewLimiter(5, 15*time.Minute),
		plays:    auth.NewLimiter(1, 6*time.Hour),
		log:      log,
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

	// Hidden admin.
	mux.HandleFunc("POST /api/admin/login", s.login)
	mux.HandleFunc("POST /api/admin/logout", s.logout)
	mux.Handle("GET /api/admin/me", s.admin(s.me))
	mux.Handle("GET /api/admin/tmdb/search", s.admin(s.adminTMDBSearch))
	mux.Handle("GET /api/admin/films", s.admin(s.adminFilms))
	mux.Handle("POST /api/admin/films", s.admin(s.adminCreateFilm))
	mux.Handle("POST /api/admin/films/{id}/uploaded", s.admin(s.adminUploaded))
	mux.Handle("POST /api/admin/films/{id}/subtitles", s.admin(s.adminSubtitleURL))
	mux.Handle("PATCH /api/admin/films/{id}", s.admin(s.adminUpdateFilm))
	mux.Handle("DELETE /api/admin/films/{id}", s.admin(s.adminDeleteFilm))

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

// admin requires a valid session cookie and, for writes, a same-site Origin (CSRF defence).
func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		if r.Method != http.MethodGet && !s.sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "bad origin")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not signed in")
			return
		}
		id, ok := s.sessions.Verify(c.Value)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "session expired")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
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
