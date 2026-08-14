package agentrunner

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"
)

// NewRequest constructs one strict outbound Runner RPC envelope with fresh
// cryptographic replay identifiers. Callers cannot provide or reuse a nonce.
func NewRequest(method string, body any, now time.Time) (Request, error) {
	if now.IsZero() {
		return Request{}, ErrInvalidInput
	}
	requestBytes := make([]byte, 16)
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(requestBytes); err != nil {
		return Request{}, ErrRuntimeUnavailable
	}
	if _, err := rand.Read(nonceBytes); err != nil {
		return Request{}, ErrRuntimeUnavailable
	}
	envelope, err := json.Marshal(struct {
		SchemaVersion string    `json:"schemaVersion"`
		Method        string    `json:"method"`
		RequestID     string    `json:"requestId"`
		SentAt        time.Time `json:"sentAt"`
		Nonce         string    `json:"nonce"`
		Body          any       `json:"body"`
	}{
		SchemaVersion: ProtocolVersion,
		Method:        method,
		RequestID:     "rpc_" + hex.EncodeToString(requestBytes),
		SentAt:        now.UTC(),
		Nonce:         base64.RawURLEncoding.EncodeToString(nonceBytes),
		Body:          body,
	})
	if err != nil {
		return Request{}, ErrInvalidInput
	}
	return DecodeRequest(envelope, now.UTC(), time.Second)
}
