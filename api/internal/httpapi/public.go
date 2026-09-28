package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
	"github.com/tony/lumen/api/internal/tmdb"
)

// Card is a poster tile. Playable means the film is hosted on this site.
type Card struct {
	TMDBID   int      `json:"tmdbId"`
	Title    string   `json:"title"`
	Year     int      `json:"year"`
	Overview string   `json:"overview"`
	Poster   string   `json:"poster"`
	Backdrop string   `json:"backdrop"`
	Genres   []string `json:"genres"`
	Runtime  int      `json:"runtime,omitempty"`
	Playable bool     `json:"playable"`
}

type Row struct {
	Title   string `json:"title"`
	GenreID int    `json:"genreId,omitempty"`
	Kind    string `json:"kind,omitempty"` // "top" renders as a numbered ranking
	Items   []Card `json:"items"`
}

func (s *Server) genreNames(ctx context.Context) map[int]string {
	m := map[int]string{}
	gs, err := s.tmdb.Genres(ctx)
	if err != nil {
		return m
	}
	for _, g := range gs {
		m[g.ID] = g.Name
	}
	return m
}

func namesOf(ids []int, names map[int]string) []string {
	out := []string{}
	for _, id := range ids {
		if n, ok := names[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

func filmCard(f store.Film, names map[int]string) Card {
	ids := make([]int, len(f.GenreIDs))
	for i, g := range f.GenreIDs {
		ids[i] = int(g)
	}
	return Card{
		TMDBID: f.TMDBID, Title: f.Title, Year: f.Year, Overview: f.Overview,
		Poster: tmdb.Image("w500", f.PosterPath), Backdrop: tmdb.Image("w1280", f.BackdropPath),
		Genres: namesOf(ids, names), Runtime: f.Runtime, Playable: true,
	}
}

func movieCard(m tmdb.Movie, names map[int]string, playable map[int]bool) Card {
	return Card{
		TMDBID: m.ID, Title: m.Title, Year: m.Year(), Overview: m.Overview,
		Poster: tmdb.Image("w500", m.PosterPath), Backdrop: tmdb.Image("w1280", m.BackdropPath),
		Genres: namesOf(m.GenreIDs, names), Playable: playable[m.ID],
	}
}

func (s *Server) playableSet(ctx context.Context) (map[int]bool, []store.Film, error) {
	films, err := s.store.Published(ctx, 0, 500, 0)
	if err != nil {
		return nil, nil, err
	}
	set := make(map[int]bool, len(films))
	for _, f := range films {
		set[f.TMDBID] = true
	}
	return set, films, nil
}

// GET /api/home: featured film, newest films, genre rows from the hosted catalog,
// plus a "trending worldwide" row from TMDB (those link to legal where-to-watch info).
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	names := s.genreNames(ctx)
	playable, films, err := s.playableSet(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	resp := struct {
		Featured *Card `json:"featured"`
		Rows     []Row `json:"rows"`
	}{Rows: []Row{}}

	if len(films) > 0 {
		c := filmCard(films[0], names) // Published() sorts featured first
		resp.Featured = &c

		newest := make([]store.Film, len(films))
		copy(newest, films)
		sort.SliceStable(newest, func(i, j int) bool { return newest[i].CreatedAt.After(newest[j].CreatedAt) })
		row := Row{Title: "Newly added"}
		for i, f := range newest {
			if i == 12 {
				break
			}
			row.Items = append(row.Items, filmCard(f, names))
		}
		resp.Rows = append(resp.Rows, row)

		// Most played today; hidden until something has been played.
		if top, err := s.store.TopToday(ctx, 5); err != nil {
			s.log.Warn("top today", "err", err)
		} else if len(top) > 0 {
			row := Row{Title: "Top 5 on Lumen today", Kind: "top"}
			for _, f := range top {
				row.Items = append(row.Items, filmCard(f, names))
			}
			resp.Rows = append(resp.Rows, row)
		}

		// Genre rows: the genres with the most hosted films (at least 3), top 4.
		byGenre := map[int][]store.Film{}
		for _, f := range films {
			for _, g := range f.GenreIDs {
				byGenre[int(g)] = append(byGenre[int(g)], f)
			}
		}
		type gc struct{ id, n int }
		var counts []gc
		for id, fs := range byGenre {
			if len(fs) >= 3 && names[id] != "" {
				counts = append(counts, gc{id, len(fs)})
			}
		}
		sort.Slice(counts, func(i, j int) bool { return counts[i].n > counts[j].n })
		for i, c := range counts {
			if i == 4 {
				break
			}
			row := Row{Title: names[c.id], GenreID: c.id}
			for j, f := range byGenre[c.id] {
				if j == 12 {
					break
				}
				row.Items = append(row.Items, filmCard(f, names))
			}
			resp.Rows = append(resp.Rows, row)
		}
	}

	if trending, err := s.tmdb.Trending(ctx); err == nil && len(trending) > 0 {
		row := Row{Title: "Trending worldwide"}
		for i, m := range trending {
			if i == 12 {
				break
			}
			row.Items = append(row.Items, movieCard(m, names, playable))
		}
		resp.Rows = append(resp.Rows, row)
	} else if err != nil {
		s.log.Warn("tmdb trending", "err", err)
	}

	cacheFor(w, 60)
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/genres
func (s *Server) genres(w http.ResponseWriter, r *http.Request) {
	gs, err := s.tmdb.Genres(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cacheFor(w, 86400)
	writeJSON(w, http.StatusOK, gs)
}

type person struct {
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Photo     string `json:"photo"`
}

type provider struct {
	Name string `json:"name"`
	Logo string `json:"logo"`
}

type subtitle struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

var langLabels = map[string]string{"vi": "Tiếng Việt", "en": "English", "fr": "Français", "de": "Deutsch", "ja": "日本語", "ko": "한국어", "zh": "中文"}

// POST /api/movies/{id}/play: counts a play for the Top 5 row. Each client IP counts
// at most once per film every few hours, so reloading the player doesn't inflate it.
func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if !s.sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	key := clientIP(r) + "|" + strconv.Itoa(id)
	if !s.plays.Allowed(key) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.store.RecordPlay(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.plays.Fail(key)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/movies/{id}: TMDB details plus playback info when the film is hosted here.
func (s *Server) movie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	d, err := s.tmdb.Movie(ctx, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := s.genreNames(ctx)
	playable, _, err := s.playableSet(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	type playback struct {
		HLS       string     `json:"hls"`
		Subtitles []subtitle `json:"subtitles"`
	}
	resp := struct {
		Card
		Director  string    `json:"director"`
		Cast      []person  `json:"cast"`
		Trailer   string    `json:"trailerKey"`
		Similar   []Card    `json:"similar"`
		Providers any       `json:"providers"`
		Playback  *playback `json:"playback"`
		Source    string    `json:"source,omitempty"`
	}{
		Card:    movieCard(d.Movie, names, playable),
		Cast:    []person{},
		Similar: []Card{},
	}
	resp.Runtime = d.Runtime
	resp.Genres = []string{}
	for _, g := range d.Genres {
		resp.Genres = append(resp.Genres, g.Name)
	}
	for _, c := range d.Credits.Crew {
		if c.Job == "Director" {
			resp.Director = c.Name
			break
		}
	}
	for i, c := range d.Credits.Cast {
		if i == 10 {
			break
		}
		resp.Cast = append(resp.Cast, person{Name: c.Name, Character: c.Character, Photo: tmdb.Image("w185", c.ProfilePath)})
	}
	for _, v := range d.Videos.Results {
		if v.Site == "YouTube" && v.Type == "Trailer" {
			resp.Trailer = v.Key
			if v.Official {
				break
			}
		}
	}
	for i, m := range d.Similar.Results {
		if i == 12 {
			break
		}
		resp.Similar = append(resp.Similar, movieCard(m, names, playable))
	}
	// Legal "where to watch" for the configured region (data from JustWatch via TMDB).
	if cp, ok := d.WatchProviders.Results[s.tmdb.Region()]; ok {
		conv := func(ps []tmdb.Provider) []provider {
			out := []provider{}
			for _, p := range ps {
				out = append(out, provider{Name: p.Name, Logo: tmdb.Image("w92", p.LogoPath)})
			}
			return out
		}
		resp.Providers = map[string]any{
			"link": cp.Link, "stream": conv(append(append(cp.Flatrate, cp.Free...), cp.Ads...)),
			"rent": conv(cp.Rent), "buy": conv(cp.Buy),
		}
	}

	if f, err := s.store.PublishedByTMDB(ctx, id); err == nil {
		pb := &playback{HLS: s.cfg.MediaBaseURL + "/" + storage.MasterKey(f.ID), Subtitles: []subtitle{}}
		for _, lang := range f.Subtitles {
			label := langLabels[lang]
			if label == "" {
				label = strings.ToUpper(lang)
			}
			pb.Subtitles = append(pb.Subtitles, subtitle{Lang: lang, Label: label, URL: s.cfg.MediaBaseURL + "/" + storage.SubtitleKey(f.ID, lang)})
		}
		resp.Playback = pb
		resp.Playable = true
		if f.Rights == "public_domain" {
			resp.Source = "Public domain"
		} else {
			resp.Source = "Published with the rights holder's permission"
		}
	}

	cacheFor(w, 300)
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/browse?genre=ID&page=: one page of hosted films, optionally by genre.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	genre, _ := strconv.Atoi(r.URL.Query().Get("genre"))
	page := pageParam(r, 0)
	films, err := s.store.Published(ctx, genre, browsePageSize, (page-1)*browsePageSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	total, err := s.store.CountPublished(ctx, genre)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := s.genreNames(ctx)
	items := []Card{}
	for _, f := range films {
		items = append(items, filmCard(f, names))
	}
	cacheFor(w, 60)
	writeJSON(w, http.StatusOK, Page[Card]{Items: items, Page: page, TotalPages: pages(total, browsePageSize), Total: total})
}

// GET /api/search?q=&page=: one page of TMDB search; within the page, films hosted here
// come first and are marked playable.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := pageParam(r, tmdb.MaxSearchPage)
	if len(q) < 2 || len(q) > 100 {
		writeJSON(w, http.StatusOK, Page[Card]{Items: []Card{}, Page: page})
		return
	}
	res, err := s.tmdb.Search(ctx, q, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := s.genreNames(ctx)
	playable, _, err := s.playableSet(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := []Card{}
	for _, m := range res.Results {
		items = append(items, movieCard(m, names, playable))
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Playable && !items[j].Playable })
	cacheFor(w, 300)
	writeJSON(w, http.StatusOK, Page[Card]{Items: items, Page: page,
		TotalPages: min(res.TotalPages, tmdb.MaxSearchPage), Total: res.TotalResults})
}
