package ragproviders

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const queryEmbeddingTestToken = "unit-test-query-token"

type queryRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip queryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestQueryEmbeddingClientSendsPrivateRequestAndValidatesVector(t *testing.T) {
	var captured *http.Request
	client, err := newQueryEmbeddingClient(
		"http://rag-worker.test:8081",
		queryEmbeddingTestToken,
		&http.Client{Transport: queryRoundTripper(func(request *http.Request) (*http.Response, error) {
			captured = request
			payload, _ := json.Marshal(queryEmbeddingResponse{
				Model: queryEmbeddingModel, Dimensions: queryEmbeddingDimensions,
				Embedding: repeatedQueryEmbedding(0.001),
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(payload))),
				Header:     make(http.Header),
			}, nil
		})},
	)
	if err != nil {
		t.Fatalf("newQueryEmbeddingClient() error = %v", err)
	}
	embedding, err := client.EmbedQuery(context.Background(), " semantic question ")
	if err != nil {
		t.Fatalf("EmbedQuery() error = %v", err)
	}
	if embedding.ModelID != queryEmbeddingModel || embedding.Dimensions != queryEmbeddingDimensions ||
		len(embedding.Vector) != queryEmbeddingDimensions {
		t.Fatalf("embedding = %#v", embedding)
	}
	if captured == nil || captured.URL.Path != queryEmbeddingPath ||
		captured.Header.Get("Authorization") != "Bearer "+queryEmbeddingTestToken {
		t.Fatalf("captured request = %#v", captured)
	}
	raw, _ := io.ReadAll(captured.Body)
	if string(raw) != `{"query":"semantic question"}` {
		t.Fatalf("request body = %s", raw)
	}
}

func TestQueryEmbeddingClientFailsClosedWithoutLeakingToken(t *testing.T) {
	client, err := newQueryEmbeddingClient(
		"http://rag-worker.test:8081",
		queryEmbeddingTestToken,
		&http.Client{Transport: queryRoundTripper(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader(queryEmbeddingTestToken)),
				Header:     make(http.Header),
			}, nil
		})},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.EmbedQuery(context.Background(), "private query")
	if !errors.Is(err, ErrQueryEmbeddingUnavailable) {
		t.Fatalf("EmbedQuery() error = %v", err)
	}
	if strings.Contains(err.Error(), queryEmbeddingTestToken) || strings.Contains(err.Error(), "private query") {
		t.Fatalf("EmbedQuery() leaked sensitive input: %v", err)
	}
}

func TestQueryEmbeddingClientRejectsUnsafeConfigAndResponses(t *testing.T) {
	if client, err := NewQueryEmbeddingClient("", "", 0); err != nil || client != nil {
		t.Fatalf("disabled client = %T/%v, want nil/nil", client, err)
	}
	for _, rawURL := range []string{"file:///tmp/query", "http://user:pass@worker.test", "http://worker.test?q=x"} {
		if _, err := NewQueryEmbeddingClient(rawURL, queryEmbeddingTestToken, 0); !errors.Is(err, ErrQueryEmbeddingInvalid) {
			t.Fatalf("NewQueryEmbeddingClient(%q) error = %v", rawURL, err)
		}
	}

	client, err := newQueryEmbeddingClient(
		"http://rag-worker.test:8081",
		queryEmbeddingTestToken,
		&http.Client{Transport: queryRoundTripper(func(_ *http.Request) (*http.Response, error) {
			payload, _ := json.Marshal(queryEmbeddingResponse{
				Model: queryEmbeddingModel, Dimensions: queryEmbeddingDimensions,
				Embedding: []float32{0.1},
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
	if _, err := client.EmbedQuery(context.Background(), "query"); !errors.Is(err, ErrQueryEmbeddingUnavailable) {
		t.Fatalf("invalid response error = %v", err)
	}
}

func repeatedQueryEmbedding(value float32) []float32 {
	vector := make([]float32, queryEmbeddingDimensions)
	for index := range vector {
		vector[index] = value
	}
	return vector
}
