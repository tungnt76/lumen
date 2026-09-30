package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Drawing is an Excalidraw sketch; its content is in the media bucket, not the database.
type Drawing struct {
	ID        string     `json:"id"` // UUID v7
	Token     string     `json:"-"`  // random part of the storage folder; never sent to the browser
	folder    string     // set for drawings made before ids became UUIDs (migration 0001)
	OwnerID   *int64     `json:"ownerId"`
	OwnerName string     `json:"ownerName,omitempty"`
	Title     string     `json:"title"`
	SizeBytes int64      `json:"sizeBytes"`
	SavedAt   *time.Time `json:"savedAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// OwnedBy reports whether userID made the drawing.
func (d Drawing) OwnedBy(userID int64) bool { return d.OwnerID != nil && *d.OwnerID == userID }

const drawingCols = `d.id::text, d.token, d.folder, d.owner_id, COALESCE(u.display_name, ''), d.title, d.size_bytes, d.saved_at, d.created_at, d.updated_at`
const drawingFrom = ` FROM drawings d LEFT JOIN users u ON u.id = d.owner_id`

func scanDrawing(row pgx.Row) (*Drawing, error) {
	var d Drawing
	err := row.Scan(&d.ID, &d.Token, &d.folder, &d.OwnerID, &d.OwnerName, &d.Title, &d.SizeBytes, &d.SavedAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

// Folder is the drawing's storage prefix, e.g. "drawings/0191…-3f9c…/". The media bucket is
// public, so the 128-bit token keeps files from being found even by someone who knows the id.
func (d Drawing) Folder() string {
	if d.folder != "" { // made before ids became UUIDs; the files stay where they were
		return d.folder
	}
	return fmt.Sprintf("drawings/%s-%s/", d.ID, d.Token)
}

// CreateDrawing makes an empty drawing owned by ownerID.
func (s *Store) CreateDrawing(ctx context.Context, title string, ownerID int64) (*Drawing, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	var id string
	if err := s.db.QueryRow(ctx, `INSERT INTO drawings (title, token, owner_id) VALUES ($1, $2, $3) RETURNING id::text`,
		title, hex.EncodeToString(b), ownerID).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetDrawing(ctx, id)
}

func (s *Store) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	return scanDrawing(s.db.QueryRow(ctx, `SELECT `+drawingCols+drawingFrom+` WHERE d.id=$1`, id))
}

// CountDrawings counts a user's drawings.
func (s *Store) CountDrawings(ctx context.Context, ownerID int64) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM drawings WHERE owner_id=$1`, ownerID).Scan(&n)
	return n, err
}

// DrawingsPage lists drawings, most recently changed first: one user's (ownerID > 0) or everyone's
// (ownerID 0, for the studio). The title search ignores case and Vietnamese tones.
func (s *Store) DrawingsPage(ctx context.Context, ownerID int64, query string, limit, offset int) ([]Drawing, int, error) {
	// Titles are few and short per user, so folding in Go keeps this free of Postgres extensions.
	rows, err := s.db.Query(ctx, `SELECT `+drawingCols+drawingFrom+` WHERE ($1 = 0 OR d.owner_id = $1)
		ORDER BY d.updated_at DESC, d.id DESC LIMIT 2000`, ownerID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	q := Fold(query)
	var all []Drawing
	for rows.Next() {
		d, err := scanDrawing(rows)
		if err != nil {
			return nil, 0, err
		}
		if q == "" || strings.Contains(Fold(d.Title), q) || strings.Contains(Fold(d.OwnerName), q) {
			all = append(all, *d)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(all)
	if offset >= total {
		return []Drawing{}, total, nil
	}
	return all[offset:min(offset+limit, total)], total, nil
}

func (s *Store) RenameDrawing(ctx context.Context, id string, title string) (*Drawing, error) {
	if _, err := s.db.Exec(ctx, `UPDATE drawings SET title=$2, updated_at=now() WHERE id=$1`, id, title); err != nil {
		return nil, err
	}
	return s.GetDrawing(ctx, id)
}

// MarkDrawingSaved records a completed save of size bytes.
func (s *Store) MarkDrawingSaved(ctx context.Context, id string, size int64) (*Drawing, error) {
	if _, err := s.db.Exec(ctx, `UPDATE drawings SET size_bytes=$2, saved_at=now(), updated_at=now() WHERE id=$1`, id, size); err != nil {
		return nil, err
	}
	return s.GetDrawing(ctx, id)
}

func (s *Store) DeleteDrawing(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM drawings WHERE id=$1`, id)
	return err
}
