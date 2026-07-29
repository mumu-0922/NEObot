package hindsightfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientUsesAuditedContractAndStripsContent(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	requests := make([]string, 0, 5)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+key {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		requests = append(requests, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPatch && strings.HasSuffix(request.URL.Path, "/config"):
			var body struct {
				Updates map[string]any `json:"updates"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("config request decode: %v", err)
			}
			if len(body.Updates) != 2 || body.Updates["retain_extraction_mode"] != "chunks" ||
				body.Updates["audit_log_enabled"] != false {
				t.Errorf("config updates = %#v", body.Updates)
			}
			_, _ = fmt.Fprint(writer, `{"bank_id":"neo-bank","config":{},"overrides":{}}`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/memories"):
			_, _ = fmt.Fprint(writer, `{"success":true,"bank_id":"neo-bank","items_count":1,"async":false}`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/recall"):
			_, _ = fmt.Fprint(writer, `{"results":[{"id":"upstream-id","text":"must not escape","metadata":{"neo_memory_id":"memory-a"}}]}`)
		case request.Method == http.MethodDelete && strings.Contains(request.URL.Path, "/documents/"):
			_, _ = fmt.Fprint(writer, `{"success":true,"message":"deleted","document_id":"doc-a","memory_units_deleted":1}`)
		case request.Method == http.MethodDelete:
			_, _ = fmt.Fprint(writer, `{"success":true,"message":"deleted","deleted_count":1}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, key, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.ConfigureBank(ctx, "neo-bank", ModeRetrievalOnly); err != nil {
		t.Fatal(err)
	}
	if err := client.Retain(ctx, "neo-bank", RetainItem{
		Content: "fixture", Timestamp: "2026-01-01T00:00:00Z",
		Metadata:   map[string]string{"neo_memory_id": "memory-a"},
		DocumentID: "doc-a", Tags: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	ids, err := client.Recall(ctx, "neo-bank", "synthetic query", RecallScope{})
	if err != nil || len(ids) != 1 || ids[0] != "memory-a" {
		t.Fatalf("Recall() = %q, %v", ids, err)
	}
	if err := client.DeleteDocument(ctx, "neo-bank", "doc-a"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteBank(ctx, "neo-bank"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 5 {
		t.Fatalf("requests = %q", requests)
	}
}

func TestHTTPClientSanitizesFailures(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "wrong api key",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(writer, "raw-secret-upstream-error")
			},
			want: "unauthorized",
		},
		{
			name: "ordinary client error",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = fmt.Fprint(writer, "fixture plaintext validation details")
			},
			want: "upstream_4xx",
		},
		{
			name: "server error",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(writer, "private database trace")
			},
			want: "upstream_5xx",
		},
		{
			name: "malformed",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, `{"results":[]}{}`)
			},
			want: "malformed_response",
		},
		{
			name: "duplicate response field",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, `{"results":[],"results":[]}`)
			},
			want: "malformed_response",
		},
		{
			name: "unknown response field",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, `{"results":[],"raw_trace":"must not escape"}`)
			},
			want: "malformed_response",
		},
		{
			name: "oversized",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(writer, strings.Repeat("x", maximumResponseBytes+1))
			},
			want: "response_too_large",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client, err := NewHTTPClient(server.URL, key, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Recall(context.Background(), "neo-bank", "query", RecallScope{})
			if FaultCode(err) != test.want || strings.Contains(err.Error(), "private") ||
				strings.Contains(err.Error(), "secret") {
				t.Fatalf("Recall() error = %v, code = %q", err, FaultCode(err))
			}
		})
	}
}

func TestHTTPClientRejectsBankMismatch(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"bank_id":"other-bank","config":{},"overrides":{}}`)
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, key, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ConfigureBank(context.Background(), "neo-bank", ModeRetrievalOnly); FaultCode(err) != "bank_mismatch" {
		t.Fatalf("ConfigureBank() error = %v, code = %q", err, FaultCode(err))
	}
}

func TestHTTPClientTimeoutAndCancellationAreBounded(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(100 * time.Millisecond):
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, key, &http.Client{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Recall(context.Background(), "neo-bank", "query", RecallScope{})
	if FaultCode(err) != "timeout" {
		t.Fatalf("timeout code = %q, error = %v", FaultCode(err), err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Recall(canceled, "neo-bank", "query", RecallScope{})
	if FaultCode(err) != "canceled" {
		t.Fatalf("canceled code = %q, error = %v", FaultCode(err), err)
	}
}

func TestScopeTagsPreserveGlobalAndScopedRules(t *testing.T) {
	if tags, match := recallTags(RecallScope{}); len(tags) != 0 || match != "exact" {
		t.Fatalf("global tags = %q, %q", tags, match)
	}
	if tags, match := recallTags(RecallScope{ProjectAlias: "p"}); len(tags) != 1 || tags[0] != "project:p" || match != "any" {
		t.Fatalf("project tags = %q, %q", tags, match)
	}
	if tags, match := recallTags(RecallScope{ProjectAlias: "p", ConversationAlias: "c"}); len(tags) != 1 || tags[0] != "conversation:c" || match != "any" {
		t.Fatalf("conversation tags = %q, %q", tags, match)
	}
}
