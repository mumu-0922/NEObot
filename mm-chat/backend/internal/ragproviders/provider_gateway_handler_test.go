package ragproviders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const providerGatewayHandlerTestToken = "provider-gateway-handler-test-token"

type fakeProviderOperations struct {
	allocateResult MinerUAllocation
	allocateError  error
	allocateCalls  int
	pollResult     MinerUPollResult
	pollError      error
	pollCalls      int
	embedResult    PassageEmbeddingResponse
	embedError     error
	embedCalls     int
}

func (operations *fakeProviderOperations) AllocateMinerU(
	_ context.Context,
	_ MinerUAllocateRequest,
) (MinerUAllocation, error) {
	operations.allocateCalls++
	return operations.allocateResult, operations.allocateError
}

func (operations *fakeProviderOperations) PollMinerU(
	_ context.Context,
	_ MinerUPollRequest,
) (MinerUPollResult, error) {
	operations.pollCalls++
	return operations.pollResult, operations.pollError
}

func (operations *fakeProviderOperations) EmbedPassages(
	_ context.Context,
	_ PassageEmbeddingRequest,
) (PassageEmbeddingResponse, error) {
	operations.embedCalls++
	return operations.embedResult, operations.embedError
}

func TestProviderGatewayHandlerAuthenticatesBeforeParsing(t *testing.T) {
	operations := &fakeProviderOperations{}
	handler := NewProviderGatewayHandler(providerGatewayHandlerTestToken, operations)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		InternalMinerUAllocatePath,
		strings.NewReader(`{"filename":"document.pdf"}`),
	)
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized || operations.allocateCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, operations.allocateCalls, recorder.Body.String())
	}
	assertProviderGatewayErrorCode(t, recorder, "RAG_PROVIDER_GATEWAY_UNAUTHORIZED")
}

func TestProviderGatewayHandlerRejectsCallerControlsAndOversizedBodies(t *testing.T) {
	operations := &fakeProviderOperations{}
	handler := NewProviderGatewayHandler(providerGatewayHandlerTestToken, operations)

	recorder := httptest.NewRecorder()
	request := providerGatewayHandlerRequest(
		InternalMinerUAllocatePath,
		`{"filename":"document.pdf","url":"https://evil.example","headers":{"Authorization":"bad"}}`,
	)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || operations.allocateCalls != 0 {
		t.Fatalf("unknown fields status=%d calls=%d body=%s", recorder.Code, operations.allocateCalls, recorder.Body.String())
	}
	assertProviderGatewayErrorCode(t, recorder, "RAG_PROVIDER_OPERATION_UNSUPPORTED")

	recorder = httptest.NewRecorder()
	request = providerGatewayHandlerRequest(
		InternalMinerUAllocatePath,
		strings.Repeat("x", providerGatewaySmallRequestBytes+1),
	)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || operations.allocateCalls != 0 {
		t.Fatalf("oversize status=%d calls=%d body=%s", recorder.Code, operations.allocateCalls, recorder.Body.String())
	}
	assertProviderGatewayErrorCode(t, recorder, "RAG_PROVIDER_REQUEST_INVALID")
}

func TestProviderGatewayHandlerRoutesClosedOperations(t *testing.T) {
	operations := &fakeProviderOperations{
		allocateResult: MinerUAllocation{
			BatchID: "fixture-batch", Filename: "document.pdf",
			UploadURL: "https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/document.pdf?signature=fixture",
		},
		pollResult: MinerUPollResult{
			BatchID: "fixture-batch", Filename: "document.pdf", State: "pending",
		},
		embedResult: PassageEmbeddingResponse{
			Model: JinaEmbeddingModel, Dimensions: JinaEmbeddingDimensions,
			Vectors: []PassageEmbeddingVector{{
				PassageID: "11111111-1111-4111-8111-111111111111",
				Embedding: repeatedProviderGatewayHandlerEmbedding(0.01),
			}},
		},
	}
	handler := NewProviderGatewayHandler(providerGatewayHandlerTestToken, operations)
	cases := []struct {
		path string
		body string
	}{
		{InternalMinerUAllocatePath, `{"filename":"document.pdf"}`},
		{InternalMinerUPollPath, `{"batchId":"fixture-batch","filename":"document.pdf"}`},
		{InternalJinaEmbeddingsPath, `{"passages":[{"passageId":"11111111-1111-4111-8111-111111111111","text":"fixture"}]}`},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, providerGatewayHandlerRequest(tc.path, tc.body))
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf(
				"%s status=%d cache=%q body=%s",
				tc.path,
				recorder.Code,
				recorder.Header().Get("Cache-Control"),
				recorder.Body.String(),
			)
		}
		if strings.Contains(recorder.Body.String(), providerGatewayHandlerTestToken) {
			t.Fatalf("%s leaked internal token", tc.path)
		}
	}
	if operations.allocateCalls != 1 || operations.pollCalls != 1 || operations.embedCalls != 1 {
		t.Fatalf(
			"operation calls = allocate:%d poll:%d embed:%d",
			operations.allocateCalls,
			operations.pollCalls,
			operations.embedCalls,
		)
	}
}

func TestProviderGatewayHandlerRejectsUnknownOperationAfterTokenGate(t *testing.T) {
	operations := &fakeProviderOperations{}
	handler := NewProviderGatewayHandler(providerGatewayHandlerTestToken, operations)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		providerGatewayHandlerRequest(
			InternalProviderPathPrefix+"generic-proxy",
			`{"operation":"GET"}`,
		),
	)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertProviderGatewayErrorCode(t, recorder, "RAG_PROVIDER_OPERATION_UNSUPPORTED")
}

func TestProviderGatewayHandlerMapsStableRedactedErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{ErrProviderGatewayOperationUnsupported, http.StatusBadRequest, "RAG_PROVIDER_OPERATION_UNSUPPORTED"},
		{ErrProviderGatewayInvalid, http.StatusBadRequest, "RAG_PROVIDER_REQUEST_INVALID"},
		{ErrProviderGatewayNotFound, http.StatusNotFound, "RAG_PROVIDER_NOT_FOUND"},
		{ErrProviderGatewayActivationRequired, http.StatusConflict, "RAG_PROVIDER_ACTIVATION_REQUIRED"},
		{ErrProviderGatewayUnavailable, http.StatusServiceUnavailable, "RAG_PROVIDER_UNAVAILABLE"},
		{errors.New("private upstream body"), http.StatusBadGateway, "RAG_PROVIDER_UPSTREAM_FAILED"},
	}
	for _, tc := range cases {
		operations := &fakeProviderOperations{allocateError: tc.err}
		handler := NewProviderGatewayHandler(providerGatewayHandlerTestToken, operations)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(
			recorder,
			providerGatewayHandlerRequest(
				InternalMinerUAllocatePath,
				`{"filename":"document.pdf"}`,
			),
		)
		if recorder.Code != tc.status || strings.Contains(recorder.Body.String(), "private upstream body") {
			t.Fatalf("error=%v status=%d body=%s", tc.err, recorder.Code, recorder.Body.String())
		}
		assertProviderGatewayErrorCode(t, recorder, tc.code)
	}
}

func providerGatewayHandlerRequest(path string, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set(InternalProviderTokenHeader, providerGatewayHandlerTestToken)
	return request
}

func assertProviderGatewayErrorCode(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	want string,
) {
	t.Helper()
	var response ErrorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != want {
		t.Fatalf("error code = %q, want %q", response.Error.Code, want)
	}
}

func repeatedProviderGatewayHandlerEmbedding(value float32) []float32 {
	vector := make([]float32, JinaEmbeddingDimensions)
	for index := range vector {
		vector[index] = value
	}
	return vector
}
