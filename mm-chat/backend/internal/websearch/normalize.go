package websearch

import (
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

func normalizeResult(sources []Source, images []Image, limit int) Result {
	result := Result{
		Sources: make([]Source, 0, min(limit, len(sources))),
		Images:  make([]Image, 0, min(limit, len(images))),
	}
	seenSources := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if len(result.Sources) >= limit {
			break
		}
		source.URL = normalizeResultURL(source.URL)
		source.Content = truncateUTF8(strings.TrimSpace(source.Content), MaxSourceContentBytes)
		if source.URL == "" || source.Content == "" {
			continue
		}
		if _, exists := seenSources[source.URL]; exists {
			continue
		}
		seenSources[source.URL] = struct{}{}
		source.Title = truncateUTF8(strings.TrimSpace(source.Title), MaxSourceTitleBytes)
		if source.Title == "" {
			source.Title = source.URL
		}
		result.Sources = append(result.Sources, source)
	}

	seenImages := make(map[string]struct{}, len(images))
	for _, image := range images {
		if len(result.Images) >= limit {
			break
		}
		image.URL = normalizeResultURL(image.URL)
		if image.URL == "" {
			continue
		}
		if _, exists := seenImages[image.URL]; exists {
			continue
		}
		seenImages[image.URL] = struct{}{}
		image.Description = truncateUTF8(
			strings.TrimSpace(image.Description), MaxImageDescription,
		)
		result.Images = append(result.Images, image)
	}
	return result
}

func normalizeResultURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxSourceURLBytes {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	if isLocalHostname(parsed.Hostname()) {
		return ""
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !isPublicIP(ip) {
		return ""
	}
	parsed.Fragment = ""
	return parsed.String()
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}
