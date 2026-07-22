package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixedResolver struct {
	execution ActiveExecution
	err       error
	calls     int
}

type fixedModeResolver struct {
	external       ActiveExecution
	builtIn        ActiveExecution
	externalCalls  int
	builtInCalls   int
	builtInRequest ModelBuiltInResolutionRequest
}

func (r *fixedModeResolver) ResolveActive(ctx context.Context) (ActiveExecution, error) {
	return r.ResolveExternal(ctx)
}

func (r *fixedModeResolver) ResolveExternal(context.Context) (ActiveExecution, error) {
	r.externalCalls++
	return r.external, nil
}

func (r *fixedModeResolver) ResolveModelBuiltIn(
	_ context.Context,
	request ModelBuiltInResolutionRequest,
) (ActiveExecution, error) {
	r.builtInCalls++
	r.builtInRequest = request
	return r.builtIn, nil
}

func (r *fixedResolver) ResolveActive(context.Context) (ActiveExecution, error) {
	r.calls++
	return r.execution, r.err
}

type searchProbe struct {
	id     ProviderID
	result Result
	err    error
	calls  []Request
}

func (p *searchProbe) ID() ProviderID { return p.id }

func (p *searchProbe) Search(_ context.Context, request Request) (Result, error) {
	p.calls = append(p.calls, request)
	return p.result, p.err
}

func TestServiceExecutesExactlyOneResolvedExternalProvider(t *testing.T) {
	provider := &searchProbe{
		id: ProviderTavily,
		result: Result{Sources: []Source{{
			Title: "Fixture", URL: "https://example.test/result", Content: "answer",
		}}},
	}
	resolver := &fixedResolver{execution: ActiveExecution{
		Mode: ExecutionExternal, External: provider,
	}}
	service := NewService(resolver)

	result, err := service.Search(context.Background(), Request{
		Query: " fixture query ", Scope: ScopeNews, MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if resolver.calls != 1 || len(provider.calls) != 1 {
		t.Fatalf("resolver/provider calls = %d/%d, want 1/1", resolver.calls, len(provider.calls))
	}
	if provider.calls[0].Query != "fixture query" || provider.calls[0].MaxResults != 3 {
		t.Fatalf("provider request = %#v", provider.calls[0])
	}
	if len(result.Sources) != 1 || result.Sources[0].Title != "Fixture" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceRejectsInvalidOrModelBuiltInExecution(t *testing.T) {
	tests := []struct {
		name      string
		execution ActiveExecution
		want      error
	}{
		{
			name:      "external without provider",
			execution: ActiveExecution{Mode: ExecutionExternal},
			want:      ErrInvalidConfig,
		},
		{
			name: "model built-in route",
			execution: ActiveExecution{
				Mode: ExecutionModelBuiltIn, ModelBuiltIn: ModelBuiltInOpenAI,
			},
			want: ErrModelBuiltInRequiresChat,
		},
		{
			name: "unsupported model built-in",
			execution: ActiveExecution{
				Mode: ExecutionModelBuiltIn, ModelBuiltIn: "unknown",
			},
			want: ErrInvalidConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&fixedResolver{execution: tt.execution})
			_, err := service.Search(context.Background(), Request{Query: "query"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Search() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestServiceResolvesSelectedSearchModeWithoutCrossModeFallback(t *testing.T) {
	provider := &searchProbe{id: ProviderTavily}
	resolver := &fixedModeResolver{
		external: ActiveExecution{Mode: ExecutionExternal, External: provider},
		builtIn: ActiveExecution{
			Mode: ExecutionModelBuiltIn, ModelBuiltIn: ModelBuiltInAnthropic,
		},
	}
	service := NewService(resolver)

	external, err := service.ResolveExternal(context.Background())
	if err != nil || external.External != provider ||
		resolver.externalCalls != 1 || resolver.builtInCalls != 0 {
		t.Fatalf("external execution = %#v/%v, calls=%d/%d", external, err, resolver.externalCalls, resolver.builtInCalls)
	}

	request := ModelBuiltInResolutionRequest{
		ProviderID: "ANTHROPIC", ModelID: "claude-sonnet-4-5",
		Protocol: ModelBuiltInAnthropic,
	}
	builtIn, err := service.ResolveModelBuiltIn(context.Background(), request)
	if err != nil || builtIn.ModelBuiltIn != ModelBuiltInAnthropic ||
		resolver.externalCalls != 1 || resolver.builtInCalls != 1 ||
		resolver.builtInRequest != request {
		t.Fatalf("built-in execution = %#v/%v, calls=%d/%d request=%#v", builtIn, err, resolver.externalCalls, resolver.builtInCalls, resolver.builtInRequest)
	}

	resolver.builtIn = resolver.external
	if _, err := service.ResolveModelBuiltIn(
		context.Background(),
		request,
	); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("cross-mode built-in resolution error = %v", err)
	}
	resolver.external = ActiveExecution{
		Mode: ExecutionModelBuiltIn, ModelBuiltIn: ModelBuiltInAnthropic,
	}
	if _, err := service.ResolveExternal(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("cross-mode external resolution error = %v", err)
	}
}

func TestServiceRedactsResolverFailure(t *testing.T) {
	sensitiveValue := strings.Join([]string{"resolver", "private", "value"}, "-")
	service := NewService(&fixedResolver{err: errors.New(sensitiveValue)})
	_, err := service.Search(context.Background(), Request{Query: "query"})
	if !errors.Is(err, ErrResolutionFailed) || strings.Contains(err.Error(), sensitiveValue) {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestHandlerExecutesResolvedProviderWithoutAcceptingConfig(t *testing.T) {
	provider := &searchProbe{
		id: ProviderExa,
		result: Result{Sources: []Source{{
			Title: "Fixture", URL: "https://example.test/result", Content: "answer",
		}}},
	}
	handler := NewHandler(NewService(&fixedResolver{execution: ActiveExecution{
		Mode: ExecutionExternal, External: provider,
	}}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		SearchPath,
		strings.NewReader(`{"query":"latest fixture","scope":"news","maxResults":4}`),
	)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	var result Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Sources) != 1 || len(provider.calls) != 1 {
		t.Fatalf("result/calls = %#v/%d", result, len(provider.calls))
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(
		http.MethodPost,
		SearchPath,
		strings.NewReader(`{"query":"latest fixture","apiKey":"plaintext"}`),
	)
	handler.ServeHTTP(recorder, request)
	assertSearchError(t, recorder, http.StatusBadRequest, "INVALID_REQUEST")
}

func TestHandlerReturnsStableBoundaryErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler *Handler
		method  string
		path    string
		body    string
		status  int
		code    string
	}{
		{
			name: "not configured", handler: NewHandler(nil), method: http.MethodPost,
			path: SearchPath, body: `{"query":"fixture"}`,
			status: http.StatusServiceUnavailable, code: "SEARCH_NOT_CONFIGURED",
		},
		{
			name: "model built-in requires chat",
			handler: NewHandler(NewService(&fixedResolver{execution: ActiveExecution{
				Mode: ExecutionModelBuiltIn, ModelBuiltIn: ModelBuiltInOpenAI,
			}})),
			method: http.MethodPost, path: SearchPath, body: `{"query":"fixture"}`,
			status: http.StatusConflict, code: "MODEL_BUILTIN_SEARCH_REQUIRES_CHAT",
		},
		{
			name: "invalid query", handler: NewHandler(nil), method: http.MethodPost,
			path: SearchPath, body: `{"query":""}`,
			status: http.StatusBadRequest, code: "INVALID_SEARCH_REQUEST",
		},
		{
			name: "method", handler: NewHandler(nil), method: http.MethodGet,
			path: SearchPath, status: http.StatusMethodNotAllowed, code: "METHOD_NOT_ALLOWED",
		},
		{
			name: "path", handler: NewHandler(nil), method: http.MethodPost,
			path: "/v1/search/other", body: `{"query":"fixture"}`,
			status: http.StatusNotFound, code: "NOT_FOUND",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			tt.handler.ServeHTTP(recorder, request)
			assertSearchError(t, recorder, tt.status, tt.code)
		})
	}
}

func assertSearchError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	var response ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q", response.Error.Code, code)
	}
}
