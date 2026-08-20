package chat

import (
	"encoding/json"
	"fmt"
	"sync"
)

const (
	activeRunStreamMaxFrames       = 1024
	activeRunStreamMaxBytes        = 4 << 20
	activeRunStreamSubscriberQueue = 64
	activeRunStreamMaxSubscribers  = 16
)

type activeRunFrame struct {
	Sequence int
	Bytes    []byte
}

type activeRunGap struct {
	After          int
	OldestSequence int
	LatestSequence int
}

type activeRunSubscription struct {
	Replay      []activeRunFrame
	Gap         *activeRunGap
	Updates     <-chan activeRunFrame
	Closed      bool
	unsubscribe func()
}

func (subscription activeRunSubscription) Unsubscribe() {
	if subscription.unsubscribe != nil {
		subscription.unsubscribe()
	}
}

type activeRunStream struct {
	mu             sync.Mutex
	runID          string
	conversationID string
	messageID      string
	frames         []activeRunFrame
	retainedBytes  int
	latestSequence int
	droppedThrough int
	subscribers    map[uint64]chan activeRunFrame
	nextSubscriber uint64
	closed         bool
}

func newActiveRunStream(runID, conversationID, messageID string) *activeRunStream {
	return &activeRunStream{
		runID: runID, conversationID: conversationID, messageID: messageID,
		subscribers: make(map[uint64]chan activeRunFrame),
	}
}

func (stream *activeRunStream) publish(event string, payload []byte) []byte {
	sequence, runID, ok := streamEventIdentity(payload)
	frameBytes := formatSSEFrame(event, payload, sequence)
	if stream == nil || !ok || runID != stream.runID {
		return frameBytes
	}
	frame := activeRunFrame{Sequence: sequence, Bytes: append([]byte(nil), frameBytes...)}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed || sequence <= stream.latestSequence {
		return frameBytes
	}
	if sequence > stream.latestSequence+1 {
		stream.droppedThrough = max(stream.droppedThrough, sequence-1)
	}
	stream.latestSequence = sequence
	if len(frame.Bytes) <= activeRunStreamMaxBytes {
		stream.frames = append(stream.frames, frame)
		stream.retainedBytes += len(frame.Bytes)
		for len(stream.frames) > activeRunStreamMaxFrames ||
			stream.retainedBytes > activeRunStreamMaxBytes {
			dropped := stream.frames[0]
			stream.frames = stream.frames[1:]
			stream.retainedBytes -= len(dropped.Bytes)
			stream.droppedThrough = max(stream.droppedThrough, dropped.Sequence)
		}
	} else {
		stream.droppedThrough = max(stream.droppedThrough, sequence)
	}
	for id, subscriber := range stream.subscribers {
		select {
		case subscriber <- cloneActiveRunFrame(frame):
		default:
			close(subscriber)
			delete(stream.subscribers, id)
		}
	}
	return frameBytes
}

func (stream *activeRunStream) subscribe(after int) activeRunSubscription {
	if stream == nil {
		return activeRunSubscription{Closed: true}
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()

	replay := make([]activeRunFrame, 0, len(stream.frames))
	for _, frame := range stream.frames {
		if frame.Sequence > after {
			replay = append(replay, cloneActiveRunFrame(frame))
		}
	}
	var gap *activeRunGap
	if after < stream.droppedThrough {
		oldest := stream.latestSequence + 1
		for _, frame := range stream.frames {
			if frame.Sequence > after {
				oldest = frame.Sequence
				break
			}
		}
		gap = &activeRunGap{
			After: after, OldestSequence: oldest, LatestSequence: stream.latestSequence,
		}
	}
	if stream.closed {
		return activeRunSubscription{Replay: replay, Gap: gap, Closed: true}
	}
	if len(stream.subscribers) >= activeRunStreamMaxSubscribers {
		return activeRunSubscription{Replay: replay, Gap: gap, Closed: true}
	}

	stream.nextSubscriber++
	id := stream.nextSubscriber
	updates := make(chan activeRunFrame, activeRunStreamSubscriberQueue)
	stream.subscribers[id] = updates
	return activeRunSubscription{
		Replay: replay, Gap: gap, Updates: updates,
		unsubscribe: func() {
			stream.mu.Lock()
			if subscriber := stream.subscribers[id]; subscriber != nil {
				close(subscriber)
				delete(stream.subscribers, id)
			}
			stream.mu.Unlock()
		},
	}
}

func (stream *activeRunStream) close() {
	if stream == nil {
		return
	}
	stream.mu.Lock()
	if !stream.closed {
		stream.closed = true
		for id, subscriber := range stream.subscribers {
			close(subscriber)
			delete(stream.subscribers, id)
		}
	}
	stream.mu.Unlock()
}

func cloneActiveRunFrame(frame activeRunFrame) activeRunFrame {
	return activeRunFrame{Sequence: frame.Sequence, Bytes: append([]byte(nil), frame.Bytes...)}
}

func streamEventIdentity(payload []byte) (int, string, bool) {
	var envelope struct {
		RunID    string `json:"runId"`
		Sequence int    `json:"sequence"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil ||
		envelope.RunID == "" || envelope.Sequence < 1 {
		return 0, "", false
	}
	return envelope.Sequence, envelope.RunID, true
}

func formatSSEFrame(event string, payload []byte, sequence int) []byte {
	prefix := ""
	if sequence > 0 {
		prefix = fmt.Sprintf("id: %d\n", sequence)
	}
	return []byte(fmt.Sprintf("%sevent: %s\ndata: %s\n\n", prefix, event, payload))
}
