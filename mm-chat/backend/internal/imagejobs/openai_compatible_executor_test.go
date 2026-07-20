package imagejobs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleExecutorGeneratesFromBase64Response(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nimage")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://provider.test/v1/images/generations" {
			t.Fatalf("url = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload openAICompatibleImageRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "gpt-image-1" || payload.Prompt != "paint" ||
			payload.N != 1 || payload.Size != "512x512" ||
			payload.ResponseFormat != "b64_json" {
			t.Fatalf("payload = %#v", payload)
		}
		return jsonResponse(http.StatusOK, openAICompatibleImageResponse{Data: []openAICompatibleImageData{{
			B64JSON: base64.StdEncoding.EncodeToString(pngBytes),
		}}}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	result, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "paint",
		Size:     "512x512",
		Count:    1,
	})

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.Images) != 1 {
		t.Fatalf("images = %d, want 1", len(result.Images))
	}
	image := result.Images[0]
	if image.JobID != "image-generate-1" ||
		image.Filename != "generated-1.png" ||
		image.ContentType != "image/png" ||
		image.Size != int64(len(pngBytes)) {
		t.Fatalf("image = %#v", image)
	}
	body, err := io.ReadAll(image.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != string(pngBytes) {
		t.Fatalf("body = %q", string(body))
	}
}

func TestOpenAICompatibleExecutorGeneratesFromURLResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case "https://provider.test/v1/images/generations":
			return jsonResponse(http.StatusOK, openAICompatibleImageResponse{Data: []openAICompatibleImageData{{
				URL: "https://provider.test/generated.webp",
			}}}), nil
		case "https://provider.test/generated.webp":
			return bytesResponse(http.StatusOK, "image/webp; charset=binary", []byte("RIFFxxxxWEBPVP8 ")), nil
		default:
			t.Fatalf("unexpected url = %s", r.URL.String())
			return nil, nil
		}
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	result, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai_compatible", ModelID: "image-model"},
		Prompt:   "paint",
	})

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.Images) != 1 ||
		result.Images[0].ContentType != "image/webp" ||
		result.Images[0].Filename != "generated-1.webp" {
		t.Fatalf("images = %#v", result.Images)
	}
}

func TestOpenAICompatibleExecutorRetriesTransientEmptyResponseOnce(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nretry")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusOK, openAICompatibleImageResponse{}), nil
		}
		return jsonResponse(http.StatusOK, openAICompatibleImageResponse{Data: []openAICompatibleImageData{{
			B64JSON: base64.StdEncoding.EncodeToString(pngBytes),
		}}}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	result, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "image-model"},
		Prompt:   "paint",
	})

	if err != nil || len(result.Images) != 1 || calls != 2 {
		t.Fatalf("Generate() = images:%d calls:%d err:%v", len(result.Images), calls, err)
	}
}

func TestOpenAICompatibleExecutorRejectsBadProviderAndStatusWithoutLeakingBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{
			"error": map[string]any{
				"code":    "invalid-api-key",
				"type":    "authentication_error",
				"message": "secret provider body",
			},
		}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "other", ModelID: "image-model"},
		Prompt:   "paint",
	})
	if err == nil || !strings.Contains(err.Error(), "provider id is not supported") {
		t.Fatalf("Generate() error = %v, want unsupported provider", err)
	}

	_, err = executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "image-model"},
		Prompt:   "paint",
	})
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("Generate() error = %v, want status 401", err)
	}
	if strings.Contains(err.Error(), "secret provider body") {
		t.Fatalf("Generate() leaked provider body: %v", err)
	}
	if got := FailureReason(err); got != "IMAGE_PROVIDER_REQUEST_HTTP_401_CODE_INVALID_API_KEY" {
		t.Fatalf("FailureReason() = %q", got)
	}
}

func TestImageFailureReasonClassifiesUnavailableExecutor(t *testing.T) {
	if got := FailureReason(ErrImageJobsUnavailable); got != "IMAGE_EXECUTOR_UNAVAILABLE" {
		t.Fatalf("FailureReason() = %q", got)
	}
}

func TestImageFailureDetectsContentPolicyViolationWithoutMessageInspection(t *testing.T) {
	err := &providerHTTPError{
		Stage:      "request",
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "CONTENT_POLICY_VIOLATION",
	}
	if !IsContentPolicyViolation(err) {
		t.Fatal("IsContentPolicyViolation() = false")
	}
	if IsContentPolicyViolation(errors.New("content_policy_violation")) {
		t.Fatal("plain error text must not be trusted as provider identity")
	}
}

func TestProviderErrorIdentityDoesNotExposeUnrecognizedLabels(t *testing.T) {
	code, errorType := providerErrorIdentity([]byte(`{
		"error": {
			"code": "provider-secret-value\r\nforged-log-line",
			"type": "custom-secret-type"
		}
	}`))

	if code != "UNRECOGNIZED" || errorType != "UNRECOGNIZED" {
		t.Fatalf("providerErrorIdentity() = %q, %q", code, errorType)
	}
}

func TestOpenAICompatibleExecutorDoesNotRetryContentPolicyViolation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"code":    "content_policy_violation",
				"type":    "invalid_request_error",
				"message": "private provider policy detail",
			},
		}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "image-model"},
		Prompt:   "paint",
	})

	if err == nil || !IsContentPolicyViolation(err) {
		t.Fatalf("Generate() error = %v, want content policy violation", err)
	}
	if got := FailureReason(err); got != "IMAGE_PROVIDER_REQUEST_HTTP_400_CODE_CONTENT_POLICY_VIOLATION" {
		t.Fatalf("FailureReason() = %q", got)
	}
	if strings.Contains(err.Error(), "private provider policy detail") {
		t.Fatalf("Generate() leaked provider body: %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want no retry", calls)
	}
}

func TestOpenAICompatibleExecutorRejectsInvalidImagePayload(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(http.StatusOK, openAICompatibleImageResponse{
			Data: []openAICompatibleImageData{{B64JSON: "not-base64"}},
		}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "image-model"},
		Prompt:   "paint",
	})

	if err == nil || !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("Generate() error = %v, want invalid base64", err)
	}
	if got := FailureReason(err); got != "IMAGE_PROVIDER_BASE64_INVALID" {
		t.Fatalf("FailureReason() = %q", got)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want no retry", calls)
	}
}

func TestOpenAICompatibleExecutorClassifiesEmptyProviderResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, openAICompatibleImageResponse{}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "image-model"},
		Prompt:   "paint",
	})

	if got := FailureReason(err); got != "IMAGE_PROVIDER_RESPONSE_EMPTY" {
		t.Fatalf("FailureReason() = %q", got)
	}
}

func newTestOpenAICompatibleExecutor(t *testing.T, client *http.Client) *OpenAICompatibleExecutor {
	t.Helper()
	executor, err := NewOpenAICompatibleExecutor(OpenAICompatibleExecutorConfig{
		BaseURL:    "https://provider.test/v1",
		APIKey:     "test-key",
		Timeout:    time.Second,
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecutor() error = %v", err)
	}
	return executor
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, payload any) *http.Response {
	encoded, _ := json.Marshal(payload)
	return bytesResponse(status, "application/json", encoded)
}

func bytesResponse(status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}
