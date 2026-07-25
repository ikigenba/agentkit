//go:build integration

package ocr

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterLiveScannedPDF(t *testing.T) {
	// R-V0MG-HDDP
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}

	source := scannedTextFixture(t, "SPECIMEN4242")
	client := New(APIKey(apiKey))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	response, err := client.Do(ctx, "scanned-specimen.pdf", source)
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := Transcript(response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "SPECIMEN4242") {
		t.Fatalf("live transcript does not contain SPECIMEN4242: %q", transcript)
	}
}

func scannedTextFixture(t *testing.T, text string) []byte {
	t.Helper()
	if text != "SPECIMEN4242" {
		t.Fatalf("no scanned fixture for %q", text)
	}
	encoded, err := os.ReadFile("testdata/scanned-specimen-4242.png.base64")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}
