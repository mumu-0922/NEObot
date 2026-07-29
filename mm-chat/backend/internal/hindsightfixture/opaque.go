package hindsightfixture

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func DeriveBankID(
	apiKey string,
	manifestHash string,
	mode Mode,
	fixtureAlias string,
	userAlias string,
) (string, error) {
	if len(apiKey) < 32 || !validSHA256(manifestHash) ||
		(mode != ModeEndToEnd && mode != ModeRetrievalOnly) ||
		!validIdentifier(fixtureAlias) || !validIdentifier(userAlias) {
		return "", errors.New("opaque bank derivation input is invalid")
	}
	return "neo-" + deriveOpaque(
		apiKey,
		"bank\x00"+manifestHash+"\x00"+string(mode)+"\x00"+fixtureAlias+"\x00"+userAlias,
	), nil
}

func deriveDocumentID(apiKey, bankID, memoryID string) string {
	return "doc-" + deriveOpaque(apiKey, "document\x00"+bankID+"\x00"+memoryID)
}

func deriveOpaque(secret, input string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write([]byte(input))
	return hex.EncodeToString(digest.Sum(nil))[:40]
}
