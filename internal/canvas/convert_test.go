package canvas

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConvertStripsCanvasVerifiersAndConvertsTime(t *testing.T) {
	if got := StripVerifier("https://canvas.example.edu/files/1?verifier=secret&x=1"); got != "https://canvas.example.edu/files/1?x=1" {
		t.Fatalf("StripVerifier() = %q", got)
	}
	got, err := LocalTime("2026-09-04T15:59:00Z", "Asia/Singapore")
	if err != nil || got != "2026-09-04T23:59:00+08:00" {
		t.Fatalf("LocalTime() = %q, %v", got, err)
	}
	markdown, err := ToMarkdown(`<p>Hello <a href="https://canvas.example.edu/files/1?verifier=no">file</a></p>`)
	if err != nil || markdown != "Hello [file](https://canvas.example.edu/files/1)\n" {
		t.Fatalf("ToMarkdown() = %q, %v", markdown, err)
	}
}

func TestSlugTruncatesByRunesNotBytes(t *testing.T) {
	// 50 multibyte runes (3 bytes each) plus an ASCII tail. Byte-based
	// truncation would cut a rune in the middle; rune-based keeps 60 runes.
	value := strings.Repeat("国", 50) + "abcdefghij"
	got := Slug(value)
	want := strings.Repeat("国", 50) + "abcdefghij"[:10]
	if got != want {
		t.Fatalf("Slug() = %q (len %d), want %q", got, len([]rune(got)), want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("Slug() produced invalid UTF-8: %q", got)
	}
}

func TestPlacementDoesNotEscapeRawDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := SafeDestination(root, "../outside.md"); err == nil {
		t.Fatal("SafeDestination accepted traversal")
	}
	got, err := SafeDestination(root, "lectures/week 1.md")
	if err != nil || got != filepath.Join(root, "lectures", "week 1.md") {
		t.Fatalf("SafeDestination() = %q, %v", got, err)
	}
}
