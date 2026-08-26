package resourceorchestrator

import (
	"net/url"
	"regexp"
	"strings"
)

const maxSupportedResourceLinkTextBytes = 16 << 10

var (
	supportedResourceIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
	supportedAIHeroSkillPath    = regexp.MustCompile(`^/skills-([a-z0-9][a-z0-9-]{0,127})/?$`)
)

// SupportedResourceLink is a parsed discovery reference. DirectInstall is set
// only for server-owned adapters that resolve immutable source without
// executing page content; other links remain Marketplace search aliases.
type SupportedResourceLink struct {
	Kind          string
	Identifier    string
	URL           string
	DirectInstall bool
}

// SingleSupportedResourceLink extracts exactly one HTTPS URL from ordinary
// user text and accepts it only when the complete URL matches a server-owned
// Skill or MCP discovery surface. Multiple URLs fail closed even when one is
// supported.
func SingleSupportedResourceLink(value string) (SupportedResourceLink, bool) {
	if len(value) == 0 || len(value) > maxSupportedResourceLinkTextBytes {
		return SupportedResourceLink{}, false
	}
	candidates := resourceHTTPSLinkCandidates(value)
	if len(candidates) != 1 {
		return SupportedResourceLink{}, false
	}
	raw := candidates[0]
	for _, kind := range []string{KindSkill, KindMCP} {
		identifier, ok := supportedResourceLinkIdentifier(kind, raw)
		if ok {
			return SupportedResourceLink{
				Kind: kind, Identifier: identifier, URL: raw,
				DirectInstall: kind == KindSkill && isAIHeroSkillLink(raw),
			}, true
		}
	}
	return SupportedResourceLink{}, false
}

func isAIHeroSkillLink(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host == "aihero.dev" || host == "www.aihero.dev"
}

func resourceHTTPSLinkCandidates(value string) []string {
	const prefix = "https://"
	candidates := make([]string, 0, 2)
	for offset := 0; offset < len(value); {
		relative := strings.Index(value[offset:], prefix)
		if relative < 0 {
			break
		}
		start := offset + relative
		end := start + len(prefix)
		for end < len(value) {
			current := value[end]
			if current >= 0x80 || current <= ' ' || current == '"' ||
				current == '\'' || current == '<' || current == '>' {
				break
			}
			end++
		}
		raw := strings.TrimRight(value[start:end], ".,;:!)]}")
		if raw != prefix {
			candidates = append(candidates, raw)
			if len(candidates) > 1 {
				return candidates
			}
		}
		offset = end
	}
	return candidates
}

// supportedResourceLinkIdentifier accepts only explicitly configured public
// discovery surfaces. Fetch/install authority is still decided separately by
// the server-owned adapter encoded in SupportedResourceLink.DirectInstall.
func supportedResourceLinkIdentifier(kind string, raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Port() != "" && parsed.Port() != "443") {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if kind == KindSkill && (host == "aihero.dev" || host == "www.aihero.dev") {
		matches := supportedAIHeroSkillPath.FindStringSubmatch(parsed.EscapedPath())
		if len(matches) != 2 || !supportedResourceIdentifier.MatchString(matches[1]) {
			return "", false
		}
		return matches[1], true
	}
	if host != "lobehub.com" && host != "www.lobehub.com" && host != "market.lobehub.com" {
		return "", false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) != 2 {
		return "", false
	}
	section := strings.ToLower(segments[0])
	if (kind == KindSkill && section != "skills") ||
		(kind == KindMCP && section != "mcp" && section != "plugins") {
		return "", false
	}
	identifier, err := url.PathUnescape(segments[1])
	if err != nil || !supportedResourceIdentifier.MatchString(identifier) {
		return "", false
	}
	return identifier, true
}
