package voicejobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleExecutorSynthesizesSpeech(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://provider.test/v1/audio/speech" {
			t.Fatalf("url = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload openAICompatibleSpeechRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "FunAudioLLM/CosyVoice2-0.5B" || payload.Input != "speak" || payload.Voice != "FunAudioLLM/CosyVoice2-0.5B:claire" {
			t.Fatalf("payload = %#v", payload)
		}
		return bytesResponse(http.StatusOK, "audio/mpeg", []byte("audio-bytes")), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	result, err := executor.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderModel,
		JobID:    "job-1",
		ModelID:  "FunAudioLLM/CosyVoice2-0.5B",
		Text:     "speak",
		VoiceID:  "FunAudioLLM/CosyVoice2-0.5B:claire",
	})

	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if result.JobID != "job-1" || result.Filename != "job-1.mp3" || result.ContentType != "audio/mpeg" || result.Size != int64(len("audio-bytes")) {
		t.Fatalf("result = %#v", result)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read result body: %v", err)
	}
	if string(body) != "audio-bytes" {
		t.Fatalf("body = %q", string(body))
	}
}

func TestOpenAICompatibleExecutorTranscribesAudio(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://provider.test/v1/audio/transcriptions" {
			t.Fatalf("url = %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		fields := map[string]string{}
		var audioBody string
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}
			content, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			if part.FormName() == "file" {
				audioBody = string(content)
				if part.FileName() != "sample.webm" {
					t.Fatalf("filename = %q", part.FileName())
				}
				continue
			}
			fields[part.FormName()] = string(content)
		}
		if audioBody != "audio-bytes" || fields["model"] != "whisper-1" || fields["language"] != "en" {
			t.Fatalf("audio=%q fields=%#v", audioBody, fields)
		}
		return jsonResponse(http.StatusOK, openAICompatibleTranscriptionResponse{Text: "transcribed"}), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	response, err := executor.Transcribe(context.Background(), TranscribeRequest{
		Provider:         ProviderModel,
		ModelID:          "whisper-1",
		Language:         "en",
		AudioFilename:    "sample.webm",
		AudioContentType: "audio/webm",
		Audio:            strings.NewReader("audio-bytes"),
	})

	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if response.Text != "transcribed" {
		t.Fatalf("Text = %q", response.Text)
	}
}

func TestOpenAICompatibleExecutorUsesDefaults(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/audio/speech":
			var payload openAICompatibleSpeechRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode speech request: %v", err)
			}
			if payload.Model != "tts-default" || payload.Voice != "nova" {
				t.Fatalf("speech payload = %#v", payload)
			}
			return bytesResponse(http.StatusOK, "audio/wav", []byte("RIFFxxxxWAVEfmt ")), nil
		case "/v1/audio/transcriptions":
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("multipart reader: %v", err)
			}
			fields := map[string]string{}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("next part: %v", err)
				}
				if part.FormName() != "file" {
					content, _ := io.ReadAll(part)
					fields[part.FormName()] = string(content)
				}
			}
			if fields["model"] != "stt-default" {
				t.Fatalf("transcription fields = %#v", fields)
			}
			return jsonResponse(http.StatusOK, openAICompatibleTranscriptionResponse{Text: "ok"}), nil
		default:
			t.Fatalf("unexpected path = %s", r.URL.Path)
			return nil, nil
		}
	})}
	executor, err := NewOpenAICompatibleExecutor(OpenAICompatibleExecutorConfig{
		BaseURL:                   "https://provider.test/v1",
		APIKey:                    "test-key",
		Timeout:                   time.Second,
		HTTPClient:                client,
		DefaultTranscriptionModel: "stt-default",
		DefaultSpeechModel:        "tts-default",
		DefaultSpeechVoice:        "nova",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecutor() error = %v", err)
	}

	if _, err := executor.Synthesize(context.Background(), SynthesizeRequest{Provider: ProviderDefault, Text: "hello"}); err != nil {
		t.Fatalf("Synthesize() with defaults error = %v", err)
	}
	if _, err := executor.Transcribe(context.Background(), TranscribeRequest{Provider: ProviderDefault, Audio: strings.NewReader("audio")}); err != nil {
		t.Fatalf("Transcribe() with defaults error = %v", err)
	}
}

func TestOpenAICompatibleExecutorRejectsProviderFailuresWithoutLeakingBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return bytesResponse(http.StatusUnauthorized, "text/plain", []byte("secret provider body")), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderModel,
		ModelID:  "tts-1",
		Text:     "private synthesis text",
	})
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("Synthesize() error = %v, want status 401", err)
	}
	if strings.Contains(err.Error(), "secret provider body") || strings.Contains(err.Error(), "private synthesis text") {
		t.Fatalf("Synthesize() leaked sensitive text: %v", err)
	}

	_, err = executor.Transcribe(context.Background(), TranscribeRequest{
		Provider: ProviderModel,
		ModelID:  "whisper-1",
		Audio:    strings.NewReader("audio"),
	})
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("Transcribe() error = %v, want status 401", err)
	}
	if strings.Contains(err.Error(), "secret provider body") {
		t.Fatalf("Transcribe() leaked provider body: %v", err)
	}
}

func TestOpenAICompatibleExecutorRejectsSuccessfulNonAudioResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return bytesResponse(http.StatusOK, "application/json", []byte(`{"error":"secret provider payload"}`)), nil
	})}
	executor := newTestOpenAICompatibleExecutor(t, client)

	_, err := executor.Synthesize(context.Background(), SynthesizeRequest{
		Provider: ProviderModel,
		ModelID:  "tts-1",
		Text:     "private synthesis text",
	})
	if !errors.Is(err, ErrVoiceProviderFailed) {
		t.Fatalf("Synthesize() error = %v, want provider failure", err)
	}
	if strings.Contains(err.Error(), "secret provider payload") || strings.Contains(err.Error(), "private synthesis text") {
		t.Fatalf("Synthesize() leaked sensitive content: %v", err)
	}
}

func TestOpenAICompatibleExecutorRejectsInvalidInputs(t *testing.T) {
	executor := newTestOpenAICompatibleExecutor(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("provider should not be called")
		return nil, nil
	})})

	if _, err := executor.Synthesize(context.Background(), SynthesizeRequest{Provider: ProviderElevenLabs, ModelID: "tts-1", Text: "hello"}); err == nil {
		t.Fatal("Synthesize() provider error = nil")
	}
	if _, err := executor.Synthesize(context.Background(), SynthesizeRequest{Provider: ProviderModel, Text: "hello"}); err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("Synthesize() missing model error = %v", err)
	}
	if _, err := executor.Transcribe(context.Background(), TranscribeRequest{Provider: ProviderModel, ModelID: "whisper-1"}); err == nil || !strings.Contains(err.Error(), "audio is required") {
		t.Fatalf("Transcribe() missing audio error = %v", err)
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

func multipartFieldValue(t *testing.T, reader *multipart.Reader, field string) string {
	t.Helper()
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return ""
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		if part.FormName() != field {
			continue
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		return string(content)
	}
}
