package store

import "testing"

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Nguyễn Đức Cường": "nguyen duc cuong",
		"Đen Vâu":          "den vau",
		"Truyện Kiều":      "truyen kieu",
		"Honoré de Balzac": "honore de balzac",
		"Sun Tzu 孙武":       "sun tzu 孙武",
		"Chí Phèo (1941)":  "chi pheo (1941)",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}
