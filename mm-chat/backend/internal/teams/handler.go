package teams

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	contentTypeJSON      = "application/json; charset=utf-8"
	maxTeamRequestBytes  = 8 << 10
	teamsPath            = "/v1/teams"
	teamsPathBase        = teamsPath + "/"
	timeFormatRFC3339UTC = "2006-01-02T15:04:05.999999999Z07:00"
)

type Handler struct {
	service *Service
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type teamMembershipDTO struct {
	TeamRole  string `json:"teamRole"`
	Status    string `json:"status"`
	JoinedAt  string `json:"joinedAt"`
	UpdatedAt string `json:"updatedAt"`
}

type teamDTO struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	MembershipRevision int64             `json:"membershipRevision"`
	MyMembership       teamMembershipDTO `json:"myMembership"`
	CreatedAt          string            `json:"createdAt"`
	UpdatedAt          string            `json:"updatedAt"`
}

type teamMemberDTO struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	TeamRole    string `json:"teamRole"`
	Status      string `json:"status"`
	JoinedAt    string `json:"joinedAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type teamInviteDTO struct {
	ID             string `json:"id"`
	TeamID         string `json:"teamId"`
	MaskedEmail    string `json:"maskedEmail"`
	TeamRole       string `json:"teamRole"`
	Status         string `json:"status"`
	DeliveryStatus string `json:"deliveryStatus"`
	ExpiresAt      string `json:"expiresAt"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type apiPageDTO[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type pageQuery struct {
	Cursor string
	Limit  int
}

type forbiddenIdentityFieldError struct {
	field string
}

func (e *forbiddenIdentityFieldError) Error() string {
	return fmt.Sprintf("forbidden identity field %q", e.field)
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		writeServiceError(w, invalidTeamPayload("query parameters are invalid"))
		return
	}
	switch {
	case r.URL.Path == teamsPath:
		h.handleTeamsCollection(w, r)
	case strings.HasPrefix(r.URL.Path, teamsPathBase):
		h.handleTeamChild(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleTeamsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		query, err := parseListQuery(r.URL.Query())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		page, err := h.service.ListTeams(r.Context(), ListTeamsInput{
			Cursor: query.Cursor,
			Limit:  query.Limit,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newTeamPageDTO(page))
	case http.MethodPost:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input CreateTeamInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeDecodeError(w, err, ErrorCodeInvalidTeamPayload)
			return
		}
		team, err := h.service.CreateTeam(r.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, newTeamDTO(team))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleTeamChild(w http.ResponseWriter, r *http.Request) {
	route, ok := parseTeamRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	switch route.kind {
	case routeKindTeam:
		h.handleTeam(w, r, route.teamID)
	case routeKindMembers:
		h.handleMembers(w, r, route.teamID)
	case routeKindMembership:
		h.handleMembership(w, r, route.teamID)
	case routeKindMember:
		h.handleMember(w, r, route.teamID, route.resourceID)
	case routeKindInvites:
		h.handleInvites(w, r, route.teamID)
	case routeKindInvite:
		h.handleInvite(w, r, route.teamID, route.resourceID)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleTeam(w http.ResponseWriter, r *http.Request, teamID string) {
	switch r.Method {
	case http.MethodGet:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		team, err := h.service.GetTeam(r.Context(), teamID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newTeamDTO(team))
	case http.MethodPatch:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input RenameTeamInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeDecodeError(w, err, ErrorCodeInvalidTeamPayload)
			return
		}
		team, err := h.service.RenameTeam(r.Context(), teamID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newTeamDTO(team))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
	}
}

func (h *Handler) handleMembers(w http.ResponseWriter, r *http.Request, teamID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	query, err := parseListQuery(r.URL.Query())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	page, err := h.service.ListMembers(r.Context(), teamID, ListTeamMembersInput{
		Cursor: query.Cursor,
		Limit:  query.Limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newTeamMemberPageDTO(page))
}

func (h *Handler) handleMembership(w http.ResponseWriter, r *http.Request, teamID string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if err := requireNoQuery(r.URL.Query()); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := h.service.LeaveTeam(r.Context(), teamID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeNoContent(w, http.StatusNoContent)
}

func (h *Handler) handleMember(
	w http.ResponseWriter,
	r *http.Request,
	teamID string,
	userID string,
) {
	switch r.Method {
	case http.MethodPatch:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input UpdateTeamMemberInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeDecodeError(w, err, ErrorCodeInvalidMembershipPayload)
			return
		}
		member, err := h.service.UpdateMember(r.Context(), teamID, userID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newTeamMemberDTO(member))
	case http.MethodDelete:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := h.service.RemoveMember(r.Context(), teamID, userID); err != nil {
			writeServiceError(w, err)
			return
		}
		writeNoContent(w, http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) handleInvites(w http.ResponseWriter, r *http.Request, teamID string) {
	switch r.Method {
	case http.MethodGet:
		query, err := parseListQuery(r.URL.Query())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		page, err := h.service.ListInvites(r.Context(), teamID, ListTeamInvitesInput{
			Cursor: query.Cursor,
			Limit:  query.Limit,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newTeamInvitePageDTO(page))
	case http.MethodPost:
		if err := requireNoQuery(r.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input CreateTeamInviteInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeDecodeError(w, err, ErrorCodeInvalidInvitePayload)
			return
		}
		invite, err := h.service.CreateInvite(r.Context(), teamID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, newTeamInviteDTO(invite))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleInvite(
	w http.ResponseWriter,
	r *http.Request,
	teamID string,
	inviteID string,
) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w, http.MethodDelete)
		return
	}
	if err := requireNoQuery(r.URL.Query()); err != nil {
		writeServiceError(w, err)
		return
	}
	if err := h.service.RevokeInvite(r.Context(), teamID, inviteID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeNoContent(w, http.StatusNoContent)
}

type routeKind int

const (
	routeKindUnknown routeKind = iota
	routeKindTeam
	routeKindMembers
	routeKindMembership
	routeKindMember
	routeKindInvites
	routeKindInvite
)

type teamRoute struct {
	kind       routeKind
	teamID     string
	resourceID string
}

func parseTeamRoute(routePath string) (teamRoute, bool) {
	remainder := strings.TrimPrefix(routePath, teamsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) == 0 || parts[0] == "" {
		return teamRoute{}, false
	}
	route := teamRoute{teamID: parts[0]}
	switch len(parts) {
	case 1:
		route.kind = routeKindTeam
		return route, true
	case 2:
		if parts[1] == "" {
			return teamRoute{}, false
		}
		switch parts[1] {
		case "members":
			route.kind = routeKindMembers
			return route, true
		case "membership":
			route.kind = routeKindMembership
			return route, true
		case "invites":
			route.kind = routeKindInvites
			return route, true
		default:
			return teamRoute{}, false
		}
	case 3:
		if parts[1] == "" || parts[2] == "" {
			return teamRoute{}, false
		}
		route.resourceID = parts[2]
		switch parts[1] {
		case "members":
			route.kind = routeKindMember
			return route, true
		case "invites":
			route.kind = routeKindInvite
			return route, true
		default:
			return teamRoute{}, false
		}
	default:
		return teamRoute{}, false
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxTeamRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return classifyDecodeError(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return classifyDecodeError(err)
	}
	return nil
}

func classifyDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return maxBytesErr
	}
	const unknownFieldPrefix = "json: unknown field \""
	message := err.Error()
	if strings.HasPrefix(message, unknownFieldPrefix) && strings.HasSuffix(message, "\"") {
		field := strings.TrimSuffix(strings.TrimPrefix(message, unknownFieldPrefix), "\"")
		if isForbiddenIdentityField(field) {
			return &forbiddenIdentityFieldError{field: field}
		}
	}
	return err
}

func isForbiddenIdentityField(field string) bool {
	normalized := normalizeIdentityField(field)
	if normalized == "" {
		return false
	}
	if normalized == "role" {
		return true
	}
	if strings.HasSuffix(normalized, "role") && normalized != "teamrole" {
		return true
	}
	if strings.Contains(normalized, "acl") ||
		strings.Contains(normalized, "revision") ||
		strings.Contains(normalized, "epoch") {
		return true
	}
	switch normalized {
	case "actor",
		"actorid",
		"actoruserid",
		"allowedcollectionids",
		"callerid",
		"calleruserid",
		"currentuserid",
		"teamid",
		"teamids",
		"userid",
		"inviteid",
		"memberid",
		"ownerid",
		"targetuserid",
		"owneruserid",
		"principalid",
		"sessionid",
		"subjectuserid",
		"createdbyuserid",
		"invitedbyuserid",
		"impersonate",
		"impersonation",
		"impersonateuserid":
		return true
	default:
		return false
	}
}

func normalizeIdentityField(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(field))
	for _, r := range field {
		switch r {
		case '_', '-', ' ', '\t', '\n', '\r':
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return strings.ToLower(builder.String())
}

func requireNoQuery(query url.Values) error {
	if len(query) == 0 {
		return nil
	}
	for key := range query {
		if isForbiddenIdentityField(key) {
			return forbiddenIdentityPayload()
		}
	}
	return invalidTeamPayload("query parameters are invalid")
}

func parseListQuery(query url.Values) (pageQuery, error) {
	if len(query) == 0 {
		return pageQuery{}, nil
	}
	for key, values := range query {
		if isForbiddenIdentityField(key) {
			return pageQuery{}, forbiddenIdentityPayload()
		}
		if key != "cursor" && key != "limit" {
			return pageQuery{}, invalidTeamPayload("query parameters are invalid")
		}
		if len(values) != 1 {
			return pageQuery{}, invalidTeamPayload("query parameters are invalid")
		}
	}
	parsed := pageQuery{}
	if values, ok := query["cursor"]; ok {
		parsed.Cursor = values[0]
	}
	if values, ok := query["limit"]; ok {
		limit, err := strconv.Atoi(strings.TrimSpace(values[0]))
		if err != nil {
			return pageQuery{}, invalidTeamPayload("query parameters are invalid")
		}
		parsed.Limit = limit
	}
	return parsed, nil
}

func newTeamMembershipDTO(value TeamMembership) teamMembershipDTO {
	return teamMembershipDTO{
		TeamRole:  strings.TrimSpace(value.TeamRole),
		Status:    strings.TrimSpace(value.Status),
		JoinedAt:  formatTimeUTC(value.JoinedAt),
		UpdatedAt: formatTimeUTC(value.UpdatedAt),
	}
}

func newTeamDTO(value Team) teamDTO {
	return teamDTO{
		ID:                 strings.TrimSpace(value.ID),
		Name:               strings.TrimSpace(value.Name),
		MembershipRevision: value.MembershipRevision,
		MyMembership:       newTeamMembershipDTO(value.MyMembership),
		CreatedAt:          formatTimeUTC(value.CreatedAt),
		UpdatedAt:          formatTimeUTC(value.UpdatedAt),
	}
}

func newTeamMemberDTO(value TeamMember) teamMemberDTO {
	return teamMemberDTO{
		UserID:      strings.TrimSpace(value.UserID),
		DisplayName: strings.TrimSpace(value.DisplayName),
		TeamRole:    strings.TrimSpace(value.TeamRole),
		Status:      strings.TrimSpace(value.Status),
		JoinedAt:    formatTimeUTC(value.JoinedAt),
		UpdatedAt:   formatTimeUTC(value.UpdatedAt),
	}
}

func newTeamInviteDTO(value TeamInvite) teamInviteDTO {
	return teamInviteDTO{
		ID:             strings.TrimSpace(value.ID),
		TeamID:         strings.TrimSpace(value.TeamID),
		MaskedEmail:    strings.TrimSpace(value.MaskedEmail),
		TeamRole:       strings.TrimSpace(value.TeamRole),
		Status:         strings.TrimSpace(value.Status),
		DeliveryStatus: strings.TrimSpace(value.DeliveryStatus),
		ExpiresAt:      formatTimeUTC(value.ExpiresAt),
		CreatedAt:      formatTimeUTC(value.CreatedAt),
		UpdatedAt:      formatTimeUTC(value.UpdatedAt),
	}
}

func newTeamPageDTO(page ApiPage[Team]) apiPageDTO[teamDTO] {
	items := make([]teamDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, newTeamDTO(item))
	}
	return apiPageDTO[teamDTO]{Items: items, NextCursor: page.NextCursor}
}

func newTeamMemberPageDTO(page ApiPage[TeamMember]) apiPageDTO[teamMemberDTO] {
	items := make([]teamMemberDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, newTeamMemberDTO(item))
	}
	return apiPageDTO[teamMemberDTO]{Items: items, NextCursor: page.NextCursor}
}

func newTeamInvitePageDTO(page ApiPage[TeamInvite]) apiPageDTO[teamInviteDTO] {
	items := make([]teamInviteDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, newTeamInviteDTO(item))
	}
	return apiPageDTO[teamInviteDTO]{Items: items, NextCursor: page.NextCursor}
}

func formatTimeUTC(value time.Time) string {
	return value.UTC().Format(timeFormatRFC3339UTC)
}

func writeDecodeError(w http.ResponseWriter, err error, invalidCode string) {
	var maxBytesErr *http.MaxBytesError
	var forbiddenErr *forbiddenIdentityFieldError
	switch {
	case errors.As(err, &maxBytesErr):
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
	case errors.As(err, &forbiddenErr):
		writeError(w, http.StatusBadRequest, ErrorCodeForbiddenIdentityField, "caller identity fields are not accepted")
	default:
		writeError(w, http.StatusBadRequest, invalidCode, invalidPayloadMessage(invalidCode))
	}
}

func invalidPayloadMessage(code string) string {
	switch strings.TrimSpace(code) {
	case ErrorCodeInvalidInvitePayload:
		return "invite request body is invalid"
	case ErrorCodeInvalidMembershipPayload:
		return "membership request body is invalid"
	default:
		return "team request body is invalid"
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	switch {
	case errors.Is(err, ErrDatabaseRequired):
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for team endpoints"}
	case errors.Is(err, ErrCursorCodecRequired):
		return http.StatusServiceUnavailable, ErrorBody{Code: "CURSOR_CODEC_REQUIRED", Message: "cursor codec is required for team list endpoints"}
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized, ErrorBody{Code: "UNAUTHENTICATED", Message: "session is invalid or expired"}
	case errors.Is(err, ErrTeamAdminRequired):
		return http.StatusForbidden, ErrorBody{Code: "TEAM_ADMIN_REQUIRED", Message: "team admin role is required"}
	case errors.Is(err, ErrTeamNotFound):
		return http.StatusNotFound, ErrorBody{Code: "TEAM_NOT_FOUND", Message: "team not found"}
	case errors.Is(err, ErrTeamMemberNotFound):
		return http.StatusNotFound, ErrorBody{Code: "TEAM_MEMBER_NOT_FOUND", Message: "team member not found"}
	case errors.Is(err, ErrInviteNotFound):
		return http.StatusNotFound, ErrorBody{Code: "INVITE_NOT_FOUND", Message: "invite not found"}
	case errors.Is(err, ErrLastTeamAdmin):
		return http.StatusConflict, ErrorBody{Code: "LAST_TEAM_ADMIN", Message: "team must retain at least one active admin"}
	case errors.Is(err, ErrInviteConflict):
		return http.StatusConflict, ErrorBody{Code: "INVITE_CONFLICT", Message: "active membership or pending invite already exists"}
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, ErrorBody{Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key conflicts with an existing request"}
	case errors.Is(err, ErrInviteNotActive):
		return http.StatusGone, ErrorBody{Code: "INVITE_NOT_ACTIVE", Message: "invite is not active"}
	case errors.Is(err, ErrInviteDeliveryUnavailable):
		return http.StatusServiceUnavailable, ErrorBody{Code: "INVITE_DELIVERY_UNAVAILABLE", Message: "invite delivery is unavailable"}
	}
	var validationErr ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, ErrorBody{Code: validationErr.Code, Message: validationErr.Message}
	}
	return http.StatusInternalServerError, ErrorBody{Code: "INTERNAL_ERROR", Message: "internal server error"}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeNoContent(w http.ResponseWriter, status int) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
