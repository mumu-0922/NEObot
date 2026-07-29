package memoryauthor

import (
	"fmt"
	"strings"
	"unicode"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func AuditRegression(
	fixtures RegressionFixtureManifest,
	corpus memoryeval.RegressionCorpus,
) (memoryeval.RegressionAudit, error) {
	promotion := false
	audit := memoryeval.RegressionAudit{
		SchemaVersion:         memoryeval.RegressionAuditSchemaVersion,
		CorpusID:              corpus.ID,
		CorpusClass:           memoryeval.RegressionCorpusClass,
		AdmissionMode:         memoryeval.RegressionAdmissionMode,
		PromotionEligible:     &promotion,
		CorpusContentSHA256:   corpus.CorpusContentSHA256,
		FixtureManifestSHA256: corpus.FixtureManifestSHA256,
		Auditor:               RegressionAuditor,
		AuditedAt:             RegressionAuditedAt,
		Verdict:               "passed",
		CaseCount:             len(corpus.Cases),
	}
	fixtureByAlias := make(map[string]Fixture, len(fixtures.Fixtures))
	for _, fixture := range fixtures.Fixtures {
		fixtureByAlias[fixture.Alias] = fixture
	}
	normalized := make(map[string]struct{}, len(corpus.Cases))
	skeletons := make(map[string]struct{}, len(corpus.Cases))
	for _, item := range corpus.Cases {
		countRegressionDistribution(&audit, item)
		normalizedQuery := normalizeQuery(item.Query)
		if _, duplicate := normalized[normalizedQuery]; duplicate {
			audit.Semantic.NormalizedDuplicateCount++
		}
		normalized[normalizedQuery] = struct{}{}
		skeletons[regressionQuerySkeleton(item.Query)] = struct{}{}

		fixture, ok := fixtureByAlias[item.FixtureAlias]
		if !ok || regressionFixtureCaseBindingError(fixture, item) != nil {
			audit.Semantic.FixtureBindingFailureCount++
			continue
		}
		allContent := regressionFixtureText(fixture)
		if sharesNumericToken(item.Query, allContent) {
			audit.Semantic.OrdinalShortcutCount++
		}
		if containsRegressionIdentifier(item, fixture, item.Query+" "+allContent) {
			audit.Semantic.IdentifierShortcutCount++
		}
		if !regressionLanguageMatches(item.Language, item.Query, allContent) {
			audit.Semantic.LanguageMismatchCount++
		}
		if !regressionScopeTextMatches(item, allContent) {
			audit.Semantic.ScopeTextMismatchCount++
		}
		semanticFailures, preferenceFailures, fallbackFailures, multiHopFailures :=
			regressionSemanticFailures(item, fixture)
		audit.Semantic.SliceSemanticFailureCount += semanticFailures
		audit.Semantic.PreferenceSemanticFailureCount += preferenceFailures
		audit.Semantic.FallbackSemanticFailureCount += fallbackFailures
		audit.Semantic.MultiHopSemanticFailureCount += multiHopFailures
	}
	audit.Semantic.QuerySkeletonCount = len(skeletons)
	for _, name := range memoryeval.CriticalSlices() {
		count := memoryeval.RegressionSliceCount{Name: name}
		for _, item := range corpus.Cases {
			if !contains(item.Slices, name) {
				continue
			}
			count.Total++
			switch item.Split {
			case "development":
				count.Development++
			case "validation":
				count.Validation++
			case "holdout":
				count.Holdout++
			}
		}
		audit.SliceCounts = append(audit.SliceCounts, count)
	}
	if !regressionAuditPasses(audit) {
		audit.Verdict = "failed"
	}
	digest, err := memoryeval.RegressionAuditContentSHA256(audit)
	if err != nil {
		return memoryeval.RegressionAudit{}, err
	}
	audit.ContentSHA256 = digest
	return audit, nil
}

func countRegressionDistribution(audit *memoryeval.RegressionAudit, item memoryeval.GoldenCase) {
	switch item.Split {
	case "development":
		audit.SplitCounts.Development++
	case "validation":
		audit.SplitCounts.Validation++
	case "holdout":
		audit.SplitCounts.Holdout++
	}
	switch item.Language {
	case "zh":
		audit.LanguageCounts.Chinese++
	case "mixed":
		audit.LanguageCounts.Mixed++
	case "en":
		audit.LanguageCounts.English++
	}
}

func regressionAuditPasses(audit memoryeval.RegressionAudit) bool {
	if audit.CaseCount != 500 ||
		audit.SplitCounts != (memoryeval.RegressionSplitCounts{Development: 300, Validation: 100, Holdout: 100}) ||
		audit.LanguageCounts != (memoryeval.RegressionLanguageCounts{Chinese: 350, Mixed: 100, English: 50}) ||
		audit.Semantic.QuerySkeletonCount < 100 ||
		audit.Semantic.NormalizedDuplicateCount != 0 ||
		audit.Semantic.OrdinalShortcutCount != 0 ||
		audit.Semantic.IdentifierShortcutCount != 0 ||
		audit.Semantic.FixtureBindingFailureCount != 0 ||
		audit.Semantic.SliceSemanticFailureCount != 0 ||
		audit.Semantic.LanguageMismatchCount != 0 ||
		audit.Semantic.ScopeTextMismatchCount != 0 ||
		audit.Semantic.PreferenceSemanticFailureCount != 0 ||
		audit.Semantic.FallbackSemanticFailureCount != 0 ||
		audit.Semantic.MultiHopSemanticFailureCount != 0 {
		return false
	}
	for _, count := range audit.SliceCounts {
		if count.Total < 50 || count.Development < 30 || count.Validation < 10 || count.Holdout < 10 {
			return false
		}
	}
	return len(audit.SliceCounts) == len(memoryeval.CriticalSlices())
}

func regressionFixtureCaseBindingError(fixture Fixture, item memoryeval.GoldenCase) error {
	if fixture.Alias != item.FixtureAlias || fixture.UserAlias != item.Scope.UserAlias {
		return fmt.Errorf("fixture authority does not match")
	}
	owned := make(map[string]FixtureMemory, len(fixture.Memories))
	for _, memory := range fixture.Memories {
		owned[memory.ID] = memory
	}
	for _, id := range item.ExpectedRelevantMemoryIDs {
		memory, ok := owned[id]
		if !ok || memory.State != StateActive || memory.UserAlias != item.Scope.UserAlias ||
			!scopeContains(item.Scope, memory.Scope) {
			return fmt.Errorf("expected Memory authority does not match")
		}
	}
	for _, exclusion := range item.Exclusions {
		memory, ok := owned[exclusion.MemoryID]
		if !ok || !exclusionMatches(exclusion.Reason, item.Scope, memory) {
			return fmt.Errorf("exclusion evidence does not match")
		}
	}
	return nil
}

func regressionSemanticFailures(item memoryeval.GoldenCase, fixture Fixture) (int, int, int, int) {
	failures, preferenceFailures, fallbackFailures, multiHopFailures := 0, 0, 0, 0
	relevantText := regressionRelevantText(item, fixture)
	query := strings.ToLower(item.Query)
	content := strings.ToLower(relevantText)
	hasSlice := func(name string) bool { return contains(item.Slices, name) }
	if hasSlice("stable_fact") && (item.ExpectedNoMemory || len(item.ExpectedRelevantMemoryIDs) == 0) {
		failures++
	}
	if hasSlice("preference_instruction") &&
		(!containsAny(query, "偏好", "指令", "preference", "instruction") ||
			!containsAny(content, "偏好", "指令", "preference", "instruction")) {
		preferenceFailures++
		failures++
	}
	if hasSlice("project_decision") &&
		(item.Scope.ProjectAlias == "" || !containsAny(query, "project", "项目") ||
			!containsAny(content, "project", "项目", "settled")) {
		failures++
	}
	if hasSlice("chinese_paraphrase") &&
		(!containsHan(item.Query) || relevantText != "" && normalizeQuery(item.Query) == normalizeQuery(relevantText)) {
		failures++
	}
	if hasSlice("mixed_language_entity") &&
		(!containsHan(item.Query) || !containsASCIIWord(item.Query) ||
			!containsHan(regressionFixtureText(fixture)) || !containsASCIIWord(regressionFixtureText(fixture))) {
		failures++
	}
	if hasSlice("temporal_correction") &&
		(len(item.ExpectedCurrentMemoryIDs) == 0 || !hasExclusionReason(item, "superseded") ||
			!containsAny(query, "更正", "当前", "latest", "correction") ||
			!containsAny(content, "当前", "current", "corrected", "更正")) {
		failures++
	}
	if hasSlice("unrelated_negative") && (!item.ExpectedNoMemory || !hasExclusionReason(item, "irrelevant")) {
		failures++
	}
	for slice, reason := range map[string]string{
		"untrusted_source": "untrusted_source",
		"secret_rejection": "secret",
		"deletion":         "deleted",
	} {
		if hasSlice(slice) && (!item.ExpectedNoMemory || !hasExclusionReason(item, reason)) {
			failures++
		}
	}
	if hasSlice("scope_isolation") &&
		(!item.ExpectedNoMemory || !hasExclusionReason(item, "cross_user")) {
		failures++
	}
	if hasSlice("failure_fallback") &&
		(item.ExpectedNoMemory ||
			!containsAny(query, "超时", "报错", "降级", "fallback", "timeout", "errors", "degrades") ||
			!containsAny(content, "超时", "报错", "降级", "fallback", "timeout", "errors", "degrades")) {
		fallbackFailures++
		failures++
	}
	if hasSlice("multi_hop") &&
		(len(item.ExpectedRelevantMemoryIDs) < 2 ||
			!containsAny(query, "结合", "组合", "combine", "constraint and action") ||
			!containsAny(content, "约束", "constraint") ||
			!containsAny(content, "执行动作", "action")) {
		multiHopFailures++
		failures++
	}
	return failures, preferenceFailures, fallbackFailures, multiHopFailures
}

func regressionRelevantText(item memoryeval.GoldenCase, fixture Fixture) string {
	relevant := make(map[string]struct{}, len(item.ExpectedRelevantMemoryIDs))
	for _, id := range item.ExpectedRelevantMemoryIDs {
		relevant[id] = struct{}{}
	}
	parts := make([]string, 0, len(relevant))
	for _, memory := range fixture.Memories {
		if _, ok := relevant[memory.ID]; ok {
			parts = append(parts, memory.CanonicalContent)
		}
	}
	return strings.Join(parts, " ")
}

func regressionFixtureText(fixture Fixture) string {
	parts := make([]string, 0, len(fixture.Memories)*2)
	for _, memory := range fixture.Memories {
		parts = append(parts, memory.CanonicalContent, memory.RawEventContent)
	}
	return strings.Join(parts, " ")
}

func regressionQuerySkeleton(value string) string {
	value = strings.ToLower(value)
	for _, term := range regressionEntities {
		value = strings.ReplaceAll(value, strings.ToLower(term), " entity ")
	}
	for _, terms := range [][]string{regressionSubjectsZH, regressionSubjectsEN} {
		for _, term := range terms {
			value = strings.ReplaceAll(value, strings.ToLower(term), " subject ")
		}
	}
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsLetter(character) {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func sharesNumericToken(left, right string) bool {
	leftTokens := numericTokens(left)
	for token := range numericTokens(right) {
		if len(token) >= 2 {
			if _, ok := leftTokens[token]; ok {
				return true
			}
		}
	}
	return false
}

func numericTokens(value string) map[string]struct{} {
	result := make(map[string]struct{})
	var builder strings.Builder
	flush := func() {
		if builder.Len() > 0 {
			result[builder.String()] = struct{}{}
			builder.Reset()
		}
	}
	for _, character := range value {
		if unicode.IsDigit(character) {
			builder.WriteRune(character)
		} else {
			flush()
		}
	}
	flush()
	return result
}

func containsRegressionIdentifier(item memoryeval.GoldenCase, fixture Fixture, text string) bool {
	text = strings.ToLower(text)
	identifiers := []string{item.ID, item.FixtureAlias, item.Scope.UserAlias, item.Scope.ProjectAlias, item.Scope.ConversationAlias}
	for _, memory := range fixture.Memories {
		identifiers = append(identifiers, memory.ID, memory.UserAlias)
	}
	for _, identifier := range identifiers {
		if identifier != "" && strings.Contains(text, strings.ToLower(identifier)) {
			return true
		}
	}
	return false
}

func regressionLanguageMatches(language, query, content string) bool {
	switch language {
	case "en":
		return !containsHan(query) && !containsHan(content) && containsASCIIWord(query) && containsASCIIWord(content)
	case "zh":
		return containsHan(query) && containsHan(content)
	case "mixed":
		return containsHan(query) && containsASCIIWord(query) && containsHan(content) && containsASCIIWord(content)
	default:
		return false
	}
}

func regressionScopeTextMatches(item memoryeval.GoldenCase, content string) bool {
	text := strings.ToLower(item.Query + " " + content)
	if item.Scope.ConversationAlias != "" {
		return containsAny(text, "当前对话", "current conversation") && containsAny(text, "project", "项目")
	}
	if item.Scope.ProjectAlias != "" {
		return containsAny(text, "project", "项目") && !containsAny(text, "当前对话", "current conversation")
	}
	return containsAny(text, "全局", "account-wide") && !containsAny(text, "project", "项目", "当前对话", "current conversation")
}

func containsAny(value string, options ...string) bool {
	for _, option := range options {
		if strings.Contains(value, strings.ToLower(option)) {
			return true
		}
	}
	return false
}

func containsHan(value string) bool {
	for _, character := range value {
		if unicode.Is(unicode.Han, character) {
			return true
		}
	}
	return false
}

func containsASCIIWord(value string) bool {
	for _, character := range value {
		if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' {
			return true
		}
	}
	return false
}

func hasExclusionReason(item memoryeval.GoldenCase, reason string) bool {
	for _, exclusion := range item.Exclusions {
		if exclusion.Reason == reason {
			return true
		}
	}
	return false
}
