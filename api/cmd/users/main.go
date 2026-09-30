// Command users seeds and manages accounts from your own machine.
//
//	go run ./cmd/users seed [-admin you@example.com]   # admin + demo member + a never-expiring invite code
//	go run ./cmd/users invite [-role member] [-uses 5] [-days 30] [-note "for the team"]
//	go run ./cmd/users list
//	go run ./cmd/users migrate   # apply pending database migrations and list them
//
// Existing accounts are never changed by seed; reset a password with: go run ./cmd/admin -email ...
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: users seed [-admin EMAIL] | invite [-role R] [-uses N] [-days D] [-note T] | list | migrate")
		os.Exit(2)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set (run through make, or source api/.env)")
		os.Exit(2)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer st.Close()

	switch os.Args[1] {
	case "seed":
		err = seed(ctx, st, os.Args[2:])
	case "invite":
		err = invite(ctx, st, os.Args[2:])
	case "list":
		err = list(ctx, st)
	case "migrate": // opening the store already applied anything pending
		var applied []string
		if applied, err = st.AppliedMigrations(ctx); err == nil {
			fmt.Println("Database is up to date. Applied migrations:")
			for _, v := range applied {
				fmt.Println("  " + v)
			}
		}
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// password makes a readable random password like "kite-7Rq2-moss-Vx9d".
func password() string {
	const letters = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	s := string(b)
	return s[:4] + "-" + s[4:8] + "-" + s[8:12] + "-" + s[12:]
}

func seed(ctx context.Context, st *store.Store, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	adminEmail := fs.String("admin", env("SEED_ADMIN_EMAIL", "admin@lumen.local"), "email for the admin account")
	memberEmail := fs.String("member", "member@lumen.local", "email for the demo member account")
	fs.Parse(args)

	type account struct{ email, name, role, avatar string }
	accounts := []account{
		{*adminEmail, "Lumen Admin", store.RoleAdmin, "film"},
		{*memberEmail, "Demo Member", store.RoleMember, "headphones"},
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ROLE\tEMAIL\tPASSWORD")
	for _, a := range accounts {
		if _, err := st.UserByEmail(ctx, a.email); err == nil {
			fmt.Fprintf(w, "%s\t%s\t(exists, unchanged)\n", a.role, a.email)
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		pw := password()
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}
		if _, err := st.UpsertUser(ctx, a.email, hash, a.name, a.role, a.avatar); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.role, strings.ToLower(a.email), pw)
	}
	w.Flush()

	// One reusable member invite that never expires, unless the seed already made one.
	invites, err := st.Invites(ctx)
	if err != nil {
		return err
	}
	var code *store.Invite
	for i := range invites {
		if invites[i].Note == "seed" && invites[i].Usable && invites[i].ExpiresAt == nil {
			code = &invites[i]
			break
		}
	}
	if code == nil {
		if code, err = st.CreateInvite(ctx, store.RoleMember, 10, nil, "seed", 0); err != nil {
			return err
		}
	}
	fmt.Printf("\nInvite code for sign-up: %s  (member, %d/%d used, never expires; link: /signup?code=%s)\n", code.Code, code.Uses, code.MaxUses, code.Code)
	fmt.Println("\nSave these passwords now: they aren't shown again. Sign in at /login; change them under Profile.")
	return nil
}

func invite(ctx context.Context, st *store.Store, args []string) error {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	role := fs.String("role", store.RoleMember, "member or admin")
	uses := fs.Int("uses", 1, "how many sign-ups the code allows")
	days := fs.Int("days", 14, "days until it expires (0 = never)")
	note := fs.String("note", "", "reminder of who it's for")
	fs.Parse(args)
	if *role != store.RoleMember && *role != store.RoleAdmin {
		return fmt.Errorf("role must be member or admin")
	}
	var exp *time.Time
	if *days > 0 {
		t := time.Now().Add(time.Duration(*days) * 24 * time.Hour)
		exp = &t
	}
	inv, err := st.CreateInvite(ctx, *role, *uses, exp, *note, 0)
	if err != nil {
		return err
	}
	expiry := "never expires"
	if exp != nil {
		expiry = "expires " + exp.Format("2006-01-02")
	}
	fmt.Printf("%s  (%s, %d use(s), %s)\n", inv.Code, inv.Role, inv.MaxUses, expiry)
	return nil
}

func list(ctx context.Context, st *store.Store) error {
	users, _, err := st.UsersPage(ctx, "", 500, 0)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tROLE\tEMAIL\tNAME\tSTATUS")
	for _, u := range users {
		status := "active"
		if u.Disabled {
			status = "disabled"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", u.ID, u.Role, u.Email, u.DisplayName, status)
	}
	return w.Flush()
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
