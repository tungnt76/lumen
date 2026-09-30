package audiosrc

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tony/lumen/api/internal/store"
)

type librivoxBook struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Language      string `json:"language"`
	CopyrightYear string `json:"copyright_year"`
	URLLibriVox   string `json:"url_librivox"`
	URLIArchive   string `json:"url_iarchive"`
	Authors       []struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	} `json:"authors"`
	Genres []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Sections []struct {
		SectionNumber string `json:"section_number"`
		Title         string `json:"title"`
		ListenURL     string `json:"listen_url"`
		Playtime      string `json:"playtime"`
		Readers       []struct {
			DisplayName string `json:"display_name"`
		} `json:"readers"`
	} `json:"sections"`
}

// LibriVox fetches a LibriVox audiobook by its numeric id (the number in librivox.org/api
// or on the book's RSS link). LibriVox recordings are public domain, and the MP3s stay on
// archive.org, so importing uses no storage.
func (c *Client) LibriVox(ctx context.Context, id int) (store.Work, []store.Track, error) {
	var res struct {
		Books []librivoxBook `json:"books"`
	}
	u := c.LibriVoxURL + "/?" + url.Values{"id": {strconv.Itoa(id)}, "extended": {"1"}, "format": {"json"}}.Encode()
	if err := c.getJSON(ctx, u, &res); err != nil {
		return store.Work{}, nil, err
	}
	if len(res.Books) == 0 {
		return store.Work{}, nil, fmt.Errorf("librivox: no book with id %d", id)
	}
	b := res.Books[0]

	var authors []string
	for _, a := range b.Authors {
		authors = append(authors, strings.TrimSpace(a.FirstName+" "+a.LastName))
	}
	genres := []string{}
	for _, g := range b.Genres {
		genres = append(genres, g.Name)
	}
	w := store.Work{
		Kind: store.KindBook, Source: "librivox", SourceID: b.ID, Title: plain(b.Title),
		Creator: strings.Join(authors, ", "), Language: languageCode(b.Language),
		Description: plain(b.Description), Genres: genres,
		License: "public_domain", RightsNote: https(b.URLLibriVox),
	}
	w.Year, _ = strconv.Atoi(b.CopyrightYear)
	if ia := strings.TrimRight(b.URLIArchive, "/"); ia != "" {
		w.CoverURL = c.ArchiveURL + "/services/img/" + ia[strings.LastIndex(ia, "/")+1:]
	}

	readers := map[string]bool{}
	var readerNames []string
	tracks := make([]store.Track, 0, len(b.Sections))
	for i, s := range b.Sections {
		if s.ListenURL == "" {
			continue
		}
		pos, err := strconv.Atoi(s.SectionNumber)
		if err != nil || pos <= 0 {
			pos = i + 1
		}
		tracks = append(tracks, store.Track{Position: pos, Title: plain(s.Title), DurationSec: seconds(s.Playtime), Audio: https(s.ListenURL)})
		for _, r := range s.Readers {
			if !readers[r.DisplayName] {
				readers[r.DisplayName] = true
				readerNames = append(readerNames, r.DisplayName)
			}
		}
	}
	if len(tracks) == 0 {
		return store.Work{}, nil, fmt.Errorf("librivox: book %d has no audio sections", id)
	}
	switch {
	case len(readerNames) <= 3:
		w.Narrator = strings.Join(readerNames, ", ")
	default:
		w.Narrator = "LibriVox volunteers"
	}
	return w, tracks, nil
}
