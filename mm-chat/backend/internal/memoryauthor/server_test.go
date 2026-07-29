package memoryauthor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReviewServerLoopbackSessionCSRFAndAction(t *testing.T) {
	root := newTestPoolRoot(t)
	reviewedAt := time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC)
	server, err := StartReviewServer(ReviewServerOptions{
		Root: root, ReviewerID: testReviewerID, Clock: func() time.Time { return reviewedAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())
	parsed, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := parsed.Fragment
	parsed.Fragment = ""
	origin := strings.TrimSuffix(parsed.String(), "/")
	client := &http.Client{Timeout: 5 * time.Second}

	response, err := client.Get(origin + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || strings.Contains(string(page), bootstrap) ||
		response.Header.Get("Cache-Control") != "no-store, max-age=0" ||
		response.Header.Get("Access-Control-Allow-Origin") != "" ||
		response.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("review page status/headers/body = %d %#v", response.StatusCode, response.Header)
	}

	request, _ := http.NewRequest(http.MethodGet, origin+"/", nil)
	request.Host = "evil.invalid"
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("invalid Host status = %d", response.StatusCode)
	}

	sessionBody, _ := json.Marshal(map[string]string{"token": bootstrap})
	request, _ = http.NewRequest(http.MethodPost, origin+"/session", bytes.NewReader(sessionBody))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing Origin session status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, origin+"/session", bytes.NewReader(sessionBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || session.CSRFToken == "" || len(response.Cookies()) != 1 {
		t.Fatalf("session response = %d %+v cookies=%d", response.StatusCode, session, len(response.Cookies()))
	}
	cookie := response.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %+v", cookie)
	}

	request, _ = http.NewRequest(http.MethodPost, origin+"/session", bytes.NewReader(sessionBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("bootstrap replay status = %d", response.StatusCode)
	}

	response, err = client.Get(origin + "/api/case?index=0")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated case status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodGet, origin+"/api/case?index=0", nil)
	request.AddCookie(cookie)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var item reviewCaseResponse
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || item.Sequence != 0 || item.Decision != DecisionPending {
		t.Fatalf("case response = %d %+v", response.StatusCode, item)
	}

	actionBody, _ := json.Marshal(reviewActionRequest{
		Action: ReviewActionAccept, CaseID: item.Snapshot.Case.ID,
		ExpectedSequence: item.Sequence, ExpectedContentSHA256: item.ContentSHA256,
	})
	request, _ = http.NewRequest(http.MethodPost, origin+"/api/action", bytes.NewReader(actionBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	request.AddCookie(cookie)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF action status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, origin+"/api/action", bytes.NewReader(actionBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(cookie)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authorized action status = %d", response.StatusCode)
	}
	state, err := LoadReviewState(root)
	if err != nil || state.Cases[0].Decision != DecisionAccepted || state.Cases[0].ReviewedAt != reviewedAt.Format(time.RFC3339) {
		t.Fatalf("review action state = %+v, %v", state.Cases[0], err)
	}
	request, _ = http.NewRequest(http.MethodGet, origin+"/api/case?index=pending", nil)
	request.AddCookie(cookie)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || item.Index != 1 {
		t.Fatalf("next pending response = %d %+v", response.StatusCode, item)
	}
}

func TestReviewServerRejectsUnknownJSONAndNonLoopbackListener(t *testing.T) {
	root := newTestPoolRoot(t)
	server, err := StartReviewServer(ReviewServerOptions{Root: root, ReviewerID: testReviewerID})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(context.Background())
	parsed, _ := url.Parse(server.URL())
	bootstrap := parsed.Fragment
	parsed.Fragment = ""
	origin := strings.TrimSuffix(parsed.String(), "/")
	client := &http.Client{Timeout: 5 * time.Second}
	body := []byte(`{"token":"` + bootstrap + `","unknown":true}`)
	request, _ := http.NewRequest(http.MethodPost, origin+"/session", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(responseBody), "invalid") {
		t.Fatalf("unknown JSON response = %d %q", response.StatusCode, responseBody)
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartReviewServer(ReviewServerOptions{
		Root: root, ReviewerID: testReviewerID, Listener: listener,
	}); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback listener error = %v", err)
	}
}
