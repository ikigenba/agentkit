package ocr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var fileStart = regexp.MustCompile(`^<file name="[^"]*">$`)

type openRouterError struct {
	code    string
	message string
}

func (e *openRouterError) Error() string {
	return fmt.Sprintf("ocr: OpenRouter error code %s: %s", e.code, e.message)
}

type responseEnvelope struct {
	Error *struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"error"`
	Choices []struct {
		Message struct {
			Annotations []struct {
				Type string `json:"type"`
				File struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"file"`
			} `json:"annotations"`
		} `json:"message"`
	} `json:"choices"`
}

func responseError(response []byte) error {
	var envelope responseEnvelope
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil
	}
	if envelope.Error == nil {
		return nil
	}
	code := strings.TrimSpace(string(envelope.Error.Code))
	if unquoted, err := strconvUnquote(code); err == nil {
		code = unquoted
	}
	if code == "" || code == "null" {
		code = "unknown"
	}
	return &openRouterError{code: code, message: envelope.Error.Message}
}

func strconvUnquote(value string) (string, error) {
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return "", err
	}
	return decoded, nil
}

// Transcript derives the readable Markdown transcript from a raw OpenRouter
// response body.
func Transcript(response []byte) (string, error) {
	if len(bytes.TrimSpace(response)) == 0 {
		return "", errors.New("ocr: empty OpenRouter response")
	}
	if err := responseError(response); err != nil {
		return "", err
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(response, &envelope); err != nil {
		return "", fmt.Errorf("ocr: decode OpenRouter response: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return "", errors.New("ocr: OpenRouter response has no choices")
	}
	annotations := envelope.Choices[0].Message.Annotations
	if len(annotations) != 1 {
		return "", fmt.Errorf("ocr: OpenRouter response has %d annotations, want exactly 1", len(annotations))
	}
	if annotations[0].Type != "file" {
		return "", fmt.Errorf("ocr: OpenRouter annotation type is %q, want file", annotations[0].Type)
	}

	var pages []string
	totalText := 0
	for _, item := range annotations[0].File.Content {
		if item.Type != "text" {
			continue
		}
		if fileStart.MatchString(item.Text) || item.Text == "</file>" {
			continue
		}
		pages = append(pages, item.Text)
		totalText += len(item.Text)
	}
	if totalText == 0 {
		return "", errors.New("ocr: OpenRouter response contains no transcript text")
	}

	var transcript strings.Builder
	for i, page := range pages {
		if i > 0 {
			transcript.WriteString("\n\n")
		}
		fmt.Fprintf(&transcript, "<!-- page %d -->\n", i+1)
		transcript.WriteString(page)
	}
	return transcript.String(), nil
}
