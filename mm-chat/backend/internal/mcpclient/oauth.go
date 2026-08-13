package mcpclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"neo-chat/mm-chat/backend/internal/providersecrets"
)

const (
	oauthStateLifetime         = 10 * time.Minute
	oauthHTTPTimeout           = 20 * time.Second
	maxOAuthMetadataBytes      = 64 << 10
	maxOAuthRegistrationBytes  = 64 << 10
	maxOAuthTokenBytes         = 64 << 10
	maxOAuthCodeBytes          = 8192
	maxOAuthReturnURLBytes     = 2048
	oauthExpirySafetyWindow    = 30 * time.Second
	protectedResourceWellKnown = "/.well-known/oauth-protected-resource"
)

type OAuthStartResult struct {
	AuthorizationURL string    `json:"authorizationUrl"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type OAuthCallbackResult struct {
	ReturnURL string
	ServerRef ServerRef
}

type oauthFlow struct {
	ServerRef          ServerRef `json:"serverRef"`
	ConversationID     string    `json:"conversationId,omitempty"`
	Verifier           string    `json:"verifier"`
	RedirectURI        string    `json:"redirectUri"`
	TokenEndpoint      string    `json:"tokenEndpoint"`
	RevocationEndpoint string    `json:"revocationEndpoint,omitempty"`
	ClientID           string    `json:"clientId"`
	ClientSecret       string    `json:"clientSecret,omitempty"`
	Scope              string    `json:"scope,omitempty"`
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type authorizationServerMetadata struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	RevocationEndpoint            string   `json:"revocation_endpoint"`
	RegistrationEndpoint          string   `json:"registration_endpoint"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
}

type oauthClientRegistrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Error        string `json:"error"`
}

type oauthTokenResponse struct {
	AccessToken  string          `json:"access_token"`
	TokenType    string          `json:"token_type"`
	RefreshToken string          `json:"refresh_token"`
	Scope        string          `json:"scope"`
	ExpiresIn    json.RawMessage `json:"expires_in"`
	Error        string          `json:"error"`
}

func (s *Service) StartOAuth(
	ctx context.Context,
	userID string,
	conversationID string,
	ref ServerRef,
	returnURL string,
) (OAuthStartResult, error) {
	if err := s.requireAdministrator(userID); err != nil {
		return OAuthStartResult{}, err
	}
	if err := s.remoteAvailable(); err != nil {
		return OAuthStartResult{}, err
	}
	if !validRelativeReturnURL(returnURL) {
		return OAuthStartResult{}, ErrOAuthStateInvalid
	}
	callback, err := parseOAuthCallback(s.config.OAuthCallbackURL)
	if err != nil {
		return OAuthStartResult{}, ErrOAuthStateInvalid
	}
	conversationScope := ConversationScope{}
	if conversationID != "" {
		conversationScope, err = s.repo.ConversationScope(ctx, userID, conversationID)
		if err != nil {
			return OAuthStartResult{}, err
		}
	}
	server, err := s.serverForUser(ctx, userID, ref, conversationScope)
	if err != nil {
		return OAuthStartResult{}, err
	}
	if server.Transport != TransportStreamableHTTP || server.AuthType != AuthOAuth || server.OAuthClient == nil {
		return OAuthStartResult{}, ErrCredentialInvalid
	}
	now := s.now().UTC()
	pending, err := s.repo.CountPendingOAuthStates(ctx, userID, now)
	if err != nil {
		return OAuthStartResult{}, err
	}
	if pending >= s.config.MaxOAuthFlows {
		return OAuthStartResult{}, ErrServerLimit
	}
	metadata, err := s.discoverOAuthMetadata(ctx, server)
	if err != nil {
		return OAuthStartResult{}, err
	}
	clientID := strings.TrimSpace(server.OAuthClient.ClientID)
	clientSecret := server.OAuthClient.ClientSecret
	if clientID == "" {
		if metadata.RegistrationEndpoint == "" || !boolField(server.Metadata, "oauthDynamicRegistration") {
			return OAuthStartResult{}, ErrCredentialInvalid
		}
		clientID, clientSecret, err = s.registerOAuthClient(ctx, server, metadata.RegistrationEndpoint, callback.String())
		if err != nil {
			return OAuthStartResult{}, err
		}
	}
	verifier, err := randomOAuthToken(48)
	if err != nil {
		return OAuthStartResult{}, ErrOAuthStateInvalid
	}
	stateToken, err := randomOAuthToken(32)
	if err != nil {
		return OAuthStartResult{}, ErrOAuthStateInvalid
	}
	stateID := uuid.NewString()
	oauthScope := strings.Join(server.OAuthClient.Scopes, " ")
	flow := oauthFlow{
		ServerRef: ref, ConversationID: conversationID,
		Verifier: verifier, RedirectURI: callback.String(),
		TokenEndpoint:      metadata.TokenEndpoint,
		RevocationEndpoint: metadata.RevocationEndpoint,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		Scope:              oauthScope,
	}
	encrypted, err := s.encryptOAuthFlow(flow, stateID)
	if err != nil {
		return OAuthStartResult{}, err
	}
	digest := sha256.Sum256([]byte(stateToken))
	expiresAt := now.Add(oauthStateLifetime)
	if err := s.repo.CreateOAuthState(ctx, OAuthState{
		ID: stateID, StateHash: hex.EncodeToString(digest[:]), UserID: userID,
		ServerRef: ref, EncryptedFlowRef: encrypted,
		ReturnURL: returnURL, ExpiresAt: expiresAt,
	}); err != nil {
		return OAuthStartResult{}, err
	}
	authorizationURL, err := url.Parse(metadata.AuthorizationEndpoint)
	if err != nil {
		return OAuthStartResult{}, ErrServerUnavailable
	}
	query := authorizationURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", clientID)
	query.Set("redirect_uri", callback.String())
	query.Set("state", stateToken)
	query.Set("code_challenge_method", "S256")
	challenge := sha256.Sum256([]byte(verifier))
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	if oauthScope != "" {
		query.Set("scope", oauthScope)
	}
	// RFC 8707 binds the access token to the selected MCP resource.
	query.Set("resource", server.EndpointURL)
	authorizationURL.RawQuery = query.Encode()
	return OAuthStartResult{AuthorizationURL: authorizationURL.String(), ExpiresAt: expiresAt}, nil
}

func (s *Service) CompleteOAuth(
	ctx context.Context,
	stateToken string,
	code string,
) (OAuthCallbackResult, error) {
	if err := s.remoteAvailable(); err != nil {
		return OAuthCallbackResult{}, err
	}
	stateToken = strings.TrimSpace(stateToken)
	code = strings.TrimSpace(code)
	if stateToken == "" || len(stateToken) > maxOAuthCodeBytes || code == "" || len(code) > maxOAuthCodeBytes {
		return OAuthCallbackResult{}, ErrOAuthStateInvalid
	}
	digest := sha256.Sum256([]byte(stateToken))
	state, err := s.repo.ConsumeOAuthState(ctx, hex.EncodeToString(digest[:]), s.now().UTC())
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	flow, err := s.decryptOAuthFlow(state.EncryptedFlowRef, state.ID)
	if err != nil || flow.ServerRef != state.ServerRef {
		return OAuthCallbackResult{}, ErrOAuthStateInvalid
	}
	scope := ConversationScope{}
	if flow.ConversationID != "" {
		scope, err = s.repo.ConversationScope(ctx, state.UserID, flow.ConversationID)
		if err != nil {
			return OAuthCallbackResult{}, ErrOAuthStateInvalid
		}
	}
	server, err := s.serverForUser(ctx, state.UserID, state.ServerRef, scope)
	if err != nil || server.AuthType != AuthOAuth {
		return OAuthCallbackResult{}, ErrOAuthStateInvalid
	}
	token, err := s.exchangeOAuthToken(ctx, server, flow, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {flow.RedirectURI},
		"code_verifier": {flow.Verifier},
		"client_id":     {flow.ClientID},
		"resource":      {server.EndpointURL},
	})
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	if err := s.persistOAuthToken(ctx, state.UserID, state.ServerRef, flow, token, storedCredential{}); err != nil {
		return OAuthCallbackResult{}, err
	}
	if state.ServerRef.Source == SourcePrivate {
		if _, err := s.ValidatePrivateServer(ctx, state.UserID, state.ServerRef.ID); err != nil {
			return OAuthCallbackResult{}, err
		}
		if flow.ConversationID != "" {
			selection, err := s.GetSelection(ctx, state.UserID, flow.ConversationID)
			if err != nil {
				return OAuthCallbackResult{}, err
			}
			selection.Mode = SelectionModeCustom
			selection.Servers = appendMarketplaceSelection(selection.Servers, state.ServerRef)
			if _, err := s.ReplaceSelection(ctx, state.UserID, selection); err != nil {
				return OAuthCallbackResult{}, err
			}
		}
	}
	return OAuthCallbackResult{ReturnURL: state.ReturnURL, ServerRef: state.ServerRef}, nil
}

func (s *Service) RevokeOAuth(ctx context.Context, userID string, ref ServerRef) error {
	if err := s.requireAdministrator(userID); err != nil {
		return err
	}
	if err := s.available(); err != nil {
		return err
	}
	stored, loadErr := s.loadCredential(ctx, userID, ref)
	if loadErr != nil && !errors.Is(loadErr, ErrCredentialRequired) {
		return loadErr
	}
	if err := s.repo.DeleteCredential(ctx, userID, ref); err != nil {
		return err
	}
	if loadErr != nil || stored.RevocationEndpoint == "" || stored.AccessToken == "" {
		return nil
	}
	server, err := s.serverForUser(ctx, userID, ref, ConversationScope{})
	if err != nil {
		return nil
	}
	values := url.Values{"token": {stored.AccessToken}, "client_id": {stored.ClientID}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, stored.RevocationEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return ErrServerUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if stored.ClientSecret != "" {
		request.SetBasicAuth(stored.ClientID, stored.ClientSecret)
	}
	client, err := s.oauthHTTPClient(ctx, server, stored.RevocationEndpoint)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return ErrServerUnavailable
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrServerUnavailable
	}
	return nil
}

func (s *Service) refreshOAuthAccessToken(
	ctx context.Context,
	userID string,
	server Server,
) (string, error) {
	key := userID + ":" + server.Ref.Key()
	value, err, _ := s.refresh.Do(key, func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oauthHTTPTimeout)
		defer cancel()
		stored, err := s.loadCredential(refreshCtx, userID, server.Ref)
		if err != nil {
			return "", err
		}
		if stored.AccessToken != "" &&
			(stored.ExpiresAt.IsZero() || s.now().Add(oauthExpirySafetyWindow).Before(stored.ExpiresAt)) {
			return stored.AccessToken, nil
		}
		if stored.RefreshToken == "" || stored.TokenEndpoint == "" || stored.ClientID == "" {
			return "", ErrCredentialRequired
		}
		flow := oauthFlow{
			ServerRef: server.Ref, TokenEndpoint: stored.TokenEndpoint,
			RevocationEndpoint: stored.RevocationEndpoint,
			ClientID:           stored.ClientID, ClientSecret: stored.ClientSecret,
			Scope: stored.Scope,
		}
		values := url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {stored.RefreshToken},
			"client_id":     {stored.ClientID},
			"resource":      {server.EndpointURL},
		}
		token, exchangeErr := s.exchangeOAuthToken(refreshCtx, server, flow, values)
		if errors.Is(exchangeErr, ErrCredentialRequired) {
			_ = s.repo.DeleteCredential(refreshCtx, userID, server.Ref)
			return "", exchangeErr
		}
		if exchangeErr != nil {
			return "", exchangeErr
		}
		if err := s.persistOAuthToken(refreshCtx, userID, server.Ref, flow, token, stored); err != nil {
			return "", err
		}
		return token.AccessToken, nil
	})
	if err != nil {
		return "", err
	}
	token, ok := value.(string)
	if !ok || token == "" {
		return "", ErrCredentialRequired
	}
	return token, nil
}

func (s *Service) discoverOAuthMetadata(
	ctx context.Context,
	server Server,
) (authorizationServerMetadata, error) {
	endpoint, err := parseEndpoint(server.EndpointURL, server.Ref.Source != SourceManifest)
	if err != nil {
		return authorizationServerMetadata{}, ErrURLBlocked
	}
	resourceMetadataURL := *endpoint
	resourceMetadataURL.RawQuery = ""
	resourceMetadataURL.Path = protectedResourceWellKnown + strings.TrimSuffix(endpoint.Path, "/")
	resourceMetadataURL.RawPath = ""
	var resource protectedResourceMetadata
	if err := s.fetchOAuthJSON(ctx, server, resourceMetadataURL.String(), &resource); err != nil {
		// Compatibility fallback for servers that publish only the origin-level
		// RFC 9728 document.
		resourceMetadataURL.Path = protectedResourceWellKnown
		if err := s.fetchOAuthJSON(ctx, server, resourceMetadataURL.String(), &resource); err != nil {
			return authorizationServerMetadata{}, err
		}
	}
	if len(resource.AuthorizationServers) != 1 {
		return authorizationServerMetadata{}, ErrServerUnavailable
	}
	authorizationServer, err := ValidateEndpoint(ctx, resource.AuthorizationServers[0], networkPolicyFor(server))
	if err != nil {
		return authorizationServerMetadata{}, ErrURLBlocked
	}
	metadataURLs := oauthAuthorizationMetadataURLs(authorizationServer)
	var metadata authorizationServerMetadata
	var fetchErr error
	for _, candidate := range metadataURLs {
		metadata = authorizationServerMetadata{}
		if fetchErr = s.fetchOAuthJSON(ctx, server, candidate, &metadata); fetchErr == nil {
			break
		}
	}
	if fetchErr != nil {
		return authorizationServerMetadata{}, fetchErr
	}
	if metadata.Issuer != "" && strings.TrimRight(metadata.Issuer, "/") != strings.TrimRight(authorizationServer.String(), "/") {
		return authorizationServerMetadata{}, ErrServerUnavailable
	}
	if !containsString(metadata.CodeChallengeMethodsSupported, "S256") ||
		(metadata.GrantTypesSupported != nil && !containsString(metadata.GrantTypesSupported, "authorization_code")) ||
		(metadata.ResponseTypesSupported != nil && !containsString(metadata.ResponseTypesSupported, "code")) {
		return authorizationServerMetadata{}, ErrServerUnavailable
	}
	if _, err := ValidateEndpoint(ctx, metadata.AuthorizationEndpoint, networkPolicyFor(server)); err != nil {
		return authorizationServerMetadata{}, ErrURLBlocked
	}
	if _, err := ValidateEndpoint(ctx, metadata.TokenEndpoint, networkPolicyFor(server)); err != nil {
		return authorizationServerMetadata{}, ErrURLBlocked
	}
	if metadata.RevocationEndpoint != "" {
		if _, err := ValidateEndpoint(ctx, metadata.RevocationEndpoint, networkPolicyFor(server)); err != nil {
			return authorizationServerMetadata{}, ErrURLBlocked
		}
	}
	if metadata.RegistrationEndpoint != "" {
		if _, err := ValidateEndpoint(ctx, metadata.RegistrationEndpoint, networkPolicyFor(server)); err != nil {
			return authorizationServerMetadata{}, ErrURLBlocked
		}
	}
	return metadata, nil
}

func (s *Service) registerOAuthClient(
	ctx context.Context,
	server Server,
	endpoint string,
	callback string,
) (string, string, error) {
	client, err := s.oauthHTTPClient(ctx, server, endpoint)
	if err != nil {
		return "", "", err
	}
	payload, err := json.Marshal(map[string]any{
		"client_name":                "Neo Chat",
		"redirect_uris":              []string{callback},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
	if err != nil {
		return "", "", ErrCredentialInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", "", ErrServerUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", "", ErrServerUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthRegistrationBytes+1))
	if err != nil || len(data) > maxOAuthRegistrationBytes {
		return "", "", ErrServerUnavailable
	}
	var registered oauthClientRegistrationResponse
	if json.Unmarshal(data, &registered) != nil || response.StatusCode < 200 || response.StatusCode >= 300 ||
		registered.Error != "" {
		return "", "", ErrServerUnavailable
	}
	registered.ClientID = strings.TrimSpace(registered.ClientID)
	registered.ClientSecret = strings.TrimSpace(registered.ClientSecret)
	if registered.ClientID == "" || len(registered.ClientID) > 2048 ||
		len(registered.ClientSecret) > maxCredentialBytes || strings.ContainsAny(registered.ClientID, "\x00\r\n") ||
		strings.ContainsAny(registered.ClientSecret, "\x00\r\n") {
		return "", "", ErrServerUnavailable
	}
	return registered.ClientID, registered.ClientSecret, nil
}

func parseOAuthCallback(raw string) (*url.URL, error) {
	callback, err := parseEndpoint(raw, false)
	if err != nil {
		return nil, err
	}
	if callback.Scheme == "https" {
		return callback, nil
	}
	host := strings.ToLower(callback.Hostname())
	if callback.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return callback, nil
	}
	return nil, ErrURLBlocked
}

func boolField(object map[string]any, name string) bool {
	value, _ := object[name].(bool)
	return value
}

func (s *Service) fetchOAuthJSON(ctx context.Context, server Server, endpoint string, target any) error {
	client, err := s.oauthHTTPClient(ctx, server, endpoint)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ErrServerUnavailable
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ErrServerUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return ErrServerUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthMetadataBytes+1))
	if err != nil || len(data) > maxOAuthMetadataBytes {
		return ErrServerUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return ErrServerUnavailable
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrServerUnavailable
	}
	return nil
}

func (s *Service) exchangeOAuthToken(
	ctx context.Context,
	server Server,
	flow oauthFlow,
	values url.Values,
) (oauthTokenResponse, error) {
	client, err := s.oauthHTTPClient(ctx, server, flow.TokenEndpoint)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, flow.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if flow.ClientSecret != "" {
		request.SetBasicAuth(flow.ClientID, flow.ClientSecret)
	}
	response, err := client.Do(request)
	if err != nil {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOAuthTokenBytes+1))
	if err != nil || len(data) > maxOAuthTokenBytes {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	var token oauthTokenResponse
	if err := json.Unmarshal(data, &token); err != nil {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	if token.Error == "invalid_grant" {
		return oauthTokenResponse{}, ErrCredentialRequired
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || token.Error != "" ||
		strings.TrimSpace(token.AccessToken) == "" || len(token.AccessToken) > maxCredentialBytes {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	token.AccessToken = strings.TrimSpace(token.AccessToken)
	token.RefreshToken = strings.TrimSpace(token.RefreshToken)
	if len(token.RefreshToken) > maxCredentialBytes || len(token.Scope) > 4096 || len(token.TokenType) > 64 {
		return oauthTokenResponse{}, ErrServerUnavailable
	}
	return token, nil
}

func (s *Service) persistOAuthToken(
	ctx context.Context,
	userID string,
	ref ServerRef,
	flow oauthFlow,
	token oauthTokenResponse,
	previous storedCredential,
) error {
	expiresAt, err := oauthTokenExpiry(s.now().UTC(), token.ExpiresIn)
	if err != nil {
		return ErrServerUnavailable
	}
	refreshToken := token.RefreshToken
	if refreshToken == "" {
		refreshToken = previous.RefreshToken
	}
	scope := strings.TrimSpace(token.Scope)
	if scope == "" {
		scope = flow.Scope
	}
	payload := storedCredential{
		AccessToken: token.AccessToken, RefreshToken: refreshToken,
		TokenType: strings.TrimSpace(token.TokenType), Scope: scope,
		ExpiresAt: expiresAt, TokenEndpoint: flow.TokenEndpoint,
		RevocationEndpoint: flow.RevocationEndpoint,
		ClientID:           flow.ClientID, ClientSecret: flow.ClientSecret,
	}
	var expiry *time.Time
	if !expiresAt.IsZero() {
		expiry = &expiresAt
	}
	return s.storeCredential(ctx, userID, ref, AuthOAuth, payload, expiry)
}

func (s *Service) oauthHTTPClient(ctx context.Context, server Server, endpoint string) (*http.Client, error) {
	validated, err := ValidateEndpoint(ctx, endpoint, networkPolicyFor(server))
	if err != nil {
		return nil, ErrURLBlocked
	}
	return NewSafeHTTPClient(networkPolicyFor(server), validated, nil, oauthHTTPTimeout), nil
}

func networkPolicyFor(server Server) NetworkPolicy {
	return NetworkPolicy{
		RequireHTTPS: server.Ref.Source != SourceManifest,
		AllowPrivate: server.Ref.Source == SourceManifest,
	}
}

func oauthAuthorizationMetadataURLs(issuer *url.URL) []string {
	base := *issuer
	base.RawQuery = ""
	base.Fragment = ""
	issuerPath := strings.TrimSuffix(base.EscapedPath(), "/")
	base.RawPath = ""
	base.Path = "/.well-known/oauth-authorization-server" + issuerPath
	first := base.String()
	base.Path = strings.TrimSuffix(issuerPath, "/") + "/.well-known/openid-configuration"
	second := base.String()
	if first == second {
		return []string{first}
	}
	return []string{first, second}
}

func oauthTokenExpiry(now time.Time, raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return time.Time{}, nil
	}
	value := strings.Trim(string(raw), "\"")
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 || seconds > int64((365*24*time.Hour)/time.Second) {
		return time.Time{}, ErrServerUnavailable
	}
	return now.Add(time.Duration(seconds) * time.Second), nil
}

func (s *Service) encryptOAuthFlow(flow oauthFlow, stateID string) (string, error) {
	if s.vault == nil {
		return "", ErrCredentialInvalid
	}
	plaintext, err := json.Marshal(flow)
	if err != nil {
		return "", ErrOAuthStateInvalid
	}
	defer clear(plaintext)
	envelope, err := s.vault.Encrypt(plaintext, oauthFlowContext(stateID))
	if err != nil {
		return "", ErrOAuthStateInvalid
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", ErrOAuthStateInvalid
	}
	return string(encoded), nil
}

func (s *Service) decryptOAuthFlow(encoded string, stateID string) (oauthFlow, error) {
	if s.vault == nil {
		return oauthFlow{}, ErrOAuthStateInvalid
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil {
		return oauthFlow{}, ErrOAuthStateInvalid
	}
	plaintext, err := s.vault.Decrypt(envelope, oauthFlowContext(stateID))
	if err != nil {
		return oauthFlow{}, ErrOAuthStateInvalid
	}
	defer clear(plaintext)
	var flow oauthFlow
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&flow); err != nil {
		return oauthFlow{}, ErrOAuthStateInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return oauthFlow{}, ErrOAuthStateInvalid
	}
	return flow, nil
}

func oauthFlowContext(stateID string) string {
	return "mcp:oauth-flow:" + stateID
}

func randomOAuthToken(bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func validRelativeReturnURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxOAuthReturnURLBytes || !strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\\\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && !parsed.IsAbs() && parsed.Host == ""
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
