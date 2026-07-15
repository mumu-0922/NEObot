package voicejobs

type Provider string

const (
	ProviderDefault    Provider = "default"
	ProviderElevenLabs Provider = "elevenlabs"
	ProviderMimo       Provider = "mimo"
	ProviderModel      Provider = "model"
)

type TranscribeRequest struct {
	Provider Provider
	ModelID  string
	Language string
}

type TranscribeResponse struct {
	Text string `json:"text"`
}

type SynthesizeRequest struct {
	Text     string   `json:"text"`
	Provider Provider `json:"provider"`
	VoiceID  string   `json:"voiceId,omitempty"`
	ModelID  string   `json:"modelId,omitempty"`
}

type SynthesizeResponse struct {
	FileID      string `json:"fileId,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}
