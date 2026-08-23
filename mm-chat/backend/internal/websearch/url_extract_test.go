package websearch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"neo-chat/mm-chat/backend/internal/safenet"
)

func TestTavilyExtractReadsExactDiscoursePost(t *testing.T) {
	client := doerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.tavily.com/extract" ||
			request.Header.Get("Authorization") != "Bearer tvly-key" {
			t.Fatalf("request = %s headers=%v", request.URL, request.Header)
		}
		body := decodeRequestBody(t, request)
		if body["urls"] != "https://linux.do/t/topic/2797040.json" ||
			body["extract_depth"] != "basic" || body["format"] != "text" ||
			body["timeout"] != float64(20) {
			t.Fatalf("body = %#v", body)
		}
		return jsonResponse(http.StatusOK, `{
  "results":[{
    "url":"https://linux.do/t/topic/2797040.json",
	"raw_content":"{\"title\":\"Harness thoughts\",`+
			`\"post_stream\":{\"posts\":[{\"post_number\":14,`+
			`\"cooked\":\"<p>exact provider post</p>\"}]}}"
  }],
  "failed_results":[]
}`), nil
	})
	providerValue, err := NewProvider(
		ProviderTavily,
		Config{APIKey: "tvly-key", Client: client},
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := providerValue.(*tavilyProvider)
	provider.validateURL = func(
		_ context.Context,
		raw string,
		_ safenet.Policy,
	) (*url.URL, error) {
		return url.Parse(raw)
	}

	result, err := provider.ExtractURL(
		context.Background(),
		"https://linux.do/t/topic/2797040/14",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Content != "exact provider post" ||
		result.Sources[0].URL != "https://linux.do/t/topic/2797040/14" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceReadURLPrefersExtractorAndFallsBackSafely(t *testing.T) {
	reader := &countingURLReader{result: Result{Sources: []Source{{
		Title: "direct", URL: "https://example.com/direct", Content: "direct content",
	}}}}
	extractor := &extractingProvider{result: Result{Sources: []Source{{
		Title: "extract", URL: "https://example.com/extract", Content: "extract content",
	}}}}
	service := NewService(nil, WithURLReader(reader))
	execution := ActiveExecution{Mode: ExecutionExternal, External: extractor}

	result, err := service.ReadURL(context.Background(), execution, "https://example.com/page")
	if err != nil || len(result.Sources) != 1 || result.Sources[0].Content != "extract content" ||
		extractor.calls != 1 || reader.calls != 0 {
		t.Fatalf("preferred extract result=%#v err=%v calls=%d/%d", result, err, extractor.calls, reader.calls)
	}

	extractor.err = &ProviderError{Provider: ProviderTavily, Code: "EXTRACT_FAILED"}
	result, err = service.ReadURL(context.Background(), execution, "https://example.com/page")
	if err != nil || len(result.Sources) != 1 || result.Sources[0].Content != "direct content" ||
		extractor.calls != 2 || reader.calls != 1 {
		t.Fatalf("fallback result=%#v err=%v calls=%d/%d", result, err, extractor.calls, reader.calls)
	}

	extractor.err = ErrURLReadBlocked
	if _, err := service.ReadURL(
		context.Background(), execution, "http://127.0.0.1/private",
	); !errors.Is(err, ErrURLReadBlocked) || reader.calls != 1 {
		t.Fatalf("blocked error=%v reader calls=%d", err, reader.calls)
	}
}

type extractingProvider struct {
	result Result
	err    error
	calls  int
}

func (provider *extractingProvider) ID() ProviderID { return ProviderTavily }
func (provider *extractingProvider) Search(context.Context, Request) (Result, error) {
	return Result{}, nil
}
func (provider *extractingProvider) ExtractURL(context.Context, string) (Result, error) {
	provider.calls++
	return provider.result, provider.err
}

type countingURLReader struct {
	result Result
	err    error
	calls  int
}

func (reader *countingURLReader) ReadURL(context.Context, string) (Result, error) {
	reader.calls++
	return reader.result, reader.err
}
