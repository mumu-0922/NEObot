package resourceorchestrator

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	supportedResourceIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
	supportedAIHeroSkillPath    = regexp.MustCompile(`^/skills-([a-z0-9][a-z0-9-]{0,127})/?$`)
)

// supportedResourceLinkIdentifier accepts only explicitly admitted public
// discovery surfaces backed by Neo Chat's existing authenticated Marketplace
// adapters. It never fetches or installs from the pasted URL; the result is
// only an identifier used by the bounded search and exact-revision install
// flow.
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
