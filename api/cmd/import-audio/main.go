// Command import-audio fills the Books and Music catalog from free, legal sources until the
// studio has an upload page. Everything runs on your own machine at no cost.
//
//	go run ./cmd/import-audio librivox [-publish] 52 1234        # LibriVox audiobooks (audio stays on archive.org)
//	go run ./cmd/import-audio archive -kind album [-publish] musopen-chopin
//	go run ./cmd/import-audio tts -lang vi [-publish] "Chí Phèo"  # Wikisource text read by a local TTS model
//	go run ./cmd/import-audio tts -out ./preview "Chí Phèo"       # try a voice: files only, no database or R2
//	go run ./cmd/import-audio local -kind album -title ... -license cc_by -source-url ... a.mp3 b.mp3
//	go run ./cmd/import-audio musicbrainz [-publish] 797fbb26-5ba0-4e72-9dc9-2501bf88b5ea  # link-out cards
//	go run ./cmd/import-audio seed [-remove]                    # demo catalog of free books and music
//	go run ./cmd/import-audio list | publish ID | unpublish ID | feature ID | delete ID
//
// librivox, archive and the catalog commands need DATABASE_URL. tts and local also need the
// R2 settings, since their audio is uploaded to the bucket (audio/{workID}/...).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tony/lumen/api/internal/audiosrc"
	"github.com/tony/lumen/api/internal/config"
	"github.com/tony/lumen/api/internal/encode"
	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

const usage = `usage: import-audio <command> [flags] [args]

commands:
  librivox  ID...           import LibriVox audiobooks by id
  archive   IDENTIFIER...   import Internet Archive audio items (Musopen, netlabels, 78s, ...)
  tts       PAGE|FILE.txt... turn Wikisource pages or text files into an AI-voiced audiobook
  local     FILE...         encode and upload your own audio files as one book or album
  musicbrainz ARTIST_MBID... list an artist's releases with links to official platforms (no audio hosted)
  seed [-remove]            add (or remove) a demo catalog of free audiobooks and music
  list                      show every work with its id and status
  publish | unpublish | feature | unfeature | delete  ID

Run "import-audio <command> -h" for a command's flags.`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "librivox":
		err = runLibriVox(ctx, args)
	case "archive":
		err = runArchive(ctx, args)
	case "tts":
		err = runTTS(ctx, args)
	case "local":
		err = runLocal(ctx, args)
	case "musicbrainz":
		err = runMusicBrainz(ctx, args)
	case "seed":
		err = runSeed(ctx, args)
	case "list":
		err = runList(ctx)
	case "publish", "unpublish", "feature", "unfeature", "delete":
		err = runFlag(ctx, cmd, args)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func openStore(ctx context.Context) (*store.Store, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, errors.New("DATABASE_URL is not set (run through make, or source api/.env)")
	}
	return store.Open(ctx, url)
}

// client identifies the importer to the source sites; Wikimedia asks for contact details.
func client() *audiosrc.Client {
	contact := os.Getenv("IMPORT_CONTACT")
	if contact == "" {
		contact = os.Getenv("SITE_ORIGIN")
	}
	if contact == "" {
		contact = "local development"
	}
	return audiosrc.New("LumenAudioImporter/1.0 (" + contact + ")")
}

// save stores a work with its tracks and, when asked, publishes and features it.
func save(ctx context.Context, st *store.Store, w store.Work, tracks []store.Track, publish, featured bool) (*store.Work, error) {
	saved, err := st.UpsertWork(ctx, w)
	if err != nil {
		return nil, err
	}
	if err := st.ReplaceTracks(ctx, saved.ID, tracks); err != nil {
		return nil, err
	}
	return setFlags(ctx, st, saved.ID, publish, featured)
}

func setFlags(ctx context.Context, st *store.Store, id string, publish, featured bool) (*store.Work, error) {
	var p, f *bool
	if publish {
		p = &publish
	}
	if featured {
		f = &featured
	}
	return st.SetWorkFlags(ctx, id, p, f)
}

func report(w *store.Work) {
	state := "draft (publish with: import-audio publish " + w.ID + ")"
	if w.Published {
		state = "published"
	}
	by := ""
	if w.Creator != "" {
		by = " by " + w.Creator
	}
	fmt.Printf("%s %s %q%s: %d tracks, %s, %s, %s\n", w.ID, w.Kind, w.Title, by, w.TrackCount,
		clock(w.DurationSec), w.License, state)
}

func clock(sec int) string {
	return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
}

// ---- librivox / archive: metadata only, audio stays on archive.org ----

func runLibriVox(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("librivox", flag.ExitOnError)
	publish := fs.Bool("publish", false, "publish right away")
	featured := fs.Bool("featured", false, "feature on the Books tab")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("usage: import-audio librivox [-publish] ID... (the id is in the book's API/RSS link, e.g. librivox.org/rss/52)")
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	c := client()
	failed := 0
	for _, a := range fs.Args() {
		id, err := strconv.Atoi(a)
		if err == nil {
			var w store.Work
			var tracks []store.Track
			if w, tracks, err = c.LibriVox(ctx, id); err == nil {
				var saved *store.Work
				if saved, err = save(ctx, st, w, tracks, *publish, *featured); err == nil {
					report(saved)
					continue
				}
			}
		}
		failed++
		fmt.Fprintf(os.Stderr, "skip librivox %s: %v\n", a, err)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d imports failed", failed, fs.NArg())
	}
	return nil
}

func runArchive(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("archive", flag.ExitOnError)
	kind := fs.String("kind", store.KindAlbum, "album or book")
	license := fs.String("license", "", "override the item's licence (only if you have checked the rights yourself)")
	publish := fs.Bool("publish", false, "publish right away")
	featured := fs.Bool("featured", false, "feature on its tab")
	fs.Parse(args)
	if fs.NArg() == 0 || (*kind != store.KindAlbum && *kind != store.KindBook) {
		return errors.New("usage: import-audio archive [-kind album|book] [-license L] [-publish] IDENTIFIER... (from archive.org/details/IDENTIFIER)")
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	c := client()
	failed := 0
	for _, id := range fs.Args() {
		w, tracks, err := c.Archive(ctx, id, *kind, *license)
		if err == nil {
			var saved *store.Work
			if saved, err = save(ctx, st, w, tracks, *publish, *featured); err == nil {
				report(saved)
				continue
			}
		}
		failed++
		fmt.Fprintf(os.Stderr, "skip archive %s: %v\n", id, err)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d imports failed", failed, fs.NArg())
	}
	return nil
}

// runMusicBrainz imports an artist's discography as cards that link to official platforms.
// Use it for current artists whose music Lumen has no licence to host.
func runMusicBrainz(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("musicbrainz", flag.ExitOnError)
	types := fs.String("types", "album,ep,single", "release types to list")
	publish := fs.Bool("publish", false, "publish right away")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("usage: import-audio musicbrainz [-types album,ep,single] [-publish] ARTIST_MBID... (from musicbrainz.org/artist/MBID)")
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	return importArtists(ctx, st, client(), fs.Args(), strings.Split(*types, ","), *publish)
}

func importArtists(ctx context.Context, st *store.Store, c *audiosrc.Client, mbids, types []string, publish bool) error {
	for _, mbid := range mbids {
		works, err := c.MusicBrainzArtist(ctx, mbid, types)
		if err != nil {
			return err
		}
		for _, w := range works {
			saved, err := save(ctx, st, w, nil, publish, false)
			if err != nil {
				return fmt.Errorf("%s: %w", w.Title, err)
			}
			report(saved)
		}
	}
	return nil
}

// ---- tts / local: audio is made or encoded here and uploaded to R2 ----

// defaultTTS runs Piper. IN is the chapter's text file, OUT the WAV file to write.
const defaultTTS = `piper -m "$PIPER_MODEL" -f "$OUT" < "$IN"`

func runTTS(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tts", flag.ExitOnError)
	lang := fs.String("lang", "vi", "Wikisource language (vi.wikisource.org) and the book's language")
	title := fs.String("title", "", "book title (default: the first page's title)")
	author := fs.String("author", "", "author (default: from the Wikisource page header)")
	year := fs.Int("year", 0, "year first published (default: from the page header)")
	sourceID := fs.String("source-id", "", "stable id for re-imports (default: the first page title)")
	sourceURL := fs.String("source-url", "", "where the text comes from; required for .txt files")
	license := fs.String("license", "public_domain", "licence of the text")
	narrator := fs.String("narrator", "Giọng đọc AI", "shown as the reader")
	tts := fs.String("tts", env("TTS_CMD", defaultTTS), "shell command that reads $IN (text) and writes $OUT (wav)")
	out := fs.String("out", "", "preview mode: write the audio to this folder only (no database, no upload)")
	publish := fs.Bool("publish", false, "publish right away")
	featured := fs.Bool("featured", false, "feature on the Books tab")
	fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New(`usage: import-audio tts [-lang vi] [-title T] [-publish] PAGE|FILE.txt... (one chapter per argument)`)
	}
	if err := needTools("ffmpeg", "ffprobe", "sh"); err != nil {
		return err
	}

	// Fetch every chapter's text first, so a bad page title fails before any slow TTS work.
	type chapter struct{ title, text string }
	var chapters []chapter
	c := client()
	w := store.Work{Kind: store.KindBook, Source: "wikisource", Language: *lang, License: *license,
		Narrator: *narrator, AIVoice: true, Title: *title, Creator: *author, Year: *year, RightsNote: *sourceURL}
	for i, a := range fs.Args() {
		if strings.HasSuffix(strings.ToLower(a), ".txt") {
			b, err := os.ReadFile(a)
			if err != nil {
				return err
			}
			w.Source = "text"
			chapters = append(chapters, chapter{strings.TrimSuffix(filepath.Base(a), filepath.Ext(a)), strings.TrimSpace(string(b))})
			continue
		}
		t, err := c.Wikisource(ctx, *lang, a)
		if err != nil {
			return err
		}
		chapters = append(chapters, chapter{t.Title, t.Body})
		if i == 0 {
			w.Title = firstNonEmpty(w.Title, t.Title)
			w.Creator = firstNonEmpty(w.Creator, t.Author)
			if w.Year == 0 {
				w.Year = t.Year
			}
			w.RightsNote = firstNonEmpty(w.RightsNote, t.URL)
		}
		fmt.Printf("fetched %q (%d characters)\n", t.Title, len([]rune(t.Body)))
	}
	if w.Title == "" {
		w.Title = chapters[0].title
	}
	w.SourceID = firstNonEmpty(*sourceID, *lang+":"+w.Title)
	if w.RightsNote == "" {
		return errors.New("-source-url is required for text files: say where the public-domain text comes from")
	}

	// Speak and encode each chapter into one folder.
	dir, err := os.MkdirTemp("", "lumen-tts-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	encoded := filepath.Join(dir, "encoded")
	if *out != "" {
		encoded = *out
	}
	if err := os.MkdirAll(encoded, 0o755); err != nil {
		return err
	}
	var tracks []store.Track
	for i, ch := range chapters {
		in := filepath.Join(dir, fmt.Sprintf("%03d.txt", i+1))
		wav := filepath.Join(dir, fmt.Sprintf("%03d.wav", i+1))
		if err := os.WriteFile(in, []byte(ch.text+"\n"), 0o644); err != nil {
			return err
		}
		start := time.Now()
		fmt.Printf("speaking %d/%d %q ...\n", i+1, len(chapters), ch.title)
		sh := exec.CommandContext(ctx, "sh", "-c", *tts)
		sh.Env = append(os.Environ(), "IN="+in, "OUT="+wav, "TTS_LANG="+*lang)
		sh.Stdout, sh.Stderr = os.Stdout, os.Stderr
		if err := sh.Run(); err != nil {
			return fmt.Errorf("tts command failed on %q: %w (set -tts or TTS_CMD; see README)", ch.title, err)
		}
		name := fmt.Sprintf("%03d.m4a", i+1)
		secs, err := encode.Audio(ctx, wav, filepath.Join(encoded, name), true)
		if err != nil {
			return err
		}
		os.Remove(wav)
		fmt.Printf("  %s of audio in %s\n", clock(secs), time.Since(start).Round(time.Second))
		tracks = append(tracks, store.Track{Position: i + 1, Title: ch.title, DurationSec: secs, Audio: name})
	}
	if *out != "" {
		fmt.Printf("preview written to %s (nothing was saved or uploaded)\n", *out)
		return nil
	}
	return uploadAndSave(ctx, w, tracks, encoded, *publish, *featured)
}

func runLocal(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("local", flag.ExitOnError)
	kind := fs.String("kind", store.KindAlbum, "album or book")
	title := fs.String("title", "", "album or book title (required)")
	creator := fs.String("creator", "", "artist or author")
	narrator := fs.String("narrator", "", "reader, for books")
	lang := fs.String("lang", "", "language code, e.g. vi or en")
	year := fs.Int("year", 0, "year")
	cover := fs.String("cover", "", "cover image URL")
	license := fs.String("license", "", "public_domain, cc0, cc_by, cc_by_sa, cc_by_nd, cc_by_nc, cc_by_nc_sa, cc_by_nc_nd, licensed or own (required)")
	sourceURL := fs.String("source-url", "", "where the audio comes from, or your note on the permission (required)")
	sourceID := fs.String("source-id", "", "stable id for re-imports (default: the title)")
	aiVoice := fs.Bool("ai-voice", false, "the reading is machine-generated")
	publish := fs.Bool("publish", false, "publish right away")
	featured := fs.Bool("featured", false, "feature on its tab")
	fs.Parse(args)
	if fs.NArg() == 0 || *title == "" || *license == "" || *sourceURL == "" || (*kind != store.KindAlbum && *kind != store.KindBook) {
		return errors.New("usage: import-audio local -kind album|book -title T -license L -source-url U [-creator C] FILE... (tracks in argument order)")
	}
	if err := needTools("ffmpeg", "ffprobe"); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "lumen-local-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	var tracks []store.Track
	for i, f := range fs.Args() {
		name := fmt.Sprintf("%03d.m4a", i+1)
		fmt.Printf("encoding %d/%d %s\n", i+1, fs.NArg(), filepath.Base(f))
		secs, err := encode.Audio(ctx, f, filepath.Join(dir, name), *kind == store.KindBook)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		tracks = append(tracks, store.Track{Position: i + 1, Title: strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)), DurationSec: secs, Audio: name})
	}
	w := store.Work{Kind: *kind, Source: "local", SourceID: firstNonEmpty(*sourceID, *title), Title: *title,
		Creator: *creator, Narrator: *narrator, Language: *lang, Year: *year, CoverURL: *cover,
		License: *license, RightsNote: *sourceURL, AIVoice: *aiVoice}
	return uploadAndSave(ctx, w, tracks, dir, *publish, *featured)
}

// uploadAndSave uploads the encoded files under a fresh audio/{workID}/{stamp}/ prefix (the
// bucket serves them as immutable, so a re-import must not reuse keys), points the tracks at
// them, then removes the files of the previous import.
func uploadAndSave(ctx context.Context, w store.Work, tracks []store.Track, dir string, publish, featured bool) error {
	cfg, err := config.Load(false)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	r2 := storage.NewR2(cfg.R2AccountID, cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket)

	saved, err := st.UpsertWork(ctx, w)
	if err != nil {
		return err
	}
	old, err := st.Tracks(ctx, saved.ID)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("%s%d/", saved.AudioFolder(), time.Now().Unix())
	fmt.Printf("uploading %d files to %s\n", len(tracks), prefix)
	if err := r2.UploadDir(ctx, dir, prefix); err != nil {
		if len(old) == 0 { // don't leave an empty draft behind from a first import
			_ = st.DeleteWork(context.Background(), saved.ID)
		}
		_ = r2.DeletePrefix(context.Background(), prefix)
		return fmt.Errorf("upload: %w", err)
	}
	for i := range tracks {
		tracks[i].Audio = prefix + tracks[i].Audio
	}
	if err := st.ReplaceTracks(ctx, saved.ID, tracks); err != nil {
		return err
	}
	for _, p := range stalePrefixes(old, prefix) {
		if err := r2.DeletePrefix(ctx, p); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not delete old files under %s: %v\n", p, err)
		}
	}
	saved, err = setFlags(ctx, st, saved.ID, publish, featured)
	if err != nil {
		return err
	}
	report(saved)
	return nil
}

// stalePrefixes lists the bucket folders of old tracks that the new import no longer uses.
func stalePrefixes(old []store.Track, current string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range old {
		if strings.Contains(t.Audio, "://") {
			continue // hosted elsewhere
		}
		p := path.Dir(t.Audio) + "/"
		if p != current && strings.HasPrefix(p, "audio/") && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ---- catalog ----

func runList(ctx context.Context) error {
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	works, err := st.AllWorks(ctx)
	if err != nil {
		return err
	}
	if len(works) == 0 {
		fmt.Println("no books or albums yet")
	}
	for i := range works {
		report(&works[i])
	}
	return nil
}

func runFlag(ctx context.Context, cmd string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: import-audio %s ID", cmd)
	}
	id := strings.ToLower(args[0])
	if !uuidRe.MatchString(id) {
		return fmt.Errorf("bad id %q: use the id shown by import-audio list", args[0])
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	w, err := st.GetWork(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no book or album %s (see: import-audio list)", id)
	} else if err != nil {
		return err
	}
	if cmd == "delete" {
		tracks, err := st.Tracks(ctx, id)
		if err != nil {
			return err
		}
		if err := st.DeleteWork(ctx, id); err != nil {
			return err
		}
		fmt.Printf("deleted %s %q\n", id, w.Title)
		if len(stalePrefixes(tracks, "")) > 0 {
			prefix := w.AudioFolder()
			cfg, err := config.Load(false)
			if err == nil {
				r2 := storage.NewR2(cfg.R2AccountID, cfg.R2Endpoint, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Bucket)
				err = r2.DeletePrefix(ctx, prefix)
			}
			if err != nil {
				return fmt.Errorf("its audio is still in R2 under %s: %w", prefix, err)
			}
			fmt.Printf("removed its audio from R2 (%s)\n", prefix)
		}
		return nil
	}
	yes, no := true, false
	var p, f *bool
	switch cmd {
	case "publish":
		p = &yes
	case "unpublish":
		p = &no
	case "feature":
		f = &yes
	case "unfeature":
		f = &no
	}
	w, err = st.SetWorkFlags(ctx, id, p, f)
	if err != nil {
		return err
	}
	if cmd == "publish" && !w.Published {
		return fmt.Errorf("%s has no tracks, so it can't be published", id)
	}
	report(w)
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func needTools(names ...string) error {
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			return fmt.Errorf("%s is not installed (macOS: brew install ffmpeg)", n)
		}
	}
	return nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
