package websearch

import (
	"context"
	"encoding/json"
	"strings"
)

const tavilyBaseURL = "https://api.tavily.com"

type tavilyProvider struct{ client providerClient }

func newTavilyProvider(config Config) (*tavilyProvider, error) {
	client, err := newProviderClient(
		ProviderTavily, config, tavilyBaseURL, "/search", true,
	)
	if err != nil {
		return nil, err
	}
	return &tavilyProvider{client: client}, nil
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
