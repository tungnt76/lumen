// Package tmdb is a minimal, cached client for the TMDB v3 API.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tony/lumen/api/internal/cache"
)

const imageURL = "https://image.tmdb.org/t/p/"

// Image builds a full image URL, or "" when path is empty. Sizes: w342, w500, w780, w1280, original.
func Image(size, path string) string {
	if path == "" {
		return ""
	}
	return imageURL + size + path
}

type Client struct {
	base   string
	token  string
	lang   string
	region string
	http   *http.Client
	cache  *cache.Cache
}

func New(base, token, lang, region string) *Client {
	return &Client{
		base:   strings.TrimRight(base, "/"),
		token:  token,
		lang:   lang,
		region: region,
		http:   &http.Client{Timeout: 10 * time.Second},
		cache:  cache.New(5000),
	}
}

func (c *Client) Region() string { return c.region }

func (c *Client) get(ctx context.Context, path string, q url.Values, ttl time.Duration, out any) error {
	if q == nil {
		q = url.Values{}
	}
	if q.Get("language") == "" {
		q.Set("language", c.lang)
	}
	u := c.base + path + "?" + q.Encode()
	body, err := c.cache.GetOrLoad(u, ttl, func() ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("tmdb %s: status %d", path, res.StatusCode)
		}
		return b, nil
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

var ErrNotFound = fmt.Errorf("tmdb: not found")

type Movie struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	ReleaseDate  string  `json:"release_date"`
	GenreIDs     []int   `json:"genre_ids"`
	VoteAverage  float64 `json:"vote_average"`
}

// Year returns the release year or 0.
func (m Movie) Year() int {
	if len(m.ReleaseDate) >= 4 {
		y, _ := strconv.Atoi(m.ReleaseDate[:4])
		return y
	}
	return 0
}

// Released returns the release date, or nil when TMDB has none.
func (m Movie) Released() *time.Time {
	t, err := time.Parse("2006-01-02", m.ReleaseDate)
	if err != nil {
		return nil
	}
	return &t
}

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Provider struct {
	Name     string `json:"provider_name"`
	LogoPath string `json:"logo_path"`
}

type CountryProviders struct {
	Link     string     `json:"link"`
	Flatrate []Provider `json:"flatrate"`
	Free     []Provider `json:"free"`
	Ads      []Provider `json:"ads"`
	Rent     []Provider `json:"rent"`
	Buy      []Provider `json:"buy"`
}

type MovieDetail struct {
	Movie
	Runtime int     `json:"runtime"`
	Genres  []Genre `json:"genres"`
	Credits struct {
		Cast []struct {
			Name        string `json:"name"`
			Character   string `json:"character"`
			ProfilePath string `json:"profile_path"`
		} `json:"cast"`
		Crew []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	} `json:"credits"`
	Videos struct {
		Results []struct {
			Key      string `json:"key"`
			Site     string `json:"site"`
			Type     string `json:"type"`
			Official bool   `json:"official"`
		} `json:"results"`
	} `json:"videos"`
	Similar struct {
		Results []Movie `json:"results"`
	} `json:"similar"`
	WatchProviders struct {
		Results map[string]CountryProviders `json:"results"`
	} `json:"watch/providers"`
}

type page struct {
	Results []Movie `json:"results"`
}

// SearchPage is one page of TMDB search results. TMDB serves at most MaxSearchPage pages.
type SearchPage struct {
	Results      []Movie `json:"results"`
	Page         int     `json:"page"`
	TotalPages   int     `json:"total_pages"`
	TotalResults int     `json:"total_results"`
}

const MaxSearchPage = 500

// Trending returns this week's trending movies.
func (c *Client) Trending(ctx context.Context) ([]Movie, error) {
	var p page
	err := c.get(ctx, "/trending/movie/week", nil, time.Hour, &p)
	return p.Results, err
}

// Search searches movies by title.
func (c *Client) Search(ctx context.Context, query string, pageNum int) (*SearchPage, error) {
	q := url.Values{"query": {query}, "page": {strconv.Itoa(pageNum)}, "include_adult": {"false"}}
	var p SearchPage
	err := c.get(ctx, "/search/movie", q, 30*time.Minute, &p)
	return &p, err
}

// Discover lists movies matching TMDB discover filters, e.g. with_genres and
// primary_release_date.gte. Adult titles are always excluded.
func (c *Client) Discover(ctx context.Context, q url.Values) ([]Movie, error) {
	q.Set("include_adult", "false")
	var p page
	err := c.get(ctx, "/discover/movie", q, time.Hour, &p)
	return p.Results, err
}

// Genres returns the movie genre list.
func (c *Client) Genres(ctx context.Context) ([]Genre, error) {
	var r struct {
		Genres []Genre `json:"genres"`
	}
	err := c.get(ctx, "/genre/movie/list", nil, 7*24*time.Hour, &r)
	return r.Genres, err
}

// Movie returns full details with credits, videos, similar titles and watch providers
// in a single request. The overview falls back to English when no translation exists.
func (c *Client) Movie(ctx context.Context, id int) (*MovieDetail, error) {
	q := url.Values{
		"append_to_response":     {"credits,videos,similar,watch/providers"},
		"include_video_language": {"vi,en,null"},
	}
	var d MovieDetail
	if err := c.get(ctx, "/movie/"+strconv.Itoa(id), q, 24*time.Hour, &d); err != nil {
		return nil, err
	}
	if d.Overview == "" && c.lang != "en-US" {
		var en Movie
		if err := c.get(ctx, "/movie/"+strconv.Itoa(id), url.Values{"language": {"en-US"}}, 24*time.Hour, &en); err == nil {
			d.Overview = en.Overview
		}
	}
	return &d, nil
}
