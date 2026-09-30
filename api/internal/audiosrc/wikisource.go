package audiosrc

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Text is a Wikisource page as plain text, ready for text-to-speech.
type Text struct {
	Title  string
	Author string // from the page header, when it has one
	Year   int
	URL    string // the page on Wikisource, for the rights note
	Body   string // paragraphs separated by blank lines
}

// Wikisource fetches a page (e.g. lang "vi", title "Chí Phèo") and returns its readable text,
// without the header box, page numbers, footnote markers or edit links.
func (c *Client) Wikisource(ctx context.Context, lang, title string) (Text, error) {
	api := strings.ReplaceAll(c.WikisourceURL, "{lang}", url.PathEscape(lang))
	var res struct {
		Parse struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"parse"`
		Error *struct {
			Info string `json:"info"`
		} `json:"error"`
	}
	u := api + "?" + url.Values{"action": {"parse"}, "page": {title}, "prop": {"text"},
		"format": {"json"}, "formatversion": {"2"}, "redirects": {"1"}}.Encode()
	if err := c.getJSON(ctx, u, &res); err != nil {
		return Text{}, err
	}
	if res.Error != nil {
		return Text{}, fmt.Errorf("wikisource %s: %q: %s", lang, title, res.Error.Info)
	}
	t, err := pageText(res.Parse.Text)
	if err != nil {
		return Text{}, err
	}
	if strings.TrimSpace(t.Body) == "" {
		return Text{}, fmt.Errorf("wikisource %s: %q has no text (is it an index of chapters? import the chapter pages instead)", lang, title)
	}
	t.Title = res.Parse.Title
	t.URL = "https://" + lang + ".wikisource.org/wiki/" + url.PathEscape(strings.ReplaceAll(res.Parse.Title, " ", "_"))
	return t, nil
}

// skipClasses mark page furniture that shouldn't be read aloud.
var skipClasses = []string{"ws-noexport", "noprint", "mw-editsection", "reference", "references", "pagenum", "ws-pagenum", "navbox", "mw-empty-elt"}

var yearRe = regexp.MustCompile(`\d{4}`)

func pageText(src string) (Text, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return Text{}, err
	}
	var t Text
	findHeader(doc, &t)
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if skip(n) {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		block := n.Type == html.ElementNode && isBlock(n.DataAtom)
		if block {
			b.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if block {
			b.WriteString("\n")
		}
	}
	walk(doc)
	t.Body = tidy(b.String())
	return t, nil
}

// findHeader reads the author and year from the Wikisource header template.
func findHeader(n *html.Node, t *Text) {
	if n.Type == html.ElementNode {
		switch attr(n, "id") {
		case "header_author_text":
			t.Author = strings.TrimSpace(textOf(n))
			return
		case "header_year_text":
			t.Year, _ = strconv.Atoi(yearRe.FindString(textOf(n)))
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		findHeader(c, t)
	}
}

func skip(n *html.Node) bool {
	switch n.Type {
	case html.CommentNode:
		return true
	case html.ElementNode:
	default:
		return false
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Sup, atom.Img, atom.Table:
		return true
	}
	for _, c := range strings.Fields(attr(n, "class")) {
		for _, s := range skipClasses {
			if c == s {
				return true
			}
		}
	}
	return false
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Br, atom.Dd, atom.Dt, atom.Li, atom.Blockquote, atom.Center,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Tr, atom.Pre:
		return true
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.ReplaceAll(b.String(), " ", " ")
}

// tidy trims each line, drops zero-width characters, and separates paragraphs with one blank line.
func tidy(s string) string {
	s = strings.NewReplacer("​", "", " ", " ", "\r", "").Replace(s)
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(spaceRe.ReplaceAllString(line, " "))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n\n")
}
