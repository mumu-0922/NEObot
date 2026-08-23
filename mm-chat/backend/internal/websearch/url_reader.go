package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/safenet"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	maxURLReadResponseBytes = int64(2 << 20)
	urlReadTimeout          = 20 * time.Second
	urlReadMaxRedirects     = 4
	urlReadUserAgent        = "neo-chat-web-reader/1.0"
)

type safeURLReader struct {
	validate func(context.Context, string, safenet.Policy) (*url.URL, error)
	client   func(safenet.Policy, *url.URL, http.Header, time.Duration) HTTPDoer
}

type discourseTarget struct {
	APIURL     *url.URL
	SourceURL  string
	PostNumber int
}

type discourseTopicResponse struct {
	Title      string `json:"title"`
	PostStream struct {
		Posts []struct {
			PostNumber int    `json:"post_number"`
			Cooked     string `json:"cooked"`
		} `json:"posts"`
	} `json:"post_stream"`
}

func newSafeURLReader() URLReader {
	return &safeURLReader{
		validate: safenet.ValidateEndpoint,
		client: func(
			policy safenet.Policy,
			baseURL *url.URL,
			headers http.Header,
			timeout time.Duration,
		) HTTPDoer {
			return safenet.NewHTTPClient(policy, baseURL, headers, timeout)
		},
	}
}

func (reader *safeURLReader) ReadURL(ctx context.Context, rawURL string) (Result, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > MaxReadURLBytes {
		return Result{}, ErrURLReadInvalid
	}
	policy := urlReadPolicy()
	parsed, err := reader.validate(ctx, rawURL, policy)
	if err != nil {
		return Result{}, ErrURLReadBlocked
	}

	target, isDiscourse := discourseURLTarget(parsed)
	requestURL := parsed
	headers := http.Header{
		"Accept":          {"text/html, text/plain;q=0.9, application/xhtml+xml;q=0.8"},
		"Accept-Encoding": {"identity"},
		"User-Agent":      {urlReadUserAgent},
	}
	if isDiscourse {
		requestURL = target.APIURL
		headers.Set("Accept", "application/json")
	}
	if _, err := reader.validate(ctx, requestURL.String(), policy); err != nil {
		return Result{}, ErrURLReadBlocked
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return Result{}, ErrURLReadInvalid
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := reader.client(policy, requestURL, headers, urlReadTimeout).Do(request)
	if err != nil {
		if errors.Is(err, safenet.ErrResponseTooLarge) {
			return Result{}, ErrURLReadTooLarge
		}
		return Result{}, fmt.Errorf("%w: request", ErrURLReadFailed)
	}
	if response == nil || response.Body == nil {
		return Result{}, ErrURLReadFailed
	}
	defer response.Body.Close()
	if response.Request == nil {
		response.Request = request
	}
	if response.Request.URL == nil {
		return Result{}, ErrURLReadFailed
	}
	if _, err := reader.validate(ctx, response.Request.URL.String(), policy); err != nil {
		return Result{}, ErrURLReadBlocked
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Result{}, fmt.Errorf("%w: status", ErrURLReadFailed)
	}
	encoding := strings.TrimSpace(strings.ToLower(response.Header.Get("Content-Encoding")))
	if encoding != "" && encoding != "identity" {
		return Result{}, ErrURLReadUnsupported
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		if errors.Is(err, safenet.ErrResponseTooLarge) {
			return Result{}, ErrURLReadTooLarge
		}
		return Result{}, fmt.Errorf("%w: response", ErrURLReadFailed)
	}
	if len(body) == 0 {
		return Result{}, ErrURLReadEmpty
	}

	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if isDiscourse {
		if contentType != "" && !strings.Contains(contentType, "json") {
			return Result{}, ErrURLReadUnsupported
		}
		return parseDiscourseTopic(body, target)
	}
	return parsePublicPage(body, contentType, response.Request)
}

func urlReadPolicy() safenet.Policy {
	return safenet.Policy{
		RejectIPLiteral:  true,
		MaxRedirects:     urlReadMaxRedirects,
		MaxResponseBytes: maxURLReadResponseBytes,
	}
}

func discourseURLTarget(input *url.URL) (discourseTarget, bool) {
	if input == nil {
		return discourseTarget{}, false
	}
	segments := strings.Split(strings.Trim(input.Path, "/"), "/")
	if len(segments) < 2 || len(segments) > 4 || segments[0] != "t" {
		return discourseTarget{}, false
	}
	topicIndex := len(segments) - 1
	postNumber := 0
	if len(segments) == 4 {
		topicIndex = 2
		value, err := strconv.Atoi(segments[3])
		if err != nil || value < 1 {
			return discourseTarget{}, false
		}
		postNumber = value
	}
	topicID, err := strconv.ParseInt(segments[topicIndex], 10, 64)
	if err != nil || topicID < 1 {
		return discourseTarget{}, false
	}
	apiURL := *input
	apiURL.Fragment = ""
	apiURL.RawQuery = ""
	apiSegments := append([]string(nil), segments[:topicIndex+1]...)
	apiURL.Path = "/" + strings.Join(apiSegments, "/") + ".json"
	apiURL.RawPath = ""
	sourceURL := *input
	sourceURL.Fragment = ""
	return discourseTarget{
		APIURL: &apiURL, SourceURL: sourceURL.String(), PostNumber: postNumber,
	}, true
}

func parseDiscourseTopic(body []byte, target discourseTarget) (Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var topic discourseTopicResponse
	if err := decoder.Decode(&topic); err != nil {
		return Result{}, fmt.Errorf("%w: discourse json", ErrURLReadFailed)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("%w: discourse trailing json", ErrURLReadFailed)
	}
	if len(topic.PostStream.Posts) == 0 {
		return Result{}, ErrURLReadEmpty
	}
	post := topic.PostStream.Posts[0]
	if target.PostNumber > 0 {
		found := false
		for _, candidate := range topic.PostStream.Posts {
			if candidate.PostNumber == target.PostNumber {
				post = candidate
				found = true
				break
			}
		}
		if !found {
			return Result{}, ErrURLReadEmpty
		}
	}
	content := readableHTMLFragment(post.Cooked)
	if content == "" {
		return Result{}, ErrURLReadEmpty
	}
	title := strings.TrimSpace(topic.Title)
	if post.PostNumber > 0 {
		title = strings.TrimSpace(title + " — #" + strconv.Itoa(post.PostNumber))
	}
	return Result{Sources: []Source{{
		Title: title, URL: target.SourceURL, Content: content,
	}}}, nil
}

func parsePublicPage(body []byte, contentType string, request *http.Request) (Result, error) {
	if contentType == "" {
		contentType = strings.ToLower(http.DetectContentType(body))
	}
	finalURL := ""
	if request != nil && request.URL != nil {
		finalURL = request.URL.String()
	}
	switch {
	case strings.Contains(contentType, "text/plain"):
		if !utf8.Valid(body) {
			return Result{}, ErrURLReadUnsupported
		}
		content := normalizeURLReadText(string(body))
		if content == "" {
			return Result{}, ErrURLReadEmpty
		}
		return Result{Sources: []Source{{Title: finalURL, URL: finalURL, Content: content}}}, nil
	case strings.Contains(contentType, "text/html"), strings.Contains(contentType, "application/xhtml+xml"):
		decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
		if err != nil {
			return Result{}, ErrURLReadUnsupported
		}
		document, err := xhtml.Parse(decoded)
		if err != nil {
			return Result{}, fmt.Errorf("%w: html", ErrURLReadFailed)
		}
		title := htmlDocumentTitle(document)
		content := readableHTMLNode(document)
		if content == "" {
			return Result{}, ErrURLReadEmpty
		}
		if title == "" {
			title = finalURL
		}
		return Result{Sources: []Source{{Title: title, URL: finalURL, Content: content}}}, nil
	default:
		return Result{}, ErrURLReadUnsupported
	}
}

func readableHTMLFragment(fragment string) string {
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return ""
	}
	var output strings.Builder
	for _, node := range nodes {
		appendReadableHTML(&output, node, false)
	}
	return normalizeURLReadText(output.String())
}

func readableHTMLNode(node *xhtml.Node) string {
	var output strings.Builder
	appendReadableHTML(&output, node, false)
	return normalizeURLReadText(output.String())
}

func appendReadableHTML(output *strings.Builder, node *xhtml.Node, hidden bool) {
	if node == nil {
		return
	}
	name := ""
	if node.Type == xhtml.ElementNode {
		name = strings.ToLower(node.Data)
		switch name {
		case "head", "script", "style", "noscript", "template", "svg", "canvas", "iframe", "object":
			hidden = true
		}
	}
	if node.Type == xhtml.TextNode && !hidden {
		output.WriteString(node.Data)
		output.WriteByte(' ')
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendReadableHTML(output, child, hidden)
	}
	if !hidden && isURLReadBlockElement(name) {
		output.WriteByte('\n')
	}
}

func htmlDocumentTitle(node *xhtml.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "title") {
		var title strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.TextNode {
				title.WriteString(child.Data)
			}
		}
		return strings.Join(strings.Fields(title.String()), " ")
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := htmlDocumentTitle(child); title != "" {
			return title
		}
	}
	return ""
}

func isURLReadBlockElement(name string) bool {
	switch name {
	case "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt", "figcaption", "figure", "footer",
		"h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre",
		"section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

func normalizeURLReadText(value string) string {
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, "\u200b", "")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		line = strings.NewReplacer(
			" .", ".", " ,", ",", " ;", ";", " :", ":", " !", "!", " ?", "?",
			" 。", "。", " ，", "，", " ；", "；", " ：", "：", " ！", "！", " ？", "？",
		).Replace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}
