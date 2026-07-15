package imagejobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	openAICompatibleProviderID              = "openai_compatible"
	openAICompatibleProviderIDOpenAI        = "openai"
	openAICompatibleProviderIDHyphenVariant = "openai-compatible"
	openAICompatibleImagesPath              = "/images/generations"
	maxOpenAICompatibleImageResponseBytes   = 8 << 20
	maxOpenAICompatibleGeneratedImageBytes  = 64 << 20
)

var (
	ErrImageProviderFailed           = errors.New("image provider failed")
	errOpenAICompatibleImageProvider = ErrImageProviderFailed
)

type OpenAICompatibleExecutorConfig struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type OpenAICompatibleExecutor struct {
	endpoint string
	apiKey   string
	timeout  time.Duration
	client   *http.Client
}

func NewOpenAICompatibleExecutor(cfg OpenAICompatibleExecutorConfig) (*OpenAICompatibleExecutor, error) {
	baseURL, err := normalizeOpenAICompatibleBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("openai-compatible image provider api key is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &OpenAICompatibleExecutor{
		endpoint: baseURL + openAICompatibleImagesPath,
		apiKey:   apiKey,
		timeout:  cfg.Timeout,
		client:   client,
	}, nil
}

func (e *OpenAICompatibleExecutor) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	modelRef, err := resolveOpenAICompatibleImageModelRef(request.ModelRef)
	if err != nil {
		return GenerateResult{}, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return GenerateResult{}, errors.New("openai-compatible image provider model is required")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return GenerateResult{}, errors.New("openai-compatible image provider prompt is required")
	}
	count := request.Count
	if count <= 0 {
		count = defaultImageCount
	}

	payload, err := json.Marshal(openAICompatibleImageRequest{
		Model:  modelRef.ModelID,
		Prompt: prompt,
		Size:   strings.TrimSpace(request.Size),
		N:      count,
	})
	if err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible image request encode failed: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if e.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible image request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible image request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return GenerateResult{}, fmt.Errorf("%w: status %d", errOpenAICompatibleImageProvider, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleImageResponseBytes+1))
	if err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible image response read failed: %w", err)
	}
	if len(body) > maxOpenAICompatibleImageResponseBytes {
		return GenerateResult{}, errors.New("openai-compatible image response is too large")
	}

	var decoded openAICompatibleImageResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible image response decode failed: %w", err)
	}
	if len(decoded.Data) == 0 {
		return GenerateResult{}, errors.New("openai-compatible image response did not include images")
	}

	images := make([]GeneratedImageResult, 0, len(decoded.Data))
	for index, item := range decoded.Data {
		image, err := e.generatedImageResult(ctx, index, item)
		if err != nil {
			return GenerateResult{}, err
		}
		images = append(images, image)
	}
	return GenerateResult{Images: images}, nil
}

func (e *OpenAICompatibleExecutor) generatedImageResult(
	ctx context.Context,
	index int,
	item openAICompatibleImageData,
) (GeneratedImageResult, error) {
	if encoded := strings.TrimSpace(item.B64JSON); encoded != "" {
		content, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return GeneratedImageResult{}, errors.New("openai-compatible image response returned invalid base64")
		}
		if len(content) == 0 {
			return GeneratedImageResult{}, errors.New("openai-compatible image response returned empty image")
		}
		if len(content) > maxOpenAICompatibleGeneratedImageBytes {
			return GeneratedImageResult{}, errors.New("openai-compatible generated image is too large")
		}
		contentType := http.DetectContentType(content)
		return GeneratedImageResult{
			JobID:       generatedImageJobID(index),
			Filename:    generatedImageFilename(index, contentType),
			ContentType: contentType,
			Size:        int64(len(content)),
			Body:        bytes.NewReader(content),
		}, nil
	}
	if imageURL := strings.TrimSpace(item.URL); imageURL != "" {
		return e.fetchGeneratedImageURL(ctx, index, imageURL)
	}
	return GeneratedImageResult{}, errors.New("openai-compatible image response missing image content")
}

func (e *OpenAICompatibleExecutor) fetchGeneratedImageURL(
	ctx context.Context,
	index int,
	imageURL string,
) (GeneratedImageResult, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return GeneratedImageResult{}, errors.New("openai-compatible image response returned invalid image url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return GeneratedImageResult{}, errors.New("openai-compatible image response returned unsupported image url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return GeneratedImageResult{}, fmt.Errorf("openai-compatible image fetch build failed: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return GeneratedImageResult{}, fmt.Errorf("openai-compatible image fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return GeneratedImageResult{}, fmt.Errorf("%w: image fetch status %d", errOpenAICompatibleImageProvider, resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleGeneratedImageBytes+1))
	if err != nil {
		return GeneratedImageResult{}, fmt.Errorf("openai-compatible image fetch read failed: %w", err)
	}
	if len(content) == 0 {
		return GeneratedImageResult{}, errors.New("openai-compatible image fetch returned empty image")
	}
	if len(content) > maxOpenAICompatibleGeneratedImageBytes {
		return GeneratedImageResult{}, errors.New("openai-compatible generated image is too large")
	}
	contentType := mediaTypeOnly(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	return GeneratedImageResult{
		JobID:       generatedImageJobID(index),
		Filename:    generatedImageFilename(index, contentType),
		ContentType: contentType,
		Size:        int64(len(content)),
		Body:        bytes.NewReader(content),
	}, nil
}

type openAICompatibleImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
	N      int    `json:"n,omitempty"`
}

type openAICompatibleImageResponse struct {
	Data []openAICompatibleImageData `json:"data"`
}

type openAICompatibleImageData struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

func resolveOpenAICompatibleImageModelRef(modelRef ModelRef) (ModelRef, error) {
	providerID := strings.ToLower(strings.TrimSpace(modelRef.ProviderID))
	switch providerID {
	case openAICompatibleProviderID, openAICompatibleProviderIDOpenAI, openAICompatibleProviderIDHyphenVariant:
		return ModelRef{ProviderID: openAICompatibleProviderID, ModelID: strings.TrimSpace(modelRef.ModelID)}, nil
	default:
		return ModelRef{}, errors.New("openai-compatible image provider id is not supported")
	}
}

func normalizeOpenAICompatibleBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", errors.New("openai-compatible image provider base url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("openai-compatible image provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("openai-compatible image provider base url must use http or https")
	}
	return value, nil
}

func mediaTypeOnly(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(mediaType)
}

func generatedImageJobID(index int) string {
	return fmt.Sprintf("image-generate-%d", index+1)
}

func generatedImageFilename(index int, contentType string) string {
	suffix := "bin"
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		suffix = "png"
	case "image/jpeg":
		suffix = "jpg"
	case "image/webp":
		suffix = "webp"
	case "image/gif":
		suffix = "gif"
	}
	return fmt.Sprintf("generated-%d.%s", index+1, suffix)
}
