package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
	"github.com/tony/lumen/api/internal/tmdb"
)

const sessionTTL = 12 * time.Hour

// POST /api/admin/login {email, password}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "bad origin")
		return
	}
	ip := clientIP(r)
	if !s.limiter.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in 15 minutes")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	a, err := s.store.AdminByEmail(r.Context(), strings.TrimSpace(in.Email))
	ok := false
	if err == nil {
		ok = auth.CheckPassword(a.PasswordHash, in.Password)
	} else {
		auth.DummyCheck(in.Password)
	}
	if !ok {
		s.limiter.Fail(ip)
		s.log.Warn("admin login failed", "ip", ip)
		writeErr(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	s.limiter.Reset(ip)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: s.sessions.Issue(a.ID, sessionTTL), Path: "/",
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"email": a.Email})
}

// POST /api/admin/logout
func (s *Server) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func adminID(r *http.Request) int64 { id, _ := r.Context().Value(ctxKey{}).(int64); return id }

// GET /api/admin/me
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.AdminByID(r.Context(), adminID(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "not signed in")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": a.Email})
}

// GET /api/admin/tmdb/search?q=&page=
func (s *Server) adminTMDBSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := pageParam(r, tmdb.MaxSearchPage)
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, Page[map[string]any]{Items: []map[string]any{}, Page: page})
		return
	}
	res, err := s.tmdb.Search(r.Context(), q, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := []map[string]any{}
	for _, m := range res.Results {
		out = append(out, map[string]any{"tmdbId": m.ID, "title": m.Title, "year": m.Year(), "poster": tmdb.Image("w185", m.PosterPath)})
	}
	writeJSON(w, http.StatusOK, Page[map[string]any]{Items: out, Page: page,
		TotalPages: min(res.TotalPages, tmdb.MaxSearchPage), Total: res.TotalResults})
}

// GET /api/admin/films?page=
func (s *Server) adminFilms(w http.ResponseWriter, r *http.Request) {
	page := pageParam(r, 0)
	films, total, err := s.store.FilmsPage(r.Context(), adminPageSize, (page-1)*adminPageSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if films == nil {
		films = []store.Film{}
	}
	writeJSON(w, http.StatusOK, Page[store.Film]{Items: films, Page: page, TotalPages: pages(total, adminPageSize), Total: total})
}

var allowedVideo = map[string]bool{"video/mp4": true, "video/x-matroska": true, "video/quicktime": true, "video/webm": true}

// POST /api/admin/films {tmdbId, rights, rightsNote, confirm, contentType}
// Creates the film and returns a presigned URL; the browser uploads the source straight to R2.
func (s *Server) adminCreateFilm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TMDBID      int    `json:"tmdbId"`
		Rights      string `json:"rights"`
		RightsNote  string `json:"rightsNote"`
		Confirm     bool   `json:"confirm"`
		ContentType string `json:"contentType"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	in.RightsNote = strings.TrimSpace(in.RightsNote)
	switch {
	case in.TMDBID <= 0:
		writeErr(w, http.StatusBadRequest, "pick a film on TMDB")
		return
	case in.Rights != "public_domain" && in.Rights != "licensed" && in.Rights != "own":
		writeErr(w, http.StatusBadRequest, "choose a rights basis")
		return
	case !in.Confirm || len(in.RightsNote) < 8:
		writeErr(w, http.StatusBadRequest, "confirm the rights and add a source link or licence reference")
		return
	case !allowedVideo[in.ContentType]:
		writeErr(w, http.StatusBadRequest, "upload an .mp4, .mkv, .mov or .webm file")
		return
	}
	d, err := s.tmdb.Movie(r.Context(), in.TMDBID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	gids := make([]int32, 0, len(d.Genres))
	for _, g := range d.Genres {
		gids = append(gids, int32(g.ID))
	}
	f, err := s.store.CreateFilm(r.Context(), store.Film{
		TMDBID: d.ID, Title: d.Title, Year: d.Year(), Overview: d.Overview, PosterPath: d.PosterPath,
		BackdropPath: d.BackdropPath, GenreIDs: gids, Runtime: d.Runtime, Rights: in.Rights, RightsNote: in.RightsNote,
		ReleaseDate: d.Released(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			writeErr(w, http.StatusConflict, "this film is already in the library")
			return
		}
		s.fail(w, r, err)
		return
	}
	url, err := s.r2.PresignPut(r.Context(), storage.SourceKey(f.ID), in.ContentType, 6*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"film": f, "uploadUrl": url})
}

// POST /api/admin/films/{id}/uploaded: the browser finished the PUT; queue encoding.
func (s *Server) adminUploaded(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	exists, err := s.r2.Exists(r.Context(), storage.SourceKey(id))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !exists {
		writeErr(w, http.StatusConflict, "source file not found in storage yet")
		return
	}
	if err := s.store.MarkUploaded(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var langRe = regexp.MustCompile(`^[a-z]{2}$`)

// POST /api/admin/films/{id}/subtitles {lang}: presigned URL for a .vtt file.
func (s *Server) adminSubtitleURL(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Lang string `json:"lang"`
	}
	if err := decode(r, &in); err != nil || !langRe.MatchString(in.Lang) {
		writeErr(w, http.StatusBadRequest, "language must be a 2-letter code like vi or en")
		return
	}
	if _, err := s.store.GetFilm(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	url, err := s.r2.PresignPut(r.Context(), storage.SubtitleKey(id, in.Lang), "text/vtt", time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.AddSubtitle(r.Context(), id, in.Lang); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"uploadUrl": url})
}

// PATCH /api/admin/films/{id} {published?, featured?}
func (s *Server) adminUpdateFilm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Published *bool `json:"published"`
		Featured  *bool `json:"featured"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	f, err := s.store.SetFlags(r.Context(), id, in.Published, in.Featured)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Published != nil && *in.Published && !f.Published {
		writeErr(w, http.StatusConflict, "only encoded films can be published")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// DELETE /api/admin/films/{id}: removes the film and its media.
func (s *Server) adminDeleteFilm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	for _, p := range []string{storage.HLSPrefix(id), storage.SourcePrefix(id)} {
		if err := s.r2.DeletePrefix(r.Context(), p); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.store.DeleteFilm(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
