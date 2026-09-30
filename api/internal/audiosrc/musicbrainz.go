package audiosrc

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tony/lumen/api/internal/store"
)

// platforms names the official listening links worth showing, keyed by host.
var platforms = []struct{ host, name string }{
	{"open.spotify.com", "Spotify"},
	{"music.apple.com", "Apple Music"},
	{"youtube.com", "YouTube channel"},
	{"soundcloud.com", "SoundCloud"},
	{"deezer.com", "Deezer"},
	{"tidal.com", "Tidal"},
}

func platformName(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(p.Host, "www.")
	for _, pl := range platforms {
		if host == pl.host {
			return pl.name
		}
	}
	return ""
}

// MusicBrainzArtist lists an artist's releases (from musicbrainz.org/artist/{mbid}) as works Lumen
// doesn't host: cover, title and year, with a YouTube search for the release plus the artist's
// official streaming pages. types is any of "album", "ep", "single".
func (c *Client) MusicBrainzArtist(ctx context.Context, mbid string, types []string) ([]store.Work, error) {
	var artist struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Genres []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"genres"`
		Relations []struct {
			URL struct {
				Resource string `json:"resource"`
			} `json:"url"`
		} `json:"relations"`
	}
	if err := c.getJSON(ctx, c.MusicBrainzURL+"/artist/"+url.PathEscape(mbid)+"?inc=url-rels+genres&fmt=json", &artist); err != nil {
		return nil, err
	}
	var links []store.Link
	seen := map[string]bool{}
	for _, pl := range platforms { // stable order: Spotify first
		for _, r := range artist.Relations {
			if u := r.URL.Resource; platformName(u) == pl.name && !seen[pl.name] {
				seen[pl.name] = true
				links = append(links, store.Link{Name: pl.name, URL: https(u)})
			}
		}
	}
	sort.SliceStable(artist.Genres, func(i, j int) bool { return artist.Genres[i].Count > artist.Genres[j].Count })
	var genres []string
	for _, g := range artist.Genres {
		if len(genres) < 2 {
			genres = append(genres, g.Name)
		}
	}

	c.pause() // MusicBrainz allows one request per second
	var res struct {
		ReleaseGroups []struct {
			ID             string   `json:"id"`
			Title          string   `json:"title"`
			PrimaryType    string   `json:"primary-type"`
			SecondaryTypes []string `json:"secondary-types"`
			FirstRelease   string   `json:"first-release-date"`
		} `json:"release-groups"`
	}
	q := url.Values{"artist": {mbid}, "type": {strings.Join(types, "|")}, "limit": {"100"}, "fmt": {"json"}}
	if err := c.getJSON(ctx, c.MusicBrainzURL+"/release-group?"+q.Encode(), &res); err != nil {
		return nil, err
	}
	// Oldest first, so the newest release gets the highest id and lists first on the site.
	sort.SliceStable(res.ReleaseGroups, func(i, j int) bool { return res.ReleaseGroups[i].FirstRelease < res.ReleaseGroups[j].FirstRelease })

	works := make([]store.Work, 0, len(res.ReleaseGroups))
	for _, rg := range res.ReleaseGroups {
		kind := rg.PrimaryType
		if len(rg.SecondaryTypes) > 0 {
			kind = rg.SecondaryTypes[0] + " " + strings.ToLower(kind) // e.g. "Live album"
		}
		w := store.Work{
			Kind: store.KindAlbum, Source: "musicbrainz", SourceID: rg.ID, Title: rg.Title, Creator: artist.Name,
			Genres: append([]string{kind}, genres...), License: "external",
			RightsNote: "https://musicbrainz.org/release-group/" + rg.ID,
			Links: append([]store.Link{{Name: "YouTube", URL: "https://www.youtube.com/results?" +
				url.Values{"search_query": {artist.Name + " " + rg.Title}}.Encode()}}, links...),
		}
		if len(rg.FirstRelease) >= 4 {
			w.Year, _ = strconv.Atoi(rg.FirstRelease[:4])
		}
		works = append(works, w)
	}
	// Check covers eight at a time; many releases have none, and the site shows a placeholder.
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range works {
		wg.Add(1)
		go func(w *store.Work) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if cover := c.ArchiveCoverURL + "/release-group/" + w.SourceID + "/front-500"; c.exists(ctx, cover) {
				w.CoverURL = cover
			}
		}(&works[i])
	}
	wg.Wait()
	if len(works) == 0 {
		return nil, fmt.Errorf("musicbrainz: %s (%s) has no releases of type %v", artist.Name, mbid, types)
	}
	return works, nil
}

func (c *Client) pause() {
	if c.MusicBrainzURL == defaultMusicBrainzURL {
		time.Sleep(1100 * time.Millisecond)
	}
}

// exists reports whether a URL answers a HEAD request with success or a redirect.
func (c *Client) exists(ctx context.Context, u string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", c.UserAgent)
	noFollow := *c.HTTP
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := noFollow.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode < 400
}
