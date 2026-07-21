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
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestAdminJinaProviderSaveTestActivateAndInvalidate(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	const fixtureCredential = "jina-connection-fixture"
	requests := 0
	client := searchHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("Authorization") != "Bearer "+fixtureCredential ||
			request.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("unexpected Jina authentication headers")
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), fixtureCredential) {
			t.Fatal("Jina request body contains credential")
		}
		switch request.URL.String() {
		case jinaEmbeddingsURL:
			var body struct {
				Model      string `json:"model"`
				Task       string `json:"task"`
				Dimensions int    `json:"dimensions"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Model != jinaEmbeddingModel ||
				body.Task != "retrieval.query" || body.Dimensions != jinaDimensions {
				t.Fatalf("embedding request = %s", raw)
			}
			vector := make([]float64, jinaDimensions)
			vector[0] = 1
			return ragProviderJSONResponse(t, map[string]any{
				"model": jinaEmbeddingModel,
				"data":  []any{map[string]any{"index": 0, "embedding": vector}},
			}), nil
		case jinaRerankURL:
			return ragProviderJSONResponse(t, map[string]any{
				"model": jinaRerankModel,
				"results": []any{map[string]any{
					"index": 0, "relevance_score": 0.99,
				}},
			}), nil
		default:
			t.Fatalf("unexpected Jina URL %s", request.URL.Redacted())
			return nil, errors.New("unreachable")
		}
	})
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "rag-v1", 41)),
		WithRAGProviderHTTPClient(client),
	)

	saved, err := service.UpsertAdminRAGProviderConfig(
		context.Background(),
		"jina",
		UpdateAdminRAGProviderConfigRequest{
			Name: "Jina AI",
			APIKeySecret: encryptedSecretEnvelope(
				t,
				privateKey,
				fixtureCredential,
				ragProviderIngressContext(RAGProviderJina),
			),
		},
	)
	if err != nil {
		t.Fatalf("UpsertAdminRAGProviderConfig() error = %v", err)
	}
	if saved.Enabled || saved.ConnectionTestValid || !saved.HasAPIKey ||
		saved.EmbeddingDimensions != jinaDimensions ||
		storedSecretAlgorithm(repo.stored.EncryptedSecretRef) != "A256GCM" ||
		strings.Contains(repo.stored.EncryptedSecretRef, fixtureCredential) {
		t.Fatalf("saved Jina provider = %#v", saved)
	}

	tested, err := service.TestAdminRAGProviderConnection(context.Background(), "jina")
	if err != nil {
		t.Fatalf("TestAdminRAGProviderConnection() error = %v", err)
	}
	if tested.Provider.Enabled || !tested.Provider.ConnectionTestValid ||
		strings.Join(tested.Checks, ",") != "embedding,rerank" || requests != 2 {
		t.Fatalf("tested Jina provider = %#v requests=%d", tested, requests)
	}

	activated, err := service.ActivateAdminRAGProvider(context.Background(), "jina")
	if err != nil {
		t.Fatalf("ActivateAdminRAGProvider() error = %v", err)
	}
	if !activated.Provider.Enabled || !activated.Provider.ConnectionTestValid || requests != 4 {
		t.Fatalf("activated Jina provider = %#v requests=%d", activated, requests)
	}
	status, err := service.RAGProviderStatus(context.Background())
	if err != nil || status.Ready ||
		status.Status != ragproviders.ServiceStatusPartial ||
		status.Providers.Jina.Status != ragproviders.ProviderStatusReady ||
		status.Providers.MinerU.Status != ragproviders.ProviderStatusMissingSecret ||
		status.Capabilities.PDFParsing || !status.Capabilities.NativeIndexing ||
		!status.Capabilities.Retrieval {
		t.Fatalf("Jina-only status = %#v, %v", status, err)
	}

	updated, err := service.UpsertAdminRAGProviderConfig(
		context.Background(),
		"jina",
		UpdateAdminRAGProviderConfigRequest{Name: "Jina AI", Enabled: false, ClearAPIKey: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.ConnectionTestValid || updated.HasAPIKey ||
		repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("cleared Jina provider = %#v", updated)
	}
}

func TestAdminMinerUProviderRequiresBYOKSave(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	const fixtureCredential = "mineru-byok-fixture"
	client := searchHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != minerUAllocateURL ||
			request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
			t.Fatalf("unexpected MinerU request %s", request.URL.Redacted())
		}
		return ragProviderJSONResponse(t, map[string]any{
			"code": 0,
			"data": map[string]any{
				"batch_id": "mm-chat-test-batch",
				"file_urls": []string{
					"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/test.pdf?signature=test",
				},
			},
			"msg": "ok",
		}), nil
	})
	vault := testProviderSecretVault(t, "rag-v1", 42)
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
		WithRAGProviderHTTPClient(client),
	)

	before, err := service.AdminRAGProviderConfigs(context.Background())
	if err != nil || len(before.Providers) != 0 || repo.ok {
		t.Fatalf("pre-save providers = %#v repo.ok=%v err=%v", before, repo.ok, err)
	}
	saved, err := service.UpsertAdminRAGProviderConfig(
		context.Background(),
		"mineru",
		UpdateAdminRAGProviderConfigRequest{
			Name: "MinerU",
			APIKeySecret: encryptedSecretEnvelope(
				t,
				privateKey,
				fixtureCredential,
				ragProviderIngressContext(RAGProviderMinerU),
			),
		},
	)
	if err != nil || !saved.HasAPIKey || strings.Contains(repo.stored.EncryptedSecretRef, fixtureCredential) {
		t.Fatalf("BYOK save = %#v err=%v", saved, err)
	}
	plaintext, err := vault.Decrypt(
		mustParseProviderSecretEnvelope(t, repo.stored.EncryptedSecretRef),
		ragProviderSecretContext(repo.stored.UserID, repo.stored.ProviderID),
	)
	if err != nil || string(plaintext) != fixtureCredential {
		t.Fatalf("vault plaintext mismatch: %v", err)
	}
	clear(plaintext)
	tested, err := service.TestAdminRAGProviderConnection(context.Background(), "mineru")
	if err != nil || strings.Join(tested.Checks, ",") != "allocate" ||
		tested.Provider.ParserModel != minerUModelVersion {
		t.Fatalf("MinerU test = %#v err=%v", tested, err)
	}

	models, err := service.AdminProviderConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models.Providers {
		if model.ID == ragProviderRecordID(RAGProviderMinerU) {
			t.Fatalf("RAG row leaked into model provider list: %#v", models)
		}
	}
}

func TestAdminRAGProviderFailedTestDoesNotActivateOrLeakBody(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "rag-v1", 43)),
		WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"secret":"must-not-escape"}`)),
			}, nil
		})),
	)
	_, err := service.UpsertAdminRAGProviderConfig(
		context.Background(),
		"jina",
		UpdateAdminRAGProviderConfigRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			"wrong-jina-fixture",
			ragProviderIngressContext(RAGProviderJina),
		)},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ActivateAdminRAGProvider(context.Background(), "jina")
	if !errors.Is(err, ErrRAGProviderConnectionFailed) {
		t.Fatalf("ActivateAdminRAGProvider() error = %v", err)
	}
	if repo.stored.Config.Enabled || repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" || strings.Contains(err.Error(), "must-not-escape") {
		t.Fatalf("failed activation mutated or leaked state: %#v err=%v", repo.stored.Config, err)
	}
}

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
	jina := create(RAGProviderJina, "jina-status-fixture")
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{minerU, jina},
		}),
		WithProviderSecretVault(vault),
	)
	status, err := service.RAGProviderStatus(context.Background())
	if err != nil || !status.Ready ||
		status.Status != ragproviders.ServiceStatusReady ||
		status.Providers.MinerU.Status != ragproviders.ProviderStatusReady ||
		status.Providers.Jina.Status != ragproviders.ProviderStatusReady ||
		status.Providers.Jina.EmbeddingDimensions != jinaDimensions ||
		!status.Capabilities.PDFParsing || !status.Capabilities.NativeIndexing ||
		!status.Capabilities.Retrieval {
		t.Fatalf("RAG provider status = %#v, %v", status, err)
	}

	jina.EncryptedSecretRef = minerU.EncryptedSecretRef
	jina.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		jina.ProviderID, RAGProviderJina, jina.EncryptedSecretRef,
	)
	service = NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{minerU, jina},
		}),
		WithProviderSecretVault(vault),
	)
	status, err = service.RAGProviderStatus(context.Background())
	if err != nil || status.Ready ||
		status.Status != ragproviders.ServiceStatusUnavailable ||
		status.Providers.Jina.Status != ragproviders.ProviderStatusUnavailable ||
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

func mustParseProviderSecretEnvelope(
	t *testing.T,
	value string,
) providersecrets.Envelope {
	t.Helper()
	envelope, err := providersecrets.ParseEnvelope(value)
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}
