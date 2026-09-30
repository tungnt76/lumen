package httpapi

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tony/lumen/api/internal/store"
)

// Studio API for accounts: users (role, disable) and invite codes.

// GET /api/admin/users?q=&page=
func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	page := pageParam(r, 0)
	users, total, err := s.store.UsersPage(r.Context(), r.URL.Query().Get("q"), adminPageSize, (page-1)*adminPageSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, Page[store.User]{Items: users, Page: page, TotalPages: pages(total, adminPageSize), Total: total})
}

// PATCH /api/admin/users/{id} {role?, disabled?}
func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if in.Role != nil && *in.Role != store.RoleAdmin && *in.Role != store.RoleMember {
		writeErr(w, http.StatusBadRequest, "role must be admin or member")
		return
	}
	me, _ := currentUser(r)
	if id == me.ID && (in.Disabled != nil && *in.Disabled || in.Role != nil && *in.Role != store.RoleAdmin) {
		writeErr(w, http.StatusBadRequest, "you can't remove your own admin access")
		return
	}
	target, err := s.store.UserByID(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	losesAdmin := target.Role == store.RoleAdmin && !target.Disabled &&
		(in.Role != nil && *in.Role != store.RoleAdmin || in.Disabled != nil && *in.Disabled)
	if losesAdmin {
		n, err := s.store.CountAdmins(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if n <= 1 {
			writeErr(w, http.StatusBadRequest, "keep at least one active admin")
			return
		}
	}
	u, err := s.store.SetUserAccess(r.Context(), id, in.Role, in.Disabled)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// GET /api/admin/invites
func (s *Server) adminInvites(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.Invites(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/admin/invites {role, maxUses, expiresInDays, note}
func (s *Server) adminCreateInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role          string `json:"role"`
		MaxUses       int    `json:"maxUses"`
		ExpiresInDays int    `json:"expiresInDays"`
		Note          string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if in.Role == "" {
		in.Role = store.RoleMember
	}
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case in.Role != store.RoleAdmin && in.Role != store.RoleMember:
		writeErr(w, http.StatusBadRequest, "role must be admin or member")
		return
	case in.MaxUses < 1 || in.MaxUses > 1000:
		writeErr(w, http.StatusBadRequest, "uses must be between 1 and 1000")
		return
	case in.ExpiresInDays < 0 || in.ExpiresInDays > 365:
		writeErr(w, http.StatusBadRequest, "expiry must be 0 (never) to 365 days")
		return
	case utf8.RuneCountInString(in.Note) > 200:
		writeErr(w, http.StatusBadRequest, "the note is too long (200 characters max)")
		return
	}
	var expires *time.Time
	if in.ExpiresInDays > 0 {
		t := time.Now().Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour)
		expires = &t
	}
	me, _ := currentUser(r)
	inv, err := s.store.CreateInvite(r.Context(), in.Role, in.MaxUses, expires, in.Note, me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

// PATCH /api/admin/invites/{code} {disabled}
func (s *Server) adminUpdateInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Disabled bool `json:"disabled"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	inv, err := s.store.SetInviteDisabled(r.Context(), r.PathValue("code"), in.Disabled)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

// DELETE /api/admin/invites/{code}
func (s *Server) adminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInvite(r.Context(), r.PathValue("code")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
