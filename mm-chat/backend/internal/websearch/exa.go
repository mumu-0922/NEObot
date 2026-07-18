package websearch

import (
	"context"
	"fmt"
)

const exaBaseURL = "https://api.exa.ai"

const exaRewritingPrompt = "You are tasked with re-writing the following text " +
	"to markdown. Ensure you do not change the meaning or story behind the text.\n\n" +
	"**Respond only the updated markdown text, and no additional text before or after.**"

type exaProvider struct{ client providerClient }

func newExaProvider(config Config) (*exaProvider, error) {
	client, err := newProviderClient(ProviderExa, config, exaBaseURL, "/search", true)
	if err != nil {
		return nil, err
	}
	return &exaProvider{client: client}, nil
}

func (p *exaProvider) ID() ProviderID { return ProviderExa }

func (p *exaProvider) Search(ctx context.Context, input Request) (Result, error) {
	input, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	category := string(input.Scope)
	if input.Scope == ScopeAcademic {
		category = "research paper"
	}
	body := struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Contents any    `json:"contents"`
	}{
		Query: input.Query, Category: category,
		Contents: map[string]any{
			"text": true,
			"summary": map[string]string{
				"query": fmt.Sprintf(
					"Given the following query from the user:\n<query>%s</query>\n\n%s",
					input.Query,
					exaRewritingPrompt,
				),
			},
			"numResults": input.MaxResults * 5,
			"livecrawl":  "auto",
			"extras":     map[string]int{"imageLinks": 3},
		},
	}
	var response struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Text    string `json:"text"`
			Summary string `json:"summary"`
			Extras  struct {
				ImageLinks []string `json:"imageLinks"`
			} `json:"extras"`
		} `json:"results"`
	}
	if err := p.client.postJSON(
		ctx, body, map[string]string{"x-api-key": p.client.apiKey}, &response,
	); err != nil {
		return Result{}, err
	}
	sources := make([]Source, 0, len(response.Results))
	images := make([]Image, 0)
	for _, item := range response.Results {
		content := item.Summary
		if content == "" {
			content = item.Text
		}
		sources = append(sources, Source{
			Title: item.Title, URL: item.URL, Content: content,
		})
		for _, imageURL := range item.Extras.ImageLinks {
			images = append(images, Image{URL: imageURL, Description: item.Text})
		}
	}
	return normalizeResult(sources, images, input.MaxResults), nil
}
