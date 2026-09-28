// Command worker encodes uploaded films into HLS and publishes the files to R2.
// Run it anywhere ffmpeg is installed (your own PC is fine on the free tier):
//
//	go run ./cmd/worker
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/encode"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load(false)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	r2 := storage.NewR2(cfg.R2AccountID, cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket)

	log.Info("worker started, waiting for uploads")
	for {
		f, err := st.ClaimJob(ctx)
		switch {
		case errors.Is(err, store.ErrNotFound):
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			log.Error("claim job", "err", err)
			time.Sleep(30 * time.Second)
			continue
		}

		log.Info("encoding", "film", f.ID, "title", f.Title)
		if err := process(ctx, cfg, st, r2, f, log); err != nil {
			log.Error("encode failed", "film", f.ID, "err", err)
			// Use a fresh context so the failure is recorded even during shutdown.
			_ = st.FailJob(context.Background(), f.ID, err.Error())
			if ctx.Err() != nil {
				return
			}
			continue
		}
		log.Info("ready", "film", f.ID)
	}
}

func process(ctx context.Context, cfg config.Config, st *store.Store, r2 *storage.R2, f *store.Film, log *slog.Logger) error {
	work, err := os.MkdirTemp(cfg.WorkDir, fmt.Sprintf("lumen-%d-", f.ID))
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	src := filepath.Join(work, "source")
	if err := r2.Download(ctx, storage.SourceKey(f.ID), src); err != nil {
		return fmt.Errorf("download source: %w", err)
	}

	lastSave := time.Time{}
	out := filepath.Join(work, "hls")
	names, err := encode.Run(ctx, src, out, func(pct int) {
		if time.Since(lastSave) > 5*time.Second { // throttle DB writes
			lastSave = time.Now()
			if err := st.SetProgress(ctx, f.ID, pct); err != nil {
				log.Warn("progress", "err", err)
			}
		}
	})
	if err != nil {
		return err
	}

	// Files overwrite any earlier encode at the same keys; subtitles are left untouched.
	if err := r2.UploadDir(ctx, out, storage.HLSPrefix(f.ID)); err != nil {
		return fmt.Errorf("upload hls: %w", err)
	}
	if err := st.FinishJob(ctx, f.ID, names); err != nil {
		return err
	}
	if cfg.DeleteSources {
		if err := r2.DeletePrefix(ctx, storage.SourcePrefix(f.ID)); err != nil {
			log.Warn("delete source", "err", err)
		}
	}
	return nil
}
