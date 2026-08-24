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

func TestActiveRunRegistryScopesOneRunPerConversation(t *testing.T) {
	registry := newActiveRunRegistry()
	releaseA, reserved := registry.reserve(
		testRunID, DevUserID, testConversationID, testMessageID,
	)
	if !reserved {
		t.Fatal("first Conversation Run was not reserved")
	}
	defer releaseA()

	if _, duplicate := registry.reserve(
		"22222222-2222-4222-8222-222222222222",
		DevUserID,
		testConversationID,
		"33333333-3333-4333-8333-333333333333",
	); duplicate {
		t.Fatal("same-user same-Conversation Run was reserved twice")
	}

	otherConversationID := "44444444-4444-4444-8444-444444444444"
	releaseB, siblingReserved := registry.reserve(
		"55555555-5555-4555-8555-555555555555",
		DevUserID,
		otherConversationID,
		"66666666-6666-4666-8666-666666666666",
	)
	if !siblingReserved {
		t.Fatal("different Conversation Run was not admitted")
	}
	defer releaseB()
	otherUserID := "77777777-7777-4777-8777-777777777777"
	releaseOtherUser, otherUserReserved := registry.reserve(
		"88888888-8888-4888-8888-888888888888",
		otherUserID,
		testConversationID,
		"99999999-9999-4999-8999-999999999999",
	)
	if !otherUserReserved {
		t.Fatal("different user's Conversation Run was not admitted")
	}
	defer releaseOtherUser()

	runs := registry.activeForUser(DevUserID)
	if len(runs) != 2 || runs[0].Status != "pending" || runs[1].Status != "pending" {
		t.Fatalf("active Runs=%#v", runs)
	}
	stream := newActiveRunStream(testRunID, testConversationID, testMessageID)
	if !registry.attach(testRunID, func() {}, stream) {
		t.Fatal("reserved Run did not accept its stream")
	}
	runs = registry.activeForUser(DevUserID)
	attachedStatus := ""
	for _, run := range runs {
		if run.ConversationID == testConversationID {
			attachedStatus = run.Status
		}
	}
	if len(runs) != 2 || attachedStatus != "streaming" {
		t.Fatalf("attached active Runs=%#v", runs)
	}
	otherUserRuns := registry.activeForUser(otherUserID)
	if len(otherUserRuns) != 1 || otherUserRuns[0].ConversationID != testConversationID {
		t.Fatalf("other user's active Runs=%#v", otherUserRuns)
	}

	releaseA()
	runs = registry.activeForUser(DevUserID)
	if len(runs) != 1 || runs[0].ConversationID != otherConversationID {
		t.Fatalf("finished Run remained active: %#v", runs)
	}
}

func TestConversationListIncludesOnlyCurrentUsersActiveRuns(t *testing.T) {
	repository := newFakeRepository()
	handler := NewHandler(NewService(repository))
	created := performRequest(handler, http.MethodPost, conversationsPath, `{"title":"Running"}`)
	assertStatus(t, created, http.StatusCreated)

	stream := newActiveRunStream(testRunID, testConversationID, testMessageID)
	unregister := handler.activeRuns.registerStream(
		testRunID, func() {}, DevUserID, testConversationID, stream,
	)
	defer unregister()

	response := performRequest(handler, http.MethodGet, conversationsPath, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, required := range []string{
		`"activeGeneration"`,
		`"runId":"` + testRunID + `"`,
		`"messageId":"` + testMessageID + `"`,
		`"status":"streaming"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("conversation list missing %s: %s", required, body)
		}
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
