package ragproviders

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const providerGatewayTestCredential = "provider-gateway-test-credential"

type queryRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip queryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type gatewayCredentialResolver struct {
	credential string
	err        error
	providers  []string
}

func (resolver *gatewayCredentialResolver) ResolveRAGProviderCredential(
	_ context.Context,
	providerID string,
) (string, error) {
	resolver.providers = append(resolver.providers, providerID)
	return resolver.credential, resolver.err
}

func TestProviderGatewayMinerUAllocateAndPollUseClosedUpstreamRequests(t *testing.T) {
	resolver := &gatewayCredentialResolver{credential: providerGatewayTestCredential}
	requests := make([]*http.Request, 0, 2)
	gateway := NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(request *http.Request) (*http.Response, error) {
				requests = append(requests, request)
				if request.Header.Get("Authorization") != "Bearer "+providerGatewayTestCredential ||
					request.Header.Get("Accept-Encoding") != "identity" {
					t.Fatalf("unexpected provider authentication headers")
				}
				switch request.URL.String() {
				case MinerUAllocateEndpoint:
					if request.Method != http.MethodPost {
						t.Fatalf("allocate method = %s", request.Method)
					}
					raw, _ := io.ReadAll(request.Body)
					var body map[string]any
					if json.Unmarshal(raw, &body) != nil || body["model_version"] != MinerUModelVersion ||
						body["is_ocr"] != true || body["enable_formula"] != true ||
						body["enable_table"] != true || strings.Contains(string(raw), providerGatewayTestCredential) {
						t.Fatalf("allocate request = %s", raw)
					}
					return gatewayJSONResponse(t, map[string]any{
						"code": 0,
						"data": map[string]any{
							"batch_id": "fixture-batch",
							"file_urls": []string{
								"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/document.pdf?signature=fixture",
							},
						},
						"msg": "ok",
					}), nil
				case MinerUPollEndpointPrefix + "fixture-batch":
					if request.Method != http.MethodGet || request.Body != nil {
						t.Fatalf("poll request = %s body=%T", request.Method, request.Body)
					}
					return gatewayJSONResponse(t, map[string]any{
						"code": 0,
						"data": map[string]any{
							"batch_id": "fixture-batch",
							"extract_result": []any{map[string]any{
								"data_id":      "fixture-data",
								"err_msg":      "",
								"file_name":    "document.pdf",
								"full_zip_url": "https://cdn-mineru.openxlab.org.cn/pdf/fixture.zip",
								"state":        "done",
							}},
						},
						"msg":      "ok",
						"trace_id": "provider-trace-not-returned",
					}), nil
				default:
					t.Fatalf("unexpected provider request %s", request.URL.Redacted())
					return nil, errors.New("unreachable")
				}
			},
		)}),
	)

	allocation, err := gateway.AllocateMinerU(
		context.Background(),
		MinerUAllocateRequest{Filename: "document.pdf"},
	)
	if err != nil {
		t.Fatalf("AllocateMinerU() error = %v", err)
	}
	if allocation.BatchID != "fixture-batch" || allocation.Filename != "document.pdf" ||
		!strings.Contains(allocation.UploadURL, "signature=fixture") {
		t.Fatalf("allocation = %#v", allocation)
	}
	poll, err := gateway.PollMinerU(context.Background(), MinerUPollRequest{
		BatchID: allocation.BatchID, Filename: allocation.Filename,
	})
	if err != nil {
		t.Fatalf("PollMinerU() error = %v", err)
	}
	if poll.State != "done" || poll.ResultURL != "https://cdn-mineru.openxlab.org.cn/pdf/fixture.zip" ||
		len(requests) != 2 || strings.Join(resolver.providers, ",") != "mineru,mineru" {
		t.Fatalf("poll = %#v requests=%d providers=%v", poll, len(requests), resolver.providers)
	}
}

func TestProviderGatewaySiliconFlowUsesFrozenProBGEProfile(t *testing.T) {
	resolver := &gatewayCredentialResolver{credential: providerGatewayTestCredential}
	var embeddingCalls int
	gateway := NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Authorization") !=
					"Bearer "+providerGatewayTestCredential {
					t.Fatal("SiliconFlow authorization header mismatch")
				}
				raw, _ := io.ReadAll(request.Body)
				if strings.Contains(string(raw), providerGatewayTestCredential) {
					t.Fatal("SiliconFlow request body contains credential")
				}
				switch request.URL.String() {
				case SiliconFlowEmbeddingsEndpoint:
					embeddingCalls++
					var body map[string]json.RawMessage
					if json.Unmarshal(raw, &body) != nil || len(body) != 3 ||
						string(body["model"]) != `"`+SiliconFlowEmbeddingModel+`"` ||
						string(body["encoding_format"]) != `"float"` {
						t.Fatalf("embedding request = %s", raw)
					}
					if _, present := body["dimensions"]; present {
						t.Fatalf("BGE request must not contain dimensions: %s", raw)
					}
					var inputs []string
					if json.Unmarshal(body["input"], &inputs) != nil || len(inputs) < 1 {
						t.Fatalf("embedding inputs = %s", body["input"])
					}
					data := make([]map[string]any, 0, len(inputs))
					for index := len(inputs) - 1; index >= 0; index-- {
						data = append(data, map[string]any{
							"object":    "embedding",
							"index":     index,
							"embedding": repeatedGatewayEmbedding(float64(index + 1)),
						})
					}
					return gatewayJSONResponse(t, map[string]any{
						"object": "list",
						"model":  SiliconFlowEmbeddingModel,
						"data":   data,
						"usage": map[string]any{
							"prompt_tokens": 4, "completion_tokens": 0, "total_tokens": 4,
						},
					}), nil
				case SiliconFlowRerankEndpoint:
					var body struct {
						Documents       []string `json:"documents"`
						Model           string   `json:"model"`
						Query           string   `json:"query"`
						TopN            int      `json:"top_n"`
						ReturnDocuments bool     `json:"return_documents"`
					}
					if json.Unmarshal(raw, &body) != nil ||
						body.Model != SiliconFlowRerankModel ||
						body.Query != "semantic query" || body.TopN != 2 ||
						body.ReturnDocuments || len(body.Documents) != 2 {
						t.Fatalf("rerank request = %s", raw)
					}
					return gatewayJSONResponse(t, map[string]any{
						"id": "rerank-fixture",
						"results": []any{
							map[string]any{
								"document": nil, "index": 1, "relevance_score": 0.9,
							},
							map[string]any{
								"document": nil, "index": 0, "relevance_score": 0.1,
							},
						},
						"meta": map[string]any{"tokens": map[string]any{"input_tokens": 4}},
					}), nil
				default:
					t.Fatalf("unexpected SiliconFlow request %s", request.URL.Redacted())
					return nil, errors.New("unreachable")
				}
			},
		)}),
	)
	profile, err := gateway.ForRetrievalProfile(RetrievalProfileSiliconFlow)
	if err != nil || profile.Profile() != SiliconFlowRetrievalProfile {
		t.Fatalf("ForRetrievalProfile() = %#v, %v", profile, err)
	}
	passages, err := gateway.EmbedSiliconFlowPassages(
		context.Background(),
		PassageEmbeddingRequest{Passages: []PassageEmbeddingInput{
			{PassageID: "11111111-1111-4111-8111-111111111111", Text: "first passage"},
			{PassageID: "22222222-2222-4222-8222-222222222222", Text: "second passage"},
		}},
	)
	if err != nil || passages.Model != SiliconFlowEmbeddingModel ||
		len(passages.Vectors) != 2 || passages.Vectors[0].Embedding[0] != 1 ||
		passages.Vectors[1].Embedding[0] != 2 {
		t.Fatalf("EmbedSiliconFlowPassages() = %#v, %v", passages, err)
	}
	embedding, err := profile.EmbedQuery(context.Background(), " semantic query ")
	if err != nil || embedding.ModelID != SiliconFlowEmbeddingModel ||
		embedding.Dimensions != SiliconFlowEmbeddingDimensions ||
		len(embedding.Vector) != SiliconFlowEmbeddingDimensions {
		t.Fatalf("EmbedQuery() = %#v, %v", embedding, err)
	}
	reranked, err := profile.Rerank(
		context.Background(),
		" semantic query ",
		[]string{"first source", "second source"},
	)
	if err != nil || len(reranked) != 2 || reranked[0].Index != 1 ||
		reranked[1].Index != 0 || embeddingCalls != 2 ||
		strings.Join(resolver.providers, ",") !=
			"siliconflow,siliconflow,siliconflow" {
		t.Fatalf(
			"reranked=%#v err=%v embeddingCalls=%d providers=%v",
			reranked,
			err,
			embeddingCalls,
			resolver.providers,
		)
	}
}

func TestProviderGatewaySiliconFlowRejectsMalformedProviderResponses(t *testing.T) {
	zeroVector := make([]float64, SiliconFlowEmbeddingDimensions)
	cases := []map[string]any{
		{
			"object": "list", "model": SiliconFlowEmbeddingModel,
			"data": []any{map[string]any{
				"object": "embedding", "index": 0, "embedding": zeroVector,
			}},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1,
			},
		},
		{
			"object": "list", "model": SiliconFlowEmbeddingModel,
			"data": []any{map[string]any{
				"object": "embedding", "index": 0,
				"embedding": repeatedGatewayEmbedding(1), "unexpected": true,
			}},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1,
			},
		},
	}
	for _, payload := range cases {
		raw, _ := json.Marshal(payload)
		if _, err := normalizeSiliconFlowEmbeddingResponse(raw, 1); !errors.Is(err, ErrProviderGatewayUpstream) {
			t.Fatalf("malformed embedding response error = %v", err)
		}
	}

	for _, payload := range []map[string]any{
		{
			"id": "rerank-fixture",
			"results": []any{
				map[string]any{"index": 0, "relevance_score": 0.9},
				map[string]any{"index": 0, "relevance_score": 0.8},
			},
		},
		{
			"id": "rerank-fixture",
			"results": []any{
				map[string]any{"index": 0, "relevance_score": 1.1},
			},
		},
		{
			"id": "rerank-fixture",
			"results": []any{
				map[string]any{
					"document": map[string]any{"text": "must stay omitted"},
					"index":    0, "relevance_score": 0.9,
				},
			},
		},
	} {
		raw, _ := json.Marshal(payload)
		count := len(payload["results"].([]any))
		if _, err := normalizeSiliconFlowRerankResponse(raw, count); !errors.Is(err, ErrProviderGatewayUpstream) {
			t.Fatalf("malformed rerank response error = %v", err)
		}
	}
}

func TestProviderGatewayFailsClosedBeforeHTTPAndRedactsProviderBodies(t *testing.T) {
	resolver := &gatewayCredentialResolver{
		credential: providerGatewayTestCredential,
		err:        ErrProviderGatewayActivationRequired,
	}
	calls := 0
	gateway := NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected call")
			},
		)}),
	)
	_, err := gateway.EmbedSiliconFlowPassages(context.Background(), PassageEmbeddingRequest{
		Passages: []PassageEmbeddingInput{{PassageID: "bad-id", Text: "private text"}},
	})
	if !errors.Is(err, ErrProviderGatewayInvalid) || len(resolver.providers) != 0 || calls != 0 {
		t.Fatalf("invalid request error=%v providers=%v calls=%d", err, resolver.providers, calls)
	}
	_, err = gateway.AllocateMinerU(
		context.Background(),
		MinerUAllocateRequest{Filename: "document.pdf"},
	)
	if !errors.Is(err, ErrProviderGatewayActivationRequired) || calls != 0 {
		t.Fatalf("inactive provider error=%v calls=%d", err, calls)
	}

	privateBody := "private-provider-body-must-not-leak"
	resolver.err = nil
	gateway = NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(privateBody)),
				}, nil
			},
		)}),
	)
	_, err = gateway.EmbedQuery(context.Background(), "private query")
	if !errors.Is(err, ErrQueryEmbeddingUnavailable) ||
		strings.Contains(err.Error(), privateBody) ||
		strings.Contains(err.Error(), providerGatewayTestCredential) ||
		strings.Contains(err.Error(), "private query") {
		t.Fatalf("direct query error leaked sensitive data: %v", err)
	}
}

func TestProviderGatewayMinerUPollRejectsUnsafeCapabilitiesAndUnknownFields(t *testing.T) {
	cases := []map[string]any{
		{
			"code": 0,
			"data": map[string]any{
				"batch_id": "fixture-batch",
				"extract_result": []any{map[string]any{
					"err_msg":      "",
					"file_name":    "document.pdf",
					"full_zip_url": "https://evil.example/pdf/fixture.zip",
					"state":        "done",
				}},
			},
			"msg": "ok",
		},
		{
			"code": 0,
			"data": map[string]any{
				"batch_id": "fixture-batch",
				"extract_result": []any{map[string]any{
					"err_msg":   "",
					"file_name": "document.pdf",
					"state":     "pending",
					"url":       "https://evil.example",
				}},
			},
			"msg": "ok",
		},
	}
	for _, payload := range cases {
		raw, _ := json.Marshal(payload)
		_, err := normalizeMinerUPollResponse(raw, "fixture-batch", "document.pdf")
		if !errors.Is(err, ErrProviderGatewayUpstream) ||
			strings.Contains(err.Error(), "evil.example") {
			t.Fatalf("unsafe poll response error = %v", err)
		}
	}
}

func TestProviderGatewayHTTPClientDisablesProxyRedirectsAndBoundsResponses(t *testing.T) {
	client := newProviderGatewayHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("provider transport = %#v", client.Transport)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://provider.example", nil)
	if err := client.CheckRedirect(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() error = %v", err)
	}

	resolver := &gatewayCredentialResolver{credential: providerGatewayTestCredential}
	gateway := NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						strings.Repeat("x", minerUGatewayMaxResponseBytes+1),
					)),
				}, nil
			},
		)}),
	)
	_, err := gateway.AllocateMinerU(
		context.Background(),
		MinerUAllocateRequest{Filename: "document.pdf"},
	)
	if !errors.Is(err, ErrProviderGatewayUpstream) {
		t.Fatalf("oversized response error = %v", err)
	}
}

func gatewayJSONResponse(t *testing.T, payload any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"application/json; charset=utf-8"},
			"Content-Encoding": []string{"identity"},
		},
		Body: io.NopCloser(strings.NewReader(string(raw))),
	}
}

func repeatedGatewayEmbedding(first float64) []float64 {
	vector := make([]float64, SiliconFlowEmbeddingDimensions)
	vector[0] = first
	return vector
}
