package chat

import (
	"regexp"
	"strings"
)

var explicitMemoryReadPatterns = []*regexp.Regexp{
	regexp.MustCompile(
		`(?:读取|读一下|查看|查一下|查询|搜索|检索|调用|使用|用一下|调取|打开).{0,12}` +
			`(?:长期|保存的|已保存的|个人)?记忆`,
	),
	regexp.MustCompile(
		`(?:根据|结合|按照).{0,16}(?:记忆|你记得的|已保存的信息)`,
	),
	regexp.MustCompile(
		`你(?:还|都)?记得.{0,16}(?:我|关于我|我们(?:上次|之前))`,
	),
	regexp.MustCompile(
		`你(?:还|都)?知道.{0,12}(?:关于我|我的(?:信息|资料|偏好|喜好|背景|姓名|名字|学校)|我是谁)`,
	),
	regexp.MustCompile(
		`(?:我叫什么(?:名字)?|我的名字(?:叫)?什么|我的(?:偏好|喜好|学校|姓名|名字)(?:是|有)?什么|` +
			`我是哪(?:个|所)?学校的|我(?:在|就读于|来自)哪(?:个|所)?学校)`,
	),
	regexp.MustCompile(
		`(?i)\b(?:read|load|search|query|check|use|access|retrieve|recall)\b.{0,32}` +
			`\b(?:saved|long[- ]term|personal)?\s*(?:memory|memories)\b`,
	),
	regexp.MustCompile(
		`(?i)\b(?:do|can)\s+you\s+(?:still\s+)?remember\s+` +
			`(?:me|my|what\s+i|what\s+we|our\s+(?:last|previous))\b`,
	),
	regexp.MustCompile(
		`(?i)\b(?:what|how\s+much)\s+do\s+you\s+know\s+about\s+me\b|` +
			`\bdo\s+you\s+know\s+my\s+(?:name|information|info|preferences?|background|school)\b`,
	),
	regexp.MustCompile(
		`(?i)\b(?:who\s+am\s+i|what(?:'s|\s+is)\s+my\s+name|` +
			`which\s+school\s+(?:do\s+i\s+(?:attend|go\s+to)|am\s+i\s+from))\b`,
	),
}

// detectExplicitMemoryReadIntent recognizes only user-controlled requests to
// consult saved personal Memory. It is deliberately narrower than semantic
// relevance: ordinary turns remain model-routed with tool_choice=auto, while
// the fixed server-side selector still decides whether any Memory is released.
func detectExplicitMemoryReadIntent(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, pattern := range explicitMemoryReadPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}
