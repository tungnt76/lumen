package audiosrc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fake serves canned responses by URL path (plus "?id=" for LibriVox) and records the User-Agent it saw.
func fake(t *testing.T, routes map[string]string) (*Client, *string) {
	var ua string
	var mu sync.Mutex // cover checks arrive concurrently
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ua = r.Header.Get("User-Agent")
		mu.Unlock()
		key := r.URL.Path
		if id := r.URL.Query().Get("id"); id != "" {
			key += "?id=" + id
		}
		body, ok := routes[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New("lumen-test (test@example.com)")
	c.LibriVoxURL = srv.URL + "/librivox"
	c.ArchiveURL = srv.URL
	c.WikisourceURL = srv.URL + "/{lang}/api.php"
	c.RetryDelay = time.Millisecond
	return c, &ua
}

func TestLibriVox(t *testing.T) {
	c, ua := fake(t, map[string]string{"/librivox/?id=52": `{"books":[{"id":"52","title":"Letters of Two Brides",
		"description":"<p>An epistolary novel.</p><p>Read by volunteers.</p>","language":"English","copyright_year":"1902",
		"url_librivox":"http://librivox.org/letters/","url_iarchive":"https://www.archive.org/details/letters_brides_0709_librivox",
		"authors":[{"first_name":"Honoré de","last_name":"Balzac"}],"genres":[{"name":"Epistolary Fiction"}],
		"sections":[
			{"section_number":"2","title":"Letter 2","listen_url":"http://www.archive.org/download/x/02.mp3","playtime":"1500","readers":[{"display_name":"Kara"}]},
			{"section_number":"1","title":"Letter\n 1","listen_url":"https://www.archive.org/download/x/01.mp3","playtime":"1764","readers":[{"display_name":"Kara"},{"display_name":"Michelle"}]},
			{"section_number":"3","title":"Missing","listen_url":"","playtime":"0","readers":[]}]}]}`})

	w, tracks, err := c.LibriVox(context.Background(), 52)
	if err != nil {
		t.Fatal(err)
	}
	if *ua != "lumen-test (test@example.com)" {
		t.Errorf("user agent %q", *ua)
	}
	if w.Kind != "book" || w.Source != "librivox" || w.SourceID != "52" || w.Creator != "Honoré de Balzac" ||
		w.Language != "en" || w.Year != 1902 || w.License != "public_domain" || w.Narrator != "Kara, Michelle" ||
		w.Description != "An epistolary novel. Read by volunteers." || w.RightsNote != "https://librivox.org/letters/" ||
		!strings.HasSuffix(w.CoverURL, "/services/img/letters_brides_0709_librivox") {
		t.Fatalf("work: %+v", w)
	}
	if len(tracks) != 2 || tracks[0].Position != 2 || tracks[0].Audio != "https://www.archive.org/download/x/02.mp3" || tracks[1].DurationSec != 1764 || tracks[1].Title != "Letter 1" {
		t.Fatalf("tracks: %+v", tracks)
	}

	if _, _, err := c.LibriVox(context.Background(), 999); err == nil {
		t.Error("missing book imported")
	}
}

func TestArchive(t *testing.T) {
	c, _ := fake(t, map[string]string{
		"/metadata/musopen-chopin": `{"metadata":{"identifier":"musopen-chopin","title":"Complete Chopin","creator":["Musopen","Aaron Dunn"],
			"date":"2012-05-01","subject":"classical; chopin","licenseurl":"http://creativecommons.org/publicdomain/zero/1.0/"},
			"files":[
				{"name":"b side.mp3","format":"VBR MP3","title":"Nocturne","track":"2/3","length":"4:05"},
				{"name":"a.mp3","format":"VBR MP3","title":"Ballade","track":"1/3","length":"300.4"},
				{"name":"a_64kb.mp3","format":"64Kbps MP3","title":"Ballade","track":"1","length":"300"},
				{"name":"cover.jpg","format":"JPEG"}]}`,
		"/metadata/no-licence": `{"metadata":{"identifier":"no-licence","title":"Mixtape"},"files":[{"name":"a.mp3","format":"VBR MP3"}]}`,
	})
	ctx := context.Background()

	w, tracks, err := c.Archive(ctx, "musopen-chopin", "album", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.License != "cc0" || w.Year != 2012 || w.Creator != "Musopen, Aaron Dunn" || len(w.Genres) != 2 ||
		!strings.HasSuffix(w.RightsNote, "/details/musopen-chopin") {
		t.Fatalf("work: %+v", w)
	}
	// Music takes the original VBR files, in track order.
	if len(tracks) != 2 || tracks[0].Title != "Ballade" || tracks[0].DurationSec != 300 || tracks[1].DurationSec != 245 ||
		!strings.HasSuffix(tracks[1].Audio, "/download/musopen-chopin/b%20side.mp3") {
		t.Fatalf("album tracks: %+v", tracks)
	}
	// Books take the small 64 kbps derivative.
	if _, tracks, _ := c.Archive(ctx, "musopen-chopin", "book", ""); len(tracks) != 1 || !strings.HasSuffix(tracks[0].Audio, "a_64kb.mp3") {
		t.Fatalf("book tracks: %+v", tracks)
	}

	if _, _, err := c.Archive(ctx, "no-licence", "album", ""); err == nil || !strings.Contains(err.Error(), "licence") {
		t.Fatalf("unlicensed item: %v", err)
	}
	if w, _, err := c.Archive(ctx, "no-licence", "album", "cc_by"); err != nil || w.License != "cc_by" {
		t.Fatalf("licence override: %+v %v", w, err)
	}
	if _, _, err := c.Archive(ctx, "missing", "album", ""); err == nil {
		t.Error("missing item imported")
	}
}

func TestLicenseFromURL(t *testing.T) {
	for u, want := range map[string]string{
		"http://creativecommons.org/licenses/publicdomain/":    "public_domain",
		"https://creativecommons.org/publicdomain/mark/1.0/":   "public_domain",
		"http://creativecommons.org/publicdomain/zero/1.0/":    "cc0",
		"https://creativecommons.org/licenses/by/4.0/":         "cc_by",
		"https://creativecommons.org/licenses/by-sa/3.0/":      "cc_by_sa",
		"https://creativecommons.org/licenses/by-nd/4.0/":      "cc_by_nd",
		"https://creativecommons.org/licenses/by-nc/4.0/":      "cc_by_nc",
		"http://creativecommons.org/licenses/by-nc-sa/2.5/":    "cc_by_nc_sa",
		"http://creativecommons.org/licenses/by-nc-nd/3.0/us/": "cc_by_nc_nd",
		"": "",
		"https://rightsstatements.org/vocab/InC/1.0/": "",
	} {
		if got := LicenseFromURL(u); got != want {
			t.Errorf("%q: got %q, want %q", u, got, want)
		}
	}
}

func TestWikisource(t *testing.T) {
	page := `<div class="mw-parser-output"><div id="headerContainer" class="ws-noexport noprint">` +
		`<span id="header&#95;title&#95;text">Chí Phèo</span><span id="header_year_text">&#160;(1941)&#160;</span>` +
		`<span id="header_author_text"><a>Nam Cao</a></span></div>` +
		`<p><span class="pagenum ws-pagenum"><span class="pagenum-inner ws-noexport">&#8203;</span></span>Hắn vừa đi   vừa chửi.<sup class="reference">[1]</sup>
</p><p>— Có hề gì?<br />Giời có của riêng nhà nào?</p><style>.x{}</style><!-- comment -->
<div class="poem"><p>Trăm năm trong cõi người ta,</p></div><ol class="references"><li>note</li></ol></div>`
	c, _ := fake(t, map[string]string{
		"/vi/api.php": `{"parse":{"title":"Chí Phèo","text":` + jsonString(page) + `}}`,
	})
	x, err := c.Wikisource(context.Background(), "vi", "Chí Phèo")
	if err != nil {
		t.Fatal(err)
	}
	want := "Hắn vừa đi vừa chửi.\n\n— Có hề gì?\n\nGiời có của riêng nhà nào?\n\nTrăm năm trong cõi người ta,"
	if x.Body != want {
		t.Errorf("body:\n%q\nwant\n%q", x.Body, want)
	}
	if x.Title != "Chí Phèo" || x.Author != "Nam Cao" || x.Year != 1941 || x.URL != "https://vi.wikisource.org/wiki/Ch%C3%AD_Ph%C3%A8o" {
		t.Errorf("meta: %+v", x)
	}
}

func TestWikisourceErrors(t *testing.T) {
	c, _ := fake(t, map[string]string{
		"/vi/api.php": `{"error":{"code":"missingtitle","info":"The page you specified doesn't exist."}}`,
		"/en/api.php": `{"parse":{"title":"Index","text":"<div class=\"ws-noexport\">only furniture</div>"}}`,
	})
	if _, err := c.Wikisource(context.Background(), "vi", "Nope"); err == nil || !strings.Contains(err.Error(), "doesn't exist") {
		t.Errorf("missing page: %v", err)
	}
	if _, err := c.Wikisource(context.Background(), "en", "Index"); err == nil {
		t.Error("empty page accepted")
	}
}

func TestSeconds(t *testing.T) {
	for in, want := range map[string]int{"1764": 1764, "1764.5": 1765, "29:24": 1764, "1:02:03": 3723, "": 0, "x:1": 0} {
		if got := seconds(in); got != want {
			t.Errorf("%q: got %d, want %d", in, got, want)
		}
	}
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

func TestMusicBrainzArtist(t *testing.T) {
	c, _ := fake(t, map[string]string{
		"/mb/artist/797fbb26": `{"id":"797fbb26","name":"Đen","genres":[{"name":"hip hop","count":1},{"name":"vietnamese hip hop","count":3}],
			"relations":[{"url":{"resource":"https://www.facebook.com/denvau"}},{"url":{"resource":"https://www.youtube.com/channel/UC1"}},
				{"url":{"resource":"https://open.spotify.com/artist/1LE"}},{"url":{"resource":"https://open.qobuz.com/artist/3"}}]}`,
		"/mb/release-group": `{"release-groups":[
			{"id":"rg-new","title":"dongvui harmony","primary-type":"Album","secondary-types":[],"first-release-date":"2022-11-09"},
			{"id":"rg-old","title":"Show Của Đen","primary-type":"Album","secondary-types":["Live"],"first-release-date":"2021-09-30"}]}`,
		"/caa/release-group/rg-new/front-500": `ok`,
	})
	c.MusicBrainzURL = strings.TrimSuffix(c.ArchiveURL, "/") + "/mb"
	c.ArchiveCoverURL = c.ArchiveURL + "/caa"

	works, err := c.MusicBrainzArtist(context.Background(), "797fbb26", []string{"album"})
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 2 || works[0].SourceID != "rg-old" || works[1].SourceID != "rg-new" {
		t.Fatalf("order: %+v", works)
	}
	old, new := works[0], works[1]
	if old.Kind != "album" || old.License != "external" || old.Creator != "Đen" || old.Year != 2021 ||
		old.Genres[0] != "Live album" || old.Genres[1] != "vietnamese hip hop" || old.CoverURL != "" {
		t.Fatalf("live album: %+v", old)
	}
	if !strings.HasSuffix(new.CoverURL, "/caa/release-group/rg-new/front-500") || new.Genres[0] != "Album" {
		t.Fatalf("album: %+v", new)
	}
	var names []string
	for _, l := range new.Links {
		names = append(names, l.Name)
	}
	if strings.Join(names, ",") != "YouTube,Spotify,YouTube channel" ||
		new.Links[0].URL != "https://www.youtube.com/results?search_query=%C4%90en+dongvui+harmony" {
		t.Fatalf("links: %+v", new.Links)
	}
}

func TestRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/flaky" && calls.Add(1) < 3:
			w.WriteHeader(525)
		case r.URL.Path == "/flaky":
			w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New("test")
	c.RetryDelay = time.Millisecond

	var v struct{ OK bool }
	if err := c.getJSON(context.Background(), srv.URL+"/flaky", &v); err != nil || !v.OK || calls.Load() != 3 {
		t.Fatalf("flaky: %v %+v after %d calls", err, v, calls.Load())
	}
	if err := c.getJSON(context.Background(), srv.URL+"/missing", &v); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404 should fail without retry: %v", err)
	}
}
