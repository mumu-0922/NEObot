package providersecrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	KeyringVersion  = 1
	EnvelopeVersion = 1
	Algorithm       = "A256GCM"

	keyBytes          = 32
	maxKeys           = 16
	maxContextBytes   = 256
	maxPlaintextBytes = 64 << 10
	maxKeyringBytes   = 64 << 10
)

var (
	ErrKeyringUnavailable = errors.New("provider secret keyring is unavailable")
	ErrInvalidKeyring     = errors.New("provider secret keyring is invalid")
	ErrInvalidContext     = errors.New("provider secret context is invalid")
	ErrContextMismatch    = errors.New("provider secret context does not match")
	ErrInvalidPlaintext   = errors.New("provider secret plaintext is invalid")
	ErrInvalidEnvelope    = errors.New("provider secret envelope is invalid")

	keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type KeyringConfig struct {
	V         int         `json:"v"`
	ActiveKID string      `json:"activeKid"`
	Keys      []KeyConfig `json:"keys"`
}

type KeyConfig struct {
	KID string `json:"kid"`
	Key string `json:"key"`
}

type Envelope struct {
	V          int    `json:"v"`
	KID        string `json:"kid"`
	Alg        string `json:"alg"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	Context    string `json:"context"`
}

type Vault struct {
	activeKID string
	keys      map[string][keyBytes]byte
}

type envelopeHeader struct {
	V       int    `json:"v"`
	KID     string `json:"kid"`
	Alg     string `json:"alg"`
	Context string `json:"context"`
}

func NewVault(config KeyringConfig) (*Vault, error) {
	activeKID := strings.TrimSpace(config.ActiveKID)
	if config.V != KeyringVersion || !validKeyID(activeKID) ||
		len(config.Keys) == 0 || len(config.Keys) > maxKeys {
		return nil, ErrInvalidKeyring
	}

	keys := make(map[string][keyBytes]byte, len(config.Keys))
	for _, item := range config.Keys {
		kid := strings.TrimSpace(item.KID)
		if !validKeyID(kid) {
			return nil, ErrInvalidKeyring
		}
		if _, exists := keys[kid]; exists {
			return nil, ErrInvalidKeyring
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(item.Key))
		if err != nil || len(decoded) != keyBytes {
			return nil, ErrInvalidKeyring
		}
		var key [keyBytes]byte
		copy(key[:], decoded)
		clear(decoded)
		keys[kid] = key
	}
	if _, ok := keys[activeKID]; !ok {
		return nil, ErrInvalidKeyring
	}

	return &Vault{activeKID: activeKID, keys: keys}, nil
}

func LoadVaultFile(path string) (*Vault, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrKeyringUnavailable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrKeyringUnavailable
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxKeyringBytes {
		return nil, ErrInvalidKeyring
	}
	encoded, err := io.ReadAll(io.LimitReader(file, maxKeyringBytes+1))
	if err != nil || len(encoded) > maxKeyringBytes {
		return nil, ErrInvalidKeyring
	}
	defer clear(encoded)

	var config KeyringConfig
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, ErrInvalidKeyring
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidKeyring
	}
	vault, err := NewVault(config)
	for index := range config.Keys {
		config.Keys[index].Key = ""
	}
	return vault, err
}

func (v *Vault) ActiveKID() string {
	if v == nil {
		return ""
	}
	return v.activeKID
}

func (v *Vault) Encrypt(plaintext []byte, context string) (Envelope, error) {
	if v == nil {
		return Envelope{}, ErrKeyringUnavailable
	}
	if !validContext(context) {
		return Envelope{}, ErrInvalidContext
	}
	if len(plaintext) == 0 || len(plaintext) > maxPlaintextBytes {
		return Envelope{}, ErrInvalidPlaintext
	}
	key, ok := v.keys[v.activeKID]
	if !ok {
		return Envelope{}, ErrInvalidKeyring
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return Envelope{}, ErrInvalidKeyring
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, ErrInvalidKeyring
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	envelope := Envelope{
		V:       EnvelopeVersion,
		KID:     v.activeKID,
		Alg:     Algorithm,
		Nonce:   base64.RawURLEncoding.EncodeToString(nonce),
		Context: context,
	}
	aad, err := envelopeAAD(envelope)
	if err != nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	envelope.Ciphertext = base64.RawURLEncoding.EncodeToString(ciphertext)
	return envelope, nil
}

func (v *Vault) Decrypt(envelope Envelope, expectedContext string) ([]byte, error) {
	if v == nil {
		return nil, ErrKeyringUnavailable
	}
	if !validContext(expectedContext) {
		return nil, ErrInvalidContext
	}
	if envelope.Context != expectedContext {
		return nil, ErrContextMismatch
	}
	if envelope.V != EnvelopeVersion || envelope.Alg != Algorithm ||
		!validKeyID(envelope.KID) || !validContext(envelope.Context) {
		return nil, ErrInvalidEnvelope
	}
	key, ok := v.keys[envelope.KID]
	if !ok {
		return nil, ErrInvalidEnvelope
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	if len(envelope.Nonce) != base64.RawURLEncoding.EncodedLen(gcm.NonceSize()) ||
		len(envelope.Ciphertext) < base64.RawURLEncoding.EncodedLen(gcm.Overhead()) ||
		len(envelope.Ciphertext) > base64.RawURLEncoding.EncodedLen(maxPlaintextBytes+gcm.Overhead()) {
		return nil, ErrInvalidEnvelope
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, ErrInvalidEnvelope
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < gcm.Overhead() ||
		len(ciphertext) > maxPlaintextBytes+gcm.Overhead() {
		return nil, ErrInvalidEnvelope
	}
	aad, err := envelopeAAD(envelope)
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil || len(plaintext) == 0 || len(plaintext) > maxPlaintextBytes {
		clear(plaintext)
		return nil, ErrInvalidEnvelope
	}
	return plaintext, nil
}

func (v *Vault) NeedsRotation(envelope Envelope) bool {
	return v != nil && envelope.KID != v.activeKID
}

func (v *Vault) Rotate(
	envelope Envelope,
	expectedContext string,
) (Envelope, bool, error) {
	plaintext, err := v.Decrypt(envelope, expectedContext)
	if err != nil {
		return Envelope{}, false, err
	}
	defer clear(plaintext)
	if !v.NeedsRotation(envelope) {
		return envelope, false, nil
	}
	rotated, err := v.Encrypt(plaintext, expectedContext)
	if err != nil {
		return Envelope{}, false, err
	}
	return rotated, true, nil
}

func envelopeAAD(envelope Envelope) ([]byte, error) {
	return json.Marshal(envelopeHeader{
		V:       envelope.V,
		KID:     envelope.KID,
		Alg:     envelope.Alg,
		Context: envelope.Context,
	})
}

func validKeyID(value string) bool {
	return value == strings.TrimSpace(value) && keyIDPattern.MatchString(value)
}

func validContext(value string) bool {
	return value == strings.TrimSpace(value) && value != "" &&
		len(value) <= maxContextBytes && !strings.ContainsRune(value, '\x00')
}
