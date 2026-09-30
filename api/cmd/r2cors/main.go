// Command r2cors sets the R2 bucket's CORS rules so browsers on the site can upload straight to
// storage (films, audio, drawings) and play media. Without them the browser blocks every upload.
//
//	go run ./cmd/r2cors                         # SITE_ORIGIN + http://localhost:3000
//	go run ./cmd/r2cors -origin https://x.com   # add more origins (repeatable)
//	go run ./cmd/r2cors -show                   # only print the current rules
//
// The R2 API token needs permission to edit bucket settings (Admin Read & Write); an
// "Object Read & Write" token can upload but not change CORS. Otherwise set the same rules in
// the Cloudflare dashboard: R2 → bucket → Settings → CORS policy.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/storage"
)

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, strings.TrimRight(v, "/")); return nil }

func main() {
	var extra list
	flag.Var(&extra, "origin", "another origin to allow (repeatable)")
	show := flag.Bool("show", false, "only print the current rules")
	flag.Parse()
	cfg, err := config.Load(false)
	if err != nil {
		fail(err)
	}
	r2 := storage.NewR2(cfg.R2AccountID, cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket)
	ctx := context.Background()

	current, err := r2.CORS(ctx)
	if err != nil {
		fail(fmt.Errorf("reading CORS: %w", err))
	}
	fmt.Printf("Bucket %s, current CORS rules:\n", cfg.R2Bucket)
	if len(current) == 0 {
		fmt.Println("  (none: browsers can't upload or read with CORS)")
	}
	for _, l := range current {
		fmt.Println("  " + l)
	}
	if *show {
		return
	}

	origins := []string{}
	seen := map[string]bool{}
	for _, o := range append([]string{cfg.SiteOrigin, "http://localhost:3000"}, extra...) {
		if o != "" && !seen[o] {
			seen[o] = true
			origins = append(origins, o)
		}
	}
	if err := r2.SetCORS(ctx, origins); err != nil {
		fmt.Fprintln(os.Stderr, "\nCouldn't set CORS:", err)
		fmt.Fprintln(os.Stderr, "If this is an access error, the R2 token can't edit bucket settings. Set this policy in the")
		fmt.Fprintln(os.Stderr, "Cloudflare dashboard instead (R2 → "+cfg.R2Bucket+" → Settings → CORS policy):")
		fmt.Fprintf(os.Stderr, "\n[{\"AllowedOrigins\":[\"%s\"],\"AllowedMethods\":[\"GET\",\"HEAD\",\"PUT\"],\"AllowedHeaders\":[\"Content-Type\",\"Range\"],\"ExposeHeaders\":[\"ETag\",\"Content-Length\",\"Content-Range\"],\"MaxAgeSeconds\":3600}]\n", strings.Join(origins, "\",\""))
		os.Exit(1)
	}
	fmt.Println("\nSet. Browsers on these sites can now upload and play media:")
	for _, o := range origins {
		fmt.Println("  " + o)
	}
	fmt.Println("Add your production site with: make r2-cors ARGS=\"-origin https://your-site.vercel.app\"")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
