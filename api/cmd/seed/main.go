// Command seed fills the catalog with recent demo films so the home page
// (hero, rows, Top 5) can be checked against the design without uploading and
// encoding real files. It also tops up every TMDB genre with recent popular films
// (-per-genre since -since). Seeded films have no video, so their player won't start.
//
//	DATABASE_URL=... TMDB_TOKEN=... go run ./cmd/seed          # add the demo films
//	DATABASE_URL=... TMDB_TOKEN=... go run ./cmd/seed -remove  # delete them again
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tony/lumen/api/internal/store"
	"github.com/tony/lumen/api/internal/tmdb"
)

// seedNote marks seeded rows so -remove never touches real uploads.
const seedNote = "seed: demo entry, no video uploaded"

type seedFilm struct {
	title  string
	year   int
	plays  int    // today's plays, drives the Top 5 order
	rights string // empty means public_domain
}

// Recent films as placeholders (the first one is featured). They are NOT public domain:
// they are stored as "licensed" only so the layout can be previewed, and must be
// removed with -remove before going live.
var films = []seedFilm{
	// Sci-fi / action
	{"Dune: Part Two", 2024, 20, "licensed"},
	{"Oppenheimer", 2023, 16, "licensed"},
	{"Top Gun: Maverick", 2022, 8, "licensed"},
	{"The Batman", 2022, 0, "licensed"},
	{"Civil War", 2024, 0, "licensed"},
	{"Mission: Impossible - The Final Reckoning", 2025, 0, "licensed"},
	// Animation / family
	{"Spider-Man: Across the Spider-Verse", 2023, 12, "licensed"},
	{"Inside Out 2", 2024, 0, "licensed"},
	{"The Wild Robot", 2024, 0, "licensed"},
	{"Flow", 2024, 0, "licensed"},
	// Horror
	{"Sinners", 2025, 10, "licensed"},
	{"The Substance", 2024, 0, "licensed"},
	{"Nosferatu", 2024, 0, "licensed"},
	{"Talk to Me", 2023, 0, "licensed"},
	{"Barbarian", 2022, 0, "licensed"},
	// Comedy / drama
	{"Everything Everywhere All at Once", 2022, 0, "licensed"},
	{"Barbie", 2023, 0, "licensed"},
	{"Poor Things", 2023, 0, "licensed"},
	{"Anora", 2024, 0, "licensed"},
	{"Glass Onion: A Knives Out Mystery", 2022, 0, "licensed"},
	{"Past Lives", 2023, 0, "licensed"},
	{"Conclave", 2024, 0, "licensed"},
}

func main() {
	remove := flag.Bool("remove", false, "delete the seeded films instead of adding them")
	since := flag.Int("since", 2020, "genre fill: earliest release year")
	perGenre := flag.Int("per-genre", 5, "genre fill: minimum films per genre since that year")
	flag.Parse()
	dbURL, token := os.Getenv("DATABASE_URL"), os.Getenv("TMDB_TOKEN")
	if dbURL == "" || (token == "" && !*remove) {
		fmt.Fprintln(os.Stderr, "usage: DATABASE_URL=... TMDB_TOKEN=... go run ./cmd/seed [-remove]")
		os.Exit(2)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer st.Close()

	if *remove {
		if err := removeSeeded(ctx, st); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	tc := tmdb.New(env("TMDB_BASE_URL", "https://api.themoviedb.org/3"), token, env("TMDB_LANGUAGE", "vi-VN"), env("TMDB_REGION", "VN"))
	added := 0
	for i, s := range films {
		f, err := seedOne(ctx, st, tc, s, i == 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s (%d): %v\n", s.title, s.year, err)
			continue
		}
		for n := 0; n < s.plays; n++ {
			if err := st.RecordPlay(ctx, f.TMDBID); err != nil {
				fmt.Fprintln(os.Stderr, "plays:", err)
				break
			}
		}
		added++
		fmt.Printf("added %s (%d) tmdb=%d plays=%d\n", f.Title, f.Year, f.TMDBID, s.plays)
	}
	added += fillGenres(ctx, st, tc, *since, *perGenre)
	backfillReleaseDates(ctx, st, tc)
	fmt.Printf("\n%d films seeded. Remove them with: make seed-remove\n", added)
}

// fillGenres tops up every TMDB genre until the catalog has at least min published
// films of that genre released in or after the since year, taking TMDB's most
// popular titles first. Like the recent films above, these are placeholders.
func fillGenres(ctx context.Context, st *store.Store, tc *tmdb.Client, since, min int) int {
	genres, err := tc.Genres(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "genres:", err)
		return 0
	}
	added := 0
	for _, g := range genres {
		have, err := countSince(ctx, st, g.ID, since)
		if err != nil {
			fmt.Fprintln(os.Stderr, "count:", err)
			return added
		}
		// Well-known titles first; fall back to fewer votes for small genres like TV Movie.
	pages:
		for _, votes := range []string{"300", "20"} {
			for page := 1; page <= 3 && have < min; page++ {
				results, err := tc.Discover(ctx, url.Values{
					"with_genres":              {strconv.Itoa(g.ID)},
					"primary_release_date.gte": {fmt.Sprintf("%d-01-01", since)},
					"primary_release_date.lte": {time.Now().Format("2006-01-02")},
					"vote_count.gte":           {votes},
					"sort_by":                  {"popularity.desc"},
					"page":                     {strconv.Itoa(page)},
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "discover %s: %v\n", g.Name, err)
					break pages
				}
				for _, m := range results {
					if have >= min {
						break pages
					}
					f, err := addByID(ctx, st, tc, m.ID, "licensed", false)
					if err != nil {
						continue // already seeded, or TMDB lookup failed
					}
					have++
					added++
					fmt.Printf("added [%s] %s (%d) tmdb=%d\n", g.Name, f.Title, f.Year, f.TMDBID)
				}
				if len(results) == 0 {
					break
				}
			}
		}
		if have < min {
			fmt.Fprintf(os.Stderr, "genre %s: only %d films since %d on TMDB\n", g.Name, have, since)
		} else {
			fmt.Printf("genre %s: %d films since %d\n", g.Name, have, since)
		}
	}
	return added
}

// backfillReleaseDates fills release_date from TMDB for films stored before the
// column existed, including real uploads, so every film sorts by release date.
func backfillReleaseDates(ctx context.Context, st *store.Store, tc *tmdb.Client) {
	fs, err := st.MissingReleaseDate(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "release dates:", err)
		return
	}
	n := 0
	for _, f := range fs {
		d, err := tc.Movie(ctx, f.TMDBID)
		if err != nil {
			continue
		}
		if t := d.Released(); t != nil && st.SetReleaseDate(ctx, f.ID, *t) == nil {
			n++
		}
	}
	if len(fs) > 0 {
		fmt.Printf("release dates filled for %d of %d films\n", n, len(fs))
	}
}

// countSince counts published films of a genre released in or after year.
func countSince(ctx context.Context, st *store.Store, genreID, year int) (int, error) {
	fs, err := st.Published(ctx, genreID, 1000, 0)
	n := 0
	for _, f := range fs {
		if f.Year >= year {
			n++
		}
	}
	return n, err
}

// seedOne finds the film on TMDB and stores it as encoded and published.
func seedOne(ctx context.Context, st *store.Store, tc *tmdb.Client, s seedFilm, featured bool) (*store.Film, error) {
	year, rights := s.year, s.rights
	if rights == "" {
		rights = "public_domain"
	}
	res, err := tc.Search(ctx, s.title, 1)
	if err != nil {
		return nil, err
	}
	id := 0
	for _, m := range res.Results {
		if m.Year() == year {
			id = m.ID
			break
		}
	}
	if id == 0 {
		return nil, fmt.Errorf("not found on TMDB")
	}
	return addByID(ctx, st, tc, id, rights, featured)
}

// addByID stores a TMDB film as encoded and published.
func addByID(ctx context.Context, st *store.Store, tc *tmdb.Client, id int, rights string, featured bool) (*store.Film, error) {
	d, err := tc.Movie(ctx, id)
	if err != nil {
		return nil, err
	}
	gids := make([]int32, 0, len(d.Genres))
	for _, g := range d.Genres {
		gids = append(gids, int32(g.ID))
	}
	f, err := st.CreateFilm(ctx, store.Film{
		TMDBID: d.ID, Title: d.Title, Year: d.Year(), Overview: d.Overview, PosterPath: d.PosterPath,
		BackdropPath: d.BackdropPath, GenreIDs: gids, Runtime: d.Runtime, Rights: rights, RightsNote: seedNote,
		ReleaseDate: d.Released(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("already in the library")
		}
		return nil, err
	}
	if err := st.FinishJob(ctx, f.ID, []string{}); err != nil {
		return nil, err
	}
	yes := true
	return st.SetFlags(ctx, f.ID, &yes, &featured)
}

func removeSeeded(ctx context.Context, st *store.Store) error {
	all, err := st.AllFilms(ctx)
	if err != nil {
		return err
	}
	n := 0
	for _, f := range all {
		if f.RightsNote != seedNote {
			continue
		}
		if err := st.DeleteFilm(ctx, f.ID); err != nil {
			return err
		}
		n++
	}
	fmt.Printf("removed %d seeded films\n", n)
	return nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
