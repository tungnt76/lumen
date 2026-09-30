package audiosrc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/tony/lumen/api/internal/store"
)

// LicenseFromURL maps a Creative Commons licence URL to a catalog licence, or "" if it isn't
// one the site can host (for example "all rights reserved" or no licence at all).
func LicenseFromURL(u string) string {
	u = strings.ToLower(u)
	switch {
	case u == "":
		return ""
	case strings.Contains(u, "/publicdomain/zero"):
		return "cc0"
	case strings.Contains(u, "/publicdomain/"), strings.Contains(u, "/licenses/publicdomain"):
		return "public_domain"
	}
	for _, l := range []struct{ path, license string }{ // most specific first
		{"/licenses/by-nc-nd/", "cc_by_nc_nd"}, {"/licenses/by-nc-sa/", "cc_by_nc_sa"}, {"/licenses/by-nc/", "cc_by_nc"},
		{"/licenses/by-nd/", "cc_by_nd"}, {"/licenses/by-sa/", "cc_by_sa"}, {"/licenses/by/", "cc_by"},
	} {
		if strings.Contains(u, l.path) {
			return l.license
		}
	}
	return ""
}

// multi decodes an Internet Archive metadata field that may be a string or a list of strings.
type multi []string

func (m *multi) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*m = multi{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*m = many
	return nil
}

func (m multi) first() string {
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

type archiveFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Title  string `json:"title"`
	Track  string `json:"track"`
	Length string `json:"length"`
}

// Audio formats in order of preference. Books prefer the small 64 kbps derivative;
// music prefers the original quality.
var (
	speechFormats = []string{"64Kbps MP3", "VBR MP3", "128Kbps MP3", "MP3", "Ogg Vorbis"}
	musicFormats  = []string{"VBR MP3", "320Kbps MP3", "256Kbps MP3", "128Kbps MP3", "MP3", "64Kbps MP3", "Ogg Vorbis"}
)

// Archive fetches an Internet Archive audio item (archive.org/details/{identifier}) as a book
// or album. The files stay on archive.org. license overrides the item's licenseurl; without
// either, the import is refused so nothing unlicensed reaches the site.
func (c *Client) Archive(ctx context.Context, identifier, kind, license string) (store.Work, []store.Track, error) {
	var res struct {
		Metadata struct {
			Identifier  multi `json:"identifier"`
			Title       multi `json:"title"`
			Creator     multi `json:"creator"`
			Description multi `json:"description"`
			Date        multi `json:"date"`
			Year        multi `json:"year"`
			Language    multi `json:"language"`
			Subject     multi `json:"subject"`
			LicenseURL  multi `json:"licenseurl"`
			MediaType   multi `json:"mediatype"`
		} `json:"metadata"`
		Files []archiveFile `json:"files"`
	}
	if err := c.getJSON(ctx, c.ArchiveURL+"/metadata/"+url.PathEscape(identifier), &res); err != nil {
		return store.Work{}, nil, err
	}
	m := res.Metadata
	if m.Identifier.first() == "" {
		return store.Work{}, nil, fmt.Errorf("archive: no item %q", identifier)
	}
	if license == "" {
		license = LicenseFromURL(m.LicenseURL.first())
	}
	if license == "" {
		return store.Work{}, nil, fmt.Errorf("archive: %q has no public-domain or Creative Commons licence (licenseurl %q); check the rights and pass -license to override",
			identifier, m.LicenseURL.first())
	}

	genres := []string{}
	for _, s := range m.Subject {
		for _, g := range strings.Split(s, ";") {
			if g = strings.TrimSpace(g); g != "" && len(genres) < 8 {
				genres = append(genres, g)
			}
		}
	}
	w := store.Work{
		Kind: kind, Source: "archive", SourceID: identifier, Title: plain(m.Title.first()),
		Creator: strings.Join(m.Creator, ", "), Language: languageCode(m.Language.first()),
		Description: plain(m.Description.first()), Genres: genres,
		CoverURL: c.ArchiveURL + "/services/img/" + url.PathEscape(identifier),
		License:  license, RightsNote: c.ArchiveURL + "/details/" + url.PathEscape(identifier),
	}
	if y := m.Year.first(); y != "" {
		w.Year, _ = strconv.Atoi(y)
	} else if d := m.Date.first(); len(d) >= 4 {
		w.Year, _ = strconv.Atoi(d[:4])
	}

	formats := musicFormats
	if kind == store.KindBook {
		formats = speechFormats
	}
	files := pickFormat(res.Files, formats)
	if len(files) == 0 {
		return store.Work{}, nil, fmt.Errorf("archive: %q has no MP3 or Ogg audio files", identifier)
	}
	tracks := make([]store.Track, len(files))
	for i, f := range files {
		title := f.Title
		if title == "" {
			title = strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))
		}
		tracks[i] = store.Track{
			Position: i + 1, Title: title, DurationSec: seconds(f.Length),
			Audio: c.ArchiveURL + "/download/" + url.PathEscape(identifier) + "/" + escapePath(f.Name),
		}
	}
	return w, tracks, nil
}

// pickFormat returns the files of the first preferred format the item has, in track order.
func pickFormat(files []archiveFile, formats []string) []archiveFile {
	for _, format := range formats {
		var out []archiveFile
		for _, f := range files {
			if f.Format == format {
				out = append(out, f)
			}
		}
		if len(out) > 0 {
			sort.SliceStable(out, func(i, j int) bool {
				ti, tj := trackNumber(out[i].Track), trackNumber(out[j].Track)
				if ti != tj {
					return ti < tj
				}
				return out[i].Name < out[j].Name
			})
			return out
		}
	}
	return nil
}

// trackNumber parses "3" or "3/12"; files without one sort last.
func trackNumber(s string) int {
	s, _, _ = strings.Cut(s, "/")
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 1 << 30
	}
	return n
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
