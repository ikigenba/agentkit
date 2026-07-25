package ocr

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestWrapImageJPEGPassesThroughBytes(t *testing.T) {
	// R-UJJV-4KZZ
	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	source.Set(0, 0, color.RGBA{R: 220, G: 15, B: 70, A: 255})
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 91}); err != nil {
		t.Fatal(err)
	}

	pdf, err := wrapImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	dictionary, stream := imageStream(t, pdf)
	if !bytes.Contains(dictionary, []byte("/Filter /DCTDecode")) {
		t.Fatalf("image dictionary lacks DCT filter: %s", dictionary)
	}
	if !bytes.Equal(stream, encoded.Bytes()) {
		t.Fatal("embedded JPEG differs from source bytes")
	}
}

func TestWrapImagePNGLosslessRGB(t *testing.T) {
	// R-UKRR-ICQO
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pixels := []color.RGBA{
		{R: 255, G: 0, B: 17, A: 255},
		{R: 8, G: 200, B: 42, A: 255},
		{R: 19, G: 33, B: 240, A: 255},
		{R: 127, G: 128, B: 129, A: 255},
	}
	for i, pixel := range pixels {
		source.SetRGBA(i%2, i/2, pixel)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	pdf, err := wrapImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	dictionary, stream := imageStream(t, pdf)
	if !bytes.Contains(dictionary, []byte("/Filter /FlateDecode")) {
		t.Fatalf("image dictionary lacks Flate filter: %s", dictionary)
	}
	zr, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	rawRGB, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}

	want := make([]byte, 0, len(pixels)*3)
	for _, pixel := range pixels {
		want = append(want, pixel.R, pixel.G, pixel.B)
	}
	if !bytes.Equal(rawRGB, want) {
		t.Fatalf("inflated RGB = %v, want %v", rawRGB, want)
	}
}

func TestWrapImageMediaBoxUses200DPI(t *testing.T) {
	// R-ULZN-W4HD
	source := image.NewRGBA(image.Rect(0, 0, 1698, 2200))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	pdf, err := wrapImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/MediaBox [0 0 611.28 792.00]")) {
		t.Fatalf("PDF does not contain expected 200 DPI MediaBox")
	}
}

func TestWrapImageXrefOffsetsAndSize(t *testing.T) {
	// R-UOFG-NNYR
	source := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	pdf, err := wrapImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	xrefStart := bytes.Index(pdf, []byte("xref\n"))
	if xrefStart < 0 {
		t.Fatal("missing xref table")
	}
	scanner := bufio.NewScanner(bytes.NewReader(pdf[xrefStart:]))
	if !scanner.Scan() || scanner.Text() != "xref" {
		t.Fatal("invalid xref header")
	}
	if !scanner.Scan() {
		t.Fatal("missing xref range")
	}
	var first, size int
	if _, err := fmt.Sscanf(scanner.Text(), "%d %d", &first, &size); err != nil {
		t.Fatalf("parse xref range: %v", err)
	}
	if first != 0 || size != 6 {
		t.Fatalf("xref range = %d %d, want 0 6", first, size)
	}
	if !scanner.Scan() {
		t.Fatal("missing free xref entry")
	}
	for objectNumber := 1; objectNumber < size; objectNumber++ {
		if !scanner.Scan() {
			t.Fatalf("missing xref entry for object %d", objectNumber)
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			t.Fatalf("invalid xref entry %q", scanner.Text())
		}
		offset, err := strconv.Atoi(fields[0])
		if err != nil {
			t.Fatal(err)
		}
		wantHeader := fmt.Appendf(nil, "%d 0 obj", objectNumber)
		if offset >= len(pdf) || !bytes.HasPrefix(pdf[offset:], wantHeader) {
			t.Fatalf("xref offset %d does not point at %q", offset, wantHeader)
		}
	}
	if !bytes.Contains(pdf, []byte("trailer\n<< /Size 6 /Root 1 0 R >>")) {
		t.Fatal("trailer Size does not match xref object count")
	}
}

func TestWrapImageRejectsUnsupportedDetectedTypes(t *testing.T) {
	// R-UPND-1FPG
	tests := []struct {
		name     string
		input    []byte
		detected string
	}{
		{name: "GIF", input: []byte("GIF89a\x01\x00\x01\x00"), detected: "image/gif"},
		{name: "text", input: []byte("not an image"), detected: "text/plain; charset=utf-8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pdf, err := wrapImage(test.input)
			if err == nil {
				t.Fatal("expected unsupported content error")
			}
			if pdf != nil {
				t.Fatalf("rejected input returned %d PDF bytes", len(pdf))
			}
			if !strings.Contains(err.Error(), test.detected) {
				t.Fatalf("error %q does not name detected type %q", err, test.detected)
			}
		})
	}
}

func imageStream(t *testing.T, pdf []byte) ([]byte, []byte) {
	t.Helper()
	header := regexp.MustCompile(`(?s)(<< /Type /XObject /Subtype /Image .*? /Length ([0-9]+) >>)\nstream\n`)
	match := header.FindSubmatchIndex(pdf)
	if match == nil {
		t.Fatal("missing image XObject stream")
	}
	length, err := strconv.Atoi(string(pdf[match[4]:match[5]]))
	if err != nil {
		t.Fatal(err)
	}
	streamStart := match[1]
	streamEnd := streamStart + length
	if streamEnd > len(pdf) {
		t.Fatalf("declared image stream length %d exceeds PDF", length)
	}
	return pdf[match[2]:match[3]], pdf[streamStart:streamEnd]
}
