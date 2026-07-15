package voicejobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const (
	openAICompatibleSpeechPath                = "/audio/speech"
	openAICompatibleTranscriptionsPath        = "/audio/transcriptions"
	maxOpenAICompatibleVoiceResponseBytes     = 10 << 20
	maxOpenAICompatibleTranscribeResponseSize = 1 << 20
	defaultOpenAICompatibleSpeechVoice        = "alloy"
)

var ErrVoiceProviderFailed = errors.New("voice provider failed")

type OpenAICompatibleExecutorConfig struct {
	BaseURL                   string
	APIKey                    string
	Timeout                   time.Duration
	HTTPClient                *http.Client
	DefaultTranscriptionModel string
	DefaultSpeechModel        string
	DefaultSpeechVoice        string
}

type OpenAICompatibleExecutor struct {
	baseURL                   string
	apiKey                    string
	timeout                   time.Duration
	client                    *http.Client
	defaultTranscriptionModel string
	defaultSpeechModel        string
	defaultSpeechVoice        string
}

func NewOpenAICompatibleExecutor(cfg OpenAICompatibleExecutorConfig) (*OpenAICompatibleExecutor, error) {
	baseURL, err := normalizeOpenAICompatibleBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("openai-compatible voice provider api key is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	voice := strings.TrimSpace(cfg.DefaultSpeechVoice)
	if voice == "" {
		voice = defaultOpenAICompatibleSpeechVoice
	}
	return &OpenAICompatibleExecutor{
		baseURL:                   baseURL,
		apiKey:                    apiKey,
		timeout:                   cfg.Timeout,
		client:                    client,
		defaultTranscriptionModel: strings.TrimSpace(cfg.DefaultTranscriptionModel),
		defaultSpeechModel:        strings.TrimSpace(cfg.DefaultSpeechModel),
		defaultSpeechVoice:        voice,
	}, nil
}

func (e *OpenAICompatibleExecutor) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	if err := requireOpenAICompatibleVoiceProvider(request.Provider); err != nil {
		return TranscribeResponse{}, err
	}
	modelID := strings.TrimSpace(request.ModelID)
	if modelID == "" {
		modelID = e.defaultTranscriptionModel
	}
	if modelID == "" {
		return TranscribeResponse{}, errors.New("openai-compatible voice transcription model is required")
	}
	if request.Audio == nil {
		return TranscribeResponse{}, errors.New("openai-compatible voice transcription audio is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(audioPartHeader(request))
	if err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription multipart build failed: %w", err)
	}
	if _, err := io.Copy(part, request.Audio); err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription audio read failed: %w", err)
	}
	if err := writer.WriteField("model", modelID); err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription model field failed: %w", err)
	}
	if language := providerTranscriptionLanguage(request.Language); language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription language field failed: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription multipart close failed: %w", err)
	}

	requestCtx, cancel := e.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		e.baseURL+openAICompatibleTranscriptionsPath,
		&body,
	)
	if err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return TranscribeResponse{}, fmt.Errorf("%w: transcription status %d", ErrVoiceProviderFailed, resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleTranscribeResponseSize+1))
	if err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription response read failed: %w", err)
	}
	if len(bodyBytes) > maxOpenAICompatibleTranscribeResponseSize {
		return TranscribeResponse{}, errors.New("openai-compatible voice transcription response is too large")
	}
	var decoded openAICompatibleTranscriptionResponse
	if err := json.Unmarshal(bodyBytes, &decoded); err != nil {
		return TranscribeResponse{}, fmt.Errorf("openai-compatible voice transcription response decode failed: %w", err)
	}
	return TranscribeResponse{Text: decoded.Text}, nil
}

func (e *OpenAICompatibleExecutor) Synthesize(ctx context.Context, request SynthesizeRequest) (SynthesizeResult, error) {
	if err := requireOpenAICompatibleVoiceProvider(request.Provider); err != nil {
		return SynthesizeResult{}, err
	}
	modelID := strings.TrimSpace(request.ModelID)
	if modelID == "" {
		modelID = e.defaultSpeechModel
	}
	if modelID == "" {
		return SynthesizeResult{}, errors.New("openai-compatible voice synthesis model is required")
	}
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return SynthesizeResult{}, errors.New("openai-compatible voice synthesis text is required")
	}
	voice := strings.TrimSpace(request.VoiceID)
	if voice == "" {
		voice = e.defaultSpeechVoice
	}

	payload, err := json.Marshal(openAICompatibleSpeechRequest{
		Model: modelID,
		Input: text,
		Voice: voice,
	})
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("openai-compatible voice synthesis request encode failed: %w", err)
	}

	requestCtx, cancel := e.requestContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		e.baseURL+openAICompatibleSpeechPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("openai-compatible voice synthesis request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/*, application/octet-stream")

	resp, err := e.client.Do(req)
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("openai-compatible voice synthesis request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return SynthesizeResult{}, fmt.Errorf("%w: synthesis status %d", ErrVoiceProviderFailed, resp.StatusCode)
	}

	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleVoiceResponseBytes+1))
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("openai-compatible voice synthesis response read failed: %w", err)
	}
	if len(audio) == 0 {
		return SynthesizeResult{}, errors.New("openai-compatible voice synthesis response returned empty audio")
	}
	if len(audio) > maxOpenAICompatibleVoiceResponseBytes {
		return SynthesizeResult{}, errors.New("openai-compatible voice synthesis response is too large")
	}
	contentType := mediaTypeOnly(resp.Header.Get("Content-Type"))
	if contentType == "" || !strings.HasPrefix(contentType, "audio/") {
		contentType = http.DetectContentType(audio)
	}
	if !strings.HasPrefix(contentType, "audio/") {
		contentType = "audio/mpeg"
	}
	return SynthesizeResult{
		JobID:       voiceJobID(request.JobID),
		Filename:    voiceFilename(request.JobID, contentType),
		ContentType: contentType,
		Size:        int64(len(audio)),
		Body:        bytes.NewReader(audio),
	}, nil
}

func (e *OpenAICompatibleExecutor) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if e.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, e.timeout)
}

type openAICompatibleSpeechRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

type openAICompatibleTranscriptionResponse struct {
	Text string `json:"text"`
}

func requireOpenAICompatibleVoiceProvider(provider Provider) error {
	switch provider {
	case ProviderDefault, ProviderModel:
		return nil
	default:
		return errors.New("openai-compatible voice provider only supports default or model requests")
	}
}

func normalizeOpenAICompatibleBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", errors.New("openai-compatible voice provider base url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("openai-compatible voice provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("openai-compatible voice provider base url must use http or https")
	}
	return value, nil
}

func audioPartHeader(request TranscribeRequest) textproto.MIMEHeader {
	filename := strings.TrimSpace(filepath.Base(strings.ReplaceAll(request.AudioFilename, "\\", "/")))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "audio.webm"
	}
	filename = safeMultipartFilename(filename)
	contentType := mediaTypeOnly(request.AudioContentType)
	if contentType == "" || !strings.HasPrefix(contentType, "audio/") {
		contentType = "application/octet-stream"
	}
	return textproto.MIMEHeader{
		"Content-Disposition": []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)},
		"Content-Type":        []string{contentType},
	}
}

func providerTranscriptionLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en", "zh", "ja":
		return strings.ToLower(strings.TrimSpace(language))
	default:
		return ""
	}
}

func mediaTypeOnly(value string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(mediaType)
}

func voiceJobID(jobID string) string {
	jobID = strings.TrimSpace(jobID)
	if jobID != "" {
		return jobID
	}
	return "voice-synthesize"
}

func voiceFilename(jobID string, contentType string) string {
	return voiceJobID(jobID) + voiceExtension(contentType)
}

func voiceExtension(contentType string) string {
	switch contentType {
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	default:
		return ".bin"
	}
}

func safeMultipartFilename(value string) string {
	value = strings.NewReplacer("\r", "_", "\n", "_", `"`, "_").Replace(value)
	if strings.TrimSpace(value) == "" {
		return "audio.webm"
	}
	return value
}
