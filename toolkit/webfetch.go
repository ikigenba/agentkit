package toolkit

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/internal/httpx"
	"golang.org/x/net/html"
)

const (
	defaultWebFetchTimeout = 10
	maxWebFetchTimeout     = 120
	maxWebFetchBodyBytes   = 5 * 1024 * 1024
)

type webFetchInput struct {
	URL            string `json:"url" jsonschema:"description=Absolute HTTP or HTTPS URL to fetch."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"description=Whole-request timeout in seconds (default 10; range 1 to 120)."`
}

// WebFetch returns a tool that fetches a URL and returns its content,
// converting HTML to markdown.
// It honors WithHTTPClient; other options are ignored.
func WebFetch(opts ...Option) agentkit.Tool {
	cfg := toolConfig("", opts...)
	client := httpx.Client(cfg.httpClient)
	return agentkit.NewTool("WebFetch", "Fetch a web page, converting HTML to readable markdown.", func(ctx context.Context, in webFetchInput) (string, error) {
		target, err := url.Parse(in.URL)
		if err != nil || target.IsAbs() == false || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
			return "", fmt.Errorf("url must be an absolute http or https URL")
		}

		timeout := in.TimeoutSeconds
		if timeout == 0 {
			timeout = defaultWebFetchTimeout
		} else if timeout < 1 || timeout > maxWebFetchTimeout {
			return "", fmt.Errorf("timeout_seconds must be between 1 and 120")
		}

		requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target.String(), nil)
		if err != nil {
			return "", fmt.Errorf("create web fetch request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("fetch %q: %w", target.String(), err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return "", fmt.Errorf("fetch %q: HTTP status %s", target.String(), resp.Status)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebFetchBodyBytes+1))
		if err != nil {
			return "", fmt.Errorf("read %q: %w", target.String(), err)
		}
		if len(body) > maxWebFetchBodyBytes {
			return "", fmt.Errorf("fetch %q: response body exceeds 5 MB cap", target.String())
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return "", fmt.Errorf("fetch %q: invalid content type %q: %w", target.String(), contentType, err)
		}
		mediaType = strings.ToLower(mediaType)
		switch {
		case mediaType == "text/html":
			markdown, err := htmlToMarkdown(strings.NewReader(string(body)))
			if err != nil {
				return "", fmt.Errorf("convert HTML from %q: %w", target.String(), err)
			}
			return capOutput(markdown), nil
		case strings.HasPrefix(mediaType, "text/"):
			return capOutput(string(body)), nil
		default:
			return "", fmt.Errorf("fetch %q: detected non-text content type %s", target.String(), mediaType)
		}
	})
}

func htmlToMarkdown(input io.Reader) (string, error) {
	document, err := html.Parse(input)
	if err != nil {
		return "", err
	}
	var renderer markdownRenderer
	renderer.render(document, renderContext{})
	return renderer.result(), nil
}

type renderContext struct {
	listMarker string
	inPre      bool
}

type markdownRenderer struct {
	output strings.Builder
}

func (r *markdownRenderer) render(node *html.Node, context renderContext) {
	if node.Type == html.CommentNode {
		return
	}
	if node.Type == html.TextNode {
		if context.inPre {
			r.output.WriteString(node.Data)
		} else {
			r.writeText(node.Data)
		}
		return
	}
	if node.Type != html.ElementNode {
		r.renderChildren(node, context)
		return
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "head", "script", "style", "template":
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level, _ := strconv.Atoi(tag[1:])
		r.ensureNewlines(2)
		r.output.WriteString(strings.Repeat("#", level))
		r.output.WriteByte(' ')
		r.renderChildren(node, context)
		r.ensureNewlines(2)
	case "p", "div":
		r.ensureNewlines(2)
		r.renderChildren(node, context)
		r.ensureNewlines(2)
	case "br":
		r.ensureNewlines(1)
	case "ul":
		r.ensureNewlines(1)
		r.renderChildren(node, renderContext{listMarker: "-"})
		r.ensureNewlines(2)
	case "ol":
		r.ensureNewlines(1)
		r.renderChildren(node, renderContext{listMarker: "1."})
		r.ensureNewlines(2)
	case "li":
		r.ensureNewlines(1)
		marker := context.listMarker
		if marker == "" {
			marker = "-"
		}
		r.output.WriteString(marker)
		r.output.WriteByte(' ')
		r.renderChildren(node, context)
		r.ensureNewlines(1)
	case "pre":
		r.ensureNewlines(2)
		r.output.WriteString("```\n")
		r.renderChildren(node, renderContext{inPre: true})
		r.ensureNewlines(1)
		r.output.WriteString("```")
		r.ensureNewlines(2)
	case "code":
		if context.inPre {
			r.renderChildren(node, context)
			return
		}
		r.output.WriteByte('`')
		r.renderChildren(node, context)
		r.output.WriteByte('`')
	case "a":
		var linkText markdownRenderer
		linkText.renderChildren(node, context)
		text := strings.TrimSpace(linkText.result())
		href := attribute(node, "href")
		if href == "" {
			r.writeText(text)
		} else {
			r.output.WriteByte('[')
			r.output.WriteString(text)
			r.output.WriteString("](")
			r.output.WriteString(href)
			r.output.WriteByte(')')
		}
	case "blockquote":
		var quote markdownRenderer
		quote.renderChildren(node, context)
		r.ensureNewlines(2)
		for _, line := range strings.Split(strings.TrimSpace(quote.result()), "\n") {
			if line == "" {
				r.output.WriteString(">\n")
			} else {
				r.output.WriteString("> ")
				r.output.WriteString(line)
				r.output.WriteByte('\n')
			}
		}
		r.ensureNewlines(2)
	case "tr":
		var cells []string
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
				var cell markdownRenderer
				cell.renderChildren(child, context)
				cells = append(cells, strings.TrimSpace(cell.result()))
			}
		}
		if len(cells) > 0 {
			r.ensureNewlines(1)
			r.output.WriteString(strings.Join(cells, " | "))
			r.ensureNewlines(1)
		}
	case "table":
		r.ensureNewlines(2)
		r.renderChildren(node, context)
		r.ensureNewlines(2)
	default:
		r.renderChildren(node, context)
	}
}

func (r *markdownRenderer) renderChildren(node *html.Node, context renderContext) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.render(child, context)
	}
}

func (r *markdownRenderer) writeText(value string) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return
	}
	leadingSpace := len(value) > 0 && unicode.IsSpace(rune(value[0]))
	if leadingSpace && r.needsSpace() {
		r.output.WriteByte(' ')
	}
	for i, field := range fields {
		if i > 0 {
			r.output.WriteByte(' ')
		}
		r.output.WriteString(field)
	}
	if unicode.IsSpace(rune(value[len(value)-1])) {
		r.output.WriteByte(' ')
	}
}

func (r *markdownRenderer) needsSpace() bool {
	value := r.output.String()
	if value == "" {
		return false
	}
	last := value[len(value)-1]
	return last != ' ' && last != '\n'
}

func (r *markdownRenderer) ensureNewlines(count int) {
	value := r.output.String()
	value = strings.TrimRight(value, " \t")
	r.output.Reset()
	r.output.WriteString(value)
	existing := 0
	for i := len(value) - 1; i >= 0 && value[i] == '\n'; i-- {
		existing++
	}
	for existing < count {
		r.output.WriteByte('\n')
		existing++
	}
}

func (r *markdownRenderer) result() string {
	return strings.TrimSpace(r.output.String())
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}
