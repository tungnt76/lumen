// Package store holds the film catalog and admin accounts in Postgres.
package store

import (
	"context"
	_ "embed"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Store struct{ db *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 5 // free-tier databases allow few connections
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(ctx); err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() { s.db.Close() }

// Film statuses.
const (
	StatusAwaitingUpload = "awaiting_upload"
	StatusQueued         = "queued"
	StatusEncoding       = "encoding"
	StatusReady          = "ready"
	StatusFailed         = "failed"
)

type Film struct {
	ID           int64      `json:"id"`
	TMDBID       int        `json:"tmdbId"`
	Title        string     `json:"title"`
	Year         int        `json:"year"`
	Overview     string     `json:"overview"`
	PosterPath   string     `json:"posterPath"`
	BackdropPath string     `json:"backdropPath"`
	GenreIDs     []int32    `json:"genreIds"`
	ReleaseDate  *time.Time `json:"releaseDate,omitempty"`
	Runtime      int        `json:"runtime"`
	Rights       string     `json:"rights"`
	RightsNote   string     `json:"rightsNote"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	Error        string     `json:"error"`
	Renditions   []string   `json:"renditions"`
	Subtitles    []string   `json:"subtitles"`
	Published    bool       `json:"published"`
	Featured     bool       `json:"featured"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

const filmCols = `id, tmdb_id, title, year, overview, poster_path, backdrop_path, genre_ids, runtime,
rights, rights_note, status, progress, error, renditions, subtitles, published, featured, created_at, updated_at, release_date`

func scanFilm(row pgx.Row) (*Film, error) {
	var f Film
	err := row.Scan(&f.ID, &f.TMDBID, &f.Title, &f.Year, &f.Overview, &f.PosterPath, &f.BackdropPath,
		&f.GenreIDs, &f.Runtime, &f.Rights, &f.RightsNote, &f.Status, &f.Progress, &f.Error,
		&f.Renditions, &f.Subtitles, &f.Published, &f.Featured, &f.CreatedAt, &f.UpdatedAt, &f.ReleaseDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &f, err
}

func (s *Store) query(ctx context.Context, sql string, args ...any) ([]Film, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Film
	for rows.Next() {
		f, err := scanFilm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// CreateFilm inserts a new film awaiting its source upload.
func (s *Store) CreateFilm(ctx context.Context, f Film) (*Film, error) {
	return scanFilm(s.db.QueryRow(ctx, `INSERT INTO films
		(tmdb_id, title, year, overview, poster_path, backdrop_path, genre_ids, runtime, rights, rights_note, release_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+filmCols,
		f.TMDBID, f.Title, f.Year, f.Overview, f.PosterPath, f.BackdropPath, f.GenreIDs, f.Runtime, f.Rights, f.RightsNote, f.ReleaseDate))
}

func (s *Store) GetFilm(ctx context.Context, id int64) (*Film, error) {
	return scanFilm(s.db.QueryRow(ctx, `SELECT `+filmCols+` FROM films WHERE id=$1`, id))
}

// PublishedByTMDB returns a playable film for a TMDB id.
func (s *Store) PublishedByTMDB(ctx context.Context, tmdbID int) (*Film, error) {
	return scanFilm(s.db.QueryRow(ctx, `SELECT `+filmCols+` FROM films
		WHERE tmdb_id=$1 AND published AND status='ready'`, tmdbID))
}

// AllFilms lists every film, newest first (capped; used by the seed tool).
func (s *Store) AllFilms(ctx context.Context) ([]Film, error) {
	return s.query(ctx, `SELECT `+filmCols+` FROM films ORDER BY created_at DESC LIMIT 500`)
}

// FilmsPage lists one page of films for the admin dashboard, newest first, with the total count.
func (s *Store) FilmsPage(ctx context.Context, limit, offset int) ([]Film, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM films`).Scan(&total); err != nil {
		return nil, 0, err
	}
	films, err := s.query(ctx, `SELECT `+filmCols+` FROM films ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	return films, total, err
}

// Published lists playable films, optionally filtered by a TMDB genre id (0 = all),
// featured first, then newest release first. id breaks ties so pages never overlap.
func (s *Store) Published(ctx context.Context, genreID, limit, offset int) ([]Film, error) {
	return s.query(ctx, `SELECT `+filmCols+` FROM films
		WHERE published AND status='ready' AND ($1 = 0 OR $1 = ANY(genre_ids))
		ORDER BY featured DESC, release_date DESC NULLS LAST, year DESC, created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, genreID, limit, offset)
}

// CountPublished counts playable films, optionally in one TMDB genre (0 = all).
func (s *Store) CountPublished(ctx context.Context, genreID int) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM films
		WHERE published AND status='ready' AND ($1 = 0 OR $1 = ANY(genre_ids))`, genreID).Scan(&n)
	return n, err
}

// MissingReleaseDate lists films stored before release dates were recorded.
func (s *Store) MissingReleaseDate(ctx context.Context) ([]Film, error) {
	return s.query(ctx, `SELECT `+filmCols+` FROM films WHERE release_date IS NULL`)
}

func (s *Store) SetReleaseDate(ctx context.Context, id int64, d time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE films SET release_date=$2, updated_at=now() WHERE id=$1`, id, d)
	return err
}

// RecordPlay counts one play today for a published film, by TMDB id.
func (s *Store) RecordPlay(ctx context.Context, tmdbID int) error {
	tag, err := s.db.Exec(ctx, `INSERT INTO film_plays (film_id, plays)
		SELECT id, 1 FROM films WHERE tmdb_id=$1 AND published AND status='ready'
		ON CONFLICT (film_id, day) DO UPDATE SET plays = film_plays.plays + 1`, tmdbID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// TopToday lists the most played published films today (database time zone), most plays first.
func (s *Store) TopToday(ctx context.Context, limit int) ([]Film, error) {
	return s.query(ctx, `SELECT `+prefixed("f.", filmCols)+` FROM films f
		JOIN film_plays p ON p.film_id = f.id AND p.day = current_date
		WHERE f.published AND f.status='ready'
		ORDER BY p.plays DESC, f.created_at DESC LIMIT $1`, limit)
}

// prefixed qualifies each column in a comma-separated list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = alias + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// MarkUploaded queues a film for encoding once its source file is in storage.
func (s *Store) MarkUploaded(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `UPDATE films SET status='queued', progress=0, error='', updated_at=now()
		WHERE id=$1 AND status IN ('awaiting_upload','failed')`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetFlags updates published/featured. A film can only be published once encoded.
func (s *Store) SetFlags(ctx context.Context, id int64, published, featured *bool) (*Film, error) {
	return scanFilm(s.db.QueryRow(ctx, `UPDATE films SET
		published = CASE WHEN $2::bool IS NULL THEN published ELSE ($2 AND status='ready') END,
		featured  = COALESCE($3, featured),
		updated_at = now()
		WHERE id=$1 RETURNING `+filmCols, id, published, featured))
}

func (s *Store) AddSubtitle(ctx context.Context, id int64, lang string) error {
	_, err := s.db.Exec(ctx, `UPDATE films SET subtitles = array_append(array_remove(subtitles, $2), $2),
		updated_at=now() WHERE id=$1`, id, lang)
	return err
}

func (s *Store) DeleteFilm(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM films WHERE id=$1`, id)
	return err
}

// ClaimJob atomically takes the next queued film (or one whose worker died) for encoding.
func (s *Store) ClaimJob(ctx context.Context) (*Film, error) {
	return scanFilm(s.db.QueryRow(ctx, `UPDATE films SET status='encoding', progress=0, error='', updated_at=now()
		WHERE id = (
			SELECT id FROM films
			WHERE status='queued' OR (status='encoding' AND updated_at < now() - interval '30 minutes')
			ORDER BY updated_at LIMIT 1 FOR UPDATE SKIP LOCKED
		) RETURNING `+filmCols))
}

func (s *Store) SetProgress(ctx context.Context, id int64, pct int) error {
	_, err := s.db.Exec(ctx, `UPDATE films SET progress=$2, updated_at=now() WHERE id=$1 AND status='encoding'`, id, pct)
	return err
}

func (s *Store) FinishJob(ctx context.Context, id int64, renditions []string) error {
	_, err := s.db.Exec(ctx, `UPDATE films SET status='ready', progress=100, renditions=$2, updated_at=now() WHERE id=$1`, id, renditions)
	return err
}

func (s *Store) FailJob(ctx context.Context, id int64, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.db.Exec(ctx, `UPDATE films SET status='failed', error=$2, updated_at=now() WHERE id=$1`, id, msg)
	return err
}

type Admin struct {
	ID           int64
	Email        string
	PasswordHash string
	TOTPSecret   string
}

func (s *Store) AdminByEmail(ctx context.Context, email string) (*Admin, error) {
	var a Admin
	err := s.db.QueryRow(ctx, `SELECT id, email, password_hash, totp_secret FROM admins WHERE email=lower($1)`, email).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.TOTPSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

func (s *Store) AdminByID(ctx context.Context, id int64) (*Admin, error) {
	var a Admin
	err := s.db.QueryRow(ctx, `SELECT id, email, password_hash, totp_secret FROM admins WHERE id=$1`, id).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.TOTPSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

// UpsertAdmin creates an admin, or resets the password and TOTP secret of an existing one.
func (s *Store) UpsertAdmin(ctx context.Context, email, hash, totp string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO admins (email, password_hash, totp_secret) VALUES (lower($1), $2, $3)
		ON CONFLICT (email) DO UPDATE SET password_hash=EXCLUDED.password_hash, totp_secret=EXCLUDED.totp_secret`,
		email, hash, totp)
	return err
}
