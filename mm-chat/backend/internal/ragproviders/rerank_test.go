package ragproviders

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestRerankClientSendsPrivateRequestAndValidatesAllScores(t *testing.T) {
	var captured *http.Request
	client, err := newRerankClient(
		"http://rag-worker.test:8081",
		queryEmbeddingTestToken,
		&http.Client{Transport: queryRoundTripper(func(request *http.Request) (*http.Response, error) {
			captured = request
			payload, _ := json.Marshal(rerankResponse{
				Model: rerankModel,
				Results: []rerankResponseResult{
					{Index: 1, RelevanceScore: -0.25},
					{Index: 0, RelevanceScore: 0.75},
				},
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(payload))),
				Header:     make(http.Header),
			}, nil
		})},
	)
	if err != nil {
		t.Fatal(err)
	}
	results, err := client.Rerank(
		context.Background(),
		" semantic question ",
		[]string{"first source", "second source"},
	)
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}
	if len(results) != 2 || results[0].Index != 1 || results[1].RelevanceScore != 0.75 {
		t.Fatalf("results = %#v", results)
	}
	if captured == nil || captured.URL.Path != rerankPath ||
		captured.Header.Get("Authorization") != "Bearer "+queryEmbeddingTestToken {
		t.Fatalf("captured request = %#v", captured)
	}
	raw, _ := io.ReadAll(captured.Body)
	if string(raw) != `{"query":"semantic question","documents":["first source","second source"]}` {
		t.Fatalf("request body = %s", raw)
	}
}

func TestRerankClientRejectsUnsafeInputsBeforeHTTP(t *testing.T) {
	calls := 0
	client, err := newRerankClient(
		"http://rag-worker.test:8081",
		queryEmbeddingTestToken,
		&http.Client{Transport: queryRoundTripper(func(_ *http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("unexpected call")
		})},
	)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		query     string
		documents []string
	}{
		{query: "", documents: []string{"source"}},
		{query: "query", documents: nil},
		{query: "query", documents: []string{" "}},
		{query: "query", documents: []string{strings.Repeat("x", maxRerankDocumentBytes+1)}},
		{query: "query", documents: make([]string, maxRerankDocuments+1)},
	}
	for _, tc := range cases {
		if _, err := client.Rerank(context.Background(), tc.query, tc.documents); !errors.Is(err, ErrRerankInvalid) {
			t.Fatalf("Rerank(%q, %d) error = %v", tc.query, len(tc.documents), err)
		}
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d", calls)
	}
}

func TestRerankClientFailsClosedOnInvalidOrSensitiveResponses(t *testing.T) {
	privateSource := "private-source-must-not-leak"
	cases := []rerankResponse{
		{Model: "wrong", Results: []rerankResponseResult{{Index: 0, RelevanceScore: 1}}},
		{Model: rerankModel, Results: []rerankResponseResult{{Index: 1, RelevanceScore: 1}}},
		{Model: rerankModel, Results: []rerankResponseResult{{Index: 0, RelevanceScore: math.NaN()}}},
	}
	for _, payload := range cases {
		client, err := newRerankClient(
			"http://rag-worker.test:8081",
			queryEmbeddingTestToken,
			&http.Client{Transport: queryRoundTripper(func(_ *http.Request) (*http.Response, error) {
				raw, _ := json.Marshal(payload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(string(raw))),
					Header:     make(http.Header),
				}, nil
			})},
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Rerank(context.Background(), "private-query", []string{privateSource})
		if !errors.Is(err, ErrRerankUnavailable) {
			t.Fatalf("Rerank() error = %v", err)
		}
		if strings.Contains(err.Error(), privateSource) || strings.Contains(err.Error(), queryEmbeddingTestToken) {
			t.Fatalf("Rerank() leaked sensitive data: %v", err)
		}
	}
}

func TestRerankClientDisabledAndUnsafeConfig(t *testing.T) {
	if client, err := NewRerankClient("", "", 0); err != nil || client != nil {
		t.Fatalf("disabled client = %T/%v", client, err)
	}
	if _, err := NewRerankClient("file:///tmp/rerank", queryEmbeddingTestToken, 0); !errors.Is(err, ErrRerankInvalid) {
		t.Fatalf("unsafe config error = %v", err)
	}
}
