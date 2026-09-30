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

// CodeProject is a set of text files edited in the Code tab. The files themselves are a JSON
// bundle in the media bucket (see Folder); the row holds what the list and access checks need.
type CodeProject struct {
	ID        string     `json:"id"` // UUID v7
	OwnerID   int64      `json:"ownerId"`
	OwnerName string     `json:"ownerName,omitempty"`
	Token     string     `json:"-"`
	Title     string     `json:"title"`
	Language  string     `json:"language"`
	Preview   string     `json:"preview"`
	FileCount int        `json:"fileCount"`
	SizeBytes int64      `json:"sizeBytes"`
	SavedAt   *time.Time `json:"savedAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Folder is the project's storage prefix, e.g. "code/0191…-3f9c…/". The bucket is public,
// so the 128-bit token keeps the files from being found by anyone who only knows the id.
func (p CodeProject) Folder() string { return fmt.Sprintf("code/%s-%s/", p.ID, p.Token) }

// BundleKey is the working copy; CommitKey a committed snapshot.
func (p CodeProject) BundleKey() string { return p.Folder() + "project.json" }
func (p CodeProject) CommitKey(commit string) string {
	return p.Folder() + "commits/" + commit + ".json"
}

const codeCols = `p.id::text, p.owner_id, COALESCE(u.display_name, ''), p.token, p.title, p.language, p.preview,
p.file_count, p.size_bytes, p.saved_at, p.created_at, p.updated_at`
const codeFrom = ` FROM code_projects p LEFT JOIN users u ON u.id = p.owner_id`

func scanProject(row pgx.Row) (*CodeProject, error) {
	var p CodeProject
	err := row.Scan(&p.ID, &p.OwnerID, &p.OwnerName, &p.Token, &p.Title, &p.Language, &p.Preview,
		&p.FileCount, &p.SizeBytes, &p.SavedAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) CreateProject(ctx context.Context, title, language string, ownerID int64) (*CodeProject, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	var id string
	if err := s.db.QueryRow(ctx, `INSERT INTO code_projects (owner_id, token, title, language) VALUES ($1,$2,$3,$4) RETURNING id::text`,
		ownerID, hex.EncodeToString(b), title, language).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) GetProject(ctx context.Context, id string) (*CodeProject, error) {
	return scanProject(s.db.QueryRow(ctx, `SELECT `+codeCols+codeFrom+` WHERE p.id=$1`, id))
}

func (s *Store) CountProjects(ctx context.Context, ownerID int64) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM code_projects WHERE owner_id=$1`, ownerID).Scan(&n)
	return n, err
}

// ProjectsPage lists one user's projects (ownerID > 0) or everyone's (0, studio), most recently
// changed first; the search matches title, language or owner, ignoring case and tones.
func (s *Store) ProjectsPage(ctx context.Context, ownerID int64, query string, limit, offset int) ([]CodeProject, int, error) {
	rows, err := s.db.Query(ctx, `SELECT `+codeCols+codeFrom+` WHERE ($1 = 0 OR p.owner_id = $1)
		ORDER BY p.updated_at DESC, p.id DESC LIMIT 2000`, ownerID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	q := Fold(query)
	all := []CodeProject{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		if q == "" || strings.Contains(Fold(p.Title+" "+p.Language+" "+p.OwnerName), q) {
			all = append(all, *p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(all)
	if offset >= total {
		return []CodeProject{}, total, nil
	}
	return all[offset:min(offset+limit, total)], total, nil
}

func (s *Store) RenameProject(ctx context.Context, id, title string) (*CodeProject, error) {
	if _, err := s.db.Exec(ctx, `UPDATE code_projects SET title=$2, updated_at=now() WHERE id=$1`, id, title); err != nil {
		return nil, err
	}
	return s.GetProject(ctx, id)
}

// MarkProjectSaved records a completed save of the working copy.
func (s *Store) MarkProjectSaved(ctx context.Context, id string, size int64, files int, language, preview string) (*CodeProject, error) {
	if _, err := s.db.Exec(ctx, `UPDATE code_projects SET size_bytes=$2, file_count=$3, language=$4, preview=$5,
		saved_at=now(), updated_at=now() WHERE id=$1`, id, size, files, language, preview); err != nil {
		return nil, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM code_projects WHERE id=$1`, id)
	return err
}

// ---- commits ----

// FileChange is one file in a commit's summary: A(dded), M(odified) or D(eleted).
type FileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type CodeCommit struct {
	ID         string       `json:"id"`
	AuthorName string       `json:"authorName"`
	Message    string       `json:"message"`
	Changes    []FileChange `json:"changes"`
	FileCount  int          `json:"fileCount"`
	SizeBytes  int64        `json:"sizeBytes"`
	CreatedAt  time.Time    `json:"createdAt"`
}

// MaxCommits is how many commits a project keeps; older ones are dropped.
const MaxCommits = 200

const commitCols = `c.id::text, COALESCE(u.display_name, ''), c.message, c.changes, c.file_count, c.size_bytes, c.created_at`
const commitFrom = ` FROM code_commits c LEFT JOIN users u ON u.id = c.author_id`

func scanCommit(row pgx.Row) (*CodeCommit, error) {
	var c CodeCommit
	err := row.Scan(&c.ID, &c.AuthorName, &c.Message, &c.Changes, &c.FileCount, &c.SizeBytes, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil && c.Changes == nil {
		c.Changes = []FileChange{}
	}
	return &c, err
}

// CreateCommit records a commit (its snapshot must already be in storage) and returns it with
// the ids of commits beyond MaxCommits, which the caller deletes from storage.
func (s *Store) CreateCommit(ctx context.Context, id, projectID string, authorID int64, message string, changes []FileChange, files int, size int64) (*CodeCommit, []string, error) {
	if changes == nil {
		changes = []FileChange{}
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO code_commits (id, project_id, author_id, message, changes, file_count, size_bytes)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, projectID, authorID, message, changes, files, size); err != nil {
		return nil, nil, err
	}
	rows, err := s.db.Query(ctx, `DELETE FROM code_commits WHERE project_id=$1 AND id IN (
		SELECT id FROM code_commits WHERE project_id=$1 ORDER BY created_at DESC, id DESC OFFSET $2) RETURNING id::text`, projectID, MaxCommits)
	if err != nil {
		return nil, nil, err
	}
	var dropped []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			rows.Close()
			return nil, nil, err
		}
		dropped = append(dropped, d)
	}
	rows.Close()
	c, err := s.GetCommit(ctx, projectID, id)
	return c, dropped, err
}

// NewCommitID returns a fresh UUID v7 (from the database's uuid_v7), so the snapshot can be
// stored under its final name before the row exists.
func (s *Store) NewCommitID(ctx context.Context) (string, error) {
	var id string
	err := s.db.QueryRow(ctx, `SELECT uuid_v7()::text`).Scan(&id)
	return id, err
}

func (s *Store) GetCommit(ctx context.Context, projectID, id string) (*CodeCommit, error) {
	return scanCommit(s.db.QueryRow(ctx, `SELECT `+commitCols+commitFrom+` WHERE c.project_id=$1 AND c.id=$2`, projectID, id))
}

// Commits lists a project's commits, newest first.
func (s *Store) Commits(ctx context.Context, projectID string) ([]CodeCommit, error) {
	rows, err := s.db.Query(ctx, `SELECT `+commitCols+commitFrom+` WHERE c.project_id=$1 ORDER BY c.created_at DESC, c.id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CodeCommit{}
	for rows.Next() {
		c, err := scanCommit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}
