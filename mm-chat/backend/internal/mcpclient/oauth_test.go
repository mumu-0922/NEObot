package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOAuthPKCEHashedSingleUseStateAndEncryptedToken(t *testing.T) {
	t.Parallel()
	var challenge string
	var tokenServer *httptest.Server
	tokenServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			writeJSON(t, writer, map[string]any{
				"resource":              tokenServer.URL + "/mcp",
				"authorization_servers": []string{tokenServer.URL},
			})
		case "/.well-known/oauth-authorization-server":
			writeJSON(t, writer, map[string]any{
				"issuer":                           tokenServer.URL,
				"authorization_endpoint":           tokenServer.URL + "/authorize",
				"token_endpoint":                   tokenServer.URL + "/token",
				"revocation_endpoint":              tokenServer.URL + "/revoke",
				"code_challenge_methods_supported": []string{"S256"},
				"grant_types_supported":            []string{"authorization_code", "refresh_token"},
				"response_types_supported":         []string{"code"},
			})
		case "/token":
			if err := request.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			verifier := request.Form.Get("code_verifier")
			digest := sha256.Sum256([]byte(verifier))
			if got := base64.RawURLEncoding.EncodeToString(digest[:]); verifier == "" || got != challenge {
				t.Errorf("PKCE verifier does not match challenge: %q", got)
			}
			writeJSON(t, writer, map[string]any{
				"access_token": "access-secret", "refresh_token": "refresh-secret",
				"token_type": "Bearer", "expires_in": 3600,
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer tokenServer.Close()

	userID := uuid.NewString()
	ref := ServerRef{Source: SourceManifest, ID: "oauth-server"}
	server := Server{
		Ref: ref, Name: "OAuth", Transport: TransportStreamableHTTP,
		EndpointURL: tokenServer.URL + "/mcp", AuthType: AuthOAuth,
		OAuthClient: &OAuthClient{ClientID: "neo-chat", Scopes: []string{"tools.read"}},
		Status:      ServerStatusNeedsAuth, Grants: []Grant{{ScopeType: "global"}},
	}
	repo := newFakeRepository()
	config := testMCPAdminConfig(userID)
	config.OAuthCallbackURL = "https://chat.example/v1/mcp/oauth/callback"
	service, err := NewService(config, repo, &fakeConnector{}, testVault(t), nil, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.StartOAuth(context.Background(), userID, "", ref, "/settings/tools")
	if err != nil {
		t.Fatalf("StartOAuth() error = %v", err)
	}
	authorizationURL, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	stateToken := authorizationURL.Query().Get("state")
	challenge = authorizationURL.Query().Get("code_challenge")
	if stateToken == "" || challenge == "" || authorizationURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL = %s", started.AuthorizationURL)
	}
	if len(repo.oauthStates) != 1 {
		t.Fatalf("oauth states = %d", len(repo.oauthStates))
	}
	for hash, state := range repo.oauthStates {
		if hash == stateToken || strings.Contains(state.EncryptedFlowRef, "verifier") {
			t.Fatalf("state was not hashed/encrypted: hash=%q flow=%q", hash, state.EncryptedFlowRef)
		}
	}

	completed, err := service.CompleteOAuth(context.Background(), stateToken, "authorization-code")
	if err != nil {
		t.Fatalf("CompleteOAuth() error = %v", err)
	}
	if completed.ReturnURL != "/settings/tools" || completed.ServerRef != ref {
		t.Fatalf("callback = %#v", completed)
	}
	credential, found, err := repo.GetCredential(context.Background(), userID, ref)
	if err != nil || !found {
		t.Fatalf("credential found=%v err=%v", found, err)
	}
	if strings.Contains(credential.EncryptedSecretRef, "access-secret") || strings.Contains(credential.EncryptedSecretRef, "refresh-secret") {
		t.Fatal("stored OAuth credential is plaintext")
	}
	if _, err := service.CompleteOAuth(context.Background(), stateToken, "authorization-code"); !errors.Is(err, ErrOAuthStateConsumed) {
		t.Fatalf("state reuse error = %v", err)
	}
}

func TestOAuthRefreshUsesSingleflight(t *testing.T) {
	t.Parallel()
	var refreshCalls atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		refreshCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh form = %#v", request.Form)
		}
		writeJSON(t, writer, map[string]any{
			"access_token": "fresh-access", "token_type": "Bearer", "expires_in": 3600,
		})
	}))
	defer tokenServer.Close()

	userID := uuid.NewString()
	ref := ServerRef{Source: SourceManifest, ID: "oauth-refresh"}
	server := Server{
		Ref: ref, Transport: TransportStreamableHTTP, EndpointURL: tokenServer.URL + "/mcp",
		AuthType: AuthOAuth, OAuthClient: &OAuthClient{ClientID: "client"},
		Status: ServerStatusReady, Grants: []Grant{{ScopeType: "global"}},
	}
	repo := newFakeRepository()
	service, err := NewService(testMCPConfig(), repo, &fakeConnector{}, testVault(t), nil, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Minute)
	err = service.storeCredential(context.Background(), userID, ref, AuthOAuth, storedCredential{
		AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: expired,
		TokenEndpoint: tokenServer.URL, ClientID: "client",
	}, &expired)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 12
	var wait sync.WaitGroup
	wait.Add(goroutines)
	errorsSeen := make(chan error, goroutines)
	for range goroutines {
		go func() {
			defer wait.Done()
			token, err := service.connectionCredential(context.Background(), userID, server)
			if err != nil {
				errorsSeen <- err
				return
			}
			if token != "fresh-access" {
				errorsSeen <- errors.New("unexpected access token")
			}
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

func TestOAuthDynamicClientRegistrationAndLoopbackCallback(t *testing.T) {
	t.Parallel()
	var registrationCalls atomic.Int32
	var tokenServer *httptest.Server
	tokenServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			writeJSON(t, writer, map[string]any{
				"resource": tokenServer.URL + "/mcp", "authorization_servers": []string{tokenServer.URL},
			})
		case "/.well-known/oauth-authorization-server":
			writeJSON(t, writer, map[string]any{
				"issuer": tokenServer.URL, "authorization_endpoint": tokenServer.URL + "/authorize",
				"token_endpoint": tokenServer.URL + "/token", "registration_endpoint": tokenServer.URL + "/register",
				"code_challenge_methods_supported": []string{"S256"},
				"grant_types_supported":            []string{"authorization_code"}, "response_types_supported": []string{"code"},
			})
		case "/register":
			registrationCalls.Add(1)
			var registration map[string]any
			if json.NewDecoder(request.Body).Decode(&registration) != nil {
				t.Error("invalid registration request")
			}
			redirects, _ := registration["redirect_uris"].([]any)
			if len(redirects) != 1 || redirects[0] != "http://localhost:18080/mm-api/v1/mcp/oauth/callback" {
				t.Errorf("redirect_uris=%#v", registration["redirect_uris"])
			}
			writeJSON(t, writer, map[string]any{"client_id": "dynamic-client"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer tokenServer.Close()

	userID := uuid.NewString()
	ref := ServerRef{Source: SourceManifest, ID: "oauth-dcr"}
	repo := newFakeRepository()
	server := Server{
		Ref: ref, Name: "OAuth DCR", Transport: TransportStreamableHTTP,
		EndpointURL: tokenServer.URL + "/mcp", AuthType: AuthOAuth,
		OAuthClient: &OAuthClient{}, Status: ServerStatusNeedsAuth,
		Metadata: map[string]any{"oauthDynamicRegistration": true}, Grants: []Grant{{ScopeType: "global"}},
	}
	config := testMCPAdminConfig(userID)
	config.OAuthCallbackURL = "http://localhost:18080/mm-api/v1/mcp/oauth/callback"
	service, err := NewService(config, repo, &fakeConnector{}, testVault(t), nil, Catalog{}, []Server{server})
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.StartOAuth(context.Background(), userID, "", ref, "/settings/tools")
	if err != nil {
		t.Fatalf("StartOAuth() error=%v", err)
	}
	authorizationURL, _ := url.Parse(started.AuthorizationURL)
	if registrationCalls.Load() != 1 || authorizationURL.Query().Get("client_id") != "dynamic-client" {
		t.Fatalf("registration calls=%d authorization=%s", registrationCalls.Load(), started.AuthorizationURL)
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode JSON: %v", err)
	}
}
