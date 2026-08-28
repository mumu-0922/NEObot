package skillsupply

import (
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (handler *Handler) handleMarketplace(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	for key := range request.URL.Query() {
		if key != "q" && key != "category" && key != "locale" && key != "sort" &&
			key != "page" && key != "pageSize" {
			writeSkillError(
				writer, http.StatusBadRequest,
				"INVALID_SKILL_MARKETPLACE_QUERY", "Skill Marketplace query is invalid",
			)
			return
		}
	}
	page, pageOK := skillQueryInt(request, "page", 1)
	pageSize, pageSizeOK := skillQueryInt(request, "pageSize", 20)
	if !pageOK || !pageSizeOK {
		writeSkillError(
			writer, http.StatusBadRequest,
			"INVALID_SKILL_MARKETPLACE_QUERY", "Skill Marketplace query is invalid",
		)
		return
	}
	result, err := handler.service.SearchMarketplace(request.Context(), MarketplaceSearchInput{
		Query: request.URL.Query().Get("q"), Category: request.URL.Query().Get("category"),
		Locale: request.URL.Query().Get("locale"), Sort: request.URL.Query().Get("sort"),
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusOK, result)
}

func (handler *Handler) handleMarketplaceItem(
	writer http.ResponseWriter,
	request *http.Request,
	suffix string,
) {
	parts := strings.Split(suffix, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	identifier := parts[0]
	user := auth.UserOrDevelopment(request.Context())
	if len(parts) == 1 {
		handler.handleMarketplaceDetail(writer, request, user.ID, identifier)
		return
	}
	if parts[1] != "install" || request.URL.RawQuery != "" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		Version string `json:"version"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	installation, err := handler.service.InstallMarketplaceSkill(
		request.Context(), user.ID, identifier, input.Version,
	)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusCreated, map[string]any{"skill": installation})
}

func (handler *Handler) handleMarketplaceDetail(
	writer http.ResponseWriter,
	request *http.Request,
	userID string,
	identifier string,
) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	for key := range request.URL.Query() {
		if key != "version" && key != "locale" {
			writeSkillError(
				writer, http.StatusBadRequest,
				"INVALID_SKILL_MARKETPLACE_ITEM", "Skill Marketplace item is invalid",
			)
			return
		}
	}
	detail, err := handler.service.GetMarketplaceSkillLocalized(
		request.Context(), userID, identifier, request.URL.Query().Get("version"),
		request.URL.Query().Get("locale"),
	)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusOK, map[string]any{"skill": detail})
}

func (handler *Handler) handleDirectInstall(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.RawQuery != "" {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer, http.MethodPost)
		} else {
			writeSkillError(
				writer, http.StatusBadRequest,
				"INVALID_DIRECT_SKILL_INSTALL", "direct Skill install is invalid",
			)
		}
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	installation, err := handler.service.InstallSkillLink(request.Context(), user.ID, input.URL)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusCreated, map[string]any{"skill": installation})
}
