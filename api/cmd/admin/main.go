// Command admin creates (or resets) a studio admin account.
// Run it from your own machine, never expose it over HTTP.
//
//	DATABASE_URL=... go run ./cmd/admin -email you@example.com
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tony/lumen/api/internal/auth"
	"github.com/tony/lumen/api/internal/store"
)

func main() {
	email := flag.String("email", "", "admin email")
	flag.Parse()
	dbURL := os.Getenv("DATABASE_URL")
	if *email == "" || dbURL == "" {
		fmt.Fprintln(os.Stderr, "usage: DATABASE_URL=... go run ./cmd/admin -email you@example.com")
		os.Exit(2)
	}

	fmt.Print("New password (min 6 chars): ")
	pw, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	pw = strings.TrimRight(pw, "\r\n")
	hash, err := auth.HashPassword(pw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Sign-in no longer asks for an authenticator code; the column still needs a value.
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.UpsertAdmin(ctx, *email, hash, secret); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("\nAdmin %s saved. Sign in at /studio with this email and password.\n", strings.ToLower(*email))
	fmt.Println("Re-run this command to reset the password.")
}
