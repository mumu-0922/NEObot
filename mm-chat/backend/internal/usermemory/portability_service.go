package usermemory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

type builtImportPlan struct {
	result                ImportDryRunResult
	manifest              MemoryPackageManifest
	mappingsHash          string
	authorityStateHash    string
	resolvedProjects      map[string]resolvedProjectMapping
	resolvedConversations map[string]ResolvedImportScope
	resultsByMemoryRef    map[string]ImportPlanItem
}

type importSupersessionDependency struct {
	targetRef  string
	reasonCode string
}

type importHistoryScopeIssue struct {
	result     string
	reasonCode string
}

type resolvedProjectMapping struct {
	mode        string
	localID     string
	scope       ResolvedImportScope
	name        string
	description string
}

func (s *Service) DryRunMemoryImport(
	ctx context.Context,
	packageReader io.ReadSeeker,
	passphrase string,
	mappings ImportMappings,
) (ImportDryRunResult, error) {
	repo, err := s.portabilityRepository()
	if err != nil {
		return ImportDryRunResult{}, err
	}
	if s.portabilityCodec == nil {
		return ImportDryRunResult{}, ErrPortabilityPlanCodecRequired
	}
	userID, err := currentPortabilityUserID(ctx)
	if err != nil {
		return ImportDryRunResult{}, err
	}
	importID, err := newUUID()
	if err != nil {
		return ImportDryRunResult{}, err
	}
	built, err := s.buildMemoryImportPlan(
		ctx, repo, packageReader, passphrase, mappings, importID,
	)
	if err != nil {
		return ImportDryRunResult{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(PortabilityPlanTTL)
	token, err := s.portabilityCodec.Encode(PortabilityPlanToken{
		UserID:             userID,
		ImportID:           importID,
		PackageSHA256:      built.result.PackageSHA256,
		ManifestSHA256:     built.result.ManifestSHA256,
		MappingsSHA256:     built.mappingsHash,
		PlanSHA256:         built.result.PlanSHA256,
		AuthorityStateHash: built.authorityStateHash,
		IssuedAt:           now.UnixMilli(),
		ExpiresAt:          expiresAt.UnixMilli(),
	})
	if err != nil {
		return ImportDryRunResult{}, err
	}
	built.result.PlanToken = token
	built.result.ExpiresAt = expiresAt.UnixMilli()
	return built.result, nil
}

func (s *Service) ConfirmMemoryImport(
	ctx context.Context,
	packageReader io.ReadSeeker,
	passphrase string,
	mappings ImportMappings,
	encodedToken string,
) (PortabilityApplyResult, error) {
	repo, err := s.portabilityRepository()
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if s.portabilityCodec == nil {
		return PortabilityApplyResult{}, ErrPortabilityPlanCodecRequired
	}
	userID, err := currentPortabilityUserID(ctx)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	token, err := s.portabilityCodec.Decode(encodedToken)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if token.UserID != userID || time.Now().UTC().UnixMilli() > token.ExpiresAt {
		return PortabilityApplyResult{}, importPlanStaleError()
	}
	packageHash, err := hashSeekable(packageReader)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	mappingsHash, err := canonicalMappingsHash(mappings)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if packageHash != token.PackageSHA256 || mappingsHash != token.MappingsSHA256 {
		return PortabilityApplyResult{}, importPlanStaleError()
	}
	if _, err := packageReader.Seek(0, io.SeekStart); err != nil {
		return PortabilityApplyResult{}, fmt.Errorf("rewind memory package for replay check: %w", err)
	}
	authenticated, err := ParseEncryptedMemoryPackage(
		packageReader, passphrase, MemoryPackageVisitorFuncs{},
	)
	if err != nil {
		return PortabilityApplyResult{}, err
	}
	if authenticated.ManifestHash != token.ManifestSHA256 {
		return PortabilityApplyResult{}, importPlanStaleError()
	}
	metadata := portabilityApplyMetadataFromToken(token)
	if completed, found, err := repo.CompletedPortabilityImport(ctx, metadata); err != nil {
		return PortabilityApplyResult{}, err
	} else if found {
		return completed, nil
	}
	built, err := s.buildMemoryImportPlan(
		ctx, repo, packageReader, passphrase, mappings, token.ImportID,
	)
	if err != nil {
		completed, found, lookupErr := repo.CompletedPortabilityImport(ctx, metadata)
		if lookupErr != nil {
			return PortabilityApplyResult{}, lookupErr
		}
		if found {
			return completed, nil
		}
		return PortabilityApplyResult{}, err
	}
	if built.result.PackageSHA256 != token.PackageSHA256 ||
		built.result.ManifestSHA256 != token.ManifestSHA256 ||
		built.mappingsHash != token.MappingsSHA256 ||
		built.result.PlanSHA256 != token.PlanSHA256 ||
		built.authorityStateHash != token.AuthorityStateHash ||
		len(built.result.ScopeRequirements) != 0 {
		completed, found, err := repo.CompletedPortabilityImport(ctx, metadata)
		if err != nil {
			return PortabilityApplyResult{}, err
		}
		if found {
			return completed, nil
		}
		return PortabilityApplyResult{}, importPlanStaleError()
	}
	metadata.ProjectCount = built.manifest.Counts.Projects
	metadata.MemoryCount = built.manifest.Counts.Memories
	metadata.RevisionCount = built.manifest.Counts.Revisions
	result, err := repo.ApplyPortabilityImport(ctx, metadata, func(sink PortabilityApplySink) error {
		if _, err := packageReader.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("rewind memory package for confirm: %w", err)
		}
		finalStates := make([]PortabilityApplyFinalState, 0)
		parsed, err := ParseEncryptedMemoryPackage(packageReader, passphrase, MemoryPackageVisitorFuncs{
			Project: func(_ int, record PortableProjectRecord) error {
				mapping := built.resolvedProjects[record.Ref]
				if mapping.mode != "create" {
					return nil
				}
				return sink.CreateProject(PortabilityApplyProject{
					ID:              mapping.localID,
					Name:            record.Name,
					Description:     record.Description,
					LifecycleStatus: record.LifecycleStatus,
				})
			},
			Memory: func(_ int, record PortableMemoryRecord) error {
				item := built.resultsByMemoryRef[record.Ref]
				if item.Result != "ADD" {
					return nil
				}
				memoryID, err := deterministicImportUUID(token.ImportID, record.Ref, "memory")
				if err != nil {
					return err
				}
				scope, err := resolvedScopeForRecord(
					record, built.resolvedProjects, built.resolvedConversations,
				)
				if err != nil {
					return err
				}
				if err := sink.AddMemory(PortabilityApplyMemory{
					ID: memoryID, Record: record, Scope: scope,
				}); err != nil {
					return err
				}
				finalState := PortabilityApplyFinalState{
					MemoryID: memoryID, LifecycleStatus: record.LifecycleStatus,
				}
				if record.SupersededByRef != "" {
					target := built.resultsByMemoryRef[record.SupersededByRef]
					if target.Result == "ADD" {
						finalState.SupersededByMemoryID, err = deterministicImportUUID(
							token.ImportID, record.SupersededByRef, "memory",
						)
					} else {
						finalState.SupersededByMemoryID = target.CurrentMemoryID
					}
					if err != nil {
						return err
					}
				}
				finalStates = append(finalStates, finalState)
				return nil
			},
			Revision: func(_ int, record PortableRevisionRecord) error {
				item := built.resultsByMemoryRef[record.MemoryRef]
				if item.Result != "ADD" {
					return nil
				}
				memoryID, err := deterministicImportUUID(token.ImportID, record.MemoryRef, "memory")
				if err != nil {
					return err
				}
				var scope ResolvedImportScope
				var supersededByMemoryID string
				if record.Prior != nil {
					scope, err = resolvedScopeForPortableScope(
						record.Prior.Scope,
						built.resolvedProjects,
						built.resolvedConversations,
					)
					if err != nil {
						return err
					}
					if record.Prior.SupersededByRef != "" {
						target := built.resultsByMemoryRef[record.Prior.SupersededByRef]
						switch target.Result {
						case "ADD":
							supersededByMemoryID, err = deterministicImportUUID(
								token.ImportID, record.Prior.SupersededByRef, "memory",
							)
						case "NOOP":
							supersededByMemoryID = target.CurrentMemoryID
						default:
							return errors.New("imported revision supersession target is unavailable")
						}
						if err != nil {
							return err
						}
					}
				}
				return sink.AddRevision(PortabilityApplyRevision{
					MemoryID: memoryID, Record: record, Scope: scope,
					SupersededByMemoryID: supersededByMemoryID,
				})
			},
		})
		if err != nil {
			return err
		}
		if parsed.ManifestHash != token.ManifestSHA256 {
			return importPlanStaleError()
		}
		for _, finalState := range finalStates {
			if err := sink.FinalizeMemory(finalState); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		completed, found, lookupErr := repo.CompletedPortabilityImport(ctx, metadata)
		if lookupErr != nil {
			return PortabilityApplyResult{}, lookupErr
		}
		if found {
			return completed, nil
		}
		return PortabilityApplyResult{}, err
	}
	return result, nil
}

func portabilityApplyMetadataFromToken(token PortabilityPlanToken) PortabilityApplyMetadata {
	return PortabilityApplyMetadata{
		ImportID:           token.ImportID,
		PackageSHA256:      token.PackageSHA256,
		ManifestSHA256:     token.ManifestSHA256,
		MappingsSHA256:     token.MappingsSHA256,
		PlanSHA256:         token.PlanSHA256,
		AuthorityStateHash: token.AuthorityStateHash,
	}
}

func (s *Service) buildMemoryImportPlan(
	ctx context.Context,
	repo PortabilityRepository,
	packageReader io.ReadSeeker,
	passphrase string,
	mappings ImportMappings,
	importID string,
) (builtImportPlan, error) {
	normalizedMappings, err := normalizeImportMappings(mappings)
	if err != nil {
		return builtImportPlan{}, err
	}
	mappingsHash, err := hashCanonicalJSON(normalizedMappings)
	if err != nil {
		return builtImportPlan{}, err
	}
	packageHash, err := hashSeekable(packageReader)
	if err != nil {
		return builtImportPlan{}, err
	}
	secretMemoryRefs, invalidMemoryRefs, err := preflightMemoryImportPackage(
		packageReader, passphrase,
	)
	if err != nil {
		return builtImportPlan{}, err
	}
	stateBefore, err := repo.PortabilityAuthorityState(ctx)
	if err != nil {
		return builtImportPlan{}, err
	}
	if !isLowerSHA256(stateBefore) {
		return builtImportPlan{}, errors.New(
			"memory portability repository returned an invalid authority state hash",
		)
	}
	projects := make(map[string]resolvedProjectMapping)
	conversations := make(map[string]ResolvedImportScope)
	items := make([]ImportPlanItem, 0)
	itemsByRef := make(map[string]ImportPlanItem)
	itemIndexByRef := make(map[string]int)
	dependencies := make(map[string][]importSupersessionDependency)
	historyScopeIssues := make(map[string]importHistoryScopeIssue)
	requirements := make(map[string]ImportScopeRequirement)
	counts := map[string]int{
		"NOOP": 0, "ADD": 0, "REVIEW": 0, "REJECT": 0, "SCOPE_REQUIRED": 0,
	}
	var settingsSuggestion *Settings

	if _, err := packageReader.Seek(0, io.SeekStart); err != nil {
		return builtImportPlan{}, fmt.Errorf("rewind memory package for dry-run: %w", err)
	}
	parsed, err := ParseEncryptedMemoryPackage(packageReader, passphrase, MemoryPackageVisitorFuncs{
		Settings: func(_ int, record PortableSettingsRecord) error {
			settings := record.Settings
			settingsSuggestion = &settings
			return nil
		},
		Project: func(_ int, record PortableProjectRecord) error {
			mapping, ok := normalizedMappings.Projects[record.Ref]
			if !ok {
				requirements["project:"+record.Ref] = ImportScopeRequirement{
					Kind:        "project",
					PortableRef: record.Ref,
					Name:        record.Name,
					Description: record.Description,
				}
				projects[record.Ref] = resolvedProjectMapping{
					mode: "required", name: record.Name, description: record.Description,
				}
				return nil
			}
			switch mapping.Mode {
			case "existing":
				project, err := repo.ResolveImportProject(ctx, mapping.ProjectID)
				if err != nil {
					return err
				}
				projects[record.Ref] = resolvedProjectMapping{
					mode:    "existing",
					localID: project.ID,
					scope: ResolvedImportScope{
						Type:            "project",
						ProjectID:       project.ID,
						ScopeGeneration: project.ScopeGeneration,
					},
					name: record.Name, description: record.Description,
				}
			case "create":
				projectID, err := deterministicImportUUID(importID, record.Ref, "project")
				if err != nil {
					return err
				}
				projects[record.Ref] = resolvedProjectMapping{
					mode:    "create",
					localID: projectID,
					scope: ResolvedImportScope{
						Type: "project", ProjectID: projectID, ScopeGeneration: 1,
					},
					name: record.Name, description: record.Description,
				}
			case "skip":
				projects[record.Ref] = resolvedProjectMapping{
					mode: "skip", scope: ResolvedImportScope{Skipped: true},
					name: record.Name, description: record.Description,
				}
			}
			return nil
		},
		Memory: func(ordinal int, record PortableMemoryRecord) error {
			recordHash, err := hashCanonicalJSON(record)
			if err != nil {
				return err
			}
			item := ImportPlanItem{
				Ordinal: ordinal, MemoryRef: record.Ref, RecordHash: recordHash,
			}
			switch {
			case secretMemoryRefs[record.Ref]:
				item.Result, item.ReasonCode = "REJECT", "SECRET_REJECTED"
			case invalidMemoryRefs[record.Ref]:
				item.Result, item.ReasonCode = "REJECT", "INVALID_MEMORY"
			default:
				scope, scopeErr := resolveImportScope(
					ctx, repo, record.Scope, normalizedMappings,
					projects, conversations, requirements,
				)
				if scopeErr != nil {
					return scopeErr
				}
				if scope.Skipped {
					item.Result, item.ReasonCode = "REJECT", "MAPPING_SKIPPED"
				} else if scope.Type == "" {
					item.Result, item.ReasonCode = "SCOPE_REQUIRED", "SCOPE_MAPPING_REQUIRED"
				} else {
					resolution, err := repo.ResolveImportMemory(ctx, ImportMemoryResolutionInput{
						NormalizedContent: normalizeSearchText(record.Content),
						SubjectKey:        strings.TrimSpace(record.SubjectKey),
						FactKey:           strings.TrimSpace(record.FactKey),
						Scope:             scope,
					})
					if err != nil {
						return err
					}
					if resolution.Result != "NOOP" &&
						resolution.Result != "ADD" &&
						resolution.Result != "REVIEW" {
						return errors.New(
							"memory portability repository returned an invalid resolution",
						)
					}
					item.Result = resolution.Result
					item.ReasonCode = resolution.ReasonCode
					item.CurrentHash = resolution.CurrentContentHash
					item.CurrentMemoryID = resolution.CurrentMemoryID
				}
			}
			items = append(items, item)
			itemIndexByRef[item.MemoryRef] = len(items) - 1
			itemsByRef[item.MemoryRef] = item
			if record.SupersededByRef != "" {
				dependencies[record.Ref] = append(
					dependencies[record.Ref],
					importSupersessionDependency{
						targetRef:  record.SupersededByRef,
						reasonCode: "SUPERSESSION_TARGET_UNAVAILABLE",
					},
				)
			}
			counts[item.Result]++
			return nil
		},
		Revision: func(_ int, record PortableRevisionRecord) error {
			if record.Prior == nil {
				return nil
			}
			historyScope, err := resolveImportScope(
				ctx, repo, record.Prior.Scope, normalizedMappings,
				projects, conversations, requirements,
			)
			if err != nil {
				return err
			}
			switch {
			case historyScope.Skipped:
				current, exists := historyScopeIssues[record.MemoryRef]
				if !exists || current.result != "SCOPE_REQUIRED" {
					historyScopeIssues[record.MemoryRef] = importHistoryScopeIssue{
						result: "REJECT", reasonCode: "HISTORY_SCOPE_UNAVAILABLE",
					}
				}
			case historyScope.Type == "":
				historyScopeIssues[record.MemoryRef] = importHistoryScopeIssue{
					result: "SCOPE_REQUIRED", reasonCode: "HISTORY_SCOPE_MAPPING_REQUIRED",
				}
			}
			if record.Prior.SupersededByRef != "" {
				dependencies[record.MemoryRef] = append(
					dependencies[record.MemoryRef],
					importSupersessionDependency{
						targetRef:  record.Prior.SupersededByRef,
						reasonCode: "HISTORY_TARGET_UNAVAILABLE",
					},
				)
			}
			return nil
		},
	})
	if err != nil {
		return builtImportPlan{}, err
	}
	for memoryRef, issue := range historyScopeIssues {
		index, exists := itemIndexByRef[memoryRef]
		if !exists || items[index].Result != "ADD" {
			continue
		}
		counts[items[index].Result]--
		items[index].Result = issue.result
		items[index].ReasonCode = issue.reasonCode
		items[index].CurrentHash = ""
		items[index].CurrentMemoryID = ""
		counts[items[index].Result]++
		itemsByRef[memoryRef] = items[index]
	}
	for changed := true; changed; {
		changed = false
		for index := range items {
			if items[index].Result != "ADD" {
				continue
			}
			for _, dependency := range dependencies[items[index].MemoryRef] {
				target, exists := itemsByRef[dependency.targetRef]
				if exists && (target.Result == "ADD" || target.Result == "NOOP") {
					continue
				}
				counts[items[index].Result]--
				items[index].Result = "REJECT"
				items[index].ReasonCode = dependency.reasonCode
				items[index].CurrentHash = ""
				items[index].CurrentMemoryID = ""
				counts[items[index].Result]++
				itemsByRef[items[index].MemoryRef] = items[index]
				changed = true
				break
			}
		}
	}
	stateAfter, err := repo.PortabilityAuthorityState(ctx)
	if err != nil {
		return builtImportPlan{}, err
	}
	if stateBefore != stateAfter {
		return builtImportPlan{}, importPlanStaleError()
	}
	requirementList := make([]ImportScopeRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		requirementList = append(requirementList, requirement)
	}
	sort.Slice(requirementList, func(i, j int) bool {
		if requirementList[i].Kind == requirementList[j].Kind {
			return requirementList[i].PortableRef < requirementList[j].PortableRef
		}
		return requirementList[i].Kind < requirementList[j].Kind
	})
	planPayload := importPlanHashPayload{
		ImportID:           importID,
		PackageSHA256:      packageHash,
		ManifestSHA256:     parsed.ManifestHash,
		MappingsSHA256:     mappingsHash,
		AuthorityStateHash: stateBefore,
		Counts:             parsed.Manifest.Counts,
		Items:              items,
		ScopeRequirements:  requirementList,
	}
	planHash, err := hashCanonicalJSON(planPayload)
	if err != nil {
		return builtImportPlan{}, err
	}
	return builtImportPlan{
		result: ImportDryRunResult{
			ImportID:           importID,
			PackageSHA256:      packageHash,
			ManifestSHA256:     parsed.ManifestHash,
			PlanSHA256:         planHash,
			Counts:             counts,
			Items:              items,
			ScopeRequirements:  requirementList,
			SettingsSuggestion: settingsSuggestion,
		},
		manifest:              parsed.Manifest,
		mappingsHash:          mappingsHash,
		authorityStateHash:    stateBefore,
		resolvedProjects:      projects,
		resolvedConversations: conversations,
		resultsByMemoryRef:    itemsByRef,
	}, nil
}

func preflightMemoryImportPackage(
	packageReader io.ReadSeeker,
	passphrase string,
) (map[string]bool, map[string]bool, error) {
	secretMemoryRefs := make(map[string]bool)
	invalidMemoryRefs := make(map[string]bool)
	projectContainsSecret := false
	if _, err := packageReader.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("rewind memory package for preflight: %w", err)
	}
	_, err := ParseEncryptedMemoryPackage(packageReader, passphrase, MemoryPackageVisitorFuncs{
		Project: func(_ int, record PortableProjectRecord) error {
			if portabilityPlaintextContainsSecret(record.Name, record.Description) {
				projectContainsSecret = true
			}
			return nil
		},
		Memory: func(_ int, record PortableMemoryRecord) error {
			if portableMemoryContainsSecret(record) {
				secretMemoryRefs[record.Ref] = true
			}
			if ValidatePortableMemoryCandidate(record) != nil {
				invalidMemoryRefs[record.Ref] = true
			}
			return nil
		},
		Revision: func(_ int, record PortableRevisionRecord) error {
			if record.Prior != nil && portableSnapshotContainsSecret(*record.Prior) {
				secretMemoryRefs[record.MemoryRef] = true
			}
			return nil
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if projectContainsSecret {
		return nil, nil, validation(
			"MEMORY_PACKAGE_SECRET_REJECTED",
			"memory package Project metadata contains secret-like content",
		)
	}
	return secretMemoryRefs, invalidMemoryRefs, nil
}

func portableMemoryContainsSecret(record PortableMemoryRecord) bool {
	values := []string{record.Content, record.SubjectKey, record.FactKey}
	values = append(values, record.Tags...)
	return portabilityPlaintextContainsSecret(values...)
}

func portableSnapshotContainsSecret(record PortableMemorySnapshot) bool {
	values := []string{record.Content, record.SubjectKey, record.FactKey}
	values = append(values, record.Tags...)
	return portabilityPlaintextContainsSecret(values...)
}

func portabilityPlaintextContainsSecret(values ...string) bool {
	for _, value := range values {
		if ClassifyMemorySensitivity(value) == SensitivitySecret {
			return true
		}
	}
	return false
}

func resolveImportScope(
	ctx context.Context,
	repo PortabilityRepository,
	scope PortableMemoryScope,
	mappings ImportMappings,
	projects map[string]resolvedProjectMapping,
	conversations map[string]ResolvedImportScope,
	requirements map[string]ImportScopeRequirement,
) (ResolvedImportScope, error) {
	switch scope.Type {
	case "global":
		return ResolvedImportScope{Type: "global", ScopeGeneration: 1}, nil
	case "project":
		mapping := projects[scope.ProjectRef]
		if mapping.mode == "required" || mapping.mode == "" {
			return ResolvedImportScope{}, nil
		}
		return mapping.scope, nil
	case "conversation":
		if resolved, ok := conversations[scope.ConversationRef]; ok {
			return resolved, nil
		}
		mapping, ok := mappings.Conversations[scope.ConversationRef]
		if !ok {
			requirements["conversation:"+scope.ConversationRef] = ImportScopeRequirement{
				Kind: "conversation", PortableRef: scope.ConversationRef,
			}
			return ResolvedImportScope{}, nil
		}
		var resolved ResolvedImportScope
		switch mapping.Mode {
		case "existing":
			conversation, err := repo.ResolveImportConversation(ctx, mapping.ConversationID)
			if err != nil {
				return ResolvedImportScope{}, err
			}
			resolved = ResolvedImportScope{
				Type:            "conversation",
				ConversationID:  conversation.ConversationID,
				ScopeGeneration: conversation.ScopeGeneration,
			}
		case "global":
			resolved = ResolvedImportScope{Type: "global", ScopeGeneration: 1}
		case "skip":
			resolved = ResolvedImportScope{Skipped: true}
		case "project":
			if mapping.ProjectID != "" {
				project, err := repo.ResolveImportProject(ctx, mapping.ProjectID)
				if err != nil {
					return ResolvedImportScope{}, err
				}
				resolved = ResolvedImportScope{
					Type:            "project",
					ProjectID:       project.ID,
					ScopeGeneration: project.ScopeGeneration,
				}
			} else {
				project := projects[mapping.ProjectRef]
				if project.mode == "required" || project.mode == "" {
					return ResolvedImportScope{}, nil
				}
				resolved = project.scope
			}
		}
		conversations[scope.ConversationRef] = resolved
		return resolved, nil
	default:
		return ResolvedImportScope{}, validation(
			"MEMORY_PACKAGE_SCOPE_INVALID", "memory package scope is invalid",
		)
	}
}

func resolvedScopeForRecord(
	record PortableMemoryRecord,
	projects map[string]resolvedProjectMapping,
	conversations map[string]ResolvedImportScope,
) (ResolvedImportScope, error) {
	switch record.Scope.Type {
	case "global":
		return ResolvedImportScope{Type: "global", ScopeGeneration: 1}, nil
	case "project":
		return projects[record.Scope.ProjectRef].scope, nil
	case "conversation":
		if scope, ok := conversations[record.Scope.ConversationRef]; ok {
			return scope, nil
		}
		return ResolvedImportScope{}, errors.New(
			"memory import conversation scope was not resolved",
		)
	default:
		return ResolvedImportScope{}, errors.New("memory import scope is invalid")
	}
}

func resolvedScopeForPortableScope(
	scope PortableMemoryScope,
	projects map[string]resolvedProjectMapping,
	conversations map[string]ResolvedImportScope,
) (ResolvedImportScope, error) {
	switch scope.Type {
	case "global":
		return ResolvedImportScope{Type: "global", ScopeGeneration: 1}, nil
	case "project":
		resolved := projects[scope.ProjectRef].scope
		if resolved.Type == "" || resolved.Skipped {
			return ResolvedImportScope{}, errors.New("memory import revision project scope was not resolved")
		}
		return resolved, nil
	case "conversation":
		resolved, ok := conversations[scope.ConversationRef]
		if !ok || resolved.Type == "" || resolved.Skipped {
			return ResolvedImportScope{}, errors.New("memory import revision conversation scope was not resolved")
		}
		return resolved, nil
	default:
		return ResolvedImportScope{}, errors.New("memory import revision scope is invalid")
	}
}

func (s *Service) portabilityRepository() (PortabilityRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(PortabilityRepository)
	if !ok {
		return nil, ErrPortabilityRepositoryRequired
	}
	return repo, nil
}

func importPlanStaleError() error {
	return validation(
		"MEMORY_IMPORT_PLAN_STALE",
		"memory import package, mappings, or current Memory state changed; run dry-run again",
	)
}
