package chat

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/localskills"
)

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
			"by file_read. By default the old text must occur exactly once.",
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
	case localFileReadToolName:
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
	case localFileWriteToolName:
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
		return workspaceToolResult(call, result, err)
	case localFileEditToolName:
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
		return workspaceToolResult(call, result, err)
	case localFileSearchToolName:
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
	default:
		return localSkillFailureResult(call, "tool_not_available"), "tool_not_available", nil
	}
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
