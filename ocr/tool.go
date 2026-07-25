package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ikigenba/agentkit"
)

const maxPreviewCharacters = 30_000

var pageBoundary = regexp.MustCompile(`(?m)^<!-- page [1-9][0-9]* -->\n`)

type ocrInput struct {
	FilePath string `json:"file_path" jsonschema:"required"`
}

// Tool returns a model-callable OCR tool whose source paths are confined to
// root and whose extraction artifacts are stored below cacheDir.
func Tool(root, cacheDir string, backend *Client) agentkit.Tool {
	return agentkit.NewTool("OCR", "Extract text from a scanned PDF or image.", func(ctx context.Context, in ocrInput) (string, error) {
		return callTool(ctx, root, cacheDir, backend, in)
	})
}

func callTool(ctx context.Context, root, cacheDir string, backend *Client, in ocrInput) (string, error) {
	if cacheDir == "" {
		return "", errors.New("ocr: cache directory is required")
	}
	if backend == nil {
		return "", errors.New("ocr: backend is required")
	}

	sourcePath, err := confinedSource(root, in.FilePath)
	if err != nil {
		return "", err
	}
	document, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("ocr: read source: %w", err)
	}

	sum := sha256.Sum256(document)
	sourceName := filepath.Base(filepath.Clean(in.FilePath))
	stem := strings.TrimSuffix(sourceName, filepath.Ext(sourceName))
	entryPath := filepath.Join(cacheDir, stem+"-"+hex.EncodeToString(sum[:4]))
	responsePath := filepath.Join(entryPath, "response.json")
	transcriptPath := filepath.Join(entryPath, "transcript.md")

	transcript, hit, err := readCache(responsePath, transcriptPath)
	if err != nil {
		return "", err
	}
	if hit {
		return toolResult(transcriptPath, transcript), nil
	}

	response, err := backend.Do(ctx, sourceName, document)
	if err != nil {
		return "", fmt.Errorf("ocr: extract document: %w", err)
	}
	transcript, err = Transcript(response)
	if err != nil {
		return "", fmt.Errorf("ocr: derive transcript: %w", err)
	}
	if err := writeCache(cacheDir, entryPath, response, transcript); err != nil {
		return "", err
	}
	return toolResult(transcriptPath, transcript), nil
}

func confinedSource(root, requested string) (string, error) {
	if root == "" {
		return "", errors.New("ocr: root is required")
	}
	if requested == "" {
		return "", errors.New("ocr: file_path is required")
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("ocr: resolve root: %w", err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("ocr: make root absolute: %w", err)
	}

	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(resolvedRoot, candidate)
	}
	resolvedSource, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("ocr: resolve source: %w", err)
	}
	resolvedSource, err = filepath.Abs(resolvedSource)
	if err != nil {
		return "", fmt.Errorf("ocr: make source absolute: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedSource)
	if err != nil {
		return "", fmt.Errorf("ocr: compare source with root: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("ocr: file_path %q escapes root", requested)
	}
	return resolvedSource, nil
}

func readCache(responsePath, transcriptPath string) (string, bool, error) {
	transcript, err := os.ReadFile(transcriptPath)
	if err == nil {
		return string(transcript), true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("ocr: read cached transcript: %w", err)
	}

	response, err := os.ReadFile(responsePath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("ocr: read cached response: %w", err)
	}
	derived, err := Transcript(response)
	if err != nil {
		return "", false, fmt.Errorf("ocr: derive cached transcript: %w", err)
	}
	if err := writeFileAtomically(transcriptPath, []byte(derived)); err != nil {
		return "", false, fmt.Errorf("ocr: repair cached transcript: %w", err)
	}
	return derived, true, nil
}

func writeCache(cacheDir, entryPath string, response []byte, transcript string) (err error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("ocr: create cache directory: %w", err)
	}
	temporary, err := os.MkdirTemp(cacheDir, ".ocr-*")
	if err != nil {
		return fmt.Errorf("ocr: create cache entry: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(temporary); err == nil && cleanupErr != nil {
			err = fmt.Errorf("ocr: clean temporary cache entry: %w", cleanupErr)
		}
	}()

	if err := os.WriteFile(filepath.Join(temporary, "response.json"), response, 0o600); err != nil {
		return fmt.Errorf("ocr: write response cache: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "transcript.md"), []byte(transcript), 0o600); err != nil {
		return fmt.Errorf("ocr: write transcript cache: %w", err)
	}
	if err := os.Rename(temporary, entryPath); err != nil {
		if _, statErr := os.Stat(entryPath); statErr == nil {
			return nil
		}
		return fmt.Errorf("ocr: install cache entry: %w", err)
	}
	return nil
}

func writeFileAtomically(path string, contents []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".transcript-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func toolResult(transcriptPath, transcript string) string {
	preview, status := transcriptPreview(transcript)
	return fmt.Sprintf("Transcript: %s\n%s\n\nPreview:\n%s", transcriptPath, status, preview)
}

func transcriptPreview(transcript string) (string, string) {
	totalCharacters := len([]rune(transcript))
	totalPages := len(pageBoundary.FindAllStringIndex(transcript, -1))
	if totalCharacters <= maxPreviewCharacters {
		return transcript, fmt.Sprintf("Status: complete (%d pages, %d characters).", totalPages, totalCharacters)
	}

	boundaries := pageBoundary.FindAllStringIndex(transcript, -1)
	bestByte := 0
	shownPages := 0
	for page := 1; page < len(boundaries); page++ {
		candidate := boundaries[page][0]
		if len([]rune(transcript[:candidate])) > maxPreviewCharacters {
			break
		}
		bestByte = candidate
		shownPages = page
	}
	if bestByte > 0 {
		preview := transcript[:bestByte]
		shownCharacters := len([]rune(preview))
		return preview, fmt.Sprintf(
			"Status: truncated (%d of %d pages shown, %d of %d characters shown).",
			shownPages, totalPages, shownCharacters, totalCharacters,
		)
	}

	runes := []rune(transcript)
	cut := maxPreviewCharacters
	for cut > 0 && runes[cut-1] != '\n' {
		cut--
	}
	if cut == 0 {
		cut = maxPreviewCharacters
	}
	preview := string(runes[:cut])
	return preview, fmt.Sprintf(
		"Status: truncated (page 1 partially shown, %d of %d characters shown across %d pages).",
		len([]rune(preview)), totalCharacters, totalPages,
	)
}
