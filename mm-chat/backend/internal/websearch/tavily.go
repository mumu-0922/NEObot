package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"neo-chat/mm-chat/backend/internal/safenet"
)

const tavilyBaseURL = "https://api.tavily.com"

type tavilyProvider struct {
	client        providerClient
	extractClient providerClient
	validateURL   func(context.Context, string, safenet.Policy) (*url.URL, error)
}

func newTavilyProvider(config Config) (*tavilyProvider, error) {
	client, err := newProviderClient(
		ProviderTavily, config, tavilyBaseURL, "/search", true,
	)
	if err != nil {
		return nil, err
	}
	extractClient, err := newProviderClient(
		ProviderTavily, config, tavilyBaseURL, "/extract", true,
	)
	if err != nil {
		return nil, err
	}
	return &tavilyProvider{
		client: client, extractClient: extractClient,
		validateURL: safenet.ValidateEndpoint,
	}, nil
}

func (p *tavilyProvider) ID() ProviderID { return ProviderTavily }

func (p *tavilyProvider) Search(ctx context.Context, input Request) (Result, error) {
	input, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	query := strings.NewReplacer("\\", "", "\"", "").Replace(input.Query)
	if strings.TrimSpace(query) == "" {
		return Result{}, ErrInvalidRequest
	}
	body := struct {
		Query                    string `json:"query"`
		SearchDepth              string `json:"search_depth"`
		Topic                    Scope  `json:"topic"`
		MaxResults               int    `json:"max_results"`
		IncludeImages            bool   `json:"include_images"`
		IncludeImageDescriptions bool   `json:"include_image_descriptions"`
		IncludeAnswer            bool   `json:"include_answer"`
		IncludeRawContent        string `json:"include_raw_content"`
	}{
		Query: query, SearchDepth: "advanced", Topic: input.Scope,
		MaxResults: input.MaxResults, IncludeImages: true,
		IncludeImageDescriptions: true, IncludeAnswer: false,
		IncludeRawContent: "markdown",
	}
	var response struct {
		Results []struct {
			Title           string `json:"title"`
			URL             string `json:"url"`
			Content         string `json:"content"`
			RawContent      string `json:"raw_content"`
			RawContentCamel string `json:"rawContent"`
		} `json:"results"`
		Images []json.RawMessage `json:"images"`
	}
	if err := p.client.postJSON(
		ctx, body, bearerHeaders(p.client.apiKey), &response,
	); err != nil {
		return Result{}, err
	}
	sources := make([]Source, 0, len(response.Results))
	for _, item := range response.Results {
		content := item.RawContent
		if content == "" {
			content = item.RawContentCamel
		}
		if content == "" {
			content = item.Content
		}
		sources = append(sources, Source{
			Title: item.Title, URL: item.URL, Content: content,
		})
	}
	images := make([]Image, 0, len(response.Images))
	for _, raw := range response.Images {
		var imageURL string
		if json.Unmarshal(raw, &imageURL) == nil {
			images = append(images, Image{URL: imageURL})
			continue
		}
		var item struct {
			URL         string `json:"url"`
			Description string `json:"description"`
		}
		if json.Unmarshal(raw, &item) == nil {
			images = append(images, Image{
				URL: item.URL, Description: item.Description,
			})
		}
	}
	return normalizeResult(sources, images, input.MaxResults), nil
}

func (p *tavilyProvider) ExtractURL(ctx context.Context, rawURL string) (Result, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || len(rawURL) > MaxReadURLBytes {
		return Result{}, ErrURLReadInvalid
	}
	policy := urlReadPolicy()
	parsed, err := p.validateURL(ctx, rawURL, policy)
	if err != nil {
		return Result{}, ErrURLReadBlocked
	}
	target, discourse := discourseURLTarget(parsed)
	extractURL := parsed.String()
	if discourse {
		extractURL = target.APIURL.String()
	}
	body := struct {
		URLs         string  `json:"urls"`
		ExtractDepth string  `json:"extract_depth"`
		Format       string  `json:"format"`
		Timeout      float64 `json:"timeout"`
	}{
		URLs: extractURL, ExtractDepth: "basic", Format: "text", Timeout: 20,
	}
	var response struct {
		Results []struct {
			URL        string `json:"url"`
			RawContent string `json:"raw_content"`
		} `json:"results"`
		FailedResults []json.RawMessage `json:"failed_results"`
	}
	if err := p.extractClient.postJSON(
		ctx, body, bearerHeaders(p.extractClient.apiKey), &response,
	); err != nil {
		return Result{}, err
	}
	if len(response.Results) == 0 || strings.TrimSpace(response.Results[0].RawContent) == "" {
		return Result{}, &ProviderError{Provider: ProviderTavily, Code: "EXTRACT_FAILED"}
	}
	content := strings.TrimSpace(response.Results[0].RawContent)
	if discourse {
		if result, err := parseDiscourseTopic(extractedJSONBytes(content), target); err == nil {
			return result, nil
		}
		return Result{}, &ProviderError{Provider: ProviderTavily, Code: "EXTRACT_DECODE_FAILED"}
	}
	return Result{Sources: []Source{{
		Title: parsed.String(), URL: parsed.String(), Content: content,
	}}}, nil
}

func extractedJSONBytes(value string) []byte {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		if newline := strings.IndexByte(value, '\n'); newline >= 0 {
			value = value[newline+1:]
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "```"))
	}
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start < 0 || end < start {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(value[start : end+1]))
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil
	}
	return document
}
