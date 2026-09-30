package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Work kinds.
const (
	KindBook  = "book"
	KindAlbum = "album"
)

// Link is an official place to listen to a work that Lumen doesn't host.
type Link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Work is an audiobook or a music album. TrackCount and DurationSec are computed from its tracks.
// A work either has tracks (hosted or linked audio files) or, with license "external", only Links.
type Work struct {
	ID          string    `json:"id"` // UUID v7
	MediaPrefix string    `json:"-"`  // storage folder of uploaded audio; see AudioFolder
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	SourceID    string    `json:"sourceId"`
	Title       string    `json:"title"`
	Creator     string    `json:"creator"`
	Narrator    string    `json:"narrator"`
	Language    string    `json:"language"`
	Year        int       `json:"year"`
	Description string    `json:"description"`
	CoverURL    string    `json:"coverUrl"`
	Genres      []string  `json:"genres"`
	License     string    `json:"license"`
	RightsNote  string    `json:"rightsNote"`
	AIVoice     bool      `json:"aiVoice"`
	Links       []Link    `json:"links"`
	Published   bool      `json:"published"`
	Featured    bool      `json:"featured"`
	TrackCount  int       `json:"trackCount"`
	DurationSec int       `json:"durationSec"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Track is a chapter of a book or a song on an album. Audio is an absolute URL or an R2 key.
type Track struct {
	Position    int    `json:"position"`
	Title       string `json:"title"`
	DurationSec int    `json:"durationSec"`
	Audio       string `json:"audio"`
}

// AudioFolder is where the work's uploaded audio lives: audio/{id}/, or the folder it had
// before ids became UUIDs (kept by migration 0001).
func (w Work) AudioFolder() string {
	if w.MediaPrefix != "" {
		return w.MediaPrefix
	}
	return "audio/" + w.ID + "/"
}

const workCols = `w.id::text, w.media_prefix, w.kind, w.source, w.source_id, w.title, w.creator, w.narrator, w.language, w.year,
w.description, w.cover_url, w.genres, w.license, w.rights_note, w.ai_voice, w.links, w.published, w.featured,
(SELECT count(*) FROM tracks t WHERE t.work_id = w.id),
(SELECT coalesce(sum(t.duration_sec), 0) FROM tracks t WHERE t.work_id = w.id),
w.created_at, w.updated_at`

func scanWork(row pgx.Row) (*Work, error) {
	var w Work
	err := row.Scan(&w.ID, &w.MediaPrefix, &w.Kind, &w.Source, &w.SourceID, &w.Title, &w.Creator, &w.Narrator, &w.Language, &w.Year,
		&w.Description, &w.CoverURL, &w.Genres, &w.License, &w.RightsNote, &w.AIVoice, &w.Links, &w.Published, &w.Featured,
		&w.TrackCount, &w.DurationSec, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &w, err
}

func (s *Store) queryWorks(ctx context.Context, sql string, args ...any) ([]Work, error) {
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Work{}
	for rows.Next() {
		w, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// UpsertWork inserts a work, or updates the metadata of the one imported from the same
// source and source id. Published and featured are left unchanged on update.
func (s *Store) UpsertWork(ctx context.Context, w Work) (*Work, error) {
	if w.Genres == nil {
		w.Genres = []string{}
	}
	if w.Links == nil {
		w.Links = []Link{}
	}
	return scanWork(s.db.QueryRow(ctx, `WITH w AS (
		INSERT INTO works (kind, source, source_id, title, creator, narrator, language, year,
			description, cover_url, genres, license, rights_note, ai_voice, links, search)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (source, source_id) DO UPDATE SET
			kind=EXCLUDED.kind, title=EXCLUDED.title, creator=EXCLUDED.creator, narrator=EXCLUDED.narrator,
			language=EXCLUDED.language, year=EXCLUDED.year, description=EXCLUDED.description,
			cover_url=EXCLUDED.cover_url, genres=EXCLUDED.genres, license=EXCLUDED.license,
			rights_note=EXCLUDED.rights_note, ai_voice=EXCLUDED.ai_voice, links=EXCLUDED.links, search=EXCLUDED.search, updated_at=now()
		RETURNING *
	) SELECT `+workCols+` FROM w`,
		w.Kind, w.Source, w.SourceID, w.Title, w.Creator, w.Narrator, w.Language, w.Year,
		w.Description, w.CoverURL, w.Genres, w.License, w.RightsNote, w.AIVoice, w.Links, searchText(w)))
}

func searchText(w Work) string {
	return Fold(strings.Join([]string{w.Title, w.Creator, w.Narrator}, " "))
}

// backfillSearch fills the search column for works stored before it existed.
func (s *Store) backfillSearch(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `SELECT id::text, title, creator, narrator FROM works WHERE search = ''`)
	if err != nil {
		return err
	}
	var todo []Work
	for rows.Next() {
		var w Work
		if err := rows.Scan(&w.ID, &w.Title, &w.Creator, &w.Narrator); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, w := range todo {
		if _, err := s.db.Exec(ctx, `UPDATE works SET search=$2 WHERE id=$1`, w.ID, searchText(w)); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceTracks swaps a work's whole track list in one transaction.
func (s *Store) ReplaceTracks(ctx context.Context, workID string, tracks []Track) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tracks WHERE work_id=$1`, workID); err != nil {
		return err
	}
	for _, t := range tracks {
		if _, err := tx.Exec(ctx, `INSERT INTO tracks (work_id, position, title, duration_sec, audio)
			VALUES ($1,$2,$3,$4,$5)`, workID, t.Position, t.Title, t.DurationSec, t.Audio); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE works SET updated_at=now() WHERE id=$1`, workID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetWorkFlags updates published/featured. A work can only be published once it has tracks
// or, for works Lumen doesn't host, official links.
func (s *Store) SetWorkFlags(ctx context.Context, id string, published, featured *bool) (*Work, error) {
	return scanWork(s.db.QueryRow(ctx, `WITH w AS (
		UPDATE works SET
			published = CASE WHEN $2::bool IS NULL THEN published
				ELSE ($2 AND (EXISTS (SELECT 1 FROM tracks WHERE work_id = works.id) OR jsonb_array_length(links) > 0)) END,
			featured  = COALESCE($3, featured),
			updated_at = now()
		WHERE id=$1 RETURNING *
	) SELECT `+workCols+` FROM w`, id, published, featured))
}

func (s *Store) DeleteWork(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM works WHERE id=$1`, id)
	return err
}

// WorkFilter narrows the published catalog. Empty fields match everything.
type WorkFilter struct {
	Kind     string
	Language string
	Query    string // matches title, creator or narrator, ignoring case and accents
}

const publishedWhere = `w.published AND w.kind = $1 AND ($2 = '' OR w.language = $2)
	AND ($3 = '' OR w.search LIKE $3 ESCAPE '\')`

func (f WorkFilter) args() []any {
	return []any{f.Kind, f.Language, likePattern(f.Query)}
}

// likePattern folds a search query into a LIKE pattern, or "" for no filter.
func likePattern(q string) string {
	q = strings.TrimSpace(Fold(q))
	if q == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

// PublishedWorks lists one page of published works, featured first, then newest first.
func (s *Store) PublishedWorks(ctx context.Context, f WorkFilter, limit, offset int) ([]Work, error) {
	return s.queryWorks(ctx, `SELECT `+workCols+` FROM works w WHERE `+publishedWhere+`
		ORDER BY w.featured DESC, w.created_at DESC, w.id DESC LIMIT $4 OFFSET $5`,
		append(f.args(), limit, offset)...)
}

func (s *Store) CountPublishedWorks(ctx context.Context, f WorkFilter) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM works w WHERE `+publishedWhere, f.args()...).Scan(&n)
	return n, err
}

// PublishedWork returns one published work.
func (s *Store) PublishedWork(ctx context.Context, id string) (*Work, error) {
	return scanWork(s.db.QueryRow(ctx, `SELECT `+workCols+` FROM works w WHERE w.id=$1 AND w.published`, id))
}

// Tracks lists a work's tracks in play order.
func (s *Store) Tracks(ctx context.Context, workID string) ([]Track, error) {
	rows, err := s.db.Query(ctx, `SELECT position, title, duration_sec, audio FROM tracks
		WHERE work_id=$1 ORDER BY position`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Track{}
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.Position, &t.Title, &t.DurationSec, &t.Audio); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AllWorks lists every work, published or not, newest first (capped; used by the import tool).
func (s *Store) AllWorks(ctx context.Context) ([]Work, error) {
	return s.queryWorks(ctx, `SELECT `+workCols+` FROM works w ORDER BY w.created_at DESC, w.id DESC LIMIT 500`)
}

// GetWork returns a work whether or not it is published.
func (s *Store) GetWork(ctx context.Context, id string) (*Work, error) {
	return scanWork(s.db.QueryRow(ctx, `SELECT `+workCols+` FROM works w WHERE w.id=$1`, id))
}

// DeleteWorkBySource removes the work imported from source/sourceID, if there is one.
func (s *Store) DeleteWorkBySource(ctx context.Context, source, sourceID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM works WHERE source=$1 AND source_id=$2`, source, sourceID)
	return err == nil && tag.RowsAffected() > 0, err
}

// WorksPage lists one page of every work (published or not) for the studio, newest first,
// optionally of one kind ("" = all) and matching a search, with the total count.
func (s *Store) WorksPage(ctx context.Context, kind, query string, limit, offset int) ([]Work, int, error) {
	where := `($1 = '' OR w.kind = $1) AND ($2 = '' OR w.search LIKE $2 ESCAPE '\')`
	q := likePattern(query)
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM works w WHERE `+where, kind, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	works, err := s.queryWorks(ctx, `SELECT `+workCols+` FROM works w WHERE `+where+`
		ORDER BY w.created_at DESC, w.id DESC LIMIT $3 OFFSET $4`, kind, q, limit, offset)
	return works, total, err
}

// WorkExists reports whether a work was already imported from source/sourceID.
func (s *Store) WorkExists(ctx context.Context, source, sourceID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM works WHERE source=$1 AND source_id=$2)`, source, sourceID).Scan(&ok)
	return ok, err
}
