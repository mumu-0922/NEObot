package runtimeconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	providerModelCatalogURL   = "https://basellm.github.io/llm-metadata/api/all.json"
	maxDiscoveryProbes        = 3
	maxDiscoveryCacheEntries  = 1024
	maxDiscoveryResponseBytes = 64 << 10
)

// Candidates are names, never proof of provider availability. The fallback is
// deliberately small; the public catalog supplies subsequent releases.
var bundledModelCandidates = []modelCandidate{{ID: "gpt-6-astra", Released: "2026-09-04"}}
var discoveryModelID = regexp.MustCompile(`^gpt-[0-9][a-z0-9.-]{0,95}$`)

type modelCandidate struct {
	ID         string `json:"id"`
	Released   string `json:"release_date"`
	Status     string `json:"status"`
	Modalities struct {
		Output []string `json:"output"`
	} `json:"modalities"`
}

type modelProbeResult struct {
	available bool
	expires   time.Time
}

type providerModelDiscovery struct {
	// Serialize explicit scans (including cache access); waiters can cancel.
	gate           chan struct{}
	client         *http.Client
	catalog        []modelCandidate
	catalogExpires time.Time
	probes         map[string]modelProbeResult
}

func newProviderModelDiscovery() providerModelDiscovery {
	return providerModelDiscovery{
		gate:   make(chan struct{}, 1),
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		probes: make(map[string]modelProbeResult),
	}
}

func (d *providerModelDiscovery) supplement(ctx context.Context, stored StoredProviderConfig, provider resolvedServerDefaultProvider, listed []string) []string {
	if !supportsModelDiscovery(provider.Type, listed) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	select {
	case d.gate <- struct{}{}:
		defer func() { <-d.gate }()
	case <-ctx.Done():
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	candidates := missingModelCandidates(d.candidates(ctx), listed)
	// Owner and row identity prevent cross-administrator evidence reuse.
	binding := stored.UserID + "\x00" + stored.ID + "\x00" + ProviderConnectionTestFingerprint(stored)
	discovered := make([]string, 0)
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		hash := sha256.Sum256([]byte(binding + "\x00" + candidate))
		if d.cachedProbe(ctx, provider, candidate, hex.EncodeToString(hash[:])) {
			discovered = append(discovered, candidate)
		}
	}
	return discovered
}

func supportsModelDiscovery(providerType ProviderType, listed []string) bool {
	if providerType == ProviderTypeOpenAI {
		return true
	}
	if providerType != ProviderTypeOpenAICompatible {
		return false
	}
	// Avoid probing GPT on a DeepSeek-only (etc.) compatible endpoint.
	for _, id := range listed {
		if isDiscoveryTextModel(id) {
			return true
		}
	}
	return false
}

func missingModelCandidates(catalog []modelCandidate, listed []string) []string {
	seen := make(map[string]bool, len(listed))
	for _, id := range listed {
		seen[id] = true
	}
	newestListed := ""
	for _, candidate := range catalog {
		if seen[candidate.ID] && candidate.Released > newestListed {
			newestListed = candidate.Released
		}
	}
	candidates := make([]string, 0, maxDiscoveryProbes)
	for _, candidate := range catalog {
		if seen[candidate.ID] || candidate.Released < newestListed {
			continue
		}
		seen[candidate.ID] = true
		candidates = append(candidates, candidate.ID)
		if len(candidates) == maxDiscoveryProbes {
			break
		}
	}
	return candidates
}

func (d *providerModelDiscovery) cachedProbe(ctx context.Context, provider resolvedServerDefaultProvider, model, key string) bool {
	result, cached := d.probes[key]
	if cached && time.Now().Before(result.expires) {
		return result.available
	}
	result.available = d.probe(ctx, provider, model)
	// Cancellation must not poison the next attempt's negative cache.
	if ctx.Err() != nil {
		return false
	}
	ttl := 15 * time.Minute
	if result.available {
		ttl = 24 * time.Hour
	}
	result.expires = time.Now().Add(ttl)
	d.pruneProbeCache()
	d.probes[key] = result
	return result.available
}

func (d *providerModelDiscovery) pruneProbeCache() {
	if len(d.probes) < maxDiscoveryCacheEntries {
		return
	}
	for key, old := range d.probes {
		if !time.Now().Before(old.expires) {
			delete(d.probes, key)
		}
	}
	if len(d.probes) >= maxDiscoveryCacheEntries {
		// Bounded process-local cache; eviction never promotes a model.
		for key := range d.probes {
			delete(d.probes, key)
			break
		}
	}
}

func isDiscoveryTextModel(id string) bool {
	if !discoveryModelID.MatchString(id) {
		return false
	}
	for _, suffix := range []string{"image", "audio", "realtime", "transcri", "tts", "embedding", "search", "codex", "-pro"} {
		if strings.Contains(id, suffix) {
			return false
		}
	}
	return true
}

func (d *providerModelDiscovery) candidates(ctx context.Context) []modelCandidate {
	if time.Now().Before(d.catalogExpires) {
		return d.catalog
	}
	candidates := append([]modelCandidate(nil), bundledModelCandidates...)
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// No provider headers or secrets ever reach the public metadata source.
	req, _ := http.NewRequestWithContext(requestCtx, http.MethodGet, providerModelCatalogURL, nil)
	body, ok := d.readResponse(req, 8<<20)
	if ok {
		var catalog struct {
			OpenAI struct {
				Models map[string]modelCandidate `json:"models"`
			} `json:"openai"`
		}
		if json.Unmarshal(body, &catalog) == nil {
			for id, candidate := range catalog.OpenAI.Models {
				if id != candidate.ID || !isDiscoveryTextModel(id) || candidate.Status == "deprecated" ||
					len(candidate.Modalities.Output) != 1 || candidate.Modalities.Output[0] != "text" {
					continue
				}
				if _, err := time.Parse("2006-01-02", candidate.Released); err != nil {
					continue
				}
				candidates = append(candidates, candidate)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Released != candidates[j].Released {
			return candidates[i].Released > candidates[j].Released
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > 128 {
		candidates = candidates[:128]
	}
	d.catalog, d.catalogExpires = candidates, time.Now().Add(time.Hour)
	return candidates
}

func (d *providerModelDiscovery) probe(ctx context.Context, provider resolvedServerDefaultProvider, model string) bool {
	base := normalizeProviderBaseURL(provider.BaseURL, provider.Type)
	endpoint, err := url.Parse(base + "/chat/completions")
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Host == "" ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return false
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	payload, _ := json.Marshal(map[string]any{
		"model": model, "messages": []map[string]string{{"role": "user", "content": "Reply exactly OK."}},
		"max_completion_tokens": 32, "stream": false,
	})
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(provider.APIKey))
	body, ok := d.readResponse(req, maxDiscoveryResponseBytes)
	if !ok {
		return false
	}
	var response struct {
		Model   string          `json:"model"`
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &response) != nil || response.Model != model ||
		(len(response.Error) != 0 && string(response.Error) != "null") || len(response.Choices) != 1 {
		return false
	}
	choice := response.Choices[0]
	return choice.Message.Role == "assistant" && strings.TrimSpace(choice.Message.Content) != "" && choice.FinishReason == "stop"
}

func (d *providerModelDiscovery) readResponse(req *http.Request, limit int64) ([]byte, bool) {
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	return body, err == nil && int64(len(body)) <= limit
}
