package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/store"
)

// Accounts: sign-up with an invite code, sign-in, profile, password and sessions.
//
// Sessions are server-side: the HttpOnly cookie carries a random 256-bit token and the
// database keeps only its SHA-256, so signing out, disabling a user or changing a password
// takes effect immediately (unlike a stateless JWT, which stays valid until it expires).

// Avatars are presets drawn by web/components/Avatar.tsx; keep the two lists in step.
var avatars = map[string]bool{
	"film": true, "headphones": true, "book": true, "music": true, "mic": true, "camera": true,
	"popcorn": true, "star": true, "moon": true, "sun": true, "leaf": true, "wave": true,
	"mountain": true, "rocket": true, "planet": true, "heart": true,
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func (s *Server) setSession(w http.ResponseWriter, token string) {
	age := int(store.SessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: age,
		HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: authHint, Value: "1", Path: "/", MaxAge: age,
		Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	for _, name := range []string{sessionCookie, authHint} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == sessionCookie,
			Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode})
	}
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) bool {
	token, err := s.store.CreateSession(r.Context(), u.ID, r.UserAgent(), clientIP(r))
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	s.setSession(w, token)
	return true
}

// POST /api/auth/login {email, password}
func (s *Server) login(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, false) }

// POST /api/admin/login: the studio sign-in, same accounts but admins only.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) { s.doLogin(w, r, true) }

func (s *Server) doLogin(w http.ResponseWriter, r *http.Request, adminOnly bool) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "bad origin")
		return
	}
	ip := clientIP(r)
	if !s.limiter.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in 15 minutes")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	u, err := s.store.UserByEmail(r.Context(), in.Email)
	ok := false
	if err == nil {
		ok = auth.CheckPassword(u.PasswordHash, in.Password)
	} else if errors.Is(err, store.ErrNotFound) {
		auth.DummyCheck(in.Password) // same timing for unknown emails
	} else {
		s.fail(w, r, err)
		return
	}
	if !ok {
		s.limiter.Fail(ip)
		s.log.Warn("sign-in failed", "ip", ip)
		writeErr(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	// Only reveal these after the right password, so they can't be used to probe emails.
	if u.Disabled {
		writeErr(w, http.StatusForbidden, "this account has been disabled")
		return
	}
	if adminOnly && u.Role != store.RoleAdmin {
		writeErr(w, http.StatusForbidden, "this account doesn't have studio access")
		return
	}
	s.limiter.Reset(ip)
	if s.startSession(w, r, u) {
		writeJSON(w, http.StatusOK, map[string]any{"user": u, "email": u.Email})
	}
}

// POST /api/auth/logout (also /api/admin/logout)
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := s.store.DeleteSessionToken(r.Context(), c.Value); err != nil {
			s.log.Warn("sign-out", "err", err)
		}
	}
	s.clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/invite {code}: whether a code can be used, before filling in the form.
func (s *Server) checkInvite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "bad origin")
		return
	}
	ip := clientIP(r)
	if !s.inviteAllowed(w, ip) {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	inv, err := s.store.InviteByCode(r.Context(), in.Code)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if inv == nil || !inv.Usable {
		s.wrongInvite(ip)
		writeErr(w, http.StatusNotFound, store.ErrInviteInvalid.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "role": inv.Role, "code": inv.Code})
}

// inviteAllowed rate-limits invite codes per IP and site-wide: a 6-digit code has only a
// million values, so spreading guesses over many IPs must not help either.
func (s *Server) inviteAllowed(w http.ResponseWriter, ip string) bool {
	if !s.invites.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many wrong codes, try again in 15 minutes")
		return false
	}
	if !s.guesses.Allowed("all") {
		writeErr(w, http.StatusTooManyRequests, "sign-up is busy right now, try again in a few minutes")
		return false
	}
	return true
}

func (s *Server) wrongInvite(ip string) {
	s.invites.Fail(ip)
	s.guesses.Fail("all")
}

type profileIn struct {
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
	Bio         string `json:"bio"`
}

// clean trims and checks profile fields; it returns a message for the first problem.
func (p *profileIn) clean() string {
	p.DisplayName = strings.Join(strings.Fields(p.DisplayName), " ")
	p.Bio = strings.TrimSpace(p.Bio)
	switch {
	case p.DisplayName == "":
		return "add your name"
	case utf8.RuneCountInString(p.DisplayName) > 60:
		return "the name is too long (60 characters max)"
	case utf8.RuneCountInString(p.Bio) > 300:
		return "the bio is too long (300 characters max)"
	case p.Avatar != "" && !avatars[p.Avatar]:
		return "pick one of the avatars"
	}
	return ""
}

func checkPassword(pw string) string {
	switch {
	case len(pw) < 8:
		return "use at least 8 characters for the password"
	case len(pw) > 72: // bcrypt ignores anything longer
		return "the password is too long (72 characters max)"
	}
	return ""
}

// POST /api/auth/signup {code, email, password, displayName, avatar}
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.sameOrigin(r) {
		writeErr(w, http.StatusForbidden, "bad origin")
		return
	}
	ip := clientIP(r)
	if !s.inviteAllowed(w, ip) {
		return
	}
	if !s.signups.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many new accounts from this network, try again in an hour")
		return
	}
	var in struct {
		Code     string `json:"code"`
		Email    string `json:"email"`
		Password string `json:"password"`
		profileIn
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if msg := in.clean(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if !emailRe.MatchString(in.Email) || len(in.Email) > 254 {
		writeErr(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	if msg := checkPassword(in.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.Avatar == "" {
		in.Avatar = "star"
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.store.SignUp(r.Context(), in.Code, in.Email, hash, in.DisplayName, in.Avatar)
	switch {
	case errors.Is(err, store.ErrInviteInvalid):
		s.wrongInvite(ip)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrEmailTaken):
		writeErr(w, http.StatusConflict, err.Error()+"; sign in instead")
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	s.signups.Fail(ip) // counts successful sign-ups
	s.log.Info("new account", "user", u.ID, "role", u.Role)
	if s.startSession(w, r, u) {
		writeJSON(w, http.StatusCreated, map[string]any{"user": u})
	}
}

// GET /api/auth/me (and /api/admin/me)
func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "email": u.Email})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) { s.authMe(w, r) }

// PATCH /api/auth/me {displayName, avatar, bio}
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	var in profileIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if msg := in.clean(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.Avatar == "" {
		in.Avatar = u.Avatar
	}
	updated, err := s.store.UpdateProfile(r.Context(), u.ID, in.DisplayName, in.Avatar, in.Bio)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

// POST /api/auth/password {current, new}: changes the password and signs out other devices.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, sid := currentUser(r)
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	ip := clientIP(r)
	if !s.limiter.Allowed(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try again in 15 minutes")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, in.Current) {
		s.limiter.Fail(ip)
		writeErr(w, http.StatusBadRequest, "your current password isn't right")
		return
	}
	if msg := checkPassword(in.New); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetPassword(r.Context(), u.ID, hash); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteOtherSessions(r.Context(), u.ID, sid); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/auth/sessions: the devices this account is signed in on.
func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	u, sid := currentUser(r)
	list, err := s.store.Sessions(r.Context(), u.ID, sid)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// DELETE /api/auth/sessions/{id}: signs out one device.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	u, _ := currentUser(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.store.DeleteSession(r.Context(), u.ID, id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/auth/sessions/others: signs out every other device.
func (s *Server) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	u, sid := currentUser(r)
	if err := s.store.DeleteOtherSessions(r.Context(), u.ID, sid); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
