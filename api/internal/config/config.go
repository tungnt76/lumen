// Package config loads settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr          string // HTTP listen address, e.g. ":8080"
	DatabaseURL   string // Postgres URL (Neon / Supabase / local)
	TMDBToken     string // TMDB v4 "API Read Access Token"
	SessionSecret []byte // HMAC key for admin session cookies (>= 32 bytes)
	SiteOrigin    string // public site origin, e.g. https://lumen.vercel.app (CSRF check)
	CookieSecure  bool   // false only for local http development

	Language string // TMDB language, default vi-VN
	Region   string // TMDB region for release dates / watch providers, default VN

	R2AccountID   string
	R2Endpoint    string // optional override (local S3 mock); default https://<account>.r2.cloudflarestorage.com
	TMDBBaseURL   string // optional override for testing; default https://api.themoviedb.org/3
	R2AccessKey   string
	R2SecretKey   string
	R2Bucket      string
	MediaBaseURL  string // public URL of the bucket, e.g. https://media.example.com or https://pub-xxx.r2.dev
	WorkDir       string // worker scratch directory
	DeleteSources bool   // worker deletes the uploaded source after a successful encode
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Load reads the configuration. needAPI adds the checks only the HTTP API needs.
func Load(needAPI bool) (Config, error) {
	c := Config{
		Addr:          env("ADDR", ":"+env("PORT", "8080")),
		DatabaseURL:   env("DATABASE_URL", ""),
		TMDBToken:     env("TMDB_TOKEN", ""),
		SessionSecret: []byte(env("SESSION_SECRET", "")),
		SiteOrigin:    strings.TrimRight(env("SITE_ORIGIN", "http://localhost:3000"), "/"),
		CookieSecure:  env("COOKIE_SECURE", "true") != "false",
		Language:      env("TMDB_LANGUAGE", "vi-VN"),
		Region:        env("TMDB_REGION", "VN"),
		R2AccountID:   env("R2_ACCOUNT_ID", ""),
		R2Endpoint:    env("R2_ENDPOINT", ""),
		TMDBBaseURL:   env("TMDB_BASE_URL", "https://api.themoviedb.org/3"),
		R2AccessKey:   env("R2_ACCESS_KEY_ID", ""),
		R2SecretKey:   env("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:      env("R2_BUCKET", ""),
		MediaBaseURL:  strings.TrimRight(env("MEDIA_BASE_URL", ""), "/"),
		WorkDir:       env("WORK_DIR", os.TempDir()),
		DeleteSources: env("DELETE_SOURCES", "true") != "false",
	}

	var missing []string
	req := map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"R2_ACCOUNT_ID":        c.R2AccountID,
		"R2_ACCESS_KEY_ID":     c.R2AccessKey,
		"R2_SECRET_ACCESS_KEY": c.R2SecretKey,
		"R2_BUCKET":            c.R2Bucket,
	}
	if needAPI {
		req["TMDB_TOKEN"] = c.TMDBToken
		req["MEDIA_BASE_URL"] = c.MediaBaseURL
	}
	for k, v := range req {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing env: %s", strings.Join(missing, ", "))
	}
	if needAPI && len(c.SessionSecret) < 32 {
		return c, errors.New("SESSION_SECRET must be at least 32 characters (try: openssl rand -hex 32)")
	}
	return c, nil
}
