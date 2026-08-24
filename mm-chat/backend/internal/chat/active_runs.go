package chat

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	activeRunStreamRetention = 30 * time.Second
	maxFinishedRunStreams    = 64
)

type activeRunEntry struct {
	cancel         context.CancelFunc
	userID         string
	conversationID string
	messageID      string
	stream         *activeRunStream
	finishedAt     time.Time
}

type activeConversationRun struct {
	RunID          string
	ConversationID string
	MessageID      string
	Status         string
}

type activeRunRegistry struct {
	mu      sync.Mutex
	entries map[string]*activeRunEntry
}

func newActiveRunRegistry() *activeRunRegistry {
	return &activeRunRegistry{entries: map[string]*activeRunEntry{}}
}

func (r *activeRunRegistry) reserve(
	runID string,
	userID string,
	conversationID string,
	messageID string,
) (func(), bool) {
	if r == nil || runID == "" || userID == "" || conversationID == "" {
		return func() {}, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.entries {
		if entry.userID == userID && entry.conversationID == conversationID &&
			entry.finishedAt.IsZero() {
			return func() {}, false
		}
	}
	r.entries[runID] = &activeRunEntry{
		userID: userID, conversationID: conversationID, messageID: messageID,
	}
	return func() { r.finish(runID, nil) }, true
}

func (r *activeRunRegistry) attach(
	runID string,
	cancel context.CancelFunc,
	stream *activeRunStream,
) bool {
	if r == nil || runID == "" || cancel == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[runID]
	if entry == nil || !entry.finishedAt.IsZero() {
		return false
	}
	entry.cancel = cancel
	entry.stream = stream
	return true
}

func (r *activeRunRegistry) activeForUser(userID string) []activeConversationRun {
	if r == nil || userID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	runs := make([]activeConversationRun, 0)
	for runID, entry := range r.entries {
		if entry.userID != userID || entry.conversationID == "" ||
			!entry.finishedAt.IsZero() {
			continue
		}
		status := "pending"
		if entry.cancel != nil {
			status = "streaming"
		}
		runs = append(runs, activeConversationRun{
			RunID: runID, ConversationID: entry.conversationID,
			MessageID: entry.messageID, Status: status,
		})
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].ConversationID == runs[j].ConversationID {
			return runs[i].RunID < runs[j].RunID
		}
		return runs[i].ConversationID < runs[j].ConversationID
	})
	return runs
}

func (r *activeRunRegistry) register(runID string, cancel context.CancelFunc) func() {
	return r.registerStream(runID, cancel, "", "", nil)
}

func (r *activeRunRegistry) registerStream(
	runID string,
	cancel context.CancelFunc,
	userID string,
	conversationID string,
	stream *activeRunStream,
) func() {
	if r == nil || runID == "" || cancel == nil {
		return func() {}
	}

	r.mu.Lock()
	r.entries[runID] = &activeRunEntry{
		cancel: cancel, userID: userID, conversationID: conversationID, stream: stream,
	}
	if stream != nil {
		r.entries[runID].messageID = stream.messageID
	}
	r.mu.Unlock()

	return func() {
		r.finish(runID, stream)
	}
}

func (r *activeRunRegistry) cancel(runID string) bool {
	if r == nil || runID == "" {
		return false
	}

	r.mu.Lock()
	entry := r.entries[runID]
	var cancel context.CancelFunc
	if entry != nil {
		cancel = entry.cancel
	}
	r.mu.Unlock()

	if cancel == nil {
		return false
	}

	cancel()
	return true
}

func (r *activeRunRegistry) streamForUser(runID, userID string) (*activeRunStream, string) {
	if r == nil || runID == "" || userID == "" {
		return nil, ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entries[runID]
	if entry == nil || entry.userID != userID || entry.stream == nil {
		return nil, ""
	}
	return entry.stream, entry.conversationID
}

func (r *activeRunRegistry) finish(runID string, stream *activeRunStream) {
	if r == nil || runID == "" {
		return
	}
	r.mu.Lock()
	entry := r.entries[runID]
	if entry == nil || (stream != nil && entry.stream != stream) {
		r.mu.Unlock()
		return
	}
	if !entry.finishedAt.IsZero() {
		r.mu.Unlock()
		return
	}
	ownedStream := entry.stream
	if ownedStream == nil {
		delete(r.entries, runID)
		r.mu.Unlock()
		return
	}
	ownedStream.close()
	entry.cancel = nil
	entry.finishedAt = time.Now()
	r.pruneFinishedLocked()
	r.mu.Unlock()
	time.AfterFunc(activeRunStreamRetention, func() {
		r.mu.Lock()
		current := r.entries[runID]
		if current == entry && !current.finishedAt.IsZero() &&
			time.Since(current.finishedAt) >= activeRunStreamRetention {
			delete(r.entries, runID)
		}
		r.mu.Unlock()
	})
}

func (r *activeRunRegistry) pruneFinishedLocked() {
	for {
		finishedCount := 0
		oldestRunID := ""
		var oldest time.Time
		for runID, entry := range r.entries {
			if entry.finishedAt.IsZero() {
				continue
			}
			finishedCount++
			if oldestRunID == "" || entry.finishedAt.Before(oldest) {
				oldestRunID, oldest = runID, entry.finishedAt
			}
		}
		if finishedCount <= maxFinishedRunStreams || oldestRunID == "" {
			return
		}
		delete(r.entries, oldestRunID)
	}
}
