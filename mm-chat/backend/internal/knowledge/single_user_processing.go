package knowledge

import (
	"context"
	"fmt"

	"neo-chat/mm-chat/backend/internal/auth"
)

// SingleUserNativeGovernanceManifest describes the local, credential-free
// parser used for non-PDF knowledge documents.
func SingleUserNativeGovernanceManifest() GovernanceManifest {
	return GovernanceManifest{
		Processor:        nativeParseProcessor,
		EndpointID:       nativeParseEndpointID,
		ModelID:          nativeParseModelID,
		ModelAPIVersion:  "api-20260717",
		AllowedPurposes:  []string{"parse"},
		AllowedDataTypes: append([]string(nil), nativeParseDataTypes...),
		Region:           "global",
		RetentionPolicy:  "none",
		DeletionContract: "delete",
		TrainingUse:      "disabled",
	}
}

func SingleUserAnswerGovernanceManifest(identity ProcessorModelIdentity) GovernanceManifest {
	return GovernanceManifest{
		Processor:        identity.Processor,
		EndpointID:       identity.EndpointID,
		ModelID:          identity.ModelID,
		ModelAPIVersion:  "api-20260716",
		AllowedPurposes:  []string{"answer"},
		AllowedDataTypes: []string{"text/plain"},
		Region:           "global",
		RetentionPolicy:  "none",
		DeletionContract: "delete",
		TrainingUse:      "disabled",
	}
}

// BootstrapSingleUserNativeProcessing ensures the local parser authority and
// consent exist for every personal collection. Replaying this operation is
// idempotent and keeps governance owned by the server rather than the browser.
func BootstrapSingleUserNativeProcessing(
	ctx context.Context,
	service *Service,
	governance *GovernanceService,
	owner auth.User,
) error {
	if service == nil || governance == nil {
		return ErrDatabaseRequired
	}
	head, err := governance.Apply(ctx, SingleUserNativeGovernanceManifest())
	if err != nil {
		return fmt.Errorf("ensure native parser governance: %w", err)
	}
	actorCtx := auth.WithUser(ctx, owner)
	cursor := ""
	for {
		page, listErr := service.ListCollections(actorCtx, ListCollectionsInput{
			Scope: ScopePersonal, Cursor: cursor, Limit: maximumPageLimit,
		})
		if listErr != nil {
			return fmt.Errorf("list single-user collections for native parser: %w", listErr)
		}
		for _, collection := range page.Items {
			_, consentErr := service.PutCollectionConsentForModel(
				actorCtx,
				collection.ID,
				ProcessorModelIdentity{
					Processor:  head.Processor,
					EndpointID: head.EndpointID,
					ModelID:    head.ModelID,
				},
				PutConsentInput{
					Purposes:      []string{"parse"},
					DataTypes:     append([]string(nil), nativeParseDataTypes...),
					PolicyVersion: "v1",
				},
			)
			if consentErr != nil {
				return fmt.Errorf(
					"ensure native parser consent for collection %s: %w",
					collection.ID,
					consentErr,
				)
			}
		}
		if page.NextCursor == "" {
			return nil
		}
		cursor = page.NextCursor
	}
}

// BootstrapSingleUserAnswerProcessing grants the fixed single-user owner and
// all existing personal collections permission to send selected hydrated text
// to the configured server answer model. The browser never writes governance
// or consent records; replay is idempotent.
func BootstrapSingleUserAnswerProcessing(
	ctx context.Context,
	service *Service,
	governance *GovernanceService,
	owner auth.User,
	identity ProcessorModelIdentity,
) error {
	if service == nil || governance == nil {
		return ErrDatabaseRequired
	}
	head, err := governance.Apply(ctx, SingleUserAnswerGovernanceManifest(identity))
	if err != nil {
		return fmt.Errorf("ensure answer provider governance: %w", err)
	}
	identity = ProcessorModelIdentity{
		Processor: head.Processor, EndpointID: head.EndpointID, ModelID: head.ModelID,
	}
	actorCtx := auth.WithUser(ctx, owner)
	if _, err := service.PutQueryConsentForModel(actorCtx, identity, PutConsentInput{
		Purposes: []string{"answer"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1",
	}); err != nil {
		return fmt.Errorf("ensure owner answer query consent: %w", err)
	}
	cursor := ""
	for {
		page, listErr := service.ListCollections(actorCtx, ListCollectionsInput{
			Scope: ScopePersonal, Cursor: cursor, Limit: maximumPageLimit,
		})
		if listErr != nil {
			return fmt.Errorf("list single-user collections for answer provider: %w", listErr)
		}
		for _, collection := range page.Items {
			if _, consentErr := service.PutCollectionConsentForModel(
				actorCtx,
				collection.ID,
				identity,
				PutConsentInput{
					Purposes: []string{"answer"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1",
				},
			); consentErr != nil {
				return fmt.Errorf(
					"ensure answer consent for collection %s: %w",
					collection.ID,
					consentErr,
				)
			}
		}
		if page.NextCursor == "" {
			return nil
		}
		cursor = page.NextCursor
	}
}
