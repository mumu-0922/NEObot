package chat

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestActiveRunStreamReplaysAfterCursorAndSignalsEvictionGap(t *testing.T) {
	stream := newActiveRunStream(testRunID, testConversationID, testMessageID)
	for sequence := 1; sequence <= activeRunStreamMaxFrames+1; sequence++ {
		payload := []byte(fmt.Sprintf(
			`{"type":"message.delta","runId":%q,"conversationId":%q,"messageId":%q,"sequence":%d,"delta":"x"}`,
			testRunID, testConversationID, testMessageID, sequence,
		))
		stream.publish("message.delta", payload)
	}
	stream.close()

	subscription := stream.subscribe(0)
	defer subscription.Unsubscribe()
	if subscription.Gap == nil || subscription.Gap.OldestSequence != 2 ||
		subscription.Gap.LatestSequence != activeRunStreamMaxFrames+1 {
		t.Fatalf("gap=%#v", subscription.Gap)
	}
	if len(subscription.Replay) != activeRunStreamMaxFrames ||
		subscription.Replay[0].Sequence != 2 {
		t.Fatalf("replay count=%d first=%#v", len(subscription.Replay), subscription.Replay[0])
	}
	if !strings.HasPrefix(string(subscription.Replay[0].Bytes), "id: 2\nevent: message.delta\n") {
		t.Fatalf("frame=%q", subscription.Replay[0].Bytes)
	}
	if !subscription.Closed {
		t.Fatal("closed stream subscription remained open")
	}
}

func TestActiveRunStreamSubscriberReceivesFutureFrameAndCloses(t *testing.T) {
	stream := newActiveRunStream(testRunID, testConversationID, testMessageID)
	subscription := stream.subscribe(0)
	defer subscription.Unsubscribe()
	payload := []byte(fmt.Sprintf(
		`{"type":"message.delta","runId":%q,"sequence":1,"delta":"live"}`,
		testRunID,
	))
	stream.publish("message.delta", payload)
	select {
	case frame := <-subscription.Updates:
		if frame.Sequence != 1 || !strings.Contains(string(frame.Bytes), `"delta":"live"`) {
			t.Fatalf("frame=%#v", frame)
		}
	default:
		t.Fatal("live subscriber did not receive published frame")
	}
	stream.close()
	if _, open := <-subscription.Updates; open {
		t.Fatal("subscriber remained open after stream close")
	}
}

func TestRunEventsEndpointReplaysAuthorizedRetainedFrames(t *testing.T) {
	repository := newFakeRepository()
	handler := NewHandler(NewService(repository))
	created := performRequest(handler, http.MethodPost, conversationsPath, `{"title":"Cursor"}`)
	assertStatus(t, created, http.StatusCreated)

	stream := newActiveRunStream(testRunID, testConversationID, testMessageID)
	for sequence := 1; sequence <= 3; sequence++ {
		payload := []byte(fmt.Sprintf(
			`{"type":"message.delta","runId":%q,"conversationId":%q,"messageId":%q,"sequence":%d,"delta":"%d"}`,
			testRunID, testConversationID, testMessageID, sequence, sequence,
		))
		stream.publish("message.delta", payload)
	}
	unregister := handler.activeRuns.registerStream(
		testRunID, func() {}, DevUserID, testConversationID, stream,
	)
	unregister()
	if unauthorized, _ := handler.activeRuns.streamForUser(
		testRunID, "ffffffff-ffff-4fff-8fff-ffffffffffff",
	); unauthorized != nil {
		t.Fatal("another user resolved the retained Run stream")
	}

	response := performRequest(
		handler, http.MethodGet, runsPathBase+testRunID+"/events?after=1", "",
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, `"sequence":1`) ||
		!strings.Contains(body, "id: 2\nevent: message.delta") ||
		!strings.Contains(body, "id: 3\nevent: message.delta") {
		t.Fatalf("unexpected replay body=%q", body)
	}

	invalid := performRequest(
		handler, http.MethodGet, runsPathBase+testRunID+"/events?after=-1", "",
	)
	assertStatus(t, invalid, http.StatusBadRequest)
}
