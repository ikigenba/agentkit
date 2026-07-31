package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
)

const (
	braveWebSearchBaseURL = "https://api.search.brave.com"
	webSearchTimeout      = 10 * time.Second
)

// BraveAPIKey is the toolkit's closed credential type for Brave Search.
type BraveAPIKey string

type webSearchInput struct {
	Query         string   `json:"query" jsonschema:"description=Search query (maximum 400 characters and 50 words)."`
	Count         int      `json:"count,omitempty" jsonschema:"description=Number of results to return from 1 to 20 (default 10)."`
	Offset        int      `json:"offset,omitempty" jsonschema:"description=Zero-based result page offset from 0 to 9 (default 0)."`
	Country       string   `json:"country,omitempty" jsonschema:"description=Two-letter country code or ALL for search results (default US)."`
	SearchLang    string   `json:"search_lang,omitempty" jsonschema:"description=Language code for search results (default en)."`
	Freshness     string   `json:"freshness,omitempty" jsonschema:"description=Freshness filter using pd/pw/pm/py or a YYYY-MM-DDtoYYYY-MM-DD range."`
	Safesearch    string   `json:"safesearch,omitempty" jsonschema:"description=Safe search level using off/moderate/strict (default moderate)."`
	ResultFilter  []string `json:"result_filter,omitempty" jsonschema:"description=Result types to include such as web and news."`
	ExtraSnippets bool     `json:"extra_snippets,omitempty" jsonschema:"description=Whether to return up to five additional excerpts per result."`
	Spellcheck    *bool    `json:"spellcheck,omitempty" jsonschema:"description=Whether Brave may spellcheck the query (default true)."`
}

type webSearchResult struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Description   string   `json:"description"`
	ExtraSnippets []string `json:"extra_snippets,omitempty"`
}

type webSearchSection struct {
	Results []webSearchResult `json:"results"`
}

type webSearchOutput struct {
	Web         []webSearchResult  `json:"web"`
	News        *[]webSearchResult `json:"news,omitempty"`
	Videos      *[]webSearchResult `json:"videos,omitempty"`
	Discussions *[]webSearchResult `json:"discussions,omitempty"`
	FAQ         *[]webSearchResult `json:"faq,omitempty"`
	Infobox     *[]webSearchResult `json:"infobox,omitempty"`
}

// WebSearch returns a tool that searches the web via the Brave Search API.
// An absent key constructs normally and fails when the tool is called.
// It honors WithBaseURL and WithHTTPClient; other options are ignored.
func WebSearch(apiKey BraveAPIKey, opts ...Option) agentkit.Tool {
	cfg := toolConfig(braveWebSearchBaseURL, opts...)
	client := httpx.Client(cfg.httpClient)
	return agentkit.NewTool("WebSearch", "Search the web via the Brave Search API.", func(ctx context.Context, in webSearchInput) (string, error) {
		if apiKey == "" {
			return "", fmt.Errorf("toolkit.WebSearch: Brave Search API key is absent: %w", agentkit.ErrMissingCredential)
		}
		if cfg.baseURL == "" {
			return "", fmt.Errorf("toolkit.WebSearch: base URL is empty: %w", agentkit.ErrInvalidConfig)
		}

		requestURL, err := url.Parse(cfg.baseURL + "/res/v1/web/search")
		if err != nil {
			return "", fmt.Errorf("construct Brave web search URL: %w", err)
		}
		query := requestURL.Query()
		query.Set("q", in.Query)
		query.Set("text_decorations", "0")
		if in.Count != 0 {
			query.Set("count", strconv.Itoa(in.Count))
		}
		if in.Offset != 0 {
			query.Set("offset", strconv.Itoa(in.Offset))
		}
		if in.Country != "" {
			query.Set("country", in.Country)
		}
		if in.SearchLang != "" {
			query.Set("search_lang", in.SearchLang)
		}
		if in.Freshness != "" {
			query.Set("freshness", in.Freshness)
		}
		if in.Safesearch != "" {
			query.Set("safesearch", in.Safesearch)
		}
		if len(in.ResultFilter) > 0 {
			query.Set("result_filter", strings.Join(in.ResultFilter, ","))
		}
		if in.ExtraSnippets {
			query.Set("extra_snippets", "true")
		}
		if in.Spellcheck != nil {
			query.Set("spellcheck", strconv.FormatBool(*in.Spellcheck))
		}
		requestURL.RawQuery = query.Encode()

		requestCtx, cancel := context.WithTimeout(ctx, webSearchTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return "", fmt.Errorf("create Brave web search request: %w", err)
		}
		req.Header.Set("X-Subscription-Token", string(apiKey))
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("Brave web search request: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read Brave web search response: %w", err)
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return "", braveWebSearchHTTPError(resp, body)
		}

		output, err := reduceWebSearchResponse(body)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(output)
		if err != nil {
			return "", fmt.Errorf("encode Brave web search result: %w", err)
		}
		return capOutput(string(encoded)), nil
	})
}

func braveWebSearchHTTPError(resp *http.Response, body []byte) error {
	var errorResponse struct {
		Error struct {
			Detail string `json:"detail"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &errorResponse)

	message := "Brave web search: HTTP status " + resp.Status
	if errorResponse.Error.Detail != "" {
		message += ": " + errorResponse.Error.Detail
	}
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		message += " (Retry-After: " + retryAfter + ")"
	}
	return fmt.Errorf("%s", message)
}

func reduceWebSearchResponse(body []byte) (webSearchOutput, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(body, &response); err != nil {
		return webSearchOutput{}, fmt.Errorf("decode Brave web search response: %w", err)
	}

	web, err := decodeWebSearchSection(response, "web")
	if err != nil {
		return webSearchOutput{}, err
	}
	output := webSearchOutput{Web: web}
	for name, destination := range map[string]**[]webSearchResult{
		"news":        &output.News,
		"videos":      &output.Videos,
		"discussions": &output.Discussions,
		"faq":         &output.FAQ,
		"infobox":     &output.Infobox,
	} {
		if _, ok := response[name]; !ok {
			continue
		}
		results, err := decodeWebSearchSection(response, name)
		if err != nil {
			return webSearchOutput{}, err
		}
		*destination = &results
	}
	return output, nil
}

func decodeWebSearchSection(response map[string]json.RawMessage, name string) ([]webSearchResult, error) {
	raw, ok := response[name]
	if !ok {
		return []webSearchResult{}, nil
	}
	var section webSearchSection
	if err := json.Unmarshal(raw, &section); err != nil {
		return nil, fmt.Errorf("decode Brave web search %s section: %w", name, err)
	}
	if section.Results == nil {
		return []webSearchResult{}, nil
	}
	return section.Results, nil
}
