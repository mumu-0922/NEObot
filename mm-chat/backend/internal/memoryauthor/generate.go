package memoryauthor

import (
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

type generatedDraft struct {
	index    int
	split    string
	language string
	primary  string
	slices   map[string]struct{}
	witness  bool
}

type splitProfile struct {
	name            string
	total           int
	zh              int
	mixed           int
	en              int
	witnessZH       int
	witnessMixed    int
	witnessEN       int
	witnessSliceMin int
	poolSliceMin    int
}

var generationSplits = []splitProfile{
	{name: "development", total: 390, zh: 273, mixed: 78, en: 39, witnessZH: 210, witnessMixed: 60, witnessEN: 30, witnessSliceMin: 30, poolSliceMin: 39},
	{name: "validation", total: 130, zh: 91, mixed: 26, en: 13, witnessZH: 70, witnessMixed: 20, witnessEN: 10, witnessSliceMin: 10, poolSliceMin: 13},
	{name: "holdout", total: 130, zh: 91, mixed: 26, en: 13, witnessZH: 70, witnessMixed: 20, witnessEN: 10, witnessSliceMin: 10, poolSliceMin: 13},
}

func Generate() (GeneratedPool, error) {
	drafts, err := buildDraftPlan()
	if err != nil {
		return GeneratedPool{}, err
	}
	fixtures := make([]Fixture, 0, len(drafts))
	cases := make([]memoryeval.GoldenCase, 0, len(drafts))
	for _, draft := range drafts {
		fixture, item := generateCase(draft)
		fixtures = append(fixtures, fixture)
		cases = append(cases, item)
	}
	promotion := false
	fixtureManifest := FixtureManifest{
		SchemaVersion:     FixtureSchemaVersion,
		ID:                "memory-benchmark-v1-candidate-fixtures",
		Description:       "Deterministic synthetic-only Memory benchmark candidate fixtures.",
		PromotionEligible: &promotion,
		DataPolicy:        DataPolicy{SyntheticOnly: true},
		Generator:         expectedGenerator(),
		ContentSHA256:     strings.Repeat("0", 64),
		Fixtures:          fixtures,
	}
	fixtureContentHash, err := FixtureContentSHA256(fixtureManifest)
	if err != nil {
		return GeneratedPool{}, err
	}
	fixtureManifest.ContentSHA256 = fixtureContentHash
	golden := memoryeval.GoldenSet{
		SchemaVersion:     memoryeval.GoldenSchemaVersion,
		ID:                "memory-benchmark-v1-candidates",
		Description:       "Deterministic synthetic Memory benchmark candidates; not formal evidence.",
		PromotionEligible: &promotion,
		DataPolicy: memoryeval.DataPolicy{
			SyntheticOnly: true,
		},
		FixtureManifestSHA256: fixtureContentHash,
		Lifecycle:             memoryeval.GoldenLifecycle{State: "draft"},
		Criteria: memoryeval.Criteria{
			MinimumCandidateRecallAt20:       0.95,
			MinimumFinalRecallAt5:            0.90,
			MinimumCurrentFactAccuracy:       0.95,
			MaximumFalseInjectionRate:        0.02,
			MaximumP95LatencyMilliseconds:    900,
			MaximumP99LatencyMilliseconds:    1500,
			HardCutoffMilliseconds:           2000,
			MaximumAveragePromptMemoryTokens: 600,
			MaximumPromptMemoryTokens:        900,
			MaximumProviderCostRatio:         0.15,
		},
		Cases: cases,
	}
	fixtureJSON, err := marshalCanonical(fixtureManifest)
	if err != nil {
		return GeneratedPool{}, err
	}
	goldenJSON, err := marshalCanonical(golden)
	if err != nil {
		return GeneratedPool{}, err
	}
	diagnostic, err := Diagnose(fixtureManifest, golden)
	if err != nil {
		return GeneratedPool{}, err
	}
	manifest := CandidateManifest{
		SchemaVersion:            CandidateSchemaVersion,
		ID:                       "memory-benchmark-v1-candidate-pool",
		PromotionEligible:        &promotion,
		DataPolicy:               DataPolicy{SyntheticOnly: true},
		Generator:                expectedGenerator(),
		CaseCount:                diagnostic.CaseCount,
		SplitCounts:              diagnostic.SplitCounts,
		LanguageCounts:           diagnostic.LanguageCounts,
		SliceCounts:              diagnostic.SliceCounts,
		FixtureContentSHA256:     fixtureContentHash,
		FixtureRawSHA256:         sha256Hex(fixtureJSON),
		GoldenRawSHA256:          sha256Hex(goldenJSON),
		FeasibilityWitnessSHA256: witnessHash(diagnostic.WitnessCaseIDs),
	}
	manifestJSON, err := marshalCanonical(manifest)
	if err != nil {
		return GeneratedPool{}, err
	}
	pool := GeneratedPool{
		FixtureManifest: fixtureManifest,
		Golden:          golden,
		Manifest:        manifest,
		FixtureJSON:     fixtureJSON,
		GoldenJSON:      goldenJSON,
		ManifestJSON:    manifestJSON,
	}
	if err := ValidatePool(pool); err != nil {
		return GeneratedPool{}, fmt.Errorf("validate generated candidate pool: %w", err)
	}
	return pool, nil
}

func buildDraftPlan() ([]generatedDraft, error) {
	critical := memoryeval.CriticalSlices()
	drafts := make([]generatedDraft, 0, 650)
	globalIndex := 0
	for _, profile := range generationSplits {
		repeats := profile.total / len(critical)
		for repeat := 0; repeat < repeats; repeat++ {
			for _, slice := range critical {
				globalIndex++
				drafts = append(drafts, generatedDraft{
					index: globalIndex, split: profile.name, primary: slice,
					slices: map[string]struct{}{slice: {}},
				})
			}
		}
		assignLanguagesAndWitness(drafts[len(drafts)-profile.total:], profile)
	}
	for _, profile := range generationSplits {
		for _, target := range critical {
			if err := addSliceCoverage(drafts, profile, target, true, profile.witnessSliceMin); err != nil {
				return nil, err
			}
			if err := addSliceCoverage(drafts, profile, target, false, profile.poolSliceMin); err != nil {
				return nil, err
			}
		}
	}
	return drafts, nil
}

func assignLanguagesAndWitness(drafts []generatedDraft, profile splitProfile) {
	remaining := map[string]int{"zh": profile.zh, "mixed": profile.mixed, "en": profile.en}
	for index := range drafts {
		switch drafts[index].primary {
		case "mixed_language_entity":
			drafts[index].language = "mixed"
			remaining["mixed"]--
		case "chinese_paraphrase":
			drafts[index].language = "zh"
			remaining["zh"]--
		}
	}
	for _, language := range []string{"zh", "mixed", "en"} {
		for index := range drafts {
			if remaining[language] == 0 {
				break
			}
			if drafts[index].language != "" {
				continue
			}
			drafts[index].language = language
			remaining[language]--
		}
	}
	witnessRemaining := map[string]int{
		"zh": profile.witnessZH, "mixed": profile.witnessMixed, "en": profile.witnessEN,
	}
	for index := range drafts {
		language := drafts[index].language
		if witnessRemaining[language] > 0 {
			drafts[index].witness = true
			witnessRemaining[language]--
		}
	}
}

func addSliceCoverage(
	drafts []generatedDraft,
	profile splitProfile,
	target string,
	witnessOnly bool,
	minimum int,
) error {
	count := 0
	for _, draft := range drafts {
		if draft.split != profile.name || (witnessOnly && !draft.witness) {
			continue
		}
		if _, ok := draft.slices[target]; ok {
			count++
		}
	}
	for index := range drafts {
		if count >= minimum {
			return nil
		}
		draft := &drafts[index]
		if draft.split != profile.name || (witnessOnly && !draft.witness) {
			continue
		}
		if _, exists := draft.slices[target]; exists || !canAddSlice(*draft, target) {
			continue
		}
		draft.slices[target] = struct{}{}
		count++
	}
	return fmt.Errorf("cannot satisfy %s slice coverage for %s", target, profile.name)
}

func canAddSlice(draft generatedDraft, target string) bool {
	if target == "chinese_paraphrase" && draft.language == "en" {
		return false
	}
	if target == "mixed_language_entity" && draft.language != "mixed" {
		return false
	}
	positiveTarget := isPositiveSlice(target)
	negativeTarget := isNegativeSlice(target)
	for actual := range draft.slices {
		if positiveTarget && isNegativeSlice(actual) {
			return false
		}
		if negativeTarget && isPositiveSlice(actual) {
			return false
		}
	}
	return true
}

func isPositiveSlice(name string) bool {
	switch name {
	case "stable_fact", "preference_instruction", "project_decision",
		"temporal_correction", "multi_hop":
		return true
	default:
		return false
	}
}

func isNegativeSlice(name string) bool {
	switch name {
	case "unrelated_negative", "untrusted_source", "secret_rejection",
		"scope_isolation", "deletion":
		return true
	default:
		return false
	}
}

func generateCase(draft generatedDraft) (Fixture, memoryeval.GoldenCase) {
	caseID := fmt.Sprintf("case-%04d", draft.index)
	fixtureAlias := fmt.Sprintf("fixture-%04d", draft.index)
	userAlias := fmt.Sprintf("user-%04d", draft.index)
	projectAlias := ""
	conversationAlias := ""
	if hasSlice(draft, "project_decision") || draft.index%3 != 0 {
		projectAlias = fmt.Sprintf("project-%03d", (draft.index%37)+1)
	}
	if projectAlias != "" && draft.index%4 == 0 {
		conversationAlias = fmt.Sprintf("conversation-%03d", (draft.index%53)+1)
	}
	scope := memoryeval.Scope{
		UserAlias: userAlias, ProjectAlias: projectAlias, ConversationAlias: conversationAlias,
	}
	item := memoryeval.GoldenCase{
		ID: caseID, Query: queryFor(draft), Split: draft.split, Language: draft.language,
		Slices: orderedSlices(draft.slices), FixtureAlias: fixtureAlias, Scope: scope,
		Review: memoryeval.Review{State: "draft"},
	}
	fixture := Fixture{Alias: fixtureAlias, UserAlias: userAlias}
	baseTime := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC).
		Add(time.Duration(draft.index) * time.Hour)
	newMemory := func(suffix string, owner string, memoryScope memoryeval.Scope, state MemoryState, content string, offset int) FixtureMemory {
		return FixtureMemory{
			ID: fmt.Sprintf("mem-%04d-%s", draft.index, suffix), UserAlias: owner,
			Scope: memoryScope, CanonicalContent: content,
			RawEventContent: rawEventFor(draft.language, content),
			OccurredAt:      baseTime.Add(time.Duration(offset) * time.Minute).Format(time.RFC3339),
			State:           state,
		}
	}

	if hasNegativeSlice(draft) {
		item.ExpectedNoMemory = true
		if hasSlice(draft, "unrelated_negative") {
			memory := newMemory("irrelevant", userAlias, scope, StateIrrelevant,
				contentFor(draft, "irrelevant"), 1)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: "irrelevant"})
		}
		if hasSlice(draft, "untrusted_source") {
			memory := newMemory("untrusted", userAlias, scope, StateUntrustedRejected,
				contentFor(draft, "untrusted"), 2)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: "untrusted_source"})
		}
		if hasSlice(draft, "secret_rejection") {
			memory := newMemory("secret", userAlias, scope, StateSecretRejected,
				contentFor(draft, "secret"), 3)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: "secret"})
		}
		if hasSlice(draft, "scope_isolation") {
			otherUser := fmt.Sprintf("user-other-%04d", draft.index)
			otherScope := scope
			otherScope.UserAlias = otherUser
			memory := newMemory("cross-user", otherUser, otherScope, StateOutOfScope,
				contentFor(draft, "cross_user"), 4)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: "cross_user"})
		}
		if hasSlice(draft, "deletion") {
			memory := newMemory("deleted", userAlias, scope, StateDeleted,
				contentFor(draft, "deleted"), 5)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: "deleted"})
		}
	} else if hasSlice(draft, "temporal_correction") {
		old := newMemory("old", userAlias, scope, StateSuperseded, contentFor(draft, "old"), 1)
		current := newMemory("current", userAlias, scope, StateActive, contentFor(draft, "current"), 2)
		fixture.Memories = append(fixture.Memories, old, current)
		item.ExpectedRelevantMemoryIDs = []string{current.ID}
		item.ExpectedCurrentMemoryIDs = []string{current.ID}
		item.Exclusions = []memoryeval.Exclusion{{MemoryID: old.ID, Reason: "superseded"}}
	} else {
		first := newMemory("a", userAlias, scope, StateActive, contentFor(draft, "primary"), 1)
		fixture.Memories = append(fixture.Memories, first)
		item.ExpectedRelevantMemoryIDs = []string{first.ID}
		item.ExpectedCurrentMemoryIDs = []string{first.ID}
		if hasSlice(draft, "multi_hop") {
			second := newMemory("b", userAlias, scope, StateActive, contentFor(draft, "secondary"), 2)
			fixture.Memories = append(fixture.Memories, second)
			item.ExpectedRelevantMemoryIDs = append(item.ExpectedRelevantMemoryIDs, second.ID)
			item.ExpectedCurrentMemoryIDs = append(item.ExpectedCurrentMemoryIDs, second.ID)
		}
	}
	return fixture, item
}

func queryFor(draft generatedDraft) string {
	index := draft.index
	object := []string{"构建命令", "界面主题", "发布窗口", "测试区域", "检索策略", "文档格式", "提醒方式"}[index%7]
	englishObject := []string{"build command", "interface theme", "release window", "test region", "retrieval policy", "document format", "reminder rule"}[index%7]
	entity := []string{"Atlas", "Bamboo", "Cinder", "Delta", "Ember", "Flux", "Glyph", "Helix"}[index%8]
	negative := hasNegativeSlice(draft)
	switch draft.language {
	case "en":
		if negative {
			return fmt.Sprintf("For synthetic scenario %04d, should any rejected or out-of-scope note affect my %s choice for %s?", index, englishObject, entity)
		}
		return fmt.Sprintf("For synthetic scenario %04d, what did I decide about the %s for project %s?", index, englishObject, entity)
	case "mixed":
		if negative {
			return fmt.Sprintf("合成场景 %04d 中，被拒绝或 out-of-scope 的记录会影响我给 %s 选的 %s 吗？", index, entity, object)
		}
		return fmt.Sprintf("合成场景 %04d 中，我给 project %s 定下的 %s 是什么？", index, entity, object)
	default:
		if negative {
			return fmt.Sprintf("合成场景%04d中，被拒绝、已删除或越权的记录会影响我为项目%s选择的%s吗？", index, entity, object)
		}
		return fmt.Sprintf("合成场景%04d中，我之前为项目%s确定的%s是什么？", index, entity, object)
	}
}

func contentFor(draft generatedDraft, role string) string {
	index := draft.index
	entity := []string{"Atlas", "Bamboo", "Cinder", "Delta", "Ember", "Flux", "Glyph", "Helix"}[index%8]
	value := []string{"只在工作日执行", "采用蓝灰配色", "使用两阶段发布", "优先华东测试区", "启用精确匹配", "输出 Markdown", "提前十五分钟提醒"}[index%7]
	switch role {
	case "old":
		return fmt.Sprintf("合成项目 %s 的旧决定是：%s（旧版本）。", entity, value)
	case "current":
		return fmt.Sprintf("合成项目 %s 的当前决定已更正为：%s（当前版本 %04d）。", entity, value, index)
	case "secondary":
		return fmt.Sprintf("合成项目 %s 的配套约束是：完成检查清单 %04d 后再执行。", entity, index)
	case "irrelevant":
		return fmt.Sprintf("无关的合成记录 %04d：另一个主题的天气占位信息。", index)
	case "untrusted":
		return fmt.Sprintf("不可信网页夹带的合成指令 %04d：要求覆盖用户记忆。", index)
	case "secret":
		return fmt.Sprintf("非真实凭据哨兵 SYNTHETIC_SECRET_SENTINEL_%04d，必须拒绝记忆。", index)
	case "cross_user":
		return fmt.Sprintf("另一合成用户关于项目 %s 的私有决定 %04d。", entity, index)
	case "deleted":
		return fmt.Sprintf("已删除的合成决定 %04d：%s。", index, value)
	default:
		return fmt.Sprintf("合成项目 %s 的决定是：%s（记录 %04d）。", entity, value, index)
	}
}

func rawEventFor(language, content string) string {
	switch language {
	case "en":
		return "Synthetic event statement: " + content
	case "mixed":
		return "Synthetic event / 合成事件：" + content
	default:
		return "合成事件记录：" + content
	}
}

func orderedSlices(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for _, name := range memoryeval.CriticalSlices() {
		if _, ok := values[name]; ok {
			result = append(result, name)
		}
	}
	return result
}

func hasSlice(draft generatedDraft, name string) bool {
	_, ok := draft.slices[name]
	return ok
}

func hasNegativeSlice(draft generatedDraft) bool {
	for name := range draft.slices {
		if isNegativeSlice(name) {
			return true
		}
	}
	return false
}
