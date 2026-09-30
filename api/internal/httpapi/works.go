package httpapi

import (
	"net/http"
	"strings"

	"github.com/tony/lumen/api/internal/store"
)

// WorkCard is a cover tile for an audiobook or album. Playable means it can be played on this
// site; otherwise it links out to official platforms.
type WorkCard struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Creator     string   `json:"creator"`
	Narrator    string   `json:"narrator,omitempty"`
	Language    string   `json:"language"`
	Year        int      `json:"year,omitempty"`
	Cover       string   `json:"cover"`
	Genres      []string `json:"genres"`
	TrackCount  int      `json:"trackCount"`
	DurationSec int      `json:"durationSec"`
	AIVoice     bool     `json:"aiVoice"`
	Playable    bool     `json:"playable"`
	Featured    bool     `json:"featured"`
}

func workCard(w store.Work) WorkCard {
	return WorkCard{
		ID: w.ID, Kind: w.Kind, Title: w.Title, Creator: w.Creator, Narrator: w.Narrator,
		Language: w.Language, Year: w.Year, Cover: w.CoverURL, Genres: w.Genres,
		TrackCount: w.TrackCount, DurationSec: w.DurationSec, AIVoice: w.AIVoice, Playable: w.TrackCount > 0,
		Featured: w.Featured,
	}
}

var licenseLabels = map[string]string{
	"public_domain": "Public domain",
	"cc0":           "CC0",
	"cc_by":         "CC BY",
	"cc_by_sa":      "CC BY-SA",
	"cc_by_nd":      "CC BY-ND",
	"cc_by_nc":      "CC BY-NC",
	"cc_by_nc_sa":   "CC BY-NC-SA",
	"cc_by_nc_nd":   "CC BY-NC-ND",
	"licensed":      "Published with the rights holder's permission",
	"own":           "Published by Lumen",
	"external":      "Not hosted on Lumen",
}

// audioURL resolves a track's audio: absolute URLs are used as-is, anything else is an R2 key.
func (s *Server) audioURL(audio string) string {
	if strings.HasPrefix(audio, "https://") || strings.HasPrefix(audio, "http://") {
		return audio
	}
	return s.cfg.MediaBaseURL + "/" + audio
}

// works serves GET /api/books and GET /api/music: ?q=&lang=&page=, one page of published works.
func (s *Server) works(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(q) > 100 {
			q = q[:100]
		}
		f := store.WorkFilter{Kind: kind, Language: r.URL.Query().Get("lang"), Query: q}
		page := pageParam(r, 0)
		list, err := s.store.PublishedWorks(r.Context(), f, browsePageSize, (page-1)*browsePageSize)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		total, err := s.store.CountPublishedWorks(r.Context(), f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		items := make([]WorkCard, 0, len(list))
		for _, wk := range list {
			items = append(items, workCard(wk))
		}
		cacheFor(w, 60)
		writeJSON(w, http.StatusOK, Page[WorkCard]{Items: items, Page: page, TotalPages: pages(total, browsePageSize), Total: total})
	}
}

// GET /api/works/{id}: a published work with its playable track list.
func (s *Server) work(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	wk, err := s.store.PublishedWork(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	tracks, err := s.store.Tracks(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type track struct {
		Position    int    `json:"position"`
		Title       string `json:"title"`
		DurationSec int    `json:"durationSec"`
		URL         string `json:"url"`
	}
	resp := struct {
		WorkCard
		Description string       `json:"description"`
		License     string       `json:"license"`
		SourceURL   string       `json:"sourceUrl"`
		Links       []store.Link `json:"links"`
		Tracks      []track      `json:"tracks"`
	}{
		WorkCard:    workCard(*wk),
		Description: wk.Description,
		License:     licenseLabels[wk.License],
		Links:       wk.Links,
		Tracks:      make([]track, 0, len(tracks)),
	}
	if strings.HasPrefix(wk.RightsNote, "https://") {
		resp.SourceURL = wk.RightsNote
	}
	for _, t := range tracks {
		resp.Tracks = append(resp.Tracks, track{Position: t.Position, Title: t.Title, DurationSec: t.DurationSec, URL: s.audioURL(t.Audio)})
	}
	cacheFor(w, 300)
	writeJSON(w, http.StatusOK, resp)
}
