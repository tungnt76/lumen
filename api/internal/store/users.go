package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Roles.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var (
	ErrEmailTaken    = errors.New("an account with this email already exists")
	ErrInviteInvalid = errors.New("this invite code isn't valid (used up, expired or mistyped)")
)

type User struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"displayName"`
	Role         string     `json:"role"`
	Avatar       string     `json:"avatar"`
	Bio          string     `json:"bio"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
}

const userCols = `id, email, password_hash, display_name, role, avatar, bio, disabled, created_at, last_login_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Avatar, &u.Bio, &u.Disabled, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email=lower($1)`, strings.TrimSpace(email)))
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

// UpsertUser creates a user or, for an existing email, resets the password and role (CLI use).
func (s *Store) UpsertUser(ctx context.Context, email, hash, name, role, avatar string) (*User, error) {
	return scanUser(s.db.QueryRow(ctx, `INSERT INTO users (email, password_hash, display_name, role, avatar)
		VALUES (lower($1), $2, $3, $4, $5)
		ON CONFLICT (email) DO UPDATE SET password_hash=EXCLUDED.password_hash, role=EXCLUDED.role, disabled=false, updated_at=now()
		RETURNING `+userCols, strings.TrimSpace(email), hash, name, role, avatar))
}

// SignUp creates a member account, consuming one use of the invite code in the same
// transaction (so a single-use code can't be spent twice). The invite decides the role.
func (s *Store) SignUp(ctx context.Context, code, email, hash, name, avatar string) (*User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var role string
	err = tx.QueryRow(ctx, `UPDATE invite_codes SET uses = uses + 1
		WHERE code=$1 AND NOT disabled AND uses < max_uses AND (expires_at IS NULL OR expires_at > now())
		RETURNING role`, NormalizeCode(code)).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteInvalid
	} else if err != nil {
		return nil, err
	}
	u, err := scanUser(tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, display_name, role, avatar)
		VALUES (lower($1), $2, $3, $4, $5) ON CONFLICT (email) DO NOTHING RETURNING `+userCols,
		strings.TrimSpace(email), hash, name, role, avatar))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrEmailTaken
	} else if err != nil {
		return nil, err
	}
	return u, tx.Commit(ctx)
}

// UpdateProfile changes what the user can edit about themselves.
func (s *Store) UpdateProfile(ctx context.Context, id int64, name, avatar, bio string) (*User, error) {
	return scanUser(s.db.QueryRow(ctx, `UPDATE users SET display_name=$2, avatar=$3, bio=$4, updated_at=now()
		WHERE id=$1 RETURNING `+userCols, id, name, avatar, bio))
}

func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, id, hash)
	return err
}

// SetUserAccess changes role and/or disabled (admin use). Disabling signs the user out everywhere.
func (s *Store) SetUserAccess(ctx context.Context, id int64, role *string, disabled *bool) (*User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `UPDATE users SET role=COALESCE($2, role), disabled=COALESCE($3, disabled), updated_at=now()
		WHERE id=$1 RETURNING `+userCols, id, role, disabled))
	if err == nil && u.Disabled {
		_, err = s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id)
	}
	return u, err
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin' AND NOT disabled`).Scan(&n)
	return n, err
}

// UsersPage lists users for the studio, newest first, optionally matching name or email.
func (s *Store) UsersPage(ctx context.Context, query string, limit, offset int) ([]User, int, error) {
	q := ""
	if t := strings.TrimSpace(strings.ToLower(query)); t != "" {
		q = "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(t) + "%"
	}
	where := `($1 = '' OR lower(display_name) LIKE $1 OR email LIKE $1)`
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE `+where, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+userCols+` FROM users WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *u)
	}
	return out, total, rows.Err()
}

// ---- sessions ----

// SessionTTL is how long a session lasts without use; each use extends it.
const SessionTTL = 30 * 24 * time.Hour

type Session struct {
	ID         int64     `json:"id"`
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	Current    bool      `json:"current"`
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// CreateSession starts a session and returns the token for the cookie (never stored).
func (s *Store) CreateSession(ctx context.Context, userID int64, userAgent, ip string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	_, err := s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, user_agent, ip, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		hashToken(token), userID, userAgent, ip, time.Now().Add(SessionTTL))
	if err == nil {
		_, err = s.db.Exec(ctx, `UPDATE users SET last_login_at=now() WHERE id=$1`, userID)
	}
	return token, err
}

// SessionUser returns the signed-in user and session id for a cookie token. Sessions of
// disabled users or past expiry don't count. Use is recorded at most every 10 minutes.
func (s *Store) SessionUser(ctx context.Context, token string) (*User, int64, error) {
	var sid int64
	var lastSeen time.Time
	var u User
	err := s.db.QueryRow(ctx, `SELECT s.id, s.last_seen_at, `+prefixed("u.", userCols)+` FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1 AND s.expires_at > now() AND NOT u.disabled`, hashToken(token)).
		Scan(&sid, &lastSeen, &u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Avatar, &u.Bio, &u.Disabled, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ErrNotFound
	} else if err != nil {
		return nil, 0, err
	}
	if time.Since(lastSeen) > 10*time.Minute {
		_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at=now(), expires_at=$2 WHERE id=$1`, sid, time.Now().Add(SessionTTL))
	}
	return &u, sid, nil
}

func (s *Store) DeleteSessionToken(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hashToken(token))
	return err
}

// DeleteSession signs out one of the user's own sessions.
func (s *Store) DeleteSession(ctx context.Context, userID, sessionID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND user_id=$2`, sessionID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteOtherSessions signs the user out everywhere except keepID (0 = everywhere).
func (s *Store) DeleteOtherSessions(ctx context.Context, userID, keepID int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND id<>$2`, userID, keepID)
	return err
}

func (s *Store) Sessions(ctx context.Context, userID, currentID int64) ([]Session, error) {
	rows, err := s.db.Query(ctx, `SELECT id, user_agent, ip, created_at, last_seen_at FROM sessions
		WHERE user_id=$1 AND expires_at > now() ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		if err := rows.Scan(&x.ID, &x.UserAgent, &x.IP, &x.CreatedAt, &x.LastSeenAt); err != nil {
			return nil, err
		}
		x.Current = x.ID == currentID
		out = append(out, x)
	}
	return out, rows.Err()
}

// ---- invite codes ----

// InviteCodeLen is the number of digits in an invite code.
const InviteCodeLen = 6

type Invite struct {
	Code      string     `json:"code"` // 6 digits, e.g. "482915"
	Role      string     `json:"role"`
	MaxUses   int        `json:"maxUses"`
	Uses      int        `json:"uses"`
	Note      string     `json:"note"`
	ExpiresAt *time.Time `json:"expiresAt"`
	Disabled  bool       `json:"disabled"`
	CreatedAt time.Time  `json:"createdAt"`
	Usable    bool       `json:"usable"`
}

// NormalizeCode uppercases a typed code and drops spaces and dashes ("482 915" → "482915").
func NormalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// newCode returns 6 random digits (leading zeros allowed).
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

const inviteCols = `code, role, max_uses, uses, note, expires_at, disabled, created_at`

func scanInvite(row pgx.Row) (*Invite, error) {
	var i Invite
	err := row.Scan(&i.Code, &i.Role, &i.MaxUses, &i.Uses, &i.Note, &i.ExpiresAt, &i.Disabled, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil {
		i.Usable = !i.Disabled && i.Uses < i.MaxUses && (i.ExpiresAt == nil || i.ExpiresAt.After(time.Now()))
	}
	return &i, err
}

// CreateInvite makes a new code. expires may be nil (never); createdBy may be 0 (CLI).
// With only a million 6-digit codes, a taken one is retried.
func (s *Store) CreateInvite(ctx context.Context, role string, maxUses int, expires *time.Time, note string, createdBy int64) (*Invite, error) {
	var by *int64
	if createdBy > 0 {
		by = &createdBy
	}
	for attempt := 0; ; attempt++ {
		code, err := newCode()
		if err != nil {
			return nil, err
		}
		inv, err := scanInvite(s.db.QueryRow(ctx, `INSERT INTO invite_codes (code, role, max_uses, expires_at, note, created_by)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (code) DO NOTHING RETURNING `+inviteCols, code, role, maxUses, expires, note, by))
		if errors.Is(err, ErrNotFound) && attempt < 20 {
			continue // code already exists
		}
		return inv, err
	}
}

// InviteByCode looks up a typed code (any grouping or case).
func (s *Store) InviteByCode(ctx context.Context, code string) (*Invite, error) {
	return scanInvite(s.db.QueryRow(ctx, `SELECT `+inviteCols+` FROM invite_codes WHERE code=$1`, NormalizeCode(code)))
}

func (s *Store) Invites(ctx context.Context) ([]Invite, error) {
	rows, err := s.db.Query(ctx, `SELECT `+inviteCols+` FROM invite_codes ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *Store) SetInviteDisabled(ctx context.Context, code string, disabled bool) (*Invite, error) {
	return scanInvite(s.db.QueryRow(ctx, `UPDATE invite_codes SET disabled=$2 WHERE code=$1 RETURNING `+inviteCols, NormalizeCode(code), disabled))
}

func (s *Store) DeleteInvite(ctx context.Context, code string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM invite_codes WHERE code=$1`, NormalizeCode(code))
	return err
}
