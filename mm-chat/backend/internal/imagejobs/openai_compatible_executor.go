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
	openAICompatibleImageResponseFormat     = "b64_json"
	maxOpenAICompatibleImageResponseBytes   = 8 << 20
	maxOpenAICompatibleImageStreamBytes     = 32 << 20
	maxOpenAICompatibleGeneratedImageBytes  = 64 << 20
	maxOpenAICompatibleImageAttempts        = 2
	openAICompatibleImagePartialImages      = 1
	openAICompatibleImageRetryDelay         = 250 * time.Millisecond
)

var ErrImageProviderFailed = errors.New("image provider failed")

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

	stream := supportsOpenAICompatibleImageStreaming(modelRef.ModelID)
	providerRequest := openAICompatibleImageRequest{
		Model:  modelRef.ModelID,
		Prompt: prompt,
		Size:   strings.TrimSpace(request.Size),
		N:      count,
		Stream: stream,
	}
	if stream {
		providerRequest.PartialImages = openAICompatibleImagePartialImages
	} else {
		providerRequest.ResponseFormat = openAICompatibleImageResponseFormat
	}
	payload, err := json.Marshal(providerRequest)
	if err != nil {
		return GenerateResult{}, newProviderStageError(
			"IMAGE_PROVIDER_REQUEST_ENCODE_FAILED",
			"openai-compatible image request encode failed",
			err,
		)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if e.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	for attempt := 1; attempt <= maxOpenAICompatibleImageAttempts; attempt++ {
		result, err := e.generateAttempt(requestCtx, payload, stream)
		if err == nil {
			return result, nil
		}
		if attempt == maxOpenAICompatibleImageAttempts || !isRetryableImageProviderError(err) {
			return GenerateResult{}, err
		}
		timer := time.NewTimer(openAICompatibleImageRetryDelay)
		select {
		case <-requestCtx.Done():
			timer.Stop()
			return GenerateResult{}, requestCtx.Err()
		case <-timer.C:
		}
	}
	return GenerateResult{}, errors.New("openai-compatible image attempts exhausted")
}

func (e *OpenAICompatibleExecutor) generateAttempt(
	ctx context.Context,
	payload []byte,
	stream bool,
) (GenerateResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(payload))
	if err != nil {
		return GenerateResult{}, newProviderStageError(
			"IMAGE_PROVIDER_REQUEST_BUILD_FAILED",
			"openai-compatible image request build failed",
			err,
		)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream, application/json")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return GenerateResult{}, newProviderStageError(
			"IMAGE_PROVIDER_REQUEST_FAILED",
			"openai-compatible image request failed",
			err,
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		errorCode, errorType := providerErrorIdentity(body)
		return GenerateResult{}, &providerHTTPError{
			Stage:      "request",
			StatusCode: resp.StatusCode,
			ErrorCode:  errorCode,
			ErrorType:  errorType,
		}
	}

	decoded, err := decodeOpenAICompatibleImageResponse(resp)
	if err != nil {
		return GenerateResult{}, err
	}
	if len(decoded.Data) == 0 {
		return GenerateResult{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_EMPTY",
			"openai-compatible image response did not include images",
			nil,
		)
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

func decodeOpenAICompatibleImageResponse(
	resp *http.Response,
) (openAICompatibleImageResponse, error) {
	if mediaTypeOnly(resp.Header.Get("Content-Type")) == "text/event-stream" {
		return decodeOpenAICompatibleImageStream(resp.Body)
	}
	return decodeOpenAICompatibleImageJSON(resp.Body)
}

func decodeOpenAICompatibleImageJSON(
	bodyReader io.Reader,
) (openAICompatibleImageResponse, error) {
	body, err := io.ReadAll(io.LimitReader(bodyReader, maxOpenAICompatibleImageResponseBytes+1))
	if err != nil {
		return openAICompatibleImageResponse{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_READ_FAILED",
			"openai-compatible image response read failed",
			err,
		)
	}
	if len(body) > maxOpenAICompatibleImageResponseBytes {
		return openAICompatibleImageResponse{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_TOO_LARGE",
			"openai-compatible image response is too large",
			nil,
		)
	}

	var decoded openAICompatibleImageResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return openAICompatibleImageResponse{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_DECODE_FAILED",
			"openai-compatible image response decode failed",
			err,
		)
	}
	return decoded, nil
}

func decodeOpenAICompatibleImageStream(
	bodyReader io.Reader,
) (openAICompatibleImageResponse, error) {
	body, err := io.ReadAll(io.LimitReader(bodyReader, maxOpenAICompatibleImageStreamBytes+1))
	if err != nil {
		return openAICompatibleImageResponse{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_READ_FAILED",
			"openai-compatible image stream read failed",
			err,
		)
	}
	if len(body) > maxOpenAICompatibleImageStreamBytes {
		return openAICompatibleImageResponse{}, newProviderStageError(
			"IMAGE_PROVIDER_RESPONSE_TOO_LARGE",
			"openai-compatible image stream is too large",
			nil,
		)
	}

	normalizedBody := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	completed := make([]openAICompatibleImageData, 0, 1)
	var lastPartial openAICompatibleImageData
	for _, frame := range bytes.Split(normalizedBody, []byte("\n\n")) {
		data := openAICompatibleSSEData(frame)
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		var event openAICompatibleImageStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return openAICompatibleImageResponse{}, newProviderStageError(
				"IMAGE_PROVIDER_RESPONSE_DECODE_FAILED",
				"openai-compatible image stream decode failed",
				err,
			)
		}
		item := openAICompatibleImageData{B64JSON: event.B64JSON, URL: event.URL}
		switch event.Type {
		case "image_generation.partial_image":
			if hasOpenAICompatibleImageData(item) {
				lastPartial = item
			}
		case "image_generation.completed":
			if hasOpenAICompatibleImageData(item) {
				completed = append(completed, item)
			}
		case "error", "image_generation.failed":
			return openAICompatibleImageResponse{}, newProviderStageError(
				"IMAGE_PROVIDER_STREAM_FAILED",
				"openai-compatible image stream failed",
				nil,
			)
		}
	}
	if len(completed) > 0 {
		return openAICompatibleImageResponse{Data: completed}, nil
	}
	if hasOpenAICompatibleImageData(lastPartial) {
		return openAICompatibleImageResponse{Data: []openAICompatibleImageData{lastPartial}}, nil
	}
	return openAICompatibleImageResponse{}, nil
}

func hasOpenAICompatibleImageData(item openAICompatibleImageData) bool {
	return strings.TrimSpace(item.B64JSON) != "" || strings.TrimSpace(item.URL) != ""
}

func openAICompatibleSSEData(frame []byte) []byte {
	var data []byte
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		value := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) > 0 {
			data = append(data, '\n')
		}
		data = append(data, value...)
	}
	return data
}

func (e *OpenAICompatibleExecutor) generatedImageResult(
	ctx context.Context,
	index int,
	item openAICompatibleImageData,
) (GeneratedImageResult, error) {
	if encoded := strings.TrimSpace(item.B64JSON); encoded != "" {
		content, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return GeneratedImageResult{}, newProviderStageError(
				"IMAGE_PROVIDER_BASE64_INVALID",
				"openai-compatible image response returned invalid base64",
				err,
			)
		}
		if len(content) == 0 {
			return GeneratedImageResult{}, newProviderStageError(
				"IMAGE_PROVIDER_IMAGE_EMPTY",
				"openai-compatible image response returned empty image",
				nil,
			)
		}
		if len(content) > maxOpenAICompatibleGeneratedImageBytes {
			return GeneratedImageResult{}, newProviderStageError(
				"IMAGE_PROVIDER_IMAGE_TOO_LARGE",
				"openai-compatible generated image is too large",
				nil,
			)
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
	return GeneratedImageResult{}, newProviderStageError(
		"IMAGE_PROVIDER_IMAGE_CONTENT_MISSING",
		"openai-compatible image response missing image content",
		nil,
	)
}

func (e *OpenAICompatibleExecutor) fetchGeneratedImageURL(
	ctx context.Context,
	index int,
	imageURL string,
) (GeneratedImageResult, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_IMAGE_URL_INVALID",
			"openai-compatible image response returned invalid image url",
			err,
		)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_IMAGE_URL_UNSUPPORTED",
			"openai-compatible image response returned unsupported image url",
			nil,
		)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_FETCH_BUILD_FAILED",
			"openai-compatible image fetch build failed",
			err,
		)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_FETCH_FAILED",
			"openai-compatible image fetch failed",
			err,
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return GeneratedImageResult{}, &providerHTTPError{
			Stage:      "image fetch",
			StatusCode: resp.StatusCode,
		}
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleGeneratedImageBytes+1))
	if err != nil {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_FETCH_READ_FAILED",
			"openai-compatible image fetch read failed",
			err,
		)
	}
	if len(content) == 0 {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_FETCH_EMPTY",
			"openai-compatible image fetch returned empty image",
			nil,
		)
	}
	if len(content) > maxOpenAICompatibleGeneratedImageBytes {
		return GeneratedImageResult{}, newProviderStageError(
			"IMAGE_PROVIDER_FETCH_TOO_LARGE",
			"openai-compatible generated image is too large",
			nil,
		)
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
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Size           string `json:"size,omitempty"`
	N              int    `json:"n,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Stream         bool   `json:"stream,omitempty"`
	PartialImages  int    `json:"partial_images,omitempty"`
}

type openAICompatibleImageResponse struct {
	Data []openAICompatibleImageData `json:"data"`
}

type openAICompatibleImageData struct {
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

type openAICompatibleImageStreamEvent struct {
	Type    string `json:"type"`
	B64JSON string `json:"b64_json"`
	URL     string `json:"url"`
}

func supportsOpenAICompatibleImageStreaming(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), "gpt-image-")
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
