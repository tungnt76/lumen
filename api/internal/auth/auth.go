// Package auth implements admin password hashing, TOTP (RFC 6238),
// signed session tokens and a login rate limiter.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ---- passwords ----

func HashPassword(pw string) (string, error) {
	if len(pw) < 6 {
		return "", errors.New("password must be at least 6 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// DummyCheck spends the same time as a real check, so unknown emails can't be detected by timing.
func DummyCheck(pw string) {
	dummyOnce.Do(func() { dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), 12) })
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// ---- TOTP ----

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random base32 secret for an authenticator app.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// TOTPURL is the otpauth:// URL to paste or turn into a QR code.
func TOTPURL(issuer, account, secret string) string {
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + v.Encode()
}

func totpAt(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code)
}

// CodeAt returns the current code for a secret (used by tests and tooling).
func CodeAt(secret string, now time.Time) string {
	key, _ := b32.DecodeString(strings.ToUpper(secret))
	return totpAt(key, uint64(now.Unix()/30))
}

// ValidTOTP checks a 6-digit code, allowing one 30-second step of clock drift.
func ValidTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	step := uint64(now.Unix() / 30)
	ok := false
	for _, c := range []uint64{step - 1, step, step + 1} {
		if subtle.ConstantTimeCompare([]byte(totpAt(key, c)), []byte(code)) == 1 {
			ok = true
		}
	}
	return ok
}

// ---- sessions ----

type Sessions struct{ key []byte }

func NewSessions(key []byte) *Sessions { return &Sessions{key: key} }

func (s *Sessions) mac(payload string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Issue returns a token "adminID.expiryUnix.signature".
func (s *Sessions) Issue(adminID int64, ttl time.Duration) string {
	payload := strconv.FormatInt(adminID, 10) + "." + strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	return payload + "." + s.mac(payload)
}

// Verify returns the admin id of a valid, unexpired token.
func (s *Sessions) Verify(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, false
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(s.mac(payload))) {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	return id, err == nil
}

// ---- rate limiting ----

// Limiter locks a key (client IP) after too many failed logins.
type Limiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	entries map[string]*limitEntry
}

type limitEntry struct {
	fails int
	first time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, entries: map[string]*limitEntry{}}
}

// Allowed reports whether key may try again.
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true
	}
	if time.Since(e.first) > l.window {
		delete(l.entries, key)
		return true
	}
	return e.fails < l.max
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || time.Since(e.first) > l.window {
		e = &limitEntry{first: time.Now()}
		l.entries[key] = e
	}
	e.fails++
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
