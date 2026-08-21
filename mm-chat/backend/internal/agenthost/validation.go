package agenthost

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxTokenBytes             = 4096
	minTokenBytes             = 32
	maxRunnerIDBytes          = 64
	maxVersionBytes           = 128
	maxWorkspacePathBytes     = 4096
	maxControlRequestBytes    = int64(16 << 10)
	maxControlResponseBytes   = int64(64 << 10)
	maxExecutionRequestBytes  = int64(128 << 10)
	maxExecutionResponseBytes = int64(72 << 20)
)

var runnerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)

func validateRunnerID(value string) error {
	if len(value) < 3 || len(value) > maxRunnerIDBytes || !runnerIDPattern.MatchString(value) {
		return errors.New("agent Host runner id is invalid")
	}
	return nil
}

func validateToken(value string) error {
	if len(value) < minTokenBytes || len(value) > maxTokenBytes ||
		strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("agent Host token is invalid")
	}
	return nil
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxVersionBytes || containsControl(value) {
		return "development"
	}
	return value
}

func validWorkspacePathInput(value string) bool {
	return value != "" && len(value) <= maxWorkspacePathBytes && utf8.ValidString(value) &&
		!containsControl(value)
}

func containsControl(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}
