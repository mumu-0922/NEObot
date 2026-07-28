package usermemory

import (
	"regexp"
	"strings"
)

const (
	SensitivityNormal    = "normal"
	SensitivitySensitive = "sensitive"
	SensitivitySecret    = "secret"
)

var (
	memorySecretAssignmentRE = regexp.MustCompile(
		`(?i)(?:api[ _-]?(?:key|token)|password|passwd|access[ _-]?token|` +
			`refresh[ _-]?token|auth[ _-]?token|session[ _-]?(?:id|token)|token|` +
			`client[ _-]?secret|secret|credentials?|otp|recovery[ _-]?code|` +
			`cookies?|private[ _-]?key|cvv|cvc|密码|口令|令牌|验证码|恢复码|` +
			`私钥|密钥|` +
			`凭证|安全码)\s*` +
			`(?:is|=|:|：|是|为)\s*["']?[^\s,，;；]{4,}`,
	)
	memorySecretTokenRE = regexp.MustCompile(
		`(?i)(?:\bsk-[a-z0-9_-]{8,}\b|\b(?:eyJ[a-zA-Z0-9_-]{8,}\.){2}` +
			`[a-zA-Z0-9_-]{8,}\b|authorization\s*:\s*bearer\s+\S+|` +
			`-----begin [a-z ]+private key-----)`,
	)
	memorySensitiveFactRE = regexp.MustCompile(
		`(?i)(?:\b(?:diagnos(?:is|ed)|disease|cancer|diabetes|salary|income|debt|` +
			`religion|religious|politic(?:s|al)|sexual orientation|lawsuit|arrested|` +
			`home address|exact location)\b|` +
			`诊断|疾病|癌症|糖尿病|工资|收入|负债|宗教|政治观点|性取向|诉讼|被捕|` +
			`家庭住址|精确位置|住在[^，。！？\s]{2,})`,
	)
	memorySentenceRE = regexp.MustCompile(`[^。！？.!?\n]+[。！？.!?]?`)
)

func ClassifyMemorySensitivity(value string) string {
	switch {
	case memorySecretAssignmentRE.MatchString(value), memorySecretTokenRE.MatchString(value):
		return SensitivitySecret
	case memorySensitiveFactRE.MatchString(value):
		return SensitivitySensitive
	default:
		return SensitivityNormal
	}
}

func RedactMemoryProviderText(value string, allowSensitive bool) string {
	segments := memorySentenceRE.FindAllString(value, -1)
	if len(segments) == 0 {
		segments = []string{value}
	}
	kept := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || ClassifyMemorySensitivity(segment) == SensitivitySecret {
			continue
		}
		if !allowSensitive && ClassifyMemorySensitivity(segment) == SensitivitySensitive {
			continue
		}
		kept = append(kept, segment)
	}
	return strings.Join(kept, " ")
}
