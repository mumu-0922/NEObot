package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const marketplaceProviderLobeHub = "lobehub"

var npmPackageNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var exactNPMVersionPattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type Marketplace interface {
	Search(context.Context, MarketplaceSearchInput) (MarketplaceSearchResult, error)
	GetItem(context.Context, string, string) (MarketplaceItemDetail, error)
}

func (s *Service) SearchMarketplace(
	ctx context.Context,
	input MarketplaceSearchInput,
) (MarketplaceSearchResult, error) {
	if err := s.marketplaceAvailable(); err != nil {
		return MarketplaceSearchResult{}, err
	}
	input.Query = strings.TrimSpace(input.Query)
	input.Category = strings.TrimSpace(input.Category)
	if len(input.Query) > 200 || input.Page < 1 || input.Page > 1000 ||
		input.PageSize < 1 || input.PageSize > 40 ||
		(input.Category != "" && !validMarketplaceCategory(input.Category)) {
		return MarketplaceSearchResult{}, ErrSelectionInvalid
	}
	return s.marketplace.Search(ctx, input)
}

func validMarketplaceCategory(value string) bool {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (s *Service) MarketplaceItem(
	ctx context.Context,
	userID string,
	identifier string,
	version string,
) (MarketplaceItemDetail, error) {
	if err := s.marketplaceAvailable(); err != nil {
		return MarketplaceItemDetail{}, err
	}
	identifier = strings.TrimSpace(identifier)
	version = strings.TrimSpace(version)
	if !validMarketplaceIdentifier(identifier) || (version != "" && !validMarketplaceVersion(version)) {
		return MarketplaceItemDetail{}, ErrSelectionInvalid
	}
	detail, err := s.marketplace.GetItem(ctx, identifier, version)
	if err != nil {
		return MarketplaceItemDetail{}, err
	}
	if detail.Identifier != identifier || (version != "" && detail.Version != version) {
		return MarketplaceItemDetail{}, ErrMarketplaceChanged
	}
	detail.Icon = boundedMarketplaceIcon(detail.Icon)
	detail = s.approveMarketplaceDetail(ctx, detail)
	detail.CanInstall = s.IsAdministrator(userID)
	installedOwner := s.config.AdministratorUserID
	if !validUUID(installedOwner) {
		detail.Installed = false
		return detail, nil
	}
	servers, err := s.repo.ListPrivateServers(ctx, installedOwner)
	if err != nil {
		return MarketplaceItemDetail{}, err
	}
	detail.Installed = marketplaceItemInstalled(servers, detail)
	return detail, nil
}

func marketplaceItemInstalled(servers []Server, detail MarketplaceItemDetail) bool {
	for _, server := range servers {
		provenance, _ := server.Metadata["marketplace"].(map[string]any)
		if server.Ref.Source == SourcePrivate &&
			stringField(provenance, "provider") == marketplaceProviderLobeHub &&
			stringField(provenance, "identifier") == detail.Identifier &&
			stringField(provenance, "version") == detail.Version {
			return true
		}
	}
	return false
}

func (s *Service) InstallMarketplaceItem(
	ctx context.Context,
	userID string,
	input MarketplaceInstallInput,
) (MarketplaceInstallResult, error) {
	if err := s.requireAdministrator(userID); err != nil {
		return MarketplaceInstallResult{}, err
	}
	if err := s.marketplaceAvailable(); err != nil {
		return MarketplaceInstallResult{}, err
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	input.Version = strings.TrimSpace(input.Version)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	if !validMarketplaceIdentifier(input.Identifier) || !validMarketplaceVersion(input.Version) {
		return MarketplaceInstallResult{}, ErrSelectionInvalid
	}
	input.DeploymentHash = strings.TrimSpace(input.DeploymentHash)
	if input.DeploymentHash != "" && (len(input.DeploymentHash) != 64 || strings.Trim(input.DeploymentHash, "0123456789abcdef") != "") {
		return MarketplaceInstallResult{}, ErrSelectionInvalid
	}
	if len(input.Secrets) > 8 {
		return MarketplaceInstallResult{}, ErrCredentialInvalid
	}
	input.CustomEndpointURL = strings.TrimSpace(input.CustomEndpointURL)
	input.CustomAuthType = strings.TrimSpace(input.CustomAuthType)
	input.CustomHeaderName = strings.TrimSpace(input.CustomHeaderName)
	input.CustomHeaderPrefix = strings.TrimSpace(input.CustomHeaderPrefix)
	input.CustomClientID = strings.TrimSpace(input.CustomClientID)
	if input.CustomEndpointURL == "" && (input.CustomAuthType != "" || input.CustomHeaderName != "" ||
		input.CustomHeaderPrefix != "" || input.CustomCredential != "" || input.CustomClientID != "") {
		return MarketplaceInstallResult{}, ErrSelectionInvalid
	}
	if input.CustomEndpointURL != "" && (input.DeploymentHash != "" || len(input.Secrets) != 0) {
		return MarketplaceInstallResult{}, ErrSelectionInvalid
	}

	var selection Selection
	if input.EnableForConversation {
		if !validUUID(input.ConversationID) || input.SelectionRevision < 0 {
			return MarketplaceInstallResult{}, ErrSelectionInvalid
		}
		current, err := s.GetSelection(ctx, userID, input.ConversationID)
		if err != nil {
			return MarketplaceInstallResult{}, err
		}
		if current.Revision != input.SelectionRevision {
			return MarketplaceInstallResult{}, ErrSelectionInvalid
		}
		selection = current
	}

	detail, err := s.marketplace.GetItem(ctx, input.Identifier, input.Version)
	if err != nil {
		return MarketplaceInstallResult{}, err
	}
	if detail.Identifier != input.Identifier || detail.Version != input.Version {
		return MarketplaceInstallResult{}, ErrMarketplaceChanged
	}
	detail.Icon = boundedMarketplaceIcon(detail.Icon)
	detail = s.approveMarketplaceDetail(ctx, detail)
	if input.CustomEndpointURL != "" {
		return s.installMarketplaceCustomRemote(ctx, userID, detail, input, selection)
	}
	target, found := s.installableMarketplaceDeployment(detail, input.DeploymentHash)
	if !found || target.deployment.Hash == "" {
		return MarketplaceInstallResult{}, ErrMarketplaceIncompatible
	}
	deployment := target.deployment
	expectedHash, err := marketplaceDeploymentHash(detail.Identifier, detail.Version, deployment)
	if err != nil || deployment.Hash != expectedHash {
		return MarketplaceInstallResult{}, ErrMarketplaceChanged
	}
	if len(deployment.SecretFields) == 0 && len(input.Secrets) != 0 {
		return MarketplaceInstallResult{}, ErrCredentialInvalid
	}
	if deployment.InstallMode == "header" && len(deployment.SecretFields) != 1 {
		return MarketplaceInstallResult{}, ErrMarketplaceIncompatible
	}

	metadata := map[string]any{
		"marketplace": map[string]any{
			"provider":       marketplaceProviderLobeHub,
			"identifier":     detail.Identifier,
			"version":        detail.Version,
			"deploymentHash": deployment.Hash,
		},
	}
	if detail.Icon != "" {
		metadata["icon"] = detail.Icon
	}
	server, found, err := s.installedMarketplaceServer(ctx, userID, detail, deployment, target.artifact)
	if err != nil {
		return MarketplaceInstallResult{}, err
	}
	if !found {
		server, err = s.createMarketplaceServer(ctx, userID, detail, deployment, target.artifact, metadata)
		if err == ErrServerConflict {
			// A concurrent install may have won the endpoint uniqueness race. Only
			// recover the exact same server-authoritative Marketplace deployment;
			// an unrelated private Server at that endpoint remains a conflict.
			server, found, err = s.installedMarketplaceServer(ctx, userID, detail, deployment, target.artifact)
			if err == nil && !found {
				err = ErrServerConflict
			}
		}
	}
	if err != nil {
		return MarketplaceInstallResult{}, err
	}
	if len(deployment.SecretFields) > 0 {
		values, secretErr := marketplaceSecretValues(deployment.SecretFields, input.Secrets)
		if secretErr != nil {
			return s.marketplaceCredentialDraft(ctx, userID, server, "credential_required")
		}
		if server.AuthType == AuthHeader {
			if secretErr = s.storeCredential(ctx, userID, server.Ref, AuthHeader,
				storedCredential{HeaderValue: values[deployment.SecretFields[0]]}, nil); secretErr != nil {
				return s.marketplaceCredentialDraft(ctx, userID, server, "credential_store_failed")
			}
		} else if server.AuthType == AuthEnv {
			if secretErr = s.storeCredential(ctx, userID, server.Ref, AuthEnv,
				storedCredential{Environment: values}, nil); secretErr != nil {
				return s.marketplaceCredentialDraft(ctx, userID, server, "credential_store_failed")
			}
		}
		server.HasCredential = true
	}
	result := MarketplaceInstallResult{Server: server}
	if server.AuthType == AuthOAuth && !server.HasCredential {
		server.Status = ServerStatusNeedsAuth
		server.LastErrorCode = "credential_required"
		validated, validationErr := s.recordValidationFailure(
			ctx, userID, server, ServerStatusNeedsAuth, "credential_required", ErrCredentialRequired,
		)
		if validated.Ref.ID != "" {
			result.Server = validated
		}
		if validationErr != nil && validated.Ref.ID == "" {
			return result, validationErr
		}
		result.ValidationError = "credential_required"
		return result, nil
	}
	validated, validationErr := s.ValidatePrivateServer(ctx, userID, server.Ref.ID)
	if validated.Ref.ID != "" {
		result.Server = validated
	}
	if validationErr != nil {
		if validated.Ref.ID == "" {
			return result, validationErr
		}
		result.ValidationError = nonEmpty(result.Server.LastErrorCode, "validation_failed")
		return result, nil
	}
	if !input.EnableForConversation {
		return result, nil
	}

	selection.Mode = SelectionModeCustom
	selection.Servers = appendMarketplaceSelection(selection.Servers, server.Ref)
	updated, err := s.ReplaceSelection(ctx, userID, selection)
	if err != nil {
		return result, err
	}
	result.Selection = &updated
	result.Enabled = true
	return result, nil
}

func (s *Service) installMarketplaceCustomRemote(
	ctx context.Context,
	userID string,
	detail MarketplaceItemDetail,
	input MarketplaceInstallInput,
	selection Selection,
) (MarketplaceInstallResult, error) {
	if !s.config.RemoteEnabled || !marketplaceSupportsCustomRemote(detail) {
		return MarketplaceInstallResult{}, ErrMarketplaceIncompatible
	}
	if _, err := ValidateEndpoint(ctx, input.CustomEndpointURL, NetworkPolicy{RequireHTTPS: true}); err != nil {
		return MarketplaceInstallResult{}, ErrURLBlocked
	}
	authType := nonEmpty(input.CustomAuthType, AuthNone)
	headerName, headerPrefix := "", ""
	switch authType {
	case AuthNone:
		if input.CustomCredential != "" || input.CustomClientID != "" || input.CustomHeaderName != "" || input.CustomHeaderPrefix != "" {
			return MarketplaceInstallResult{}, ErrCredentialInvalid
		}
	case AuthHeader:
		headerName = nonEmpty(input.CustomHeaderName, "Authorization")
		headerPrefix = input.CustomHeaderPrefix
		if input.CustomCredential == "" || len(input.CustomCredential) > maxCredentialBytes ||
			strings.ContainsAny(input.CustomCredential, "\x00\r\n") || input.CustomClientID != "" {
			return MarketplaceInstallResult{}, ErrCredentialRequired
		}
	case AuthOAuth:
		if input.CustomCredential != "" || input.CustomHeaderName != "" || input.CustomHeaderPrefix != "" {
			return MarketplaceInstallResult{}, ErrCredentialInvalid
		}
	default:
		return MarketplaceInstallResult{}, ErrCredentialInvalid
	}
	metadata := map[string]any{
		"marketplace": map[string]any{
			"provider": marketplaceProviderLobeHub, "identifier": detail.Identifier,
			"version": detail.Version, "connectionMode": "custom_remote",
		},
	}
	if detail.Icon != "" {
		metadata["icon"] = detail.Icon
	}
	if authType == AuthOAuth && input.CustomClientID == "" {
		metadata["oauthDynamicRegistration"] = true
	}
	server, err := s.CreatePrivateServer(ctx, userID, CreateServerInput{
		Name: detail.Name, EndpointURL: input.CustomEndpointURL, AuthType: authType,
		HeaderName: headerName, HeaderPrefix: headerPrefix, ClientID: input.CustomClientID,
		Metadata: metadata,
	})
	if err != nil {
		return MarketplaceInstallResult{}, err
	}
	if authType == AuthHeader {
		if err := s.storeCredential(ctx, userID, server.Ref, AuthHeader,
			storedCredential{HeaderValue: input.CustomCredential}, nil); err != nil {
			return s.marketplaceCredentialDraft(ctx, userID, server, "credential_store_failed")
		}
		server.HasCredential = true
	}
	result := MarketplaceInstallResult{Server: server}
	if authType == AuthOAuth {
		validated, validationErr := s.recordValidationFailure(
			ctx, userID, server, ServerStatusNeedsAuth, "credential_required", ErrCredentialRequired,
		)
		if validated.Ref.ID != "" {
			result.Server = validated
		}
		if validationErr != nil && validated.Ref.ID == "" {
			return result, validationErr
		}
		result.ValidationError = "credential_required"
		return result, nil
	}
	validated, validationErr := s.ValidatePrivateServer(ctx, userID, server.Ref.ID)
	if validated.Ref.ID != "" {
		result.Server = validated
		if authType == AuthHeader {
			result.Server.HasCredential = true
		}
	}
	if validationErr != nil {
		if validated.Ref.ID == "" {
			return result, validationErr
		}
		result.ValidationError = nonEmpty(result.Server.LastErrorCode, "validation_failed")
		return result, nil
	}
	if !input.EnableForConversation {
		return result, nil
	}
	selection.Mode = SelectionModeCustom
	selection.Servers = appendMarketplaceSelection(selection.Servers, result.Server.Ref)
	updated, err := s.ReplaceSelection(ctx, userID, selection)
	if err != nil {
		return result, err
	}
	result.Selection, result.Enabled = &updated, true
	return result, nil
}

func marketplaceSupportsCustomRemote(detail MarketplaceItemDetail) bool {
	for _, deployment := range detail.Deployments {
		if deployment.ConnectionType == "http" || deployment.InstallMode == "header" ||
			deployment.InstallMode == "oauth" || len(deployment.SecretFields) > 0 {
			return true
		}
	}
	return false
}

func (s *Service) createMarketplaceServer(
	ctx context.Context,
	userID string,
	detail MarketplaceItemDetail,
	deployment MarketplaceDeployment,
	artifact *Server,
	metadata map[string]any,
) (Server, error) {
	if artifact != nil {
		metadata["runnerArtifactId"] = artifact.Ref.ID
		if dynamic, ok := artifact.Metadata["dynamicRunnerArtifact"].(DynamicRunnerArtifact); ok {
			metadata["dynamicRunnerArtifact"] = dynamicRunnerArtifactMap(dynamic)
		}
		return s.createPrivateRunnerServer(ctx, userID, detail.Name, *artifact, metadata)
	}
	authType, headerName, headerPrefix := marketplaceRemoteAuth(deployment)
	if authType == AuthOAuth {
		metadata["oauthDynamicRegistration"] = true
	}
	return s.CreatePrivateServer(ctx, userID, CreateServerInput{
		Name: detail.Name, EndpointURL: deployment.EndpointURL,
		AuthType: authType, HeaderName: headerName, HeaderPrefix: headerPrefix, Metadata: metadata,
	})
}

func dynamicRunnerArtifactMap(artifact DynamicRunnerArtifact) map[string]any {
	return map[string]any{
		"id": artifact.ID, "packageSpec": artifact.PackageSpec,
		"args":        append([]string(nil), artifact.Args...),
		"secretEnv":   append([]string(nil), artifact.SecretEnv...),
		"idleSeconds": artifact.IdleSeconds, "lifetimeSeconds": artifact.LifetimeSeconds,
	}
}

func (s *Service) installedMarketplaceServer(
	ctx context.Context,
	userID string,
	detail MarketplaceItemDetail,
	deployment MarketplaceDeployment,
	artifact *Server,
) (Server, bool, error) {
	servers, err := s.repo.ListPrivateServers(ctx, userID)
	if err != nil {
		return Server{}, false, err
	}
	for _, server := range servers {
		if marketplaceServerMatches(server, detail, deployment, artifact) {
			return server, true, nil
		}
	}
	return Server{}, false, nil
}

func marketplaceServerMatches(
	server Server,
	detail MarketplaceItemDetail,
	deployment MarketplaceDeployment,
	artifact *Server,
) bool {
	if server.Ref.Source != SourcePrivate || server.Metadata == nil {
		return false
	}
	provenance, _ := server.Metadata["marketplace"].(map[string]any)
	if stringField(provenance, "provider") != marketplaceProviderLobeHub ||
		stringField(provenance, "identifier") != detail.Identifier ||
		stringField(provenance, "version") != detail.Version ||
		stringField(provenance, "deploymentHash") != deployment.Hash {
		return false
	}
	if artifact != nil {
		expectedAuth := AuthNone
		if len(deployment.SecretFields) > 0 {
			expectedAuth = AuthEnv
		}
		return server.Transport == TransportStdio && server.AuthType == expectedAuth &&
			server.EndpointURL == "runner://"+artifact.Ref.ID &&
			stringField(server.Metadata, "runnerArtifactId") == artifact.Ref.ID
	}
	authType, headerName, headerPrefix := marketplaceRemoteAuth(deployment)
	if server.Transport != TransportStreamableHTTP || server.EndpointURL != deployment.EndpointURL ||
		server.AuthType != authType {
		return false
	}
	if authType != AuthHeader {
		return true
	}
	return server.HeaderAuth != nil && server.HeaderAuth.Name == headerName &&
		server.HeaderAuth.Prefix == headerPrefix
}

func marketplaceRemoteAuth(deployment MarketplaceDeployment) (string, string, string) {
	if deployment.InstallMode == "oauth" {
		return AuthOAuth, "", ""
	}
	if deployment.InstallMode != "header" {
		return AuthNone, "", ""
	}
	headerName := nonEmpty(deployment.HeaderName, "Authorization")
	headerPrefix := ""
	if strings.EqualFold(headerName, "Authorization") {
		headerPrefix = "Bearer "
	}
	return AuthHeader, headerName, headerPrefix
}

func (s *Service) marketplaceCredentialDraft(
	ctx context.Context,
	userID string,
	server Server,
	code string,
) (MarketplaceInstallResult, error) {
	result := MarketplaceInstallResult{Server: server, ValidationError: code}
	updated, err := s.recordValidationFailure(
		ctx, userID, server, ServerStatusNeedsAuth, code, ErrCredentialRequired,
	)
	if err != nil && updated.Ref.ID == "" {
		return result, nil
	}
	if updated.Ref.ID != "" {
		result.Server = updated
	}
	return result, nil
}

func (s *Service) marketplaceAvailable() error {
	if err := s.available(); err != nil {
		return err
	}
	if !s.config.MarketplaceEnabled {
		return ErrMarketplaceDisabled
	}
	if s.marketplace == nil {
		return ErrMarketplaceUnavailable
	}
	return nil
}

func validMarketplaceIdentifier(value string) bool {
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "?#\\\r\n") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, character := range part {
			if (character >= 'a' && character <= 'z') ||
				(character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') ||
				strings.ContainsRune("._@-", character) {
				continue
			}
			return false
		}
	}
	return true
}

func validMarketplaceVersion(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._+-", character) {
			continue
		}
		return false
	}
	return true
}

type marketplaceInstallTarget struct {
	deployment MarketplaceDeployment
	artifact   *Server
}

func (s *Service) installableMarketplaceDeployment(
	detail MarketplaceItemDetail,
	requestedHash string,
) (marketplaceInstallTarget, bool) {
	candidates := make([]marketplaceInstallTarget, 0, len(detail.Deployments))
	for _, deployment := range detail.Deployments {
		if deployment.Hash == "" || (requestedHash != "" && deployment.Hash != requestedHash) ||
			(deployment.Compatibility != MarketplaceCompatibilityInstallable &&
				deployment.Compatibility != MarketplaceCompatibilityNeedsConfig) {
			continue
		}
		switch deployment.ConnectionType {
		case "http":
			if !s.config.RemoteEnabled || deployment.EndpointURL == "" {
				continue
			}
			if _, err := parseEndpoint(deployment.EndpointURL, true); err != nil {
				continue
			}
			candidates = append(candidates, marketplaceInstallTarget{deployment: deployment})
		case "stdio":
			if !s.config.StdioEnabled {
				continue
			}
			artifact, ok := s.marketplaceArtifact(detail.Identifier, detail.Version, deployment)
			if !ok {
				artifact, ok = dynamicMarketplaceArtifact(detail, deployment)
				if !ok {
					continue
				}
			}
			artifactCopy := artifact
			candidates = append(candidates, marketplaceInstallTarget{
				deployment: deployment, artifact: &artifactCopy,
			})
		}
	}
	if len(candidates) == 0 {
		return marketplaceInstallTarget{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].deployment.Recommended != candidates[j].deployment.Recommended {
			return candidates[i].deployment.Recommended
		}
		return candidates[i].deployment.Hash < candidates[j].deployment.Hash
	})
	return candidates[0], true
}

func marketplaceSecretValues(required []string, input map[string]string) (map[string]string, error) {
	if len(required) == 0 || len(input) != len(required) {
		return nil, ErrCredentialRequired
	}
	result := make(map[string]string, len(required))
	for _, name := range required {
		value, found := input[name]
		if !found || value == "" || len(value) > maxCredentialBytes || strings.ContainsAny(value, "\x00\r\n") {
			return nil, ErrCredentialInvalid
		}
		result[name] = value
	}
	return result, nil
}

func (s *Service) approveMarketplaceDetail(ctx context.Context, detail MarketplaceItemDetail) MarketplaceItemDetail {
	discoveryCtx, cancelDiscovery := context.WithTimeout(ctx, 3*time.Second)
	defer cancelDiscovery()
	detail.Deployments = append([]MarketplaceDeployment(nil), detail.Deployments...)
	for index := range detail.Deployments {
		deployment := &detail.Deployments[index]
		deployment.Args = append([]string(nil), deployment.Args...)
		deployment.SecretFields = append([]string(nil), deployment.SecretFields...)
		if deployment.ConnectionType == "http" && deployment.InstallMode == "direct" &&
			deployment.EndpointURL != "" && s.config.RemoteEnabled {
			probe := Server{
				Ref:       ServerRef{Source: SourcePrivate, ID: "00000000-0000-0000-0000-000000000000"},
				Transport: TransportStreamableHTTP, EndpointURL: deployment.EndpointURL,
				AuthType: AuthOAuth,
			}
			if metadata, err := s.discoverOAuthMetadata(discoveryCtx, probe); err == nil {
				deployment.InstallMode = "oauth"
				if metadata.RegistrationEndpoint != "" {
					deployment.Compatibility = MarketplaceCompatibilityInstallable
					deployment.CompatibilityReason = "OAuth authorization required; automatic client registration supported"
				} else {
					deployment.Compatibility = MarketplaceCompatibilityIncompatible
					deployment.CompatibilityReason = "OAuth authorization is required but automatic client registration is unavailable"
				}
			}
		}
		if deployment.ConnectionType != "stdio" || deployment.Hash == "" ||
			(deployment.Compatibility != MarketplaceCompatibilityRequiresRunner &&
				deployment.Compatibility != MarketplaceCompatibilityNeedsConfig) {
			continue
		}
		artifact, ok := s.marketplaceArtifact(detail.Identifier, detail.Version, *deployment)
		if !ok {
			artifact, ok = dynamicMarketplaceArtifact(detail, *deployment)
			if !ok {
				deployment.CompatibilityReason = "A bounded npm/npx Runner package is required"
				continue
			}
		}
		deployment.SecretFields = append([]string(nil), artifact.Command.UserSecretEnv...)
		if !s.config.StdioEnabled {
			deployment.CompatibilityReason = "The approved shared Runner artifact is disabled"
			continue
		}
		deployment.Compatibility = MarketplaceCompatibilityInstallable
		if len(deployment.SecretFields) > 0 {
			deployment.InstallMode = "runner_env"
			deployment.CompatibilityReason = "Approved Runner artifact; configuration required"
		} else {
			deployment.InstallMode = "direct"
			deployment.CompatibilityReason = "Approved shared Runner artifact"
		}
	}
	return detail
}

func dynamicMarketplaceArtifact(detail MarketplaceItemDetail, deployment MarketplaceDeployment) (Server, bool) {
	if deployment.ConnectionType != "stdio" || deployment.InstallationMethod != "npm" ||
		deployment.Command != "npx" || deployment.Hash == "" || !validMarketplaceVersion(detail.Version) {
		return Server{}, false
	}
	packageName := npmPackageBase(deployment.PackageName)
	if packageName == "" {
		for _, argument := range deployment.Args {
			if candidate := npmPackageBase(argument); candidate != "" {
				packageName = candidate
				break
			}
		}
	}
	if packageName == "" {
		return Server{}, false
	}
	packageSpec := strings.TrimSpace(deployment.PackageSpec)
	if packageSpec == "" {
		packageSpec = packageName + "@" + detail.Version
	}
	if npmPackageBase(packageSpec) != packageName || !validDynamicPackageSpec(packageSpec) {
		return Server{}, false
	}
	extraArgs := make([]string, 0, len(deployment.Args))
	for _, argument := range deployment.Args {
		if argument == "-y" || argument == "--yes" || npmPackageBase(argument) == packageName {
			continue
		}
		extraArgs = append(extraArgs, argument)
	}
	id := "npm-" + deployment.Hash[:48]
	dynamic := DynamicRunnerArtifact{
		ID: id, PackageSpec: packageSpec, Args: extraArgs,
		SecretEnv:       append([]string(nil), deployment.SecretFields...),
		IdleSeconds:     int64(defaultRunnerIdle / time.Second),
		LifetimeSeconds: int64(defaultRunnerLifetime / time.Second),
	}
	return Server{
		Ref: ServerRef{Source: SourceManifest, ID: id}, Name: detail.Name,
		Icon: boundedMarketplaceIcon(detail.Icon), Transport: TransportStdio,
		AuthType: func() string {
			if len(dynamic.SecretEnv) > 0 {
				return AuthEnv
			}
			return AuthNone
		}(),
		Status: ServerStatusReady,
		Command: &Command{
			Argv: append([]string{"/usr/local/bin/npx", "--yes", packageSpec}, extraArgs...),
			Env:  map[string]string{}, UserSecretEnv: append([]string(nil), dynamic.SecretEnv...),
			IdleTimeout: time.Duration(dynamic.IdleSeconds) * time.Second,
			MaxLifetime: time.Duration(dynamic.LifetimeSeconds) * time.Second,
		},
		Metadata: map[string]any{
			"marketplaceArtifact": MarketplaceArtifact{
				Provider: marketplaceProviderLobeHub, Identifier: detail.Identifier,
				Version: detail.Version, ConnectionType: deployment.ConnectionType,
				InstallationMethod: deployment.InstallationMethod, DeploymentHash: deployment.Hash,
			},
			"dynamicRunnerArtifact": dynamic,
		},
	}, true
}

func npmPackageBase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\\/:?#%\x00\r\n\t") && !strings.HasPrefix(value, "@") {
		return ""
	}
	base := value
	if strings.HasPrefix(base, "@") {
		if slash := strings.IndexByte(base, '/'); slash <= 1 {
			return ""
		} else if at := strings.IndexByte(base[slash+1:], '@'); at >= 0 {
			base = base[:slash+1+at]
		}
	} else if at := strings.IndexByte(base, '@'); at >= 0 {
		base = base[:at]
	}
	base = strings.ToLower(base)
	if !npmPackageNamePattern.MatchString(base) {
		return ""
	}
	return base
}

func npmPackageSelector(packageName string, args []string, fallbackVersion string) string {
	packageName = npmPackageBase(packageName)
	if packageName == "" {
		return ""
	}
	for _, argument := range args {
		argument = strings.TrimSpace(argument)
		if npmPackageBase(argument) != packageName || argument == packageName {
			continue
		}
		if strings.HasPrefix(packageName, "@") {
			if separator := strings.LastIndexByte(argument, '@'); separator > strings.IndexByte(packageName, '/') {
				return argument[separator+1:]
			}
			continue
		}
		if separator := strings.LastIndexByte(argument, '@'); separator > 0 {
			return argument[separator+1:]
		}
	}
	return strings.TrimSpace(fallbackVersion)
}

func (s *Service) marketplaceArtifact(
	identifier string,
	version string,
	deployment MarketplaceDeployment,
) (Server, bool) {
	for _, id := range s.sharedServerIDs(SourceManifest) {
		server, err := s.sharedServer(ServerRef{Source: SourceManifest, ID: id})
		if err != nil || server.Transport != TransportStdio || server.Command == nil {
			continue
		}
		artifact, ok := server.Metadata["marketplaceArtifact"].(MarketplaceArtifact)
		if !ok || artifact.Provider != marketplaceProviderLobeHub ||
			artifact.Identifier != identifier || artifact.Version != version ||
			artifact.ConnectionType != deployment.ConnectionType ||
			artifact.InstallationMethod != deployment.InstallationMethod ||
			artifact.DeploymentHash != deployment.Hash {
			continue
		}
		return server, true
	}
	return Server{}, false
}

func marketplaceArtifactFromServer(server Server) (MarketplaceArtifact, bool) {
	artifact, ok := server.Metadata["marketplaceArtifact"].(MarketplaceArtifact)
	return artifact, ok
}

func marketplaceDeploymentHash(
	identifier string,
	version string,
	deployment MarketplaceDeployment,
) (string, error) {
	projection := struct {
		Provider           string   `json:"provider"`
		Identifier         string   `json:"identifier"`
		Version            string   `json:"version"`
		ConnectionType     string   `json:"connectionType"`
		InstallationMethod string   `json:"installationMethod"`
		EndpointURL        string   `json:"endpointUrl"`
		Command            string   `json:"command"`
		Args               []string `json:"args"`
		PackageName        string   `json:"packageName"`
		SecretFields       []string `json:"secretFields"`
	}{
		Provider:           marketplaceProviderLobeHub,
		Identifier:         identifier,
		Version:            version,
		ConnectionType:     deployment.ConnectionType,
		InstallationMethod: deployment.InstallationMethod,
		EndpointURL:        deployment.EndpointURL,
		Command:            deployment.Command,
		Args:               append([]string(nil), deployment.Args...),
		PackageName:        deployment.PackageName,
		SecretFields:       append([]string(nil), deployment.SecretFields...),
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func appendMarketplaceSelection(input []SelectionServer, ref ServerRef) []SelectionServer {
	for _, item := range input {
		if item.Ref == ref {
			return input
		}
	}
	return append(input, SelectionServer{Ref: ref, DisabledTools: []string{}})
}

func cloneObject(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
