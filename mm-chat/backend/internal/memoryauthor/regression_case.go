package memoryauthor

import (
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func generateRegressionCase(
	draft regressionDraft,
	profile regressionGenerationProfile,
) (Fixture, memoryeval.GoldenCase) {
	scenario := newRegressionScenario(draft, profile.generator.Seed)
	fixtureAlias := opaqueRegressionIDForSeed(profile.generator.Seed, "fixture", draft.index)
	item := memoryeval.GoldenCase{
		ID:           opaqueRegressionIDForSeed(profile.generator.Seed, "case", draft.index),
		Query:        regressionQueryForProfile(scenario, profile),
		Split:        draft.split,
		Language:     draft.language,
		Slices:       orderedSlices(draft.slices),
		FixtureAlias: fixtureAlias,
		Scope:        scenario.scope,
		Review:       memoryeval.Review{State: "draft"},
	}
	fixture := Fixture{Alias: fixtureAlias, UserAlias: scenario.scope.UserAlias}
	baseTime := time.Date(2026, time.February, 1, 8, 0, 0, 0, time.UTC).
		Add(time.Duration(draft.index) * time.Hour)
	newMemory := func(
		role string,
		owner string,
		scope memoryeval.Scope,
		state MemoryState,
		content string,
		offset int,
	) FixtureMemory {
		return FixtureMemory{
			ID:               opaqueRegressionIDForSeed(profile.generator.Seed, "memory-"+role, draft.index),
			UserAlias:        owner,
			Scope:            scope,
			CanonicalContent: content,
			RawEventContent:  regressionRawEvent(draft.language, content),
			OccurredAt:       baseTime.Add(time.Duration(offset) * time.Minute).Format(time.RFC3339),
			State:            state,
		}
	}

	if hasRegressionNegative(draft) {
		item.ExpectedNoMemory = true
		for _, slice := range regressionCoreSlices {
			if !isNegativeSlice(slice) || !regressionHasSlice(draft, slice) {
				continue
			}
			owner, scope, state, reason := scenario.scope.UserAlias, scenario.scope, StateIrrelevant, "irrelevant"
			switch slice {
			case "untrusted_source":
				state, reason = StateUntrustedRejected, "untrusted_source"
			case "secret_rejection":
				state, reason = StateSecretRejected, "secret"
			case "scope_isolation":
				owner = opaqueRegressionIDForSeed(profile.generator.Seed, "other-user", draft.index)
				scope.UserAlias = owner
				state, reason = StateOutOfScope, "cross_user"
			case "deletion":
				state, reason = StateDeleted, "deleted"
			}
			memory := newMemory(
				slice,
				owner,
				scope,
				state,
				regressionNegativeContentForProfile(scenario, slice, profile),
				len(fixture.Memories)+1,
			)
			fixture.Memories = append(fixture.Memories, memory)
			item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: memory.ID, Reason: reason})
		}
		return fixture, item
	}

	if regressionHasSlice(draft, "temporal_correction") {
		old := newMemory(
			"superseded",
			scenario.scope.UserAlias,
			scenario.scope,
			StateSuperseded,
			regressionOldContent(scenario),
			1,
		)
		fixture.Memories = append(fixture.Memories, old)
		item.Exclusions = append(item.Exclusions, memoryeval.Exclusion{MemoryID: old.ID, Reason: "superseded"})
	}
	primary := newMemory(
		"primary",
		scenario.scope.UserAlias,
		scenario.scope,
		StateActive,
		regressionPositiveContent(scenario, false),
		2,
	)
	fixture.Memories = append(fixture.Memories, primary)
	item.ExpectedRelevantMemoryIDs = append(item.ExpectedRelevantMemoryIDs, primary.ID)
	item.ExpectedCurrentMemoryIDs = append(item.ExpectedCurrentMemoryIDs, primary.ID)
	if regressionHasSlice(draft, "multi_hop") {
		secondary := newMemory(
			"secondary",
			scenario.scope.UserAlias,
			scenario.scope,
			StateActive,
			regressionPositiveContent(scenario, true),
			3,
		)
		fixture.Memories = append(fixture.Memories, secondary)
		item.ExpectedRelevantMemoryIDs = append(item.ExpectedRelevantMemoryIDs, secondary.ID)
		item.ExpectedCurrentMemoryIDs = append(item.ExpectedCurrentMemoryIDs, secondary.ID)
	}
	return fixture, item
}

func newRegressionScenario(draft regressionDraft, seed string) regressionScenario {
	index := draft.index
	entity := regressionEntities[index/len(regressionSubjectsZH)]
	project := ""
	conversation := ""
	if regressionHasSlice(draft, "project_decision") || index%3 != 0 {
		project = opaqueRegressionIDForSeed(seed, "project", index%47)
	}
	if project != "" && index%3 == 2 {
		conversation = opaqueRegressionIDForSeed(seed, "conversation", index%61)
	}
	return regressionScenario{
		draft:      draft,
		entity:     entity,
		subjectZH:  regressionSubjectsZH[index%len(regressionSubjectsZH)],
		subjectEN:  regressionSubjectsEN[index%len(regressionSubjectsEN)],
		valueZH:    regressionValuesZH[(index*7+3)%len(regressionValuesZH)],
		valueEN:    regressionValuesEN[(index*7+3)%len(regressionValuesEN)],
		oldValueZH: regressionValuesZH[(index*7+9)%len(regressionValuesZH)],
		oldValueEN: regressionValuesEN[(index*7+9)%len(regressionValuesEN)],
		scope: memoryeval.Scope{
			UserAlias:         opaqueRegressionIDForSeed(seed, "user", index),
			ProjectAlias:      project,
			ConversationAlias: conversation,
		},
	}
}
