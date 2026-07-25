package knowledge

import (
	"strings"
	"unicode"
)

const (
	minimumSourceNameKeyRunes = 6
	maximumSourceNameKeyRunes = 256
	maximumSourceNameBytes    = 512
)

// QueryExplicitlyNamesSource mirrors the metadata-only source-name routing
// boundary in migration 048. A match may influence ranking, but it is never
// source text or Citation authority.
func QueryExplicitlyNamesSource(query string, sourceName string) bool {
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" || len([]byte(sourceName)) > maximumSourceNameBytes ||
		strings.ContainsAny(sourceName, "\r\n\x00") {
		return false
	}

	baseName := sourceName
	if extension := strings.LastIndexByte(baseName, '.'); extension >= 0 {
		extensionRunes := []rune(baseName[extension+1:])
		if len(extensionRunes) >= 1 && len(extensionRunes) <= 16 {
			baseName = baseName[:extension]
		}
	}
	sourceKey := sourceNameKey(baseName)
	sourceKeyLength := len([]rune(sourceKey))
	if sourceKeyLength < minimumSourceNameKeyRunes ||
		sourceKeyLength > maximumSourceNameKeyRunes {
		return false
	}
	return strings.Contains(sourceNameKey(query), sourceKey)
}

func sourceNameKey(value string) string {
	var normalized strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			normalized.WriteRune(unicode.ToLower(character))
		}
	}
	return normalized.String()
}
