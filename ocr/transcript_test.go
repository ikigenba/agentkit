package ocr

import (
	"os"
	"strings"
	"testing"
)

func TestTranscriptOrdersPagesAndRemovesSentinels(t *testing.T) {
	// R-UTB2-6QXJ
	got, err := Transcript(readFixture(t, "multi-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- page 1 -->\nFirst physical page\n\n<!-- page 2 -->\nSecond physical page"
	if got != want {
		t.Fatalf("Transcript() = %q, want %q", got, want)
	}
	if strings.Contains(got, "<file name=") || strings.Contains(got, "</file>") ||
		strings.Contains(got, "generated reply must be discarded") {
		t.Fatalf("Transcript() retained framing or generated reply: %q", got)
	}
}

func TestTranscriptOmitsLargeImageURL(t *testing.T) {
	// R-UUIY-KIO8
	response := readFixture(t, "large-image.json")
	if len(response) < 20_000 {
		t.Fatalf("fixture is only %d bytes, want an image payload exceeding 20,000", len(response))
	}
	got, err := Transcript(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "data:image") || strings.Contains(got, "AAAA") {
		t.Fatalf("Transcript() retained image payload: %.100q", got)
	}
	if got != "<!-- page 1 -->\nVisible page text" {
		t.Fatalf("Transcript() = %q", got)
	}
}

func TestTranscriptPreservesBlankPageSlot(t *testing.T) {
	// R-UVQU-YAEX
	got, err := Transcript(readFixture(t, "blank-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- page 1 -->\nBefore\n\n<!-- page 2 -->\n \n\n<!-- page 3 -->\nAfter"
	if got != want {
		t.Fatalf("Transcript() = %q, want %q", got, want)
	}
}

func TestTranscriptStripsSentinelsByPattern(t *testing.T) {
	// R-UWYR-C25M
	got, err := Transcript(readFixture(t, "sentinel-lookalike.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<file is literal page text") {
		t.Fatalf("Transcript() discarded sentinel-lookalike page: %q", got)
	}
	if !strings.Contains(got, "<!-- page 1 -->") {
		t.Fatalf("Transcript() omitted page marker: %q", got)
	}
}

func TestTranscriptRejectsEmptyAnnotationsAndText(t *testing.T) {
	// R-UY6N-PTWB
	tests := map[string]string{
		"absent":    `{"choices":[{"message":{}}]}`,
		"empty":     `{"choices":[{"message":{"annotations":[]}}]}`,
		"no text":   `{"choices":[{"message":{"annotations":[{"type":"file","file":{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}}]}}]}`,
		"sentinels": `{"choices":[{"message":{"annotations":[{"type":"file","file":{"content":[{"type":"text","text":"<file name=\"empty.pdf\">"},{"type":"text","text":"</file>"}]}}]}}]}`,
	}
	for name, response := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Transcript([]byte(response))
			if err == nil {
				t.Fatalf("Transcript() = %q, want error", got)
			}
		})
	}
}

func TestTranscriptSurfacesBodyCarriedProviderError(t *testing.T) {
	// R-UZEK-3LN0
	got, err := Transcript(readFixture(t, "error-413.json"))
	if err == nil {
		t.Fatalf("Transcript() = %q, want error", got)
	}
	if !strings.Contains(err.Error(), "413") ||
		!strings.Contains(err.Error(), "Request Entity Too Large") {
		t.Fatalf("error = %q, want provider code and message", err)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
