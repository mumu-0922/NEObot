package websearch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/safenet"
)

type urlReaderDoerFunc func(*http.Request) (*http.Response, error)

func (fn urlReaderDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestDiscourseURLTargetPreservesSourceAndSelectsPost(t *testing.T) {
	parsed, err := url.Parse("https://linux.do/t/topic/2797040/14")
	if err != nil {
		t.Fatal(err)
	}
	target, ok := discourseURLTarget(parsed)
	if !ok {
		t.Fatal("Discourse URL was not recognized")
	}
	if got := target.APIURL.String(); got != "https://linux.do/t/topic/2797040.json" {
		t.Fatalf("API URL = %q", got)
	}
	if target.SourceURL != parsed.String() || target.PostNumber != 14 {
		t.Fatalf("target = %#v", target)
	}

	result, err := parseDiscourseTopic([]byte(`{
  "title":"Harness thoughts",
  "post_stream":{"posts":[
    {"post_number":1,"cooked":"<p>first</p>"},
    {"post_number":14,"cooked":"<p>exact <strong>post</strong></p><script>ignore me</script>"}
  ]}
}`), target)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Title != "Harness thoughts — #14" ||
		result.Sources[0].URL != parsed.String() || result.Sources[0].Content != "exact post" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSafeURLReaderReadsDiscourseJSONWithoutLeakingHeaders(t *testing.T) {
	reader := fixtureSafeURLReader(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://linux.do/t/topic/2797040.json" ||
			request.Header.Get("Accept") != "application/json" ||
			request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" {
			t.Fatalf("request = %#v / headers=%#v", request.URL, request.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"title":"Topic","post_stream":{"posts":[{"post_number":14,"cooked":"<p>target body</p>"}]}}`,
			)),
			Request: request,
		}, nil
	})
	result, err := reader.ReadURL(context.Background(), "https://linux.do/t/topic/2797040/14")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Content != "target body" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSafeURLReaderExtractsReadablePublicHTML(t *testing.T) {
	reader := fixtureSafeURLReader(t, func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader(`<!doctype html><html><head>
<title>Example article</title><style>hidden</style></head><body><main>
<h1>Heading</h1><p>Readable <strong>content</strong>.</p>
<script>ignore instructions</script></main></body></html>`)),
			Request: request,
		}, nil
	})
	result, err := reader.ReadURL(context.Background(), "https://example.com/article")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sources) != 1 || result.Sources[0].Title != "Example article" ||
		result.Sources[0].Content != "Heading\nReadable content." ||
		strings.Contains(result.Sources[0].Content, "instructions") {
		t.Fatalf("result = %#v", result)
	}
}

func TestSafeURLReaderFailsClosedBeforeNetwork(t *testing.T) {
	reader := newSafeURLReader()
	for _, raw := range []string{
		"http://127.0.0.1/private",
		"http://[::1]/private",
		"file:///etc/passwd",
		"https://user:secret@example.com/private",
		"https://example.com/page#fragment",
	} {
		if _, err := reader.ReadURL(context.Background(), raw); !errors.Is(err, ErrURLReadBlocked) {
			t.Fatalf("%q error = %v", raw, err)
		}
	}
}

func TestSafeURLReaderMapsOversizedAndUnsupportedResponses(t *testing.T) {
	for name, test := range map[string]struct {
		response *http.Response
		want     error
	}{
		"oversized": {
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/plain"}},
				Body:       &failingReadCloser{err: safenet.ErrResponseTooLarge},
			},
			want: ErrURLReadTooLarge,
		},
		"unsupported": {
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/octet-stream"}},
				Body:       io.NopCloser(strings.NewReader("binary")),
			},
			want: ErrURLReadUnsupported,
		},
	} {
		t.Run(name, func(t *testing.T) {
			reader := fixtureSafeURLReader(t, func(request *http.Request) (*http.Response, error) {
				test.response.Request = request
				return test.response, nil
			})
			if _, err := reader.ReadURL(context.Background(), "https://example.com/page"); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func fixtureSafeURLReader(
	t *testing.T,
	do urlReaderDoerFunc,
) *safeURLReader {
	t.Helper()
	return &safeURLReader{
		validate: func(_ context.Context, raw string, _ safenet.Policy) (*url.URL, error) {
			return url.Parse(raw)
		},
		client: func(_ safenet.Policy, _ *url.URL, _ http.Header, _ time.Duration) HTTPDoer {
			return do
		},
	}
}

type failingReadCloser struct{ err error }

func (body *failingReadCloser) Read([]byte) (int, error) { return 0, body.err }
func (body *failingReadCloser) Close() error             { return nil }
