package usermemory

import (
	"net/http"
	"strings"
)

type projectRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	ExpectedRevision int64  `json:"expectedRevision"`
	LifecycleStatus  string `json:"lifecycleStatus"`
}

type governanceMemoryRequest struct {
	Type             string   `json:"type"`
	Content          string   `json:"content"`
	Importance       int      `json:"importance"`
	Tags             []string `json:"tags"`
	ExpectedRevision int64    `json:"expectedRevision"`
	ScopeType        string   `json:"scopeType"`
	ProjectID        string   `json:"projectId"`
	ConversationID   string   `json:"conversationId"`
	Sensitivity      string   `json:"sensitivity"`
}

type governanceMemoryDeleteRequest struct {
	ExpectedRevision int64 `json:"expectedRevision"`
}

type memoryReviewDecisionRequest struct {
	Decision      string `json:"decision"`
	EditedContent string `json:"editedContent"`
}

type l2SceneEnabledRequest struct {
	ExpectedRevision int64 `json:"expectedRevision"`
	Enabled          *bool `json:"enabled"`
}

func (h *Handler) handleGovernance(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.URL.Path == memoryGovernancePath {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		snapshot, err := h.service.GovernanceSnapshot(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}

	remainder := strings.TrimPrefix(r.URL.Path, memoryGovernancePathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) == 2 && parts[0] == "scenes" && parts[1] == "rebuild" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := h.service.RebuildGovernanceL2Scenes(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if len(parts) == 3 && parts[0] == "scenes" && parts[1] != "" {
		switch parts[2] {
		case "details":
			if r.Method != http.MethodGet {
				methodNotAllowed(w, http.MethodGet)
				return
			}
			detail, err := h.service.GovernanceL2SceneDetail(r.Context(), parts[1])
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, detail)
			return
		case "enabled":
			if r.Method != http.MethodPost {
				methodNotAllowed(w, http.MethodPost)
				return
			}
			var request l2SceneEnabledRequest
			if err := decodeJSON(r, &request); err != nil {
				writeDecodeError(w, err)
				return
			}
			if request.Enabled == nil {
				writeServiceError(w, validation(
					"INVALID_MEMORY_L2_SCENE_ENABLED",
					"memory L2 Scene enabled value is required",
				))
				return
			}
			scene, err := h.service.SetGovernanceL2SceneEnabled(
				r.Context(),
				L2SceneEnabledInput{
					SceneID: parts[1], ExpectedRevision: request.ExpectedRevision,
					Enabled: *request.Enabled,
				},
			)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, scene)
			return
		case "rebuild":
			if r.Method != http.MethodPost {
				methodNotAllowed(w, http.MethodPost)
				return
			}
			result, err := h.service.RebuildGovernanceL2Scene(r.Context(), parts[1])
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
	}
	if len(parts) == 1 && parts[0] == "memories" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var request governanceMemoryRequest
		if err := decodeJSON(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		memory, err := h.service.CreateGovernanceMemory(r.Context(), request.mutationInput(""))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, memory)
		return
	}
	if len(parts) == 3 && parts[0] == "memories" && parts[1] != "" && parts[2] == "details" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		detail, err := h.service.GovernanceMemoryDetail(r.Context(), parts[1])
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}
	if len(parts) != 2 || parts[0] != "memories" || parts[1] == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request governanceMemoryRequest
		if err := decodeJSON(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		memory, err := h.service.UpdateGovernanceMemory(r.Context(), request.mutationInput(parts[1]))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memory)
	case http.MethodDelete:
		var request governanceMemoryDeleteRequest
		if err := decodeJSON(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		progress, err := h.service.DeleteGovernanceMemory(r.Context(), parts[1], request.ExpectedRevision)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, progress)
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) handleProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.URL.Path == memoryProjectsPath {
		switch r.Method {
		case http.MethodGet:
			projects, err := h.service.ListProjects(r.Context())
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, Page[MemoryProject]{Items: projects})
		case http.MethodPost:
			var request projectRequest
			if err := decodeJSON(r, &request); err != nil {
				writeDecodeError(w, err)
				return
			}
			project, err := h.service.CreateProject(r.Context(), request.Name, request.Description)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, project)
		default:
			methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
		}
		return
	}
	projectID := strings.TrimPrefix(r.URL.Path, memoryProjectsPathBase)
	if projectID == "" || strings.Contains(projectID, "/") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w, http.MethodPatch)
		return
	}
	var request projectRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	project, err := h.service.UpdateProject(r.Context(), UpdateProjectInput{
		ProjectID: projectID, ExpectedRevision: request.ExpectedRevision,
		Name: request.Name, Description: request.Description,
		LifecycleStatus: request.LifecycleStatus,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) handleReviews(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.URL.Path == memoryReviewsPath {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		snapshot, err := h.service.GovernanceSnapshot(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, Page[MemoryReviewSuggestion]{Items: snapshot.Reviews})
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, memoryReviewsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "decision" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request memoryReviewDecisionRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	result, err := h.service.DecideMemoryReview(r.Context(), MemoryReviewDecisionInput{
		SuggestionID: parts[0], Decision: request.Decision,
		EditedContent: request.EditedContent,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (request governanceMemoryRequest) mutationInput(memoryID string) GovernanceMemoryMutationInput {
	return GovernanceMemoryMutationInput{
		MemoryID: memoryID, ExpectedRevision: request.ExpectedRevision,
		Candidate: Candidate{Type: request.Type, Content: request.Content,
			Importance: request.Importance, Tags: request.Tags},
		ScopeType: request.ScopeType, ProjectID: request.ProjectID,
		ConversationID: request.ConversationID, Sensitivity: request.Sensitivity,
	}
}
