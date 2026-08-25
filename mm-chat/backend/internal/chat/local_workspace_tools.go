package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"path"
	"strings"

	"neo-chat/mm-chat/backend/internal/localskills"
)

type WorkspaceFileReference struct {
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
	FileName    string `json:"fileName"`
	MimeType    string `json:"mimeType"`
	Size        int64  `json:"size"`
	Version     string `json:"version"`
}

func workspacePublishFileDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localPublishFileToolName,
		Description: "Publish one final workspace file as an authenticated chat download. " +
			"Call this only when an attachment is explicitly needed. Repeating the same " +
			"unchanged path returns the existing artifact instead of creating a duplicate.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"path", "displayName", "contentType"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"displayName": map[string]any{
					"type": []string{"string", "null"}, "minLength": 1, "maxLength": 255,
				},
				"contentType": map[string]any{
					"type": []string{"string", "null"}, "minLength": 1, "maxLength": 255,
				},
			},
		},
		Strict: true,
	}}
}

func workspaceFileReadDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localFileReadToolName,
		Description: "Read one bounded UTF-8 window from a workspace-relative file and return " +
			"the SHA-256 version of the complete file. Read before every write or edit.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"path", "offset", "limit"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"offset": map[string]any{
					"type": []string{"integer", "null"}, "minimum": 0,
					"maximum": localskills.MaxWorkspaceFileBytes,
				},
				"limit": map[string]any{
					"type": []string{"integer", "null"}, "minimum": 1,
					"maximum": localskills.MaxWorkspaceReadWindowBytes,
				},
			},
		},
		Strict: true,
	}}
}

func workspaceFileWriteDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localFileWriteToolName,
		Description: "Atomically create or replace one UTF-8 workspace file with version " +
			"protection. expectedVersion is mandatory; use 'absent' only for a new file.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"path", "content", "expectedVersion"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"content": map[string]any{
					"type": "string", "maxLength": localskills.MaxWorkspaceWriteBytes,
				},
				"expectedVersion": map[string]any{
					"type": "string", "minLength": 6, "maxLength": 71,
				},
			},
		},
		Strict: true,
	}}
}

func workspaceFileEditDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localFileEditToolName,
		Description: "Replace exact UTF-8 text in a workspace file using the version returned " +
			"by read. By default the old text must occur exactly once.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{
				"path", "oldText", "newText", "replaceAll", "expectedVersion",
			},
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
				"oldText": map[string]any{
					"type": "string", "minLength": 1,
					"maxLength": localskills.MaxWorkspaceWriteBytes,
				},
				"newText": map[string]any{
					"type": "string", "maxLength": localskills.MaxWorkspaceWriteBytes,
				},
				"replaceAll":      map[string]any{"type": "boolean"},
				"expectedVersion": map[string]any{"type": "string", "minLength": 71, "maxLength": 71},
			},
		},
		Strict: true,
	}}
}

func workspaceFileSearchDefinition() ToolDefinition {
	return ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: localFileSearchToolName,
		Description: "Search bounded UTF-8 workspace files for literal text. Generated dependency " +
			"trees and symlinks are skipped. glob may match a relative path or base name.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"path", "query", "glob", "maxResults"},
			"properties": map[string]any{
				"path": map[string]any{"type": []string{"string", "null"}, "maxLength": 4096},
				"query": map[string]any{
					"type": "string", "minLength": 1, "maxLength": 4096,
				},
				"glob": map[string]any{"type": []string{"string", "null"}, "maxLength": 512},
				"maxResults": map[string]any{
					"type": []string{"integer", "null"}, "minimum": 1,
					"maximum": localskills.MaxWorkspaceSearchResults,
				},
			},
		},
		Strict: true,
	}}
}

func (runtime *localSkillToolRuntime) executeWorkspaceToolCall(
	ctx context.Context,
	call ProviderToolCall,
) (ProviderToolResult, string, error) {
	switch call.Name {
	case localFileReadToolName, legacyFileReadToolName:
		var arguments struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		result, err := runtime.executor.ReadWorkspaceFile(ctx, localskills.FileReadRequest{
			Path: arguments.Path, Offset: arguments.Offset, Limit: arguments.Limit,
		})
		return workspaceToolResult(call, result, err)
	case localFileWriteToolName, legacyFileWriteToolName:
		var arguments struct {
			Path            string `json:"path"`
			Content         string `json:"content"`
			ExpectedVersion string `json:"expectedVersion"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		result, err := runtime.executor.WriteWorkspaceFile(ctx, localskills.FileWriteRequest{
			Path: arguments.Path, Content: arguments.Content,
			ExpectedVersion: arguments.ExpectedVersion,
		})
		if err == nil {
			runtime.recordWorkspaceFile(WorkspaceFileReference{
				Path: result.Path, FileName: path.Base(result.Path),
				MimeType: workspaceReferenceMIMEType(result.Path),
				Size:     int64(result.Size), Version: result.Version,
			})
		}
		return workspaceToolResult(call, result, err)
	case localFileEditToolName, legacyFileEditToolName:
		var arguments struct {
			Path            string `json:"path"`
			OldText         string `json:"oldText"`
			NewText         string `json:"newText"`
			ReplaceAll      bool   `json:"replaceAll"`
			ExpectedVersion string `json:"expectedVersion"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		result, err := runtime.executor.EditWorkspaceFile(ctx, localskills.FileEditRequest{
			Path: arguments.Path, OldText: arguments.OldText, NewText: arguments.NewText,
			ReplaceAll: arguments.ReplaceAll, ExpectedVersion: arguments.ExpectedVersion,
		})
		if err == nil {
			runtime.recordWorkspaceFile(WorkspaceFileReference{
				Path: result.Path, FileName: path.Base(result.Path),
				MimeType: workspaceReferenceMIMEType(result.Path),
				Size:     int64(result.Size), Version: result.Version,
			})
		}
		return workspaceToolResult(call, result, err)
	case localFileSearchToolName, legacyFileSearchToolName:
		var arguments struct {
			Path       string `json:"path"`
			Query      string `json:"query"`
			Glob       string `json:"glob"`
			MaxResults int    `json:"maxResults"`
		}
		if !decodeStrictToolArguments(call.Arguments, &arguments) {
			return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
		}
		result, err := runtime.executor.SearchWorkspaceFiles(ctx, localskills.FileSearchRequest{
			Path: arguments.Path, Query: arguments.Query, Glob: arguments.Glob,
			MaxResults: arguments.MaxResults,
		})
		return workspaceToolResult(call, result, err)
	case localPublishFileToolName:
		return runtime.executePublishFileToolCall(ctx, call)
	default:
		return localSkillFailureResult(call, "tool_not_available"), "tool_not_available", nil
	}
}

func (runtime *localSkillToolRuntime) captureWorkspaceFiles(
	ctx context.Context,
	paths []string,
) ([]WorkspaceFileReference, error) {
	if runtime == nil || runtime.workspaceID == "" || len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > maxWorkspaceFileReferences {
		return nil, localskills.ErrWorkspaceInvalidInput
	}
	seen := make(map[string]struct{}, len(paths))
	references := make([]WorkspaceFileReference, 0, len(paths))
	for _, filePath := range paths {
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			return nil, localskills.ErrWorkspaceInvalidInput
		}
		snapshot, err := runtime.executor.ReadWorkspaceArtifact(
			ctx, filePath, maxWorkspaceReferenceBytes,
		)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[snapshot.Path]; duplicate {
			continue
		}
		seen[snapshot.Path] = struct{}{}
		reference := WorkspaceFileReference{
			WorkspaceID: runtime.workspaceID,
			Path:        snapshot.Path, FileName: path.Base(snapshot.Path),
			MimeType: workspaceReferenceMIMEType(snapshot.Path),
			Size:     int64(len(snapshot.Body)), Version: snapshot.Version,
		}
		runtime.recordWorkspaceFile(reference)
		references = append(references, reference)
	}
	return references, nil
}

func (runtime *localSkillToolRuntime) recordWorkspaceFile(reference WorkspaceFileReference) {
	if runtime == nil || runtime.workspaceID == "" {
		return
	}
	runtime.workspaceFilesMu.Lock()
	defer runtime.workspaceFilesMu.Unlock()
	reference.WorkspaceID = runtime.workspaceID
	reference.Path = strings.TrimSpace(reference.Path)
	if reference.Path == "" || reference.Version == "" {
		return
	}
	if reference.FileName == "" {
		reference.FileName = path.Base(reference.Path)
	}
	if reference.MimeType == "" {
		reference.MimeType = workspaceReferenceMIMEType(reference.Path)
	}
	if index, exists := runtime.workspaceFileIndex[reference.Path]; exists {
		runtime.workspaceFiles[index] = reference
		return
	}
	if len(runtime.workspaceFiles) >= maxWorkspaceFileReferences {
		return
	}
	runtime.workspaceFileIndex[reference.Path] = len(runtime.workspaceFiles)
	runtime.workspaceFiles = append(runtime.workspaceFiles, reference)
}

func (runtime *localSkillToolRuntime) workspaceFileOutputBlocks(messageID string) []any {
	if runtime == nil {
		return nil
	}
	runtime.workspaceFilesMu.Lock()
	defer runtime.workspaceFilesMu.Unlock()
	if len(runtime.workspaceFiles) == 0 {
		return nil
	}
	blocks := make([]any, 0, len(runtime.workspaceFiles))
	for _, reference := range runtime.workspaceFiles {
		digest := sha256.Sum256([]byte(reference.WorkspaceID + "\x00" + reference.Path))
		blocks = append(blocks, map[string]any{
			"id":   strings.TrimSpace(messageID) + "-workspace-file-" + hex.EncodeToString(digest[:8]),
			"type": "workspace_file", "workspaceId": reference.WorkspaceID,
			"path": reference.Path, "fileName": reference.FileName,
			"mimeType": reference.MimeType, "size": reference.Size,
			"version": reference.Version,
		})
	}
	return blocks
}

func workspaceReferenceMIMEType(filePath string) string {
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(filePath)))
	if contentType == "" {
		return "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return "application/octet-stream"
	}
	return mediaType
}

func (runtime *localSkillToolRuntime) executePublishFileToolCall(
	ctx context.Context,
	call ProviderToolCall,
) (ProviderToolResult, string, error) {
	var arguments struct {
		Path        string  `json:"path"`
		DisplayName *string `json:"displayName"`
		ContentType *string `json:"contentType"`
	}
	if !decodeStrictToolArguments(call.Arguments, &arguments) ||
		!runtime.artifactPublishingAvailable() {
		return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
	}
	snapshot, err := runtime.executor.ReadWorkspaceArtifact(
		ctx, arguments.Path, runtime.artifactMaxBytes,
	)
	if err != nil {
		return workspaceToolResult(call, nil, err)
	}
	if len(snapshot.Body) == 0 {
		return localSkillFailureResult(call, "empty_file"), "empty_file", nil
	}
	key := snapshot.Path + "\x00" + snapshot.Version
	if artifact, ok := runtime.publishedByVersion[key]; ok {
		return publishedArtifactToolResult(call, snapshot, artifact, true), "", nil
	}
	if len(runtime.publishedArtifacts) >= maxPublishedArtifactsPerTurn {
		return localSkillFailureResult(call, "artifact_count_exhausted"), "artifact_count_exhausted", nil
	}
	if int64(len(snapshot.Body)) > runtime.artifactMaxBytes-runtime.artifactBytes {
		return localSkillFailureResult(call, "artifact_bytes_exhausted"), "artifact_bytes_exhausted", nil
	}
	displayName, ok := normalizeArtifactDisplayName(snapshot.Path, arguments.DisplayName)
	if !ok {
		return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
	}
	contentType, ok := normalizeArtifactContentType(displayName, arguments.ContentType)
	if !ok {
		return localSkillFailureResult(call, "arguments_invalid"), "arguments_invalid", nil
	}
	artifact, err := runtime.artifactPublisher.PublishWorkspaceArtifact(
		ctx,
		WorkspaceArtifactPublishInput{
			ConversationID: runtime.jobScope.ConversationID,
			FileName:       displayName, MimeType: contentType, Body: snapshot.Body,
		},
	)
	if err != nil || strings.TrimSpace(artifact.FileID) == "" {
		return localSkillFailureResult(call, "publish_failed"), "publish_failed", nil
	}
	runtime.artifactBytes += int64(len(snapshot.Body))
	runtime.publishedArtifacts = append(runtime.publishedArtifacts, artifact)
	runtime.publishedByVersion[key] = artifact
	return publishedArtifactToolResult(call, snapshot, artifact, false), "", nil
}

func normalizeArtifactDisplayName(workspacePath string, supplied *string) (string, bool) {
	name := path.Base(workspacePath)
	if supplied != nil {
		name = strings.TrimSpace(*supplied)
	}
	if name == "" || name == "." || name == ".." || len(name) > 255 ||
		strings.ContainsAny(name, "/\\\x00") {
		return "", false
	}
	return name, true
}

func normalizeArtifactContentType(fileName string, supplied *string) (string, bool) {
	contentType := ""
	if supplied != nil {
		contentType = strings.TrimSpace(*supplied)
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(fileName))
	}
	if contentType == "" {
		return "application/octet-stream", true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" || len(mediaType) > 255 {
		return "", false
	}
	return mediaType, true
}

func publishedArtifactToolResult(
	call ProviderToolCall,
	snapshot localskills.WorkspaceArtifactSnapshot,
	artifact WorkspaceArtifact,
	alreadyPublished bool,
) ProviderToolResult {
	return localSkillSuccessResult(call, map[string]any{
		"result": map[string]any{
			"path": snapshot.Path, "version": snapshot.Version,
			"fileId": artifact.FileID, "fileName": artifact.FileName,
			"contentType": artifact.MimeType, "size": artifact.Size,
			"sha256": artifact.SHA256, "alreadyPublished": alreadyPublished,
		},
	})
}

func (runtime *localSkillToolRuntime) publishedAttachmentInputs() []AttachmentInput {
	if runtime == nil || len(runtime.publishedArtifacts) == 0 {
		return nil
	}
	attachments := make([]AttachmentInput, 0, len(runtime.publishedArtifacts))
	for _, artifact := range runtime.publishedArtifacts {
		attachments = append(attachments, AttachmentInput{
			Source: "server", FileID: artifact.FileID, Purpose: "output",
		})
	}
	return attachments
}

func (runtime *localSkillToolRuntime) discardUnlinkedPublishedArtifacts(
	ctx context.Context,
	linked []Attachment,
) {
	if runtime == nil || runtime.artifactPublisher == nil || len(runtime.publishedArtifacts) == 0 {
		return
	}
	linkedIDs := make(map[string]struct{}, len(linked))
	for _, attachment := range linked {
		linkedIDs[strings.TrimSpace(attachment.FileID)] = struct{}{}
	}
	kept := make([]WorkspaceArtifact, 0, len(runtime.publishedArtifacts))
	for _, artifact := range runtime.publishedArtifacts {
		if _, ok := linkedIDs[strings.TrimSpace(artifact.FileID)]; ok {
			kept = append(kept, artifact)
			continue
		}
		_ = runtime.artifactPublisher.DeleteWorkspaceArtifact(ctx, artifact.FileID)
	}
	runtime.publishedArtifacts = kept
}

func workspaceToolResult(
	call ProviderToolCall,
	payload any,
	err error,
) (ProviderToolResult, string, error) {
	if err == nil {
		return localSkillSuccessResult(call, map[string]any{"result": payload}), "", nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ProviderToolResult{}, "", err
	}
	category := workspaceToolFailureCategory(err)
	return localSkillFailureResult(call, category), category, nil
}

func workspaceToolFailureCategory(err error) string {
	switch {
	case errors.Is(err, localskills.ErrWorkspaceVersionConflict):
		return "version_conflict"
	case errors.Is(err, localskills.ErrWorkspaceFileNotFound):
		return "file_not_found"
	case errors.Is(err, localskills.ErrWorkspaceFileTooLarge):
		return "file_too_large"
	case errors.Is(err, localskills.ErrWorkspaceInvalidUTF8):
		return "invalid_utf8"
	case errors.Is(err, localskills.ErrWorkspaceEditConflict):
		return "edit_conflict"
	case errors.Is(err, localskills.ErrWorkspaceInvalidPath):
		return "path_invalid"
	case errors.Is(err, localskills.ErrWorkspaceInvalidInput):
		return "arguments_invalid"
	default:
		return "execution_failed"
	}
}
