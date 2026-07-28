package chat

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestDetectDirectMemoryActionIntentIsExplicit(t *testing.T) {
	tests := []struct {
		value string
		want  string
		ok    bool
	}{
		{"请记住我喜欢简短回答", "remember", true},
		{"Please remember that I prefer concise replies", "remember", true},
		{"更正记忆：我喜欢详细回答", "correct", true},
		{"Correct the memory about my reply style", "correct", true},
		{"帮我忘记刚才我说的", "forget", true},
		{"Delete that memory", "forget", true},
		{"你记得我喜欢什么吗", "", false},
		{"我忘记带钥匙了", "", false},
		{"I forgot my password", "", false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, ok := detectDirectMemoryActionIntent(test.value)
			if got != test.want || ok != test.ok {
				t.Fatalf("intent = %q/%t, want %q/%t", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestPlanDirectMemoryActionStrictJSON(t *testing.T) {
	valid := `{"schemaVersion":"neo-chat.memory-user-action.v1","action":"remember",` +
		`"memoryType":"preference","content":"I prefer concise replies",` +
		`"importance":5,"tags":["reply"],"sensitivity":"normal",` +
		`"scopeType":"global","confidence":0.99,"targets":[]}`
	tests := []struct {
		name string
		body string
	}{
		{"missing", strings.Replace(valid, `,"targets":[]`, "", 1)},
		{"unknown", strings.Replace(valid, `"targets":[]`, `"targets":[],"extra":true`, 1)},
		{"duplicate", strings.Replace(valid, `"action":"remember"`, `"action":"remember","action":"remember"`, 1)},
		{"trailing", valid + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &directActionProvider{body: test.body}
			_, err := planDirectMemoryAction(
				context.Background(), provider, ModelRef{ModelID: "fixture"},
				"remember", "remember this", usermemory.DirectActionContext{},
			)
			if err == nil {
				t.Fatal("malformed planner output unexpectedly accepted")
			}
		})
	}
	provider := &directActionProvider{body: valid}
	plan, err := planDirectMemoryAction(
		context.Background(), provider, ModelRef{ModelID: "fixture"},
		"remember", "remember this", usermemory.DirectActionContext{},
	)
	if err != nil || plan.Content == nil || *plan.Content != "I prefer concise replies" {
		t.Fatalf("valid plan = %#v/%v", plan, err)
	}
}

func TestPrepareDirectMemoryActionRejectsSecretWithoutProviderEgress(t *testing.T) {
	repo := newDirectActionMemoryRepository()
	provider := &directActionProvider{body: "provider must not run"}
	handler := NewHandler(
		nil,
		WithUserMemoryService(usermemory.NewService(repo)),
	)
	preparation := handler.prepareDirectMemoryAction(
		context.Background(),
		"20000000-0000-4000-8000-000000000001",
		Message{
			ID:   "30000000-0000-4000-8000-000000000001",
			Role: "user", Status: "completed",
			Content: "请记住 password: super-secret-value",
		},
		Message{ID: "30000000-0000-4000-8000-000000000002"},
		provider,
		ModelRef{ModelID: "fixture"},
	)
	if provider.calls != 0 {
		t.Fatalf("secret reached provider %d times", provider.calls)
	}
	if preparation.Result == nil || preparation.Result.Status != "rejected" {
		t.Fatalf("preparation = %#v", preparation)
	}
	if repo.applied.PreflightResultCode != "SECRET_REJECTED" ||
		repo.applied.Content != "" || repo.applied.NormalizedContent != "" {
		t.Fatalf("secret apply input = %#v", repo.applied)
	}
}

func TestPrepareDirectMemoryActionRequiresCompletedCurrentUserMessage(t *testing.T) {
	tests := []Message{
		{
			ID:   "30000000-0000-4000-8000-000000000001",
			Role: "assistant", Status: "completed", Content: "请记住我喜欢简短回答",
		},
		{
			ID:   "30000000-0000-4000-8000-000000000002",
			Role: "user", Status: "streaming", Content: "请记住我喜欢简短回答",
		},
	}
	for _, message := range tests {
		repo := newDirectActionMemoryRepository()
		provider := &directActionProvider{body: "provider must not run"}
		handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
		preparation := handler.prepareDirectMemoryAction(
			context.Background(),
			"20000000-0000-4000-8000-000000000001",
			message,
			Message{ID: "30000000-0000-4000-8000-000000000003"},
			provider,
			ModelRef{ModelID: "fixture"},
		)
		if preparation.Result != nil || preparation.DegradationCode != "" ||
			provider.calls != 0 || repo.applied.ActionID != "" {
			t.Fatalf("non-current-user action = %#v provider=%d apply=%#v",
				preparation, provider.calls, repo.applied)
		}
	}
}

func TestPrepareDirectMemoryActionBindsVisibleTargetAndRejectsSpoof(t *testing.T) {
	const memoryID = "50000000-0000-4000-8000-000000000001"
	tests := []struct {
		name             string
		targetID         string
		expectedRevision int64
		scopeType        string
		wantPreflight    string
		wantTargetCount  int
	}{
		{"visible", memoryID, 7, "global", "", 1},
		{"spoof", "50000000-0000-4000-8000-000000000099", 7, "global", "TARGET_INVALID", 0},
		{"stale", memoryID, 6, "global", "REVISION_STALE", 0},
		{"scope mismatch", memoryID, 7, "project", "TARGET_SCOPE_MISMATCH", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newDirectActionMemoryRepository()
			repo.context = usermemory.DirectActionContext{
				ProjectID: "60000000-0000-4000-8000-000000000001",
				Memories: []usermemory.DirectActionMemory{{
					ID: memoryID, Revision: 7, Type: "preference",
					Content: "Use detailed replies", ScopeType: "global",
					AuthorityKind: "manual", Sensitivity: "normal",
				}},
			}
			provider := &directActionProvider{body: `{"schemaVersion":"neo-chat.memory-user-action.v1",` +
				`"action":"correct","memoryType":"preference","content":"Use concise replies",` +
				`"importance":5,"tags":["reply"],"sensitivity":"normal",` +
				`"scopeType":"` + test.scopeType + `","confidence":0.99,"targets":[{"memoryId":"` +
				test.targetID + `","expectedRevision":` +
				strconv.FormatInt(test.expectedRevision, 10) + `}]}`}
			handler := NewHandler(nil, WithUserMemoryService(usermemory.NewService(repo)))
			preparation := handler.prepareDirectMemoryAction(
				context.Background(),
				"20000000-0000-4000-8000-000000000001",
				Message{
					ID:   "30000000-0000-4000-8000-000000000001",
					Role: "user", Status: "completed",
					Content: "更正记忆：我喜欢简短回答",
				},
				Message{ID: "30000000-0000-4000-8000-000000000002"},
				provider,
				ModelRef{ModelID: "fixture"},
			)
			if preparation.Result == nil || provider.calls != 1 {
				t.Fatalf("preparation/provider = %#v/%d", preparation, provider.calls)
			}
			if repo.applied.PreflightResultCode != test.wantPreflight ||
				len(repo.applied.Targets) != test.wantTargetCount {
				t.Fatalf("apply = %#v", repo.applied)
			}
			if strings.Contains(provider.last.Prompt, repo.context.ProjectID) {
				t.Fatal("provider input exposed bound project id")
			}
		})
	}
}

type directActionProvider struct {
	body  string
	calls int
	last  ProviderRequest
}

func (p *directActionProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.calls++
	p.last = input
	events := make(chan ProviderEvent, 1)
	events <- ProviderEvent{Type: ProviderEventDelta, Delta: p.body}
	close(events)
	return events, nil
}

type directActionMemoryRepository struct {
	*chatMemoryRepository
	context usermemory.DirectActionContext
	applied usermemory.DirectActionApplyInput
}

func newDirectActionMemoryRepository() *directActionMemoryRepository {
	return &directActionMemoryRepository{
		chatMemoryRepository: &chatMemoryRepository{settings: usermemory.DefaultSettings()},
	}
}

func (r *directActionMemoryRepository) HydrateDirectAction(
	_ context.Context,
	_ usermemory.DirectActionHydrationInput,
) (usermemory.DirectActionContext, error) {
	return r.context, nil
}

func (r *directActionMemoryRepository) ApplyDirectAction(
	_ context.Context,
	input usermemory.DirectActionApplyInput,
) (usermemory.DirectActionResult, error) {
	r.applied = input
	return usermemory.DirectActionResult{
		ActionID: input.ActionID,
		Status: func() string {
			if input.PreflightStatus != "" {
				return input.PreflightStatus
			}
			return "applied"
		}(),
		ResultCode: func() string {
			if input.PreflightResultCode != "" {
				return input.PreflightResultCode
			}
			return "DIRECT_CORRECTED"
		}(),
		MemoryID:       "50000000-0000-4000-8000-000000000001",
		MemoryRevision: 8,
		ScopeType:      input.ScopeType,
		ActivityID:     input.ActivityID,
	}, nil
}

func (r *directActionMemoryRepository) ListActivities(
	context.Context, string, int,
) ([]usermemory.MemoryActivity, error) {
	return nil, nil
}

func (r *directActionMemoryRepository) ListMessageUsages(
	context.Context, string,
) ([]usermemory.MessageMemoryUsage, error) {
	return nil, nil
}

func (r *directActionMemoryRepository) UndoActivity(
	context.Context, usermemory.UndoActivityInput,
) (usermemory.UndoActivityResult, error) {
	return usermemory.UndoActivityResult{}, nil
}

var _ usermemory.ActionRepository = (*directActionMemoryRepository)(nil)
