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

func TestProviderGatewayJinaPassageQueryAndRerankUseDirectFixedProfiles(t *testing.T) {
	resolver := &gatewayCredentialResolver{credential: providerGatewayTestCredential}
	var tasks []string
	var endpoints []string
	gateway := NewProviderGateway(
		resolver,
		WithProviderGatewayHTTPClient(&http.Client{Transport: queryRoundTripper(
			func(request *http.Request) (*http.Response, error) {
				endpoints = append(endpoints, request.URL.String())
				if request.Header.Get("Authorization") != "Bearer "+providerGatewayTestCredential {
					t.Fatal("Jina authorization header mismatch")
				}
				raw, _ := io.ReadAll(request.Body)
				if strings.Contains(string(raw), providerGatewayTestCredential) {
					t.Fatal("Jina request body contains credential")
				}
				switch request.URL.String() {
				case JinaEmbeddingsEndpoint:
					var body struct {
						Dimensions int                 `json:"dimensions"`
						Input      []map[string]string `json:"input"`
						Model      string              `json:"model"`
						Task       string              `json:"task"`
					}
					if json.Unmarshal(raw, &body) != nil || body.Model != JinaEmbeddingModel ||
						body.Dimensions != JinaEmbeddingDimensions || len(body.Input) < 1 {
						t.Fatalf("embedding request = %s", raw)
					}
					tasks = append(tasks, body.Task)
					data := make([]map[string]any, 0, len(body.Input))
					for index := len(body.Input) - 1; index >= 0; index-- {
						vector := repeatedGatewayEmbedding(float64(index + 1))
						data = append(data, map[string]any{"index": index, "embedding": vector})
					}
					return gatewayJSONResponse(t, map[string]any{
						"model": JinaEmbeddingModel,
						"data":  data,
						"usage": map[string]any{"total_tokens": 4},
					}), nil
				case JinaRerankEndpoint:
					var body struct {
						Documents       []string `json:"documents"`
						Model           string   `json:"model"`
						Query           string   `json:"query"`
						TopN            int      `json:"top_n"`
						ReturnDocuments bool     `json:"return_documents"`
					}
					if json.Unmarshal(raw, &body) != nil || body.Model != JinaRerankModel ||
						body.Query != "semantic query" || body.TopN != 2 || body.ReturnDocuments ||
						len(body.Documents) != 2 {
						t.Fatalf("rerank request = %s", raw)
					}
					return gatewayJSONResponse(t, map[string]any{
						"model": JinaRerankModel,
						"results": []any{
							map[string]any{"index": 1, "relevance_score": 0.9},
							map[string]any{"index": 0, "relevance_score": -0.1},
						},
					}), nil
				default:
					t.Fatalf("unexpected Jina request %s", request.URL.Redacted())
					return nil, errors.New("unreachable")
				}
			},
		)}),
	)

	passageResponse, err := gateway.EmbedPassages(
		context.Background(),
		PassageEmbeddingRequest{Passages: []PassageEmbeddingInput{
			{PassageID: "11111111-1111-4111-8111-111111111111", Text: "first passage"},
			{PassageID: "22222222-2222-4222-8222-222222222222", Text: "second passage"},
		}},
	)
	if err != nil {
		t.Fatalf("EmbedPassages() error = %v", err)
	}
	if len(passageResponse.Vectors) != 2 || passageResponse.Vectors[0].Embedding[0] != 1 ||
		passageResponse.Vectors[1].Embedding[0] != 2 ||
		passageResponse.Vectors[1].PassageID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("passage response = %#v", passageResponse)
	}
	query, err := gateway.EmbedQuery(context.Background(), " semantic query ")
	if err != nil || query.ModelID != JinaEmbeddingModel ||
		query.Dimensions != JinaEmbeddingDimensions || len(query.Vector) != JinaEmbeddingDimensions {
		t.Fatalf("EmbedQuery() = %#v, %v", query, err)
	}
	reranked, err := gateway.Rerank(
		context.Background(),
		" semantic query ",
		[]string{"first source", "second source"},
	)
	if err != nil || len(reranked) != 2 || reranked[0].Index != 1 || reranked[1].Index != 0 {
		t.Fatalf("Rerank() = %#v, %v", reranked, err)
	}
	if strings.Join(tasks, ",") != "retrieval.passage,retrieval.query" || len(endpoints) != 3 {
		t.Fatalf("tasks=%v endpoints=%v", tasks, endpoints)
	}
	for _, endpoint := range endpoints {
		if strings.Contains(endpoint, "/internal/retrieval/") {
			t.Fatalf("direct Jina adapter called Python route: %s", endpoint)
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
	_, err := gateway.EmbedPassages(context.Background(), PassageEmbeddingRequest{
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
	vector := make([]float64, JinaEmbeddingDimensions)
	vector[0] = first
	return vector
}
