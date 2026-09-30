package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/store"
)

func TestAccountsFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	hash, _ := auth.HashPassword("admin password 1")
	admin, err := st.UpsertUser(ctx, "boss@flow.test", hash, "Boss", store.RoleAdmin, "film")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := st.CreateInvite(ctx, store.RoleMember, 1, nil, "flow test", 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{SessionSecret: []byte("0123456789abcdef0123456789abcdef"), SiteOrigin: "https://lumen.test"}
	srv := httptest.NewServer(New(cfg, st, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()

	// Each "browser" has its own cookie jar.
	browser := func() func(method, path, body string, out any) int {
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar}
		return func(method, path, body string, out any) int {
			t.Helper()
			req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
			req.Header.Set("Origin", "https://lumen.test")
			res, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if out != nil && res.StatusCode < 300 {
				json.NewDecoder(res.Body).Decode(out)
			}
			return res.StatusCode
		}
	}
	phone, laptop, adminB := browser(), browser(), browser()

	if code := phone("GET", "/api/auth/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("me signed out: %d", code)
	}
	// Invite check, then sign-up with bad input, then for real.
	if code := phone("POST", "/api/auth/invite", `{"code":"0000000"}`, nil); code != http.StatusNotFound {
		t.Fatalf("bad code: %d", code)
	}
	var check struct {
		Valid bool
		Role  string
	}
	if code := phone("POST", "/api/auth/invite", `{"code":"`+strings.ToLower(inv.Code)+`"}`, &check); code != http.StatusOK || !check.Valid || check.Role != "member" {
		t.Fatalf("good code: %d %+v", code, check)
	}
	signup := func(body string) int { return phone("POST", "/api/auth/signup", body, nil) }
	for _, bad := range []string{
		`{"code":"` + inv.Code + `","email":"not-an-email","password":"long enough","displayName":"Mai"}`,
		`{"code":"` + inv.Code + `","email":"mai@flow.test","password":"short","displayName":"Mai"}`,
		`{"code":"` + inv.Code + `","email":"mai@flow.test","password":"long enough","displayName":" "}`,
		`{"code":"` + inv.Code + `","email":"mai@flow.test","password":"long enough","displayName":"Mai","avatar":"evil"}`,
	} {
		if code := signup(bad); code != http.StatusBadRequest {
			t.Fatalf("bad sign-up %s: %d", bad, code)
		}
	}
	var me struct{ User store.User }
	if code := phone("POST", "/api/auth/signup", `{"code":"`+inv.Code+`","email":"Mai@flow.test","password":"long enough","displayName":"Mai  Trần","avatar":"moon"}`, &me); code != http.StatusCreated ||
		me.User.Role != "member" || me.User.DisplayName != "Mai Trần" || me.User.Avatar != "moon" {
		t.Fatalf("sign up: %d %+v", code, me)
	}
	defer func() { u, _ := st.UserByEmail(ctx, "mai@flow.test"); st.SetUserAccess(ctx, u.ID, nil, nil) }()
	if code := laptop("POST", "/api/auth/signup", `{"code":"`+inv.Code+`","email":"x@flow.test","password":"long enough","displayName":"X"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("single-use code reused: %d", code)
	}

	// Signed in right away; members can't use the studio.
	if code := phone("GET", "/api/auth/me", "", &me); code != http.StatusOK || me.User.Email != "mai@flow.test" {
		t.Fatalf("me: %d %+v", code, me)
	}
	if code := phone("GET", "/api/admin/films", "", nil); code != http.StatusForbidden {
		t.Fatalf("member in studio: %d", code)
	}
	if code := laptop("POST", "/api/admin/login", `{"email":"mai@flow.test","password":"long enough"}`, nil); code != http.StatusForbidden {
		t.Fatalf("member studio sign-in: %d", code)
	}

	// Profile.
	if code := phone("PATCH", "/api/auth/me", `{"displayName":"Mai T.","avatar":"rocket","bio":"Thích sách nói"}`, &me); code != http.StatusOK || me.User.Avatar != "rocket" || me.User.Bio != "Thích sách nói" {
		t.Fatalf("profile: %d %+v", code, me)
	}

	// Second device, then the password change signs it out.
	if code := laptop("POST", "/api/auth/login", `{"email":"MAI@flow.test","password":"long enough"}`, nil); code != http.StatusOK {
		t.Fatalf("laptop sign-in: %d", code)
	}
	var sessions []store.Session
	phone("GET", "/api/auth/sessions", "", &sessions)
	if len(sessions) != 2 {
		t.Fatalf("sessions: %+v", sessions)
	}
	if code := phone("POST", "/api/auth/password", `{"current":"wrong","new":"brand new pw"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("wrong current password: %d", code)
	}
	if code := phone("POST", "/api/auth/password", `{"current":"long enough","new":"brand new pw"}`, nil); code != http.StatusNoContent {
		t.Fatalf("change password: %d", code)
	}
	if code := laptop("GET", "/api/auth/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("other device still signed in: %d", code)
	}
	if code := phone("GET", "/api/auth/me", "", nil); code != http.StatusOK {
		t.Fatalf("this device signed out: %d", code)
	}

	// Admin: users and invites.
	if code := adminB("POST", "/api/admin/login", `{"email":"boss@flow.test","password":"admin password 1"}`, nil); code != http.StatusOK {
		t.Fatalf("admin sign-in: %d", code)
	}
	var created store.Invite
	if code := adminB("POST", "/api/admin/invites", `{"role":"member","maxUses":3,"expiresInDays":7,"note":"team"}`, &created); code != http.StatusCreated || created.MaxUses != 3 || created.ExpiresAt == nil {
		t.Fatalf("create invite: %d %+v", code, created)
	}
	if code := adminB("PATCH", "/api/admin/invites/"+created.Code, `{"disabled":true}`, &created); code != http.StatusOK || created.Usable {
		t.Fatalf("disable invite: %d %+v", code, created)
	}
	if code := adminB("DELETE", "/api/admin/invites/"+created.Code, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete invite: %d", code)
	}
	var users Page[store.User]
	if code := adminB("GET", "/api/admin/users?q=mai", "", &users); code != http.StatusOK || users.Total != 1 {
		t.Fatalf("users: %d %+v", code, users)
	}
	mai := strconv.FormatInt(users.Items[0].ID, 10)
	if code := adminB("PATCH", "/api/admin/users/"+strconv.FormatInt(admin.ID, 10), `{"role":"member"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("admin demoted self: %d", code)
	}
	if code := adminB("PATCH", "/api/admin/users/"+mai, `{"disabled":true}`, nil); code != http.StatusOK {
		t.Fatalf("disable user: %d", code)
	}
	if code := phone("GET", "/api/auth/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("disabled user still signed in: %d", code)
	}
	if code := phone("POST", "/api/auth/login", `{"email":"mai@flow.test","password":"brand new pw"}`, nil); code != http.StatusForbidden {
		t.Fatalf("disabled user signed in: %d", code)
	}
	if code := adminB("PATCH", "/api/admin/users/"+mai, `{"disabled":false,"role":"admin"}`, nil); code != http.StatusOK {
		t.Fatalf("re-enable and promote: %d", code)
	}
	// At most 5 new accounts per IP per hour, even with a code that has uses left.
	many, err := st.CreateInvite(ctx, store.RoleMember, 20, nil, "limit test", 0)
	if err != nil {
		t.Fatal(err)
	}
	fresh := browser()
	codes := []int{}
	for i := 0; i < 5; i++ { // one account already came from this IP (Mai)
		codes = append(codes, fresh("POST", "/api/auth/signup", `{"code":"`+many.Code+`","email":"bulk`+strconv.Itoa(i)+`@flow.test","password":"long enough","displayName":"Bulk"}`, nil))
	}
	if codes[3] != http.StatusCreated || codes[4] != http.StatusTooManyRequests {
		t.Fatalf("sign-up limit per IP: %v", codes)
	}

	if code := adminB("POST", "/api/auth/logout", "", nil); code != http.StatusNoContent {
		t.Fatalf("sign out: %d", code)
	}
	if code := adminB("GET", "/api/admin/users", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("signed out admin: %d", code)
	}
}
