package runtimeconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

type discoveryRoundTripper func(*http.Request) (*http.Response, error)

func (f discoveryRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func seedDiscoveryCatalog(d *providerModelDiscovery, ids ...string) {
	d.catalogExpires = time.Now().Add(time.Hour)
	for _, id := range ids {
		d.catalog = append(d.catalog, modelCandidate{ID: id, Released: "2026-09-04"})
	}
}

func TestDiscoveryAddsOnlyVerifiedModelsAndCachesPerConnection(t *testing.T) {
	var probes atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-5.6-sol"}]}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		probes.Add(1)
		var body struct {
			Model    string `json:"model"`
			Max      int    `json:"max_completion_tokens"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Max != 32 || len(body.Messages) != 1 || body.Messages[0].Content != "Reply exactly OK." {
			t.Error("unbounded/non-synthetic probe")
		}
		if body.Model == "gpt-6-rejected" {
			http.Error(w, "fixture provider secret error", 404)
			return
		}
		_, _ = io.WriteString(w, `{"model":"gpt-6-astra","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	vault := testProviderSecretVault(t, "discovery-fixture", 21)
	repo := &fakeProviderConfigRepository{ok: true, stored: testStoredVaultProvider(t, vault, "CUSTOM", ProviderTypeOpenAICompatible, upstream.URL+"/v1", "fixture-key")}
	s := NewService(config.Config{}, WithProviderConfigRepository(repo), WithProviderSecretVault(vault))
	seedDiscoveryCatalog(&s.modelDiscovery, "gpt-6-astra", "gpt-6-rejected")
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		NewHandler(s).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/providers/CUSTOM/discover", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("discovery status = %d", rec.Code)
		}
		var response AdminProviderConnectionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if strings.Join(response.Models, ",") != "gpt-5.6-sol,gpt-6-astra" || strings.Join(response.DiscoveredModels, ",") != "gpt-6-astra" {
			t.Fatalf("response = %#v", response)
		}
		if response.Provider.Enabled {
			t.Fatal("discovery unexpectedly activated provider")
		}
	}
	if probes.Load() != 2 {
		t.Fatalf("repeat refresh issued %d probes", probes.Load())
	}
	// Actual vault credential change invalidates both positive and negative evidence.
	repo.stored = testStoredVaultProvider(t, vault, "CUSTOM", ProviderTypeOpenAICompatible, upstream.URL+"/v1", "rotated-fixture-key")
	if _, err := s.DiscoverAdminProviderModels(context.Background(), "CUSTOM"); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 4 {
		t.Fatal("credential rotation reused old evidence")
	}
	before := probes.Load()
	if _, err := s.TestAdminProviderConnection(context.Background(), "CUSTOM"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateAdminProvider(context.Background(), "CUSTOM"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProviderModelsForContext(context.Background(), ProviderModelsRequest{Provider: ProviderRuntimeConfig{ID: "CUSTOM", Source: "server-stored"}}); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != before {
		t.Fatal("passive/test/activate path probed")
	}
}

func TestDiscoveryProbeRejectsFalseSuccessAndRedirects(t *testing.T) {
	for name, body := range map[string]string{
		"empty": `{}`, "malformed": `{`,
		"wrong-model":  `{"model":"gpt-other","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`,
		"error":        `{"model":"gpt-6-astra","error":{"message":"private"}}`,
		"empty-output": `{"model":"gpt-6-astra","choices":[{"message":{"role":"assistant","content":""},"finish_reason":"length"}]}`,
		"oversized":    strings.Repeat("x", maxDiscoveryResponseBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer upstream.Close()
			d := newProviderModelDiscovery()
			if d.probe(context.Background(), resolvedServerDefaultProvider{Type: ProviderTypeOpenAICompatible, BaseURL: upstream.URL, APIKey: "fixture"}, "gpt-6-astra") {
				t.Fatal("promoted invalid completion")
			}
		})
	}
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Store(true) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer origin.Close()
	d := newProviderModelDiscovery()
	if d.probe(context.Background(), resolvedServerDefaultProvider{Type: ProviderTypeOpenAI, BaseURL: origin.URL, APIKey: "fixture"}, "gpt-6-astra") || redirected.Load() {
		t.Fatal("followed redirect/promoted model")
	}
}

func TestDiscoveryCatalogIsCredentialFreeAndFiltersNonTextModels(t *testing.T) {
	d := newProviderModelDiscovery()
	var calls int
	d.client.Transport = discoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != providerModelCatalogURL || r.Header.Get("Authorization") != "" {
			t.Fatal("catalog destination/credential violation")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"openai":{"models":{
			"gpt-7-new":{"id":"gpt-7-new","release_date":"2026-09-05","modalities":{"output":["text"]}},
			"gpt-7-audio":{"id":"gpt-7-audio","release_date":"2026-09-05","modalities":{"output":["text","audio"]}},
			"gpt-7-old":{"id":"gpt-7-old","status":"deprecated","release_date":"2026-09-05","modalities":{"output":["text"]}},
			"gpt-7-mismatch":{"id":"gpt-7-other","release_date":"2026-09-05","modalities":{"output":["text"]}}
		}}}`)), Header: make(http.Header)}, nil
	})
	for i := 0; i < 2; i++ {
		candidates := d.candidates(context.Background())
		if len(candidates) != 2 || candidates[0].ID != "gpt-7-new" || candidates[1].ID != "gpt-6-astra" {
			t.Fatalf("candidates = %#v", candidates)
		}
	}
	if calls != 1 {
		t.Fatal("catalog was not cached")
	}
	d.catalogExpires = time.Time{}
	d.client.Transport = discoveryRoundTripper(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	if got := d.candidates(context.Background()); len(got) != 1 || got[0].ID != "gpt-6-astra" {
		t.Fatal("missing offline fallback")
	}
}

func TestDiscoveryBoundsProbesAndRespectsCancellation(t *testing.T) {
	d := newProviderModelDiscovery()
	seedDiscoveryCatalog(&d, "gpt-6-a", "gpt-6-b", "gpt-6-c", "gpt-6-d")
	var calls int
	d.client.Transport = discoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 8*time.Second {
			t.Error("probe lacks bounded deadline")
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("private error")), Header: make(http.Header)}, nil
	})
	provider := resolvedServerDefaultProvider{Type: ProviderTypeOpenAICompatible, BaseURL: "https://fixture.invalid/v1", APIKey: "fixture"}
	stored := StoredProviderConfig{UserID: "owner", ProviderID: "CUSTOM", EncryptedSecretRef: "fixture", Config: StoredProviderConfigPayload{Type: ProviderTypeOpenAICompatible, BaseURL: provider.BaseURL}}
	for i := 0; i < 2; i++ {
		if got := d.supplement(context.Background(), stored, provider, []string{"gpt-5.6-sol"}); len(got) != 0 {
			t.Fatal("promoted failure")
		}
	}
	if calls != maxDiscoveryProbes {
		t.Fatalf("calls = %d", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := d.supplement(ctx, stored, provider, []string{"gpt-5.6-sol"}); len(got) != 0 || calls != maxDiscoveryProbes {
		t.Fatal("cancelled scan made calls")
	}
	if got := d.supplement(context.Background(), stored, provider, []string{"deepseek-chat"}); len(got) != 0 || calls != maxDiscoveryProbes {
		t.Fatal("probed unrelated provider")
	}
	provider.Type = ProviderTypeGemini
	if got := d.supplement(context.Background(), stored, provider, []string{"gpt-5.6-sol"}); len(got) != 0 || calls != maxDiscoveryProbes {
		t.Fatal("probed unsupported protocol")
	}
}

func TestDiscoveryCancelsInflightProbeWithoutCachingFailure(t *testing.T) {
	d := newProviderModelDiscovery()
	seedDiscoveryCatalog(&d, "gpt-6-astra")
	started := make(chan struct{})
	d.client.Transport = discoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.supplement(ctx, StoredProviderConfig{}, resolvedServerDefaultProvider{Type: ProviderTypeOpenAI, BaseURL: "https://fixture.invalid/v1", APIKey: "fixture"}, nil)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("probe ignored cancellation")
	}
	if len(d.probes) != 0 || len(d.gate) != 0 {
		t.Fatal("cancelled probe poisoned cache or retained gate")
	}
}

func TestDiscoveryRejectsConfigurationChangedDuringProbe(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"gpt-5.6-sol"}]}`)
	}))
	defer upstream.Close()
	vault := testProviderSecretVault(t, "discovery-fixture", 21)
	repo := &fakeProviderConfigRepository{ok: true, stored: testStoredVaultProvider(t, vault, "CUSTOM", ProviderTypeOpenAICompatible, upstream.URL, "fixture")}
	s := NewService(config.Config{}, WithProviderConfigRepository(repo), WithProviderSecretVault(vault))
	seedDiscoveryCatalog(&s.modelDiscovery, "gpt-6-astra")
	s.modelDiscovery.client.Transport = discoveryRoundTripper(func(*http.Request) (*http.Response, error) {
		repo.stored.Config.BaseURL = "https://changed.invalid/v1"
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"gpt-6-astra","choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`)), Header: make(http.Header)}, nil
	})
	if _, err := s.DiscoverAdminProviderModels(context.Background(), "CUSTOM"); err != ErrProviderConfigChanged {
		t.Fatalf("error = %v", err)
	}
}
