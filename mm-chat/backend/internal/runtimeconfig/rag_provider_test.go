package runtimeconfig

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestRAGProviderStatusRequiresBothAttestedVaultRecords(t *testing.T) {
	vault := testProviderSecretVault(t, "rag-v1", 45)
	const userID = "00000000-0000-0000-0000-000000000001"
	create := func(providerID RAGProviderID, secret string) StoredProviderConfig {
		recordID := ragProviderRecordID(providerID)
		envelope, err := vault.Encrypt(
			[]byte(secret),
			ragProviderSecretContext(userID, recordID),
		)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		stored := StoredProviderConfig{
			ID: "status-" + string(providerID), UserID: userID,
			ProviderID: recordID, Label: ragProviderDefaultName(providerID),
			EncryptedSecretRef: string(encoded),
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindRAG, RAGProvider: string(providerID), Enabled: true,
			},
		}
		stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
			stored.ProviderID, providerID, stored.EncryptedSecretRef,
		)
		stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return stored
	}
	minerU := create(RAGProviderMinerU, "mineru-status-fixture")
	siliconflow := create(RAGProviderSiliconFlow, "siliconflow-status-fixture")
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{minerU, siliconflow},
		}),
		WithProviderSecretVault(vault),
	)
	status, err := service.RAGProviderStatus(context.Background())
	if err != nil || !status.Ready ||
		status.Status != ragproviders.ServiceStatusReady ||
		status.Providers.MinerU.Status != ragproviders.ProviderStatusReady ||
		status.Providers.SiliconFlow.Status != ragproviders.ProviderStatusReady ||
		status.Providers.SiliconFlow.EmbeddingDimensions != siliconFlowDimensions ||
		!status.Capabilities.PDFParsing || !status.Capabilities.NativeIndexing ||
		!status.Capabilities.Retrieval {
		t.Fatalf("RAG provider status = %#v, %v", status, err)
	}

	siliconflow.EncryptedSecretRef = minerU.EncryptedSecretRef
	siliconflow.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		siliconflow.ProviderID, RAGProviderSiliconFlow, siliconflow.EncryptedSecretRef,
	)
	service = NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{minerU, siliconflow},
		}),
		WithProviderSecretVault(vault),
	)
	status, err = service.RAGProviderStatus(context.Background())
	if err != nil || status.Ready ||
		status.Status != ragproviders.ServiceStatusUnavailable ||
		status.Providers.SiliconFlow.Status != ragproviders.ProviderStatusUnavailable ||
		!status.Capabilities.PDFParsing || status.Capabilities.NativeIndexing ||
		status.Capabilities.Retrieval {
		t.Fatalf("copied-context RAG status = %#v, %v", status, err)
	}
}

func TestRAGConnectionTestRejectsOversizedResponsesAndUnsafeUploadTargets(t *testing.T) {
	client := newRAGProviderHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion == 0 || client.CheckRedirect == nil {
		t.Fatalf("RAG provider HTTP client is not fail-closed: %#v", client)
	}

	t.Run("oversized MinerU response", func(t *testing.T) {
		service := NewService(
			config.Config{},
			WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(
						strings.Repeat("x", minerUMaxResponseBytes+1),
					)),
				}, nil
			})),
		)
		if err := service.testMinerUConnection(
			context.Background(),
			"bounded-mineru-fixture",
		); !errors.Is(err, ErrRAGProviderConnectionFailed) {
			t.Fatalf("oversized MinerU response error = %v", err)
		}
	})

	for value, want := range map[string]bool{
		"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/test.pdf?signature=x": true,
		"https://evil.example/api-upload/test.pdf?signature=x":                        false,
		"http://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/test.pdf?signature=x":  false,
		"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/../x?signature=x":     false,
		"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/test.pdf":             false,
	} {
		if got := validMinerUSignedUploadURL(value); got != want {
			t.Errorf("validMinerUSignedUploadURL(%q) = %t, want %t", value, got, want)
		}
	}
}

func TestSiliconFlowConnectionTestUsesFrozenProBGEModels(t *testing.T) {
	const fixtureCredential = "siliconflow-connection-fixture"
	var endpoints []string
	service := NewService(
		config.Config{},
		WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(
			request *http.Request,
		) (*http.Response, error) {
			endpoints = append(endpoints, request.URL.String())
			if request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
				t.Fatal("SiliconFlow connection test authorization mismatch")
			}
			raw, _ := io.ReadAll(request.Body)
			var body map[string]json.RawMessage
			if json.Unmarshal(raw, &body) != nil ||
				strings.Contains(string(raw), fixtureCredential) {
				t.Fatalf("invalid SiliconFlow connection request: %s", raw)
			}
			switch request.URL.String() {
			case siliconFlowEmbeddingsURL:
				if len(body) != 3 ||
					string(body["model"]) != `"`+siliconFlowEmbeddingModel+`"` {
					t.Fatalf("embedding connection body = %s", raw)
				}
				if _, present := body["dimensions"]; present {
					t.Fatalf("BGE connection request contains dimensions: %s", raw)
				}
				return ragProviderJSONResponse(t, map[string]any{
					"object": "list", "model": siliconFlowEmbeddingModel,
					"data": []any{map[string]any{
						"object": "embedding", "index": 0,
						"embedding": repeatedRAGProviderEmbedding(1),
					}},
					"usage": map[string]any{
						"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1,
					},
				}), nil
			case siliconFlowRerankURL:
				if string(body["model"]) != `"`+siliconFlowRerankModel+`"` ||
					string(body["return_documents"]) != "false" ||
					string(body["top_n"]) != "1" {
					t.Fatalf("rerank connection body = %s", raw)
				}
				return ragProviderJSONResponse(t, map[string]any{
					"id": "rerank-connection-fixture",
					"results": []any{map[string]any{
						"index": 0, "relevance_score": 0.9,
					}},
				}), nil
			default:
				t.Fatalf("unexpected SiliconFlow endpoint %s", request.URL.Redacted())
				return nil, errors.New("unreachable")
			}
		})),
	)
	checks, err := service.testRAGProviderConnection(
		context.Background(),
		RAGProviderSiliconFlow,
		fixtureCredential,
	)
	if err != nil || strings.Join(checks, ",") != "embedding,rerank" ||
		strings.Join(endpoints, ",") !=
			siliconFlowEmbeddingsURL+","+siliconFlowRerankURL {
		t.Fatalf("SiliconFlow checks=%v endpoints=%v error=%v", checks, endpoints, err)
	}
}

func ragProviderTestBYOKKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	return privateKey, pemValue
}

func ragProviderJSONResponse(t *testing.T, payload any) *http.Response {
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

func repeatedRAGProviderEmbedding(first float64) []float64 {
	vector := make([]float64, siliconFlowDimensions)
	vector[0] = first
	return vector
}
