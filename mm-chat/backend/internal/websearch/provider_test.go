package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func decodeRequestBody(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	defer request.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestNewProviderRejectsUnknownMissingKeyAndUnsafeBaseURL(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		id     ProviderID
		config Config
	}{
		"unknown": {id: "other", config: Config{APIKey: "key"}},
		"missing key": {
			id: ProviderTavily, config: Config{},
		},
		"plain http": {
			id:     ProviderTavily,
			config: Config{APIKey: "key", BaseURL: "http://api.example/search"},
		},
		"private ip": {
			id:     ProviderTavily,
			config: Config{APIKey: "key", BaseURL: "https://127.0.0.1"},
		},
		"localhost": {
			id:     ProviderTavily,
			config: Config{APIKey: "key", BaseURL: "https://localhost"},
		},
		"userinfo": {
			id:     ProviderTavily,
			config: Config{APIKey: "key", BaseURL: "https://user@example.com"},
		},
		"query": {
			id:     ProviderTavily,
			config: Config{APIKey: "key", BaseURL: "https://example.com?target=x"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewProvider(test.id, test.config); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewProvider() error = %v", err)
			}
		})
	}
}

func TestTavilyRequestAndResponseNormalization(t *testing.T) {
	t.Parallel()
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.tavily.com/search" ||
			request.Header.Get("Authorization") != "Bearer tvly-key" ||
			request.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("request = %s headers=%v", request.URL, request.Header)
		}
		body := decodeRequestBody(t, request)
		if body["query"] != "neochat" || body["topic"] != "news" ||
			body["max_results"] != float64(2) || body["include_images"] != true {
			t.Fatalf("body = %#v", body)
		}
		return jsonResponse(http.StatusOK, `{
          "results":[
            {"title":"Neo","url":"https://example.com/neo#part","content":"fallback","raw_content":"markdown"},
            {"title":"duplicate","url":"https://example.com/neo","content":"duplicate"},
            {"title":"private","url":"http://127.0.0.1/internal","content":"skip"}
          ],
          "images":["https://example.com/a.png",{"url":"https://example.com/b.png","description":"B"}]
        }`), nil
	})
	provider, err := NewProvider(
		ProviderTavily, Config{APIKey: "tvly-key", Client: client},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Search(context.Background(), Request{
		Query: `"neo\\chat"`, Scope: ScopeNews, MaxResults: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Content != "markdown" ||
		result.Sources[0].URL != "https://example.com/neo" {
		t.Fatalf("sources = %#v", result.Sources)
	}
	if len(result.Images) != 2 || result.Images[0].URL != "https://example.com/a.png" ||
		result.Images[1].Description != "B" {
		t.Fatalf("images = %#v", result.Images)
	}
}

func TestFirecrawlAllowsMissingKeyAndMapsObjectData(t *testing.T) {
	t.Parallel()
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.firecrawl.dev/v2/search" ||
			request.Header.Get("Authorization") != "" {
			t.Fatalf("request = %s headers=%v", request.URL, request.Header)
		}
		body := decodeRequestBody(t, request)
		if body["limit"] != float64(4) {
			t.Fatalf("body = %#v", body)
		}
		return jsonResponse(http.StatusOK, `{
          "data":{"web":[{"title":"Fire","url":"https://example.com/fire","markdown":"# Fire"}],
          "images":[{"title":"Image","imageUrl":"https://example.com/fire.png"}]}
        }`), nil
	})
	provider, err := NewProvider(ProviderFirecrawl, Config{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Search(context.Background(), Request{
		Query: "neo chat", MaxResults: 4,
	})
	if err != nil || len(result.Sources) != 1 ||
		result.Sources[0].Content != "# Fire" || len(result.Images) != 1 {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

func TestExaUsesAPIKeyHeaderAndLegacyRequestShape(t *testing.T) {
	t.Parallel()
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("x-api-key") != "exa-key" ||
			request.Header.Get("Authorization") != "" {
			t.Fatalf("headers = %v", request.Header)
		}
		body := decodeRequestBody(t, request)
		contents, ok := body["contents"].(map[string]any)
		if !ok || body["category"] != "research paper" ||
			contents["numResults"] != float64(10) || contents["livecrawl"] != "auto" {
			t.Fatalf("body = %#v", body)
		}
		return jsonResponse(http.StatusOK, `{
          "results":[{"title":"Paper","url":"https://example.com/paper","text":"paper text",
          "summary":"paper summary","extras":{"imageLinks":["https://example.com/paper.png"]}}]
        }`), nil
	})
	provider, err := NewProvider(
		ProviderExa, Config{APIKey: "exa-key", Client: client},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Search(context.Background(), Request{
		Query: "neo paper", Scope: ScopeAcademic, MaxResults: 2,
	})
	if err != nil || len(result.Sources) != 1 ||
		result.Sources[0].Content != "paper summary" ||
		len(result.Images) != 1 || result.Images[0].Description != "paper text" {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

func TestBochaMapsImageDescriptionFromMatchingWebResult(t *testing.T) {
	t.Parallel()
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
	          "data":{"webPages":{"value":[{"name":"Bocha","url":"https://example.com/result","snippet":"snippet"}]},
	          "images":{"value":[{
	            "contentUrl":"https://example.com/image.jpg",
	            "hostPageUrl":"https://example.com/result"
	          }]}}
	        }`), nil
	})
	provider, err := NewProvider(
		ProviderBocha, Config{APIKey: "bocha-key", Client: client},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Search(context.Background(), Request{Query: "neo"})
	if err != nil || len(result.Images) != 1 || result.Images[0].Description != "Bocha" {
		t.Fatalf("result = %#v, err=%v", result, err)
	}
}

func TestProviderErrorsAreBoundedAndRedacted(t *testing.T) {
	t.Parallel()
	secret := "secret-upstream-body"
	provider, err := NewProvider(ProviderTavily, Config{
		APIKey: "key",
		Client: doerFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusServiceUnavailable, secret), nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Search(context.Background(), Request{Query: "neo"})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Status != http.StatusServiceUnavailable ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("error = %#v", err)
	}

	provider, err = NewProvider(ProviderTavily, Config{
		APIKey: "key",
		Client: doerFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, strings.Repeat("x", MaxResponseBytes+1)), nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Search(context.Background(), Request{Query: "neo"})
	if !errors.As(err, &providerErr) || providerErr.Code != "RESPONSE_TOO_LARGE" {
		t.Fatalf("oversize error = %#v", err)
	}
}

func TestSearchRequestValidationAndTransportFailure(t *testing.T) {
	t.Parallel()
	provider, err := NewProvider(ProviderTavily, Config{
		APIKey: "key",
		Client: doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("contains secret")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []Request{
		{},
		{Query: strings.Repeat("x", MaxQueryBytes+1)},
		{Query: "neo", MaxResults: MaxResults + 1},
		{Query: "neo", Scope: "other"},
	} {
		if _, err := provider.Search(context.Background(), input); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Search(%#v) error = %v", input, err)
		}
	}
	_, err = provider.Search(context.Background(), Request{Query: "neo"})
	if err == nil || strings.Contains(err.Error(), "contains secret") {
		t.Fatalf("transport error = %v", err)
	}
}

func TestResponseShapeEncodingAndTrailingJSONFailClosed(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		response *http.Response
		code     string
	}{
		"content type": {
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("{}")),
			},
			code: "RESPONSE_CONTENT_TYPE_INVALID",
		},
		"encoding": {
			response: func() *http.Response {
				response := jsonResponse(http.StatusOK, `{}`)
				response.Header.Set("Content-Encoding", "gzip")
				return response
			}(),
			code: "RESPONSE_ENCODING_INVALID",
		},
		"malformed": {
			response: jsonResponse(http.StatusOK, `{`),
			code:     "RESPONSE_DECODE_FAILED",
		},
		"trailing": {
			response: jsonResponse(http.StatusOK, `{}`+`{}`),
			code:     "RESPONSE_DECODE_FAILED",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			provider, err := NewProvider(ProviderTavily, Config{
				APIKey: "key",
				Client: doerFunc(func(*http.Request) (*http.Response, error) {
					return test.response, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Search(context.Background(), Request{Query: "neo"})
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Code != test.code {
				t.Fatalf("error = %#v, want %s", err, test.code)
			}
		})
	}
}

func TestNormalizationBoundsUTF8AndRejectsLocalResults(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("界", MaxSourceContentBytes)
	result := normalizeResult([]Source{
		{URL: "https://example.com/a#fragment", Content: content},
		{URL: "https://localhost/private", Content: "skip"},
	}, []Image{
		{URL: "http://10.0.0.1/image.png"},
		{URL: "https://example.com/image.png", Description: strings.Repeat("d", MaxImageDescription+10)},
	}, 5)
	if len(result.Sources) != 1 || len(result.Sources[0].Content) > MaxSourceContentBytes ||
		!utf8.ValidString(result.Sources[0].Content) ||
		result.Sources[0].URL != "https://example.com/a" {
		t.Fatalf("sources = %#v", result.Sources)
	}
	if len(result.Images) != 1 || len(result.Images[0].Description) != MaxImageDescription {
		t.Fatalf("images = %#v", result.Images)
	}
}

func TestPublicIPPolicy(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]bool{
		"8.8.8.8":     true,
		"1.1.1.1":     true,
		"127.0.0.1":   false,
		"10.0.0.1":    false,
		"169.254.1.1": false,
		"::1":         false,
		"fc00::1":     false,
	} {
		if got := isPublicIP(net.ParseIP(value)); got != want {
			t.Fatalf("isPublicIP(%s) = %v, want %v", value, got, want)
		}
	}
}
