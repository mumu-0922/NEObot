package usermemory

import (
	"context"
	"testing"
	"time"
)

func TestSearchRelevantReturnsOnlyRelatedTopMemory(t *testing.T) {
	repo := &fakeRepository{
		settings:      Settings{Enabled: true, SearchEnabled: true},
		settingsFound: true,
		memories: []Memory{
			memoryFixture("11111111-1111-4111-8111-111111111111", "preference", "用户偏好简洁回答；以后问到部署代号时只回答该代号", 5),
			memoryFixture("22222222-2222-4222-8222-222222222222", "fact", "用户住在上海", 5),
		},
	}
	service := NewService(repo)

	got, err := service.SearchRelevant(context.Background(), "请继续用简洁风格回答", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != repo.memories[0].ID {
		t.Fatalf("SearchRelevant() = %#v, want concise preference only", got)
	}
	if len(repo.markedUsed) != 1 || repo.markedUsed[0] != got[0].ID {
		t.Fatalf("marked used = %#v", repo.markedUsed)
	}

	got, err = service.SearchRelevant(context.Background(), "量子纠缠实验结果是什么", 5)
	if err != nil || len(got) != 0 {
		t.Fatalf("unrelated SearchRelevant() = %#v/%v, want empty", got, err)
	}
	got, err = service.SearchRelevant(context.Background(), "只回答数字：17 乘以 19 等于多少", 5)
	if err != nil || len(got) != 0 {
		t.Fatalf("low-information phrase SearchRelevant() = %#v/%v, want empty", got, err)
	}
}

func TestSearchAndExtractionStopWhenMemoryDisabled(t *testing.T) {
	repo := &fakeRepository{
		settings:      DefaultSettings(),
		settingsFound: true,
		memories: []Memory{
			memoryFixture("11111111-1111-4111-8111-111111111111", "preference", "Use Go", 5),
		},
	}
	service := NewService(repo)

	got, err := service.SearchRelevant(context.Background(), "Use Go", 5)
	if err != nil || len(got) != 0 || repo.listCalls != 0 {
		t.Fatalf("disabled search = %#v/%v listCalls=%d", got, err, repo.listCalls)
	}
	created, err := service.StoreExtracted(context.Background(), ExtractionInput{
		ConversationID: "33333333-3333-4333-8333-333333333333",
		MessageID:      "44444444-4444-4444-8444-444444444444",
		Candidates: []Candidate{{
			Type: "preference", Content: "Use Go", Importance: 5,
		}},
	})
	if err != nil || len(created) != 0 || repo.createCalls != 0 {
		t.Fatalf("disabled extraction = %#v/%v createCalls=%d", created, err, repo.createCalls)
	}
}

func TestMemorySettingsRequireExplicitOptIn(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	settings, err := service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || !settings.SearchEnabled || settings.AutoRecordEnabled {
		t.Fatalf("default settings = %#v", settings)
	}
	enabled := true
	settings, err = service.UpdateSettings(context.Background(), SettingsPatch{
		Enabled: &enabled, AutoRecordEnabled: &enabled,
	})
	if err != nil || !settings.Enabled || !settings.AutoRecordEnabled {
		t.Fatalf("updated settings = %#v/%v", settings, err)
	}
}

func TestStoreExtractedSkipsInvalidCandidateWithoutFailingBatch(t *testing.T) {
	repo := &fakeRepository{
		settings:      Settings{Enabled: true, SearchEnabled: true, AutoRecordEnabled: true},
		settingsFound: true,
	}
	service := NewService(repo)
	created, err := service.StoreExtracted(context.Background(), ExtractionInput{
		ConversationID: "33333333-3333-4333-8333-333333333333",
		MessageID:      "44444444-4444-4444-8444-444444444444",
		Candidates: []Candidate{
			{Type: "unknown", Content: "drop me"},
			{Type: "preference", Content: "Use concise answers", Importance: 4, Tags: []string{"Style"}},
		},
	})
	if err != nil || len(created) != 1 {
		t.Fatalf("StoreExtracted() = %#v/%v", created, err)
	}
	if created[0].Source != "ai" || created[0].Tags[0] != "style" {
		t.Fatalf("created memory = %#v", created[0])
	}
}

func memoryFixture(id string, memoryType string, content string, importance int) Memory {
	now := time.Now().UTC()
	return Memory{
		ID: id, Type: memoryType, Content: content,
		NormalizedContent: normalizeSearchText(content),
		Importance:        importance, Tags: []string{}, Source: "manual", Enabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
}

type fakeRepository struct {
	settings      Settings
	settingsFound bool
	memories      []Memory
	listCalls     int
	createCalls   int
	markedUsed    []string
}

func (r *fakeRepository) GetSettings(context.Context) (Settings, bool, error) {
	return r.settings, r.settingsFound, nil
}

func (r *fakeRepository) UpsertSettings(_ context.Context, input Settings) (Settings, error) {
	r.settings = input
	r.settingsFound = true
	return input, nil
}

func (r *fakeRepository) List(context.Context) ([]Memory, error) {
	r.listCalls++
	result := make([]Memory, 0, len(r.memories))
	for _, memory := range r.memories {
		if memory.DeletedAt == nil {
			result = append(result, memory)
		}
	}
	return result, nil
}

func (r *fakeRepository) Create(_ context.Context, input CreateInput) (Memory, error) {
	r.createCalls++
	now := time.Now().UTC()
	memory := Memory{
		ID: input.ID, Type: input.Type, Content: input.Content,
		NormalizedContent: input.NormalizedContent, Importance: input.Importance,
		Tags: input.Tags, Source: input.Source,
		SourceConversationID: input.SourceConversationID,
		SourceMessageID:      input.SourceMessageID,
		Enabled:              input.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	r.memories = append(r.memories, memory)
	return memory, nil
}

func (r *fakeRepository) Update(_ context.Context, id string, input UpdateInput) (Memory, error) {
	for index := range r.memories {
		if r.memories[index].ID != id {
			continue
		}
		r.memories[index].Type = input.Type
		r.memories[index].Content = input.Content
		r.memories[index].NormalizedContent = input.NormalizedContent
		r.memories[index].Importance = input.Importance
		r.memories[index].Tags = input.Tags
		r.memories[index].Enabled = input.Enabled
		return r.memories[index], nil
	}
	return Memory{}, ErrMemoryNotFound
}

func (r *fakeRepository) Delete(_ context.Context, id string) error {
	for index := range r.memories {
		if r.memories[index].ID == id {
			now := time.Now().UTC()
			r.memories[index].DeletedAt = &now
			return nil
		}
	}
	return ErrMemoryNotFound
}

func (r *fakeRepository) MarkUsed(_ context.Context, ids []string, _ time.Time) error {
	r.markedUsed = append(r.markedUsed, ids...)
	return nil
}

var _ Repository = (*fakeRepository)(nil)
