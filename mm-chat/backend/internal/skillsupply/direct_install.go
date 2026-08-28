package skillsupply

import (
	"context"
	"strings"
)

// InstallDirectSkillLink resolves one allowlisted exact LobeHub/GitHub Skill
// path (plus the legacy AIHero compatibility path), pins mutable sources,
// validates the package without executing source instructions, and installs it
// into only the requesting owner's private library. It deliberately bypasses
// Store review/publication.
func (service *Service) InstallDirectSkillLink(
	ctx context.Context,
	userID, rawURL, expectedName string,
) (Installation, error) {
	if service == nil || service.repository == nil || service.objects == nil {
		return Installation{}, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	expectedName = strings.TrimSpace(expectedName)
	if !validUserID(userID) {
		return Installation{}, validationError("INVALID_DIRECT_SKILL_INSTALL", "direct Skill install is invalid")
	}
	if identifier, parseErr := ParseLobeHubSkillURL(rawURL); parseErr == nil {
		if identifier != expectedName {
			return Installation{}, ErrInvalidSource
		}
		detail, detailErr := service.GetMarketplaceSkill(ctx, userID, identifier, "")
		if detailErr != nil {
			return Installation{}, detailErr
		}
		return service.InstallMarketplaceSkill(ctx, userID, identifier, detail.Version)
	}
	if !skillNamePattern.MatchString(expectedName) {
		return Installation{}, validationError("INVALID_DIRECT_SKILL_INSTALL", "direct Skill install is invalid")
	}
	source, err := service.direct.Fetch(ctx, rawURL, expectedName)
	if err != nil {
		return Installation{}, err
	}
	candidate, err := service.ingest(ctx, userID, source)
	if err != nil {
		return Installation{}, err
	}
	return service.installPrivateCandidate(ctx, userID, candidate)
}
