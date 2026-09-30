// Command reset deletes all Lumen data so you can seed again from scratch.
//
//	go run ./cmd/reset            # drop every Lumen table in DATABASE_URL (asks you to type "delete")
//	go run ./cmd/reset -storage   # also delete uploaded audio (audio/) and drawings (drawings/) from R2
//
// Film videos under hls/ are left alone. Afterwards, start the API or run make migrate to recreate
// the tables, then make seed-all.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

func main() {
	withStorage := flag.Bool("storage", false, "also delete audio/ and drawings/ from the R2 bucket")
	yes := flag.Bool("yes", false, "don't ask (for scripts)")
	flag.Parse()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fail("DATABASE_URL is not set (run through make, or source api/.env)")
	}
	host := dbURL
	if u, err := url.Parse(dbURL); err == nil {
		host = u.Host + u.Path
	}

	fmt.Printf("This deletes ALL Lumen data in %s:\n  films, books, music, drawings, users, sessions and invite codes.\n", host)
	var r2 *storage.R2
	if *withStorage {
		cfg, err := config.Load(false)
		if err != nil {
			fail(err.Error())
		}
		r2 = storage.NewR2(cfg.R2AccountID, cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket)
		fmt.Printf("  Also deletes audio/ and drawings/ in the R2 bucket %s (film videos in hls/ stay).\n", cfg.R2Bucket)
	}
	if !*yes {
		fmt.Print("It can't be undone. Type delete to continue: ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(answer) != "delete" {
			fmt.Println("Cancelled; nothing was deleted.")
			return
		}
	}

	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		fail("database: " + err.Error())
	}
	if err := st.ResetAll(ctx); err != nil {
		st.Close()
		fail("reset: " + err.Error())
	}
	st.Close()
	fmt.Println("Database tables dropped.")
	if r2 != nil {
		for _, prefix := range []string{"audio/", "drawings/"} {
			if err := r2.DeletePrefix(ctx, prefix); err != nil {
				fail("storage " + prefix + ": " + err.Error())
			}
		}
		fmt.Println("Removed audio/ and drawings/ from storage.")
	}

	// Recreate the empty schema right away (UUID ids, all migrations applied).
	st, err = store.Open(ctx, dbURL)
	if err != nil {
		fail("recreate: " + err.Error())
	}
	st.Close()
	fmt.Println("\nFresh, empty database ready. Next: make seed-all   (films, books, music, then accounts)")
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
