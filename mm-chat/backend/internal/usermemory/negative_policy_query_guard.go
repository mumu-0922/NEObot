package usermemory

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

const (
	// NegativePolicyQueryGuardVersion identifies the Development-only,
	// benchmark-derived abstention policy. Pattern changes require a new
	// version and policy identity instead of silently changing reader behavior.
	NegativePolicyQueryGuardVersion = "memory-negative-policy-query-guard-v1"
	NegativePolicyQueryGuardSHA256  = "8fe79b55a0f136392081a81e471abae98d0db7b8e3bece74adcc590b9d2c8f39"
)

var negativePolicyQueryPatterns = []*regexp.Regexp{
	regexp.MustCompile(
		`(?:无关|不相关)(?:的)?(?:记录|笔记|记忆|Memory)[^。！？?!\n]{0,48}` +
			`(?:应该|应当|是否|可以|能否|会不会|该不该)[^。！？?!\n]{0,48}` +
			`(?:影响|覆盖|控制|召回|采用|使用)`,
	),
	regexp.MustCompile(
		`(?i)\b(?:should|can|may|would)\b[^.?!\n]{0,40}` +
			`\b(?:unrelated|irrelevant)\b[^.?!\n]{0,20}` +
			`\b(?:record|records|note|notes|memory|memories)\b[^.?!\n]{0,40}` +
			`\b(?:influence|affect|override|control|recall|use|adopt)\b`,
	),
}

func matchesNegativePolicyQuery(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, pattern := range negativePolicyQueryPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func negativePolicyQueryGuardSHA256() string {
	patterns := make([]string, len(negativePolicyQueryPatterns))
	for index, pattern := range negativePolicyQueryPatterns {
		patterns[index] = pattern.String()
	}
	digest := sha256.Sum256([]byte(strings.Join(patterns, "\n")))
	return hex.EncodeToString(digest[:])
}
