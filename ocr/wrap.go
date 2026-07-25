// Package ocr provides document normalization for OCR.
package ocr

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image/jpeg"
	"image/png"
	"net/http"
)

const wrapDPI = 200.0

// wrapImage embeds a raster image in a minimal one-page PDF. PDF input is
// already normalized and is returned unchanged.
func wrapImage(raw []byte) ([]byte, error) {
	contentType := http.DetectContentType(raw)

	switch contentType {
	case "application/pdf":
		return raw, nil
	case "image/jpeg":
		config, err := jpeg.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("decode JPEG: %w", err)
		}
		return writeImagePDF(raw, config.Width, config.Height, "/DCTDecode")
	case "image/png":
		decoded, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("decode PNG: %w", err)
		}

		bounds := decoded.Bounds()
		rgb := make([]byte, 0, bounds.Dx()*bounds.Dy()*3)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := decoded.At(x, y).RGBA()
				rgb = append(rgb, byte(r>>8), byte(g>>8), byte(b>>8))
			}
		}

		var compressed bytes.Buffer
		zw := zlib.NewWriter(&compressed)
		if _, err := zw.Write(rgb); err != nil {
			return nil, fmt.Errorf("compress PNG pixels: %w", err)
		}
		if err := zw.Close(); err != nil {
			return nil, fmt.Errorf("finish compressing PNG pixels: %w", err)
		}

		return writeImagePDF(compressed.Bytes(), bounds.Dx(), bounds.Dy(), "/FlateDecode")
	default:
		return nil, fmt.Errorf("unsupported detected content type %q", contentType)
	}
}

func writeImagePDF(imageData []byte, width, height int, filter string) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image dimensions %dx%d", width, height)
	}

	pageWidth := float64(width) / wrapDPI * 72
	pageHeight := float64(height) / wrapDPI * 72
	content := fmt.Appendf(nil, "q\n%.2f 0 0 %.2f 0 0 cm\n/Im0 Do\nQ\n", pageWidth, pageHeight)

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		fmt.Appendf(nil, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /XObject << /Im0 5 0 R >> >> /Contents 4 0 R >>", pageWidth, pageHeight),
		streamObject(nil, content),
		streamObject(fmt.Appendf(nil, "/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter %s", width, height, filter), imageData),
	}

	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		objectNumber := i + 1
		offsets[objectNumber] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", objectNumber)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}

	xrefOffset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(offsets))
	pdf.WriteString("0000000000 65535 f \n")
	for objectNumber := 1; objectNumber < len(offsets); objectNumber++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[objectNumber])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset)

	return pdf.Bytes(), nil
}

func streamObject(dictionary, stream []byte) []byte {
	var object bytes.Buffer
	object.WriteString("<< ")
	object.Write(dictionary)
	fmt.Fprintf(&object, " /Length %d >>\nstream\n", len(stream))
	object.Write(stream)
	object.WriteString("\nendstream")
	return object.Bytes()
}
