package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fold lowercases s and strips diacritics, so Vietnamese text matches when typed without
// tones: "Nguyễn Đức Cường" becomes "nguyen duc cuong". Used for catalog search.
func Fold(s string) string {
	s = strings.NewReplacer("đ", "d", "Đ", "d").Replace(s)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(out)
}
