package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const marketplaceProviderLobeHub = "lobehub"

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
	return s.approveMarketplaceDetail(detail), nil
}

func (s *Service) InstallMarketplaceItem(
	ctx context.Context,
	userID string,
	input MarketplaceInstallInput,
) (MarketplaceInstallResult, error) {
	if err := s.marketplaceAvailable(); err != nil {
		return MarketplaceInstallResult{}, err
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	input.Version = strings.TrimSpace(input.Version)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	if !validMarketplaceIdentifier(input.Identifier) || !validMarketplaceVersion(input.Version) {
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
	detail = s.approveMarketplaceDetail(detail)
	target, found := s.installableMarketplaceDeployment(detail)
	if !found || target.deployment.Hash == "" {
		return MarketplaceInstallResult{}, ErrMarketplaceIncompatible
	}
	deployment := target.deployment
	expectedHash, err := marketplaceDeploymentHash(detail.Identifier, detail.Version, deployment)
	if err != nil || deployment.Hash != expectedHash {
		return MarketplaceInstallResult{}, ErrMarketplaceChanged
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
	var server Server
	if target.artifact != nil {
		metadata["runnerArtifactId"] = target.artifact.Ref.ID
		server, err = s.createPrivateRunnerServer(ctx, userID, detail.Name, *target.artifact, metadata)
	} else {
		server, err = s.CreatePrivateServer(ctx, userID, CreateServerInput{
			Name: detail.Name, EndpointURL: deployment.EndpointURL,
			AuthType: AuthNone, Metadata: metadata,
		})
	}
	if err != nil {
		return MarketplaceInstallResult{}, err
	}
	result := MarketplaceInstallResult{Server: server}
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
) (marketplaceInstallTarget, bool) {
	candidates := make([]marketplaceInstallTarget, 0, len(detail.Deployments))
	for _, deployment := range detail.Deployments {
		if deployment.Compatibility != MarketplaceCompatibilityInstallable || deployment.Hash == "" {
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
				continue
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

func (s *Service) approveMarketplaceDetail(detail MarketplaceItemDetail) MarketplaceItemDetail {
	detail.Deployments = append([]MarketplaceDeployment(nil), detail.Deployments...)
	for index := range detail.Deployments {
		deployment := &detail.Deployments[index]
		deployment.Args = append([]string(nil), deployment.Args...)
		if deployment.ConnectionType != "stdio" || deployment.Hash == "" ||
			deployment.Compatibility != MarketplaceCompatibilityRequiresRunner {
			continue
		}
		if _, ok := s.marketplaceArtifact(detail.Identifier, detail.Version, *deployment); !ok {
			deployment.CompatibilityReason = "No approved shared Runner artifact is available"
			continue
		}
		if !s.config.StdioEnabled {
			deployment.CompatibilityReason = "The approved shared Runner artifact is disabled"
			continue
		}
		deployment.Compatibility = MarketplaceCompatibilityInstallable
		deployment.CompatibilityReason = "Approved shared Runner artifact"
	}
	return detail
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
