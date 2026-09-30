package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tony/lumen/api/internal/store"
)

// The demo catalog: public-domain audiobooks and music that play from archive.org (no storage),
// plus link-out cards for a current Vietnamese artist whose music Lumen can't host.
var (
	seedBooks = []int{
		714, // Around the World in Eighty Days (featured)
		314, // Adventures of Sherlock Holmes
		253, // Pride and Prejudice
		175, // A Little Princess
		119, // The Art of War
		193, // Aesop's Fables, Volume 3
	}
	// Internet Archive items, with a clearer title, the composer as creator where the item's own
	// metadata names the uploader, and a public-domain portrait from Wikimedia Commons as cover
	// (archive.org only has a waveform image for these).
	seedAlbums = []struct{ id, title, creator, cover string }{
		{"musopen-chopin", "The Complete Chopin Collection", "Frédéric Chopin · Musopen", commons("e/e8/Frederic_Chopin_photo.jpeg")}, // featured
		{"SymphonyNo.5", "Symphony No. 5", "Ludwig van Beethoven", commons("6/6f/Beethoven.jpg")},
		{"MoonlightSonata_845", "Moonlight Sonata", "Ludwig van Beethoven · Paul Pitman", commons("6/6f/Beethoven.jpg")},
		{"Sonata.no.16InCMajor", "Piano Sonata No. 16 in C major", "Wolfgang Amadeus Mozart", commons("1/1e/Wolfgang-amadeus-mozart_1.jpg")},
		{"musopen-brahms-symphony-premix", "Brahms Symphonies", "Johannes Brahms · Musopen", commons("1/15/JohannesBrahms.jpg")},
	}
	seedArtists = []string{
		"797fbb26-5ba0-4e72-9dc9-2501bf88b5ea", // Đen (Đen Vâu): official links only
	}
	// Read by the local TTS model when one is configured; needs R2 for the audio.
	seedTTS = []string{"Chí Phèo"}
)

// commons is a 960px thumbnail of a Wikimedia Commons file, by its hashed path.
func commons(path string) string {
	return "https://thumb.wikimedia.org/wikipedia/commons/thumb/" + path + "/960px-" + path[strings.LastIndex(path, "/")+1:]
}

func runSeed(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	remove := fs.Bool("remove", false, "delete the demo works instead of adding them")
	refresh := fs.Bool("refresh", false, "re-import works that already exist instead of skipping them")
	fs.Parse(args)
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	c := client()

	if *remove {
		n := 0
		del := func(source, id string) {
			if ok, err := st.DeleteWorkBySource(ctx, source, id); err != nil {
				fmt.Fprintf(os.Stderr, "delete %s %s: %v\n", source, id, err)
			} else if ok {
				n++
			}
		}
		for _, id := range seedBooks {
			del("librivox", strconv.Itoa(id))
		}
		for _, a := range seedAlbums {
			del("archive", a.id)
		}
		for _, mbid := range seedArtists {
			works, err := c.MusicBrainzArtist(ctx, mbid, []string{"album", "ep", "single"})
			if err != nil {
				return err
			}
			for _, w := range works {
				del(w.Source, w.SourceID)
			}
		}
		fmt.Printf("removed %d demo works (AI-voiced books stay; delete them with: import-audio list / delete ID)\n", n)
		return nil
	}

	// Works already in the catalog are skipped without fetching anything, so the seed can be
	// re-run safely; -refresh re-imports them (metadata and tracks, keeping published/featured).
	added, skipped, failed := 0, 0, 0
	exists := func(source, id, title string) bool {
		if *refresh {
			return false
		}
		ok, err := st.WorkExists(ctx, source, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check %s %s: %v\n", source, id, err)
			return false
		}
		if ok {
			skipped++
			fmt.Printf("exists  %s, skipped\n", title)
		}
		return ok
	}
	saveOne := func(label string, w store.Work, tracks []store.Track, err error, featured bool) {
		if err == nil {
			var saved *store.Work
			if saved, err = save(ctx, st, w, tracks, true, featured); err == nil {
				added++
				report(saved)
				return
			}
		}
		failed++
		fmt.Fprintf(os.Stderr, "skip %s: %v\n", label, err)
	}

	for i, id := range seedBooks {
		if exists("librivox", strconv.Itoa(id), fmt.Sprintf("LibriVox book %d", id)) {
			continue
		}
		w, tracks, err := c.LibriVox(ctx, id)
		saveOne(fmt.Sprintf("librivox %d", id), w, tracks, err, i == 0)
	}
	for i, a := range seedAlbums {
		if exists("archive", a.id, a.title) {
			continue
		}
		w, tracks, err := c.Archive(ctx, a.id, store.KindAlbum, "")
		w.Title, w.Creator, w.CoverURL = a.title, a.creator, a.cover
		saveOne("archive "+a.id, w, tracks, err, i == 0)
	}
	for _, mbid := range seedArtists {
		works, err := c.MusicBrainzArtist(ctx, mbid, []string{"album", "ep", "single"})
		if err != nil {
			failed++
			fmt.Fprintln(os.Stderr, "skip musicbrainz:", err)
			continue
		}
		for _, w := range works {
			if exists(w.Source, w.SourceID, fmt.Sprintf("%q by %s", w.Title, w.Creator)) {
				continue
			}
			saveOne(w.Title, w, nil, nil, false)
		}
	}

	if os.Getenv("PIPER_MODEL") != "" || os.Getenv("TTS_CMD") != "" {
		for _, page := range seedTTS {
			if exists("wikisource", "vi:"+page, page+" (AI voice)") {
				continue
			}
			if err := runTTS(ctx, []string{"-lang", "vi", "-publish", page}); err != nil {
				failed++
				fmt.Fprintf(os.Stderr, "skip tts %q: %v\n", page, err)
			} else {
				added++
			}
		}
	} else {
		fmt.Println("\nTip: set PIPER_MODEL (or TTS_CMD) and the R2 keys, then run seed again to add Vietnamese AI-voiced books such as Chí Phèo.")
	}
	fmt.Printf("\n%d added, %d already there (skipped), %d failed.\n", added, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("%d seed imports failed (see above)", failed)
	}
	fmt.Println("\nDemo catalog ready. Remove it with: make seed-audio-remove")
	return nil
}
