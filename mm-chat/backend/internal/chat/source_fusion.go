package chat

import (
	"strings"
	"unicode"
)

const sourceFusionVersion = "source-fusion/v1"

type sourceQuestionClass string

const (
	sourceQuestionCurrentPublic sourceQuestionClass = "current_public"
	sourceQuestionKnowledge     sourceQuestionClass = "knowledge"
	sourceQuestionGeneral       sourceQuestionClass = "general"
)

type sourceAuthority string

const (
	sourceAuthorityMixed     sourceAuthority = "mixed"
	sourceAuthorityKnowledge sourceAuthority = "knowledge"
	sourceAuthorityWeb       sourceAuthority = "web"
	sourceAuthorityModel     sourceAuthority = "model"
)

type sourceSearchReason string

const (
	sourceSearchDisabled             sourceSearchReason = "disabled"
	sourceSearchCurrentPublic        sourceSearchReason = "current_public"
	sourceSearchKnowledgeSufficient  sourceSearchReason = "knowledge_sufficient"
	sourceSearchKnowledgeUnavailable sourceSearchReason = "knowledge_unavailable"
)

type sourceFusionPlan struct {
	QuestionClass   sourceQuestionClass
	Authority       sourceAuthority
	SearchEnabled   bool
	SearchRequested bool
	SearchReason    sourceSearchReason
}

func planSourceFusion(
	question string,
	searchEnabled bool,
	knowledge autoRAGDecision,
) sourceFusionPlan {
	knowledgeReady := knowledge.ReadyForAnswer()
	currentPublic := hasCurrentPublicIntent(question)
	plan := sourceFusionPlan{
		QuestionClass: sourceQuestionGeneral,
		Authority:     sourceAuthorityModel,
		SearchEnabled: searchEnabled,
		SearchReason:  sourceSearchDisabled,
	}

	if knowledgeReady {
		plan.QuestionClass = sourceQuestionKnowledge
		plan.Authority = sourceAuthorityKnowledge
	}
	if currentPublic {
		plan.QuestionClass = sourceQuestionCurrentPublic
	}
	if !searchEnabled {
		return plan
	}
	if knowledgeReady && !currentPublic {
		plan.SearchReason = sourceSearchKnowledgeSufficient
		return plan
	}

	plan.SearchRequested = true
	if currentPublic {
		plan.SearchReason = sourceSearchCurrentPublic
	} else {
		plan.SearchReason = sourceSearchKnowledgeUnavailable
	}
	if knowledgeReady {
		plan.Authority = sourceAuthorityMixed
	} else {
		plan.Authority = sourceAuthorityWeb
	}
	return plan
}

func hasCurrentPublicIntent(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	if normalized == "" {
		return false
	}
	words := " " + strings.Join(strings.FieldsFunc(normalized, func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsNumber(value)
	}), " ") + " "
	for _, marker := range []string{
		"today", "latest", "current", "currently", "now", "recent",
		"this week", "this month", "this year", "live", "news", "weather",
		"price", "exchange rate", "stock", "official site", "online", "web",
		"internet", "search for", "look up",
	} {
		if strings.Contains(words, " "+marker+" ") {
			return true
		}
	}
	for _, marker := range []string{
		"今天", "今日", "最新", "当前", "现在", "近期", "最近", "今年",
		"本周", "本月", "实时", "新闻", "天气", "价格", "汇率", "股价",
		"官网", "网上", "网络", "互联网", "搜索", "搜一下", "查一下",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func withSourceFusionMessageMetadata(
	base map[string]any,
	plan sourceFusionPlan,
	knowledge autoRAGDecision,
	diagnostics sourceFusionDiagnostics,
) map[string]any {
	metadata := ensureObject(base)
	knowledgeOutcome := strings.TrimSpace(knowledge.Outcome)
	if knowledgeOutcome == "" {
		knowledgeOutcome = "not_selected"
	}
	metadata["fusion"] = map[string]any{
		"version":                      sourceFusionVersion,
		"questionClass":                plan.QuestionClass,
		"authority":                    plan.Authority,
		"searchEnabled":                plan.SearchEnabled,
		"searchRequested":              plan.SearchRequested,
		"searchReason":                 plan.SearchReason,
		"knowledgeOutcome":             knowledgeOutcome,
		"webQueryDerivedFromKnowledge": diagnostics.WebQueryDerived,
		"stages": map[string]any{
			"knowledge": map[string]any{
				"outcome":    knowledgeOutcome,
				"durationMs": diagnostics.KnowledgeDurationMillis,
			},
			"router": map[string]any{
				"outcome":    plan.SearchReason,
				"durationMs": diagnostics.RouterDurationMillis,
			},
			"webResolve": map[string]any{
				"outcome":    diagnostics.WebResolveOutcome,
				"durationMs": diagnostics.WebResolveDurationMillis,
			},
			"webExecute": map[string]any{
				"outcome":    diagnostics.WebExecuteOutcome,
				"durationMs": diagnostics.WebExecuteDurationMillis,
			},
		},
	}
	if diagnostics.DegradationReason != "" {
		metadata["fusion"].(map[string]any)["degradationReason"] =
			diagnostics.DegradationReason
	}
	return metadata
}
