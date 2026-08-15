package agentchildcanary

import (
	"strings"
	"time"
)

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func fingerprint(char byte) string          { return "sha256:" + strings.Repeat(string(char), 64) }
func repeat(value string, count int) string { return strings.Repeat(value, count) }
