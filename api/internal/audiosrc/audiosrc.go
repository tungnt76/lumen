// Package audiosrc fetches free, legal audiobooks, music and texts to import into the catalog:
// LibriVox audiobooks, Internet Archive audio items (e.g. Musopen, netlabels), Wikisource texts
// (for text-to-speech) and MusicBrainz artist discographies (link-out only). None need an API key.
package audiosrc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client talks to the source APIs. The base URLs can be overridden in tests.
type Client struct {
	HTTP            *http.Client
	UserAgent       string        // Wikimedia asks for a descriptive agent with contact info
	LibriVoxURL     string        // default https://librivox.org/api/feed/audiobooks
	ArchiveURL      string        // default https://archive.org
	WikisourceURL   string        // default https://{lang}.wikisource.org/w/api.php; "{lang}" is replaced
	MusicBrainzURL  string        // default https://musicbrainz.org/ws/2
	ArchiveCoverURL string        // Cover Art Archive, default https://coverartarchive.org
	RetryDelay      time.Duration // wait before the first retry, doubled after
}

const defaultMusicBrainzURL = "https://musicbrainz.org/ws/2"

func New(userAgent string) *Client {
	return &Client{
		HTTP:            &http.Client{Timeout: 60 * time.Second},
		UserAgent:       userAgent,
		LibriVoxURL:     "https://librivox.org/api/feed/audiobooks",
		ArchiveURL:      "https://archive.org",
		WikisourceURL:   "https://{lang}.wikisource.org/w/api.php",
		MusicBrainzURL:  defaultMusicBrainzURL,
		ArchiveCoverURL: "https://coverartarchive.org",
		RetryDelay:      2 * time.Second,
	}
}

// getJSON fetches and decodes a JSON document, retrying twice on network errors and 5xx
// responses (the free APIs sit behind CDNs that sometimes fail briefly).
func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * c.RetryDelay):
			}
		}
		var retry bool
		if retry, err = c.getJSONOnce(ctx, u, v); err == nil || !retry {
			return err
		}
	}
	return err
}

func (c *Client) getJSONOnce(ctx context.Context, u string, v any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return ctx.Err() == nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res.StatusCode >= 500, fmt.Errorf("GET %s: %s", u, res.Status)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 32<<20)).Decode(v); err != nil {
		return false, fmt.Errorf("GET %s: %w", u, err)
	}
	return false, nil
}

var (
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
	wsRe    = regexp.MustCompile(`\s+`)
)

// plain turns a short HTML description into plain text.
func plain(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	for _, r := range [][2]string{{"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}, {"&quot;", `"`}, {"&#39;", "'"}, {"&nbsp;", " "}} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

// https upgrades plain-http archive.org links so pages served over https can play them.
func https(u string) string {
	if strings.HasPrefix(u, "http://") {
		return "https://" + u[len("http://"):]
	}
	return u
}

// seconds parses "1764", "1764.5", "29:24" or "1:02:03".
func seconds(s string) int {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":") {
		f, _ := strconv.ParseFloat(s, 64)
		return int(f + 0.5)
	}
	total := 0
	for _, p := range strings.Split(s, ":") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

// languageCodes maps English language names (LibriVox) and ISO 639-2 codes (Internet Archive)
// to the ISO 639-1 codes stored in the catalog.
var languageCodes = map[string]string{
	"english": "en", "eng": "en", "vietnamese": "vi", "vie": "vi", "french": "fr", "fre": "fr", "fra": "fr",
	"german": "de", "ger": "de", "deu": "de", "spanish": "es", "spa": "es", "italian": "it", "ita": "it",
	"portuguese": "pt", "por": "pt", "dutch": "nl", "dut": "nl", "nld": "nl", "russian": "ru", "rus": "ru",
	"chinese": "zh", "chi": "zh", "zho": "zh", "japanese": "ja", "jpn": "ja", "korean": "ko", "kor": "ko",
	"latin": "la", "lat": "la", "greek": "el", "gre": "el", "ell": "el", "polish": "pl", "pol": "pl",
}

func languageCode(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if code, ok := languageCodes[n]; ok {
		return code
	}
	if len(n) == 2 {
		return n
	}
	return ""
}
