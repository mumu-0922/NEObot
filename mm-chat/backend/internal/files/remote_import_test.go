package files

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type remoteDoerFunc func(*http.Request) (*http.Response, error)

func (f remoteDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type remoteFetcherFunc func(context.Context, string, int64) (fetchedRemoteFile, error)

func (f remoteFetcherFunc) Fetch(
	ctx context.Context,
	rawURL string,
	maxBytes int64,
) (fetchedRemoteFile, error) {
	return f(ctx, rawURL, maxBytes)
}

func TestSecureRemoteFileFetcherFetchesBoundedIdentityResponse(t *testing.T) {
	var request *http.Request
	finalURL, err := url.Parse("https://cdn.example.test/files/fallback.txt")
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &secureRemoteFileFetcher{doer: remoteDoerFunc(func(req *http.Request) (*http.Response, error) {
		request = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type":        []string{"text/plain; charset=utf-8"},
				"Content-Disposition": []string{`attachment; filename="remote notes.txt"`},
			},
			Body:          io.NopCloser(strings.NewReader("hello remote")),
			ContentLength: int64(len("hello remote")),
			Request:       &http.Request{URL: finalURL},
		}, nil
	})}

	file, err := fetcher.Fetch(context.Background(), "https://example.test/input", 1024)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if request == nil || request.Header.Get("Accept-Encoding") != "identity" {
		t.Fatalf("request = %#v", request)
	}
	if file.filename != "remote notes.txt" || file.mimeType != "text/plain" ||
		string(file.body) != "hello remote" {
		t.Fatalf("file = %#v", file)
	}
}

func TestSecureRemoteFileFetcherRejectsInvalidURLBeforeRequest(t *testing.T) {
	called := false
	fetcher := &secureRemoteFileFetcher{doer: remoteDoerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected request")
	})}

	for _, rawURL := range []string{
		"http://example.test/file.txt",
		"https://user:pass@example.test/file.txt",
		"https://localhost/file.txt",
		"https://service.local/file.txt",
		"https://127.0.0.1/file.txt",
		"https://10.0.0.1/file.txt",
		"https://[::1]/file.txt",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err := fetcher.Fetch(context.Background(), rawURL, 1024)
			assertValidationCode(t, err, "REMOTE_URL")
		})
	}
	if called {
		t.Fatal("remote doer was called for invalid URL")
	}
}

func TestSecureRemoteFileFetcherRejectsUnsafeResponses(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		err      error
		maxBytes int64
		wantCode string
	}{
		{
			name: "timeout", err: context.DeadlineExceeded,
			maxBytes: 8, wantCode: "REMOTE_FETCH_TIMEOUT",
		},
		{
			name: "status", response: remoteResponse(http.StatusForbidden, "denied"),
			maxBytes: 8, wantCode: "REMOTE_UPSTREAM_STATUS",
		},
		{
			name: "compressed", response: func() *http.Response {
				response := remoteResponse(http.StatusOK, "hello")
				response.Header.Set("Content-Encoding", "gzip")
				return response
			}(),
			maxBytes: 8, wantCode: "REMOTE_CONTENT_ENCODING_UNSUPPORTED",
		},
		{
			name: "declared too large", response: func() *http.Response {
				response := remoteResponse(http.StatusOK, "hello")
				response.ContentLength = 9
				return response
			}(),
			maxBytes: 8, wantCode: "FILE_TOO_LARGE",
		},
		{
			name: "streamed too large", response: remoteResponse(http.StatusOK, "123456789"),
			maxBytes: 8, wantCode: "FILE_TOO_LARGE",
		},
		{
			name: "empty", response: remoteResponse(http.StatusOK, ""),
			maxBytes: 8, wantCode: "REMOTE_FILE_EMPTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &secureRemoteFileFetcher{doer: remoteDoerFunc(func(*http.Request) (*http.Response, error) {
				return tt.response, tt.err
			})}
			_, err := fetcher.Fetch(context.Background(), "https://example.test/file.txt", tt.maxBytes)
			assertValidationCode(t, err, tt.wantCode)
		})
	}
}

func TestRemoteHTTPClientRevalidatesRedirects(t *testing.T) {
	client := newRemoteFileHTTPClient()
	previous := &http.Request{URL: mustURL(t, "https://example.test/start")}

	if err := client.CheckRedirect(
		&http.Request{URL: mustURL(t, "https://127.0.0.1/private")},
		[]*http.Request{previous},
	); err == nil {
		t.Fatal("private redirect was accepted")
	}
	if err := client.CheckRedirect(
		&http.Request{URL: mustURL(t, "https://cdn.example.test/file.txt")},
		[]*http.Request{previous},
	); err != nil {
		t.Fatalf("public redirect error = %v", err)
	}
	if err := client.CheckRedirect(
		&http.Request{URL: mustURL(t, "https://cdn.example.test/file.txt")},
		[]*http.Request{previous, previous, previous},
	); err == nil {
		t.Fatal("redirect limit was not enforced")
	}
}

func TestResolvePublicRemoteAddressRejectsMixedDNSAnswers(t *testing.T) {
	lookup := func(
		context.Context,
		string,
		string,
	) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("10.0.0.7"),
		}, nil
	}
	_, err := resolvePublicRemoteAddress(context.Background(), "example.test:443", lookup)
	assertValidationCode(t, err, "REMOTE_URL_BLOCKED")
}

func TestResolvePublicRemoteAddressPinsValidatedAddress(t *testing.T) {
	lookup := func(
		context.Context,
		string,
		string,
	) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	target, err := resolvePublicRemoteAddress(context.Background(), "example.test:443", lookup)
	if err != nil {
		t.Fatalf("resolvePublicRemoteAddress() error = %v", err)
	}
	if target != "93.184.216.34:443" {
		t.Fatalf("target = %q", target)
	}
}

func TestServiceImportRemoteUsesExistingUploadContract(t *testing.T) {
	repo := newFakeRepository()
	store := newFakeObjectStore()
	service := NewService(repo, store)
	service.newID = func() (string, error) { return testFileID, nil }
	service.remoteFetcher = remoteFetcherFunc(func(
		_ context.Context,
		rawURL string,
		maxBytes int64,
	) (fetchedRemoteFile, error) {
		if rawURL != "https://example.test/notes.txt" || maxBytes != 1024 {
			t.Fatalf("fetch input = %q/%d", rawURL, maxBytes)
		}
		return fetchedRemoteFile{
			filename: "notes.txt",
			mimeType: "text/plain",
			body:     []byte("remote body"),
		}, nil
	})

	record, err := service.ImportRemote(context.Background(), RemoteImportInput{
		URL:            "https://example.test/notes.txt",
		Purpose:        "chat",
		ConversationID: "conversation-1",
		ClientFileID:   "client-file-1",
	}, 1024)
	if err != nil {
		t.Fatalf("ImportRemote() error = %v", err)
	}
	if record.ID != testFileID || record.OriginalFilename != "notes.txt" ||
		record.MimeType != "text/plain" || record.ByteSize != int64(len("remote body")) {
		t.Fatalf("record = %#v", record)
	}
	if record.Metadata["conversationId"] != "conversation-1" ||
		record.Metadata["clientFileId"] != "client-file-1" {
		t.Fatalf("metadata = %#v", record.Metadata)
	}
	if _, ok := record.Metadata["url"]; ok {
		t.Fatalf("remote URL persisted in metadata: %#v", record.Metadata)
	}
	if got := string(store.objects[objectKeyFor(testFileID)].payload); got != "remote body" {
		t.Fatalf("stored payload = %q", got)
	}
}

func remoteResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: -1,
	}
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func assertValidationCode(t *testing.T, err error, want string) {
	t.Helper()
	var validationError ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error = %v, want ValidationError containing %q", err, want)
	}
	if !strings.Contains(validationError.Code, want) {
		t.Fatalf("validation code = %q, want containing %q", validationError.Code, want)
	}
}
