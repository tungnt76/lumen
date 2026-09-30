package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tony/lumen/api/internal/store"
)

// Studio API for audiobooks and music: import from free sources, create a book or album,
// upload its audio straight to R2, arrange tracks, publish, feature and delete.

// GET /api/admin/works?kind=&q=&page=
func (s *Server) adminWorks(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != store.KindBook && kind != store.KindAlbum {
		kind = ""
	}
	page := pageParam(r, 0)
	works, total, err := s.store.WorksPage(r.Context(), kind, r.URL.Query().Get("q"), adminPageSize, (page-1)*adminPageSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, Page[store.Work]{Items: works, Page: page, TotalPages: pages(total, adminPageSize), Total: total})
}

type adminTrack struct {
	store.Track
	URL string `json:"url"`
}

// GET /api/admin/works/{id}: a work (published or not) with its tracks.
func (s *Server) adminWork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	s.writeAdminWork(w, r, id)
}

func (s *Server) writeAdminWork(w http.ResponseWriter, r *http.Request, id string) {
	wk, err := s.store.GetWork(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tracks, err := s.store.Tracks(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]adminTrack, len(tracks))
	for i, t := range tracks {
		out[i] = adminTrack{Track: t, URL: s.audioURL(t.Audio)}
	}
	writeJSON(w, http.StatusOK, map[string]any{"work": wk, "tracks": out})
}

// Licences a studio upload may use ("external" is only for link-out imports).
var hostable = map[string]bool{
	"public_domain": true, "cc0": true, "cc_by": true, "cc_by_sa": true, "cc_by_nd": true,
	"cc_by_nc": true, "cc_by_nc_sa": true, "cc_by_nc_nd": true, "licensed": true, "own": true,
}

// POST /api/admin/works: creates a draft book or album for uploading your own audio.
func (s *Server) adminCreateWork(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind        string `json:"kind"`
		Title       string `json:"title"`
		Creator     string `json:"creator"`
		Narrator    string `json:"narrator"`
		Language    string `json:"language"`
		Year        int    `json:"year"`
		Description string `json:"description"`
		CoverURL    string `json:"coverUrl"`
		License     string `json:"license"`
		RightsNote  string `json:"rightsNote"`
		AIVoice     bool   `json:"aiVoice"`
		Confirm     bool   `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	trim := func(s *string, max int) {
		*s = strings.TrimSpace(*s)
		if utf8.RuneCountInString(*s) > max {
			*s = string([]rune(*s)[:max])
		}
	}
	trim(&in.Title, 300)
	trim(&in.Creator, 300)
	trim(&in.Narrator, 300)
	trim(&in.Description, 5000)
	trim(&in.CoverURL, 1000)
	trim(&in.RightsNote, 1000)
	switch {
	case in.Kind != store.KindBook && in.Kind != store.KindAlbum:
		writeErr(w, http.StatusBadRequest, "choose book or album")
		return
	case in.Title == "":
		writeErr(w, http.StatusBadRequest, "add a title")
		return
	case in.Language != "" && !langRe.MatchString(in.Language):
		writeErr(w, http.StatusBadRequest, "language must be a 2-letter code like vi or en")
		return
	case in.CoverURL != "" && !strings.HasPrefix(in.CoverURL, "https://"):
		writeErr(w, http.StatusBadRequest, "the cover must be an https:// image URL")
		return
	case !hostable[in.License]:
		writeErr(w, http.StatusBadRequest, "choose a licence")
		return
	case !in.Confirm || len(in.RightsNote) < 8:
		writeErr(w, http.StatusBadRequest, "confirm the rights and add a source link or licence reference")
		return
	}
	wk, err := s.store.UpsertWork(r.Context(), store.Work{
		Kind: in.Kind, Source: "studio", SourceID: randomID(), Title: in.Title, Creator: in.Creator,
		Narrator: in.Narrator, Language: in.Language, Year: in.Year, Description: in.Description,
		CoverURL: in.CoverURL, License: in.License, RightsNote: in.RightsNote, AIVoice: in.AIVoice,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wk)
}

// POST /api/admin/works/import {source, id, kind, publish}: fetches from LibriVox, the
// Internet Archive or MusicBrainz, the same as the import-audio command.
func (s *Server) adminImportWorks(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source  string `json:"source"`
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Publish bool   `json:"publish"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	in.ID = sourceID(in.Source, strings.TrimSpace(in.ID))
	// Finish before the site's /api proxy gives up (about 30 s): LibriVox often takes 15-20 s.
	ctx, cancel := context.WithTimeout(r.Context(), importTimeout)
	defer cancel()
	sourceErr := func(err error) {
		msg := err.Error()
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "the source didn't answer in time (LibriVox is often slow); try again in a minute, or use the import-audio command"
		}
		writeErr(w, http.StatusBadGateway, msg)
	}
	type item struct {
		work   store.Work
		tracks []store.Track
	}
	var items []item
	switch in.Source {
	case "librivox":
		n, err := strconv.Atoi(in.ID)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "a LibriVox id is a number, e.g. 52")
			return
		}
		wk, tracks, err := s.audio.LibriVox(ctx, n)
		if err != nil {
			sourceErr(err)
			return
		}
		items = append(items, item{wk, tracks})
	case "archive":
		if in.Kind != store.KindBook && in.Kind != store.KindAlbum {
			in.Kind = store.KindAlbum
		}
		if !archiveIDRe.MatchString(in.ID) {
			writeErr(w, http.StatusBadRequest, "paste the identifier from archive.org/details/IDENTIFIER")
			return
		}
		wk, tracks, err := s.audio.Archive(ctx, in.ID, in.Kind, "")
		if err != nil {
			sourceErr(err)
			return
		}
		items = append(items, item{wk, tracks})
	case "musicbrainz":
		if !mbidRe.MatchString(in.ID) {
			writeErr(w, http.StatusBadRequest, "paste the artist id from musicbrainz.org/artist/ID")
			return
		}
		works, err := s.audio.MusicBrainzArtist(ctx, in.ID, []string{"album", "ep", "single"})
		if err != nil {
			sourceErr(err)
			return
		}
		for _, wk := range works {
			items = append(items, item{wk, nil})
		}
	default:
		writeErr(w, http.StatusBadRequest, "choose LibriVox, Internet Archive or MusicBrainz")
		return
	}

	ctx = r.Context() // saving is quick; don't let a slow fetch cut it short
	saved := make([]store.Work, 0, len(items))
	for _, it := range items {
		wk, err := s.store.UpsertWork(ctx, it.work)
		if err == nil && it.tracks != nil {
			err = s.store.ReplaceTracks(ctx, wk.ID, it.tracks)
		}
		if err == nil && in.Publish {
			yes := true
			wk, err = s.store.SetWorkFlags(ctx, wk.ID, &yes, nil)
		} else if err == nil {
			wk, err = s.store.GetWork(ctx, wk.ID) // fresh track count
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		saved = append(saved, *wk)
	}
	writeJSON(w, http.StatusOK, map[string]any{"works": saved})
}

// importTimeout bounds a studio import; tests shorten it.
var importTimeout = 28 * time.Second

var (
	archiveIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)
	mbidRe      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	idInURLRe   = map[string]*regexp.Regexp{
		"archive":     regexp.MustCompile(`archive\.org/details/([^/?#]+)`),
		"musicbrainz": regexp.MustCompile(`musicbrainz\.org/artist/([0-9a-f-]{36})`),
		"librivox":    regexp.MustCompile(`librivox\.org/rss/(\d+)`),
	}
)

// sourceID accepts either a bare id or a pasted page link and returns the id.
func sourceID(source, s string) string {
	if re, ok := idInURLRe[source]; ok {
		if m := re.FindStringSubmatch(s); m != nil {
			return m[1]
		}
	}
	return s
}

// PATCH /api/admin/works/{id} {published?, featured?}
func (s *Server) adminUpdateWork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
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
	wk, err := s.store.SetWorkFlags(r.Context(), id, in.Published, in.Featured)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Published != nil && *in.Published && !wk.Published {
		writeErr(w, http.StatusConflict, "add at least one track before publishing")
		return
	}
	writeJSON(w, http.StatusOK, wk)
}

// DELETE /api/admin/works/{id}: removes the work and any audio uploaded for it.
func (s *Server) adminDeleteWork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	wk, err := s.store.GetWork(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if s.r2 != nil {
		if err := s.r2.DeletePrefix(r.Context(), wk.AudioFolder()); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if err := s.store.DeleteWork(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Audio the browser may upload: formats every browser plays, so no encoding step is needed.
var audioTypes = map[string]string{"audio/mpeg": "mp3", "audio/mp3": "mp3", "audio/mp4": "m4a", "audio/x-m4a": "m4a", "audio/aac": "aac"}

// POST /api/admin/works/{id}/uploads {contentType}: a presigned URL for one audio file. The
// browser PUTs the file, then adds its key to the track list with PUT .../tracks.
func (s *Server) adminWorkUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		ContentType string `json:"contentType"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	ext, ok := audioTypes[in.ContentType]
	if !ok {
		writeErr(w, http.StatusBadRequest, "upload .mp3 or .m4a files")
		return
	}
	wk, err := s.store.GetWork(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if wk.License == "external" {
		writeErr(w, http.StatusConflict, "this work links to official platforms and can't host audio")
		return
	}
	key := fmt.Sprintf("%su%s.%s", wk.AudioFolder(), randomID(), ext)
	url, err := s.r2.PresignPut(r.Context(), key, in.ContentType, 2*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "uploadUrl": url})
}

// uploadNameRe matches the file name adminWorkUpload gives an upload inside the work's folder.
var uploadNameRe = regexp.MustCompile(`^u[0-9a-f]{16}\.(mp3|m4a|aac)$`)

// isUpload reports whether key is a file uploaded for this work (not an outside URL or other key).
func isUpload(wk *store.Work, key string) bool {
	rest, ok := strings.CutPrefix(key, wk.AudioFolder())
	return ok && uploadNameRe.MatchString(rest)
}

// PUT /api/admin/works/{id}/tracks [{title, durationSec, audio}]: replaces the track list in
// the given order. Audio must be a track the work already has, or a file uploaded for it.
func (s *Server) adminSetTracks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in []struct {
		Title       string `json:"title"`
		DurationSec int    `json:"durationSec"`
		Audio       string `json:"audio"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if len(in) > 2000 {
		writeErr(w, http.StatusBadRequest, "too many tracks")
		return
	}
	ctx := r.Context()
	wk, err := s.store.GetWork(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	old, err := s.store.Tracks(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	existing := map[string]bool{}
	for _, t := range old {
		existing[t.Audio] = true
	}

	tracks := make([]store.Track, 0, len(in))
	kept := map[string]bool{}
	for i, t := range in {
		title := strings.TrimSpace(t.Title)
		if title == "" || utf8.RuneCountInString(title) > 300 {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("track %d needs a title (up to 300 characters)", i+1))
			return
		}
		if !existing[t.Audio] {
			if !isUpload(wk, t.Audio) || wk.License == "external" {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("track %d: unknown audio file", i+1))
				return
			}
			if s.r2 == nil {
				writeErr(w, http.StatusServiceUnavailable, "storage is not configured")
				return
			}
			found, err := s.r2.Exists(ctx, t.Audio)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if !found {
				writeErr(w, http.StatusConflict, fmt.Sprintf("track %d: the upload hasn't finished", i+1))
				return
			}
		}
		kept[t.Audio] = true
		tracks = append(tracks, store.Track{Position: i + 1, Title: title, DurationSec: max(0, t.DurationSec), Audio: t.Audio})
	}
	if err := s.store.ReplaceTracks(ctx, id, tracks); err != nil {
		s.fail(w, r, err)
		return
	}
	// Free the storage of uploaded files that were removed from the list.
	for _, t := range old {
		if !kept[t.Audio] && isUpload(wk, t.Audio) && s.r2 != nil {
			if err := s.r2.Delete(ctx, t.Audio); err != nil {
				s.log.Warn("delete removed track", "key", t.Audio, "err", err)
			}
		}
	}
	s.writeAdminWork(w, r, id)
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
