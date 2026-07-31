package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/retry"
)

const (
	defaultBaseURL = "https://openrouter.ai/api/v1"
	defaultModel   = "google/gemini-2.5-flash-lite"
)

// APIKey authenticates an OpenRouter OCR request.
type APIKey string

// Option configures a Client.
type Option func(*Client)

// WithModel sets the inexpensive chat model charged for receiving the parser's
// output. The model does not perform the OCR.
func WithModel(model string) Option {
	return func(c *Client) {
		c.model = model
	}
}

// WithBaseURL points the client at an alternate API root.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// Client sends documents to OpenRouter's file-parser plugin.
type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	clock      retry.Clock
}

// New constructs an OpenRouter OCR client.
func New(apiKey APIKey, opts ...Option) *Client {
	c := &Client{
		apiKey:  string(apiKey),
		baseURL: defaultBaseURL,
		clock:   retry.RealClock{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type openRouterRequest struct {
	Model     string              `json:"model"`
	Messages  []openRouterMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens"`
	Plugins   []openRouterPlugin  `json:"plugins"`
}

type openRouterMessage struct {
	Role    string                  `json:"role"`
	Content []openRouterContentPart `json:"content"`
}

type openRouterContentPart struct {
	Type string         `json:"type"`
	File openRouterFile `json:"file"`
}

type openRouterFile struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

type openRouterPlugin struct {
	ID  string              `json:"id"`
	PDF openRouterPDFPlugin `json:"pdf"`
}

type openRouterPDFPlugin struct {
	Engine string `json:"engine"`
}

// Do sends document to OpenRouter and returns the raw response body. Call
// Transcript to derive readable Markdown from the body.
func (c *Client) Do(ctx context.Context, filename string, document []byte) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("ocr: OpenRouter client is nil: %w", agentkit.ErrInvalidConfig)
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("ocr: OpenRouter API key is absent: %w", agentkit.ErrMissingCredential)
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("ocr: OpenRouter base URL is absent: %w", agentkit.ErrInvalidConfig)
	}
	pdf, err := wrapImage(document)
	if err != nil {
		return nil, fmt.Errorf("ocr: normalize document: %w", err)
	}
	model := c.model
	if model == "" {
		model = defaultModel
	}
	body, err := json.Marshal(openRouterRequest{
		Model: model,
		Messages: []openRouterMessage{{
			Role: "user",
			Content: []openRouterContentPart{{
				Type: "file",
				File: openRouterFile{
					Filename: filename,
					FileData: "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(pdf),
				},
			}},
		}},
		MaxTokens: 1,
		Plugins: []openRouterPlugin{{
			ID:  "file-parser",
			PDF: openRouterPDFPlugin{Engine: "mistral-ocr"},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("ocr: encode OpenRouter request: %w", err)
	}

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	clock := c.clock
	if clock == nil {
		clock = retry.RealClock{}
	}
	return retry.Do(ctx, retry.Policy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		MaxElapsed:  5 * time.Second,
	}, clock, func() ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("ocr: create OpenRouter request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ocr: OpenRouter request: %w", err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("ocr: read OpenRouter response: %w", err)
		}
		if bodyErr := responseError(raw); bodyErr != nil {
			return nil, bodyErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &openRouterError{
				code:    strconv.Itoa(resp.StatusCode),
				message: http.StatusText(resp.StatusCode),
			}
		}
		return raw, nil
	}, func(err error) retry.Decision {
		var providerErr *openRouterError
		if !errors.As(err, &providerErr) {
			return retry.Decision{}
		}
		code, conversionErr := strconv.Atoi(providerErr.code)
		return retry.Decision{
			Retryable: conversionErr == nil &&
				(code == http.StatusRequestTimeout ||
					code == http.StatusTooManyRequests ||
					code >= http.StatusInternalServerError),
		}
	}, nil)
}
