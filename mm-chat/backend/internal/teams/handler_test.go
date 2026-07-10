package teams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

var handlerFixedNow = time.Date(2026, 7, 10, 12, 0, 0, 123456789, time.UTC)

func TestHandlerRouteMatrix(t *testing.T) {
	repo := &handlerRepo{}
	handler := newReadyHandler(t, repo)
	teamID := "44444444-4444-4444-8444-444444444444"
	userID := "55555555-5555-4555-8555-555555555555"
	inviteID := "66666666-6666-4666-8666-666666666666"

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantAllow  string
	}{
		{name: "teams get", method: http.MethodGet, path: teamsPath, wantStatus: http.StatusOK},
		{name: "teams post", method: http.MethodPost, path: teamsPath, body: `{"name":"Research","idempotencyKey":"create-1"}`, wantStatus: http.StatusCreated},
		{name: "teams patch", method: http.MethodPatch, path: teamsPath, wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet + ", " + http.MethodPost},
		{name: "team get", method: http.MethodGet, path: teamsPath + "/" + teamID, wantStatus: http.StatusOK},
		{name: "team patch", method: http.MethodPatch, path: teamsPath + "/" + teamID, body: `{"name":"Renamed"}`, wantStatus: http.StatusOK},
		{name: "team delete", method: http.MethodDelete, path: teamsPath + "/" + teamID, wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet + ", " + http.MethodPatch},
		{name: "members get", method: http.MethodGet, path: teamsPath + "/" + teamID + "/members", wantStatus: http.StatusOK},
		{name: "members post", method: http.MethodPost, path: teamsPath + "/" + teamID + "/members", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet},
		{name: "membership delete", method: http.MethodDelete, path: teamsPath + "/" + teamID + "/membership", wantStatus: http.StatusNoContent},
		{name: "membership get", method: http.MethodGet, path: teamsPath + "/" + teamID + "/membership", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodDelete},
		{name: "member patch", method: http.MethodPatch, path: teamsPath + "/" + teamID + "/members/" + userID, body: `{"teamRole":"member"}`, wantStatus: http.StatusOK},
		{name: "member delete", method: http.MethodDelete, path: teamsPath + "/" + teamID + "/members/" + userID, wantStatus: http.StatusNoContent},
		{name: "member get", method: http.MethodGet, path: teamsPath + "/" + teamID + "/members/" + userID, wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodPatch + ", " + http.MethodDelete},
		{name: "invites get", method: http.MethodGet, path: teamsPath + "/" + teamID + "/invites", wantStatus: http.StatusOK},
		{name: "invites post", method: http.MethodPost, path: teamsPath + "/" + teamID + "/invites", body: `{"email":"owner@example.test","teamRole":"member","idempotencyKey":"invite-1"}`, wantStatus: http.StatusCreated},
		{name: "invites patch", method: http.MethodPatch, path: teamsPath + "/" + teamID + "/invites", wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodGet + ", " + http.MethodPost},
		{name: "invite delete", method: http.MethodDelete, path: teamsPath + "/" + teamID + "/invites/" + inviteID, wantStatus: http.StatusNoContent},
		{name: "invite get", method: http.MethodGet, path: teamsPath + "/" + teamID + "/invites/" + inviteID, wantStatus: http.StatusMethodNotAllowed, wantAllow: http.MethodDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performTeamRequest(handler, tt.method, tt.path, tt.body, true)
			assertTeamStatus(t, rec, tt.wantStatus)
			if tt.wantStatus == http.StatusNoContent {
				if got := rec.Header().Get("Content-Type"); got != "" {
					t.Fatalf("204 Content-Type = %q, want empty", got)
				}
				if rec.Body.Len() != 0 {
					t.Fatalf("204 body = %q, want empty", rec.Body.String())
				}
			} else {
				assertJSONHeaders(t, rec)
			}
			if tt.wantAllow != "" && rec.Header().Get("Allow") != tt.wantAllow {
				t.Fatalf("Allow = %q, want %q", rec.Header().Get("Allow"), tt.wantAllow)
			}
		})
	}
}

func TestHandlerRoutesReturnSafeDTOsAndOpaqueCursor(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	createdAt := time.Date(2026, 7, 10, 20, 30, 40, 987654321, zone)
	updatedAt := createdAt.Add(2 * time.Hour)
	joinedAt := createdAt.Add(-24 * time.Hour)
	expiresAt := createdAt.Add(72 * time.Hour)
	teamID := "44444444-4444-4444-8444-444444444444"
	memberUserID := "55555555-5555-4555-8555-555555555555"
	inviteID := "66666666-6666-4666-8666-666666666666"

	repo := &handlerRepo{
		createTeamResult: Team{
			ID:   teamID,
			Name: "Research",
			MyMembership: TeamMembership{
				TeamRole:  TeamRoleAdmin,
				Status:    MembershipStatusActive,
				JoinedAt:  joinedAt,
				UpdatedAt: updatedAt,
			},
			MembershipRevision: 1,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		},
		listTeamsResult: TeamPageResult{
			Items: []Team{{
				ID:   teamID,
				Name: "Research",
				MyMembership: TeamMembership{
					TeamRole:  TeamRoleAdmin,
					Status:    MembershipStatusActive,
					JoinedAt:  joinedAt,
					UpdatedAt: updatedAt,
				},
				MembershipRevision: 2,
				CreatedAt:          createdAt,
				UpdatedAt:          updatedAt,
			}},
			HasMore: true,
		},
		getTeamResult: Team{
			ID:   teamID,
			Name: "Research",
			MyMembership: TeamMembership{
				TeamRole:  TeamRoleAdmin,
				Status:    MembershipStatusActive,
				JoinedAt:  joinedAt,
				UpdatedAt: updatedAt,
			},
			MembershipRevision: 2,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		},
		renameTeamResult: Team{
			ID:   teamID,
			Name: "Research Ops",
			MyMembership: TeamMembership{
				TeamRole:  TeamRoleAdmin,
				Status:    MembershipStatusActive,
				JoinedAt:  joinedAt,
				UpdatedAt: updatedAt,
			},
			MembershipRevision: 3,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		},
		listMembersResult: TeamMemberPageResult{
			Items: []TeamMember{{
				UserID:      memberUserID,
				DisplayName: "Operator",
				TeamRole:    TeamRoleMember,
				Status:      MembershipStatusActive,
				JoinedAt:    joinedAt,
				UpdatedAt:   updatedAt,
			}},
			HasMore: true,
		},
		updateMemberResult: TeamMember{
			UserID:      memberUserID,
			DisplayName: "Operator",
			TeamRole:    TeamRoleAdmin,
			Status:      MembershipStatusActive,
			JoinedAt:    joinedAt,
			UpdatedAt:   updatedAt,
		},
		createInviteResult: TeamInviteRecord{
			ID:             inviteID,
			TeamID:         teamID,
			Email:          "owner@example.test",
			TeamRole:       TeamRoleMember,
			Status:         InviteStatusPending,
			DeliveryStatus: InviteDeliverySent,
			ExpiresAt:      expiresAt,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		},
		listInvitesResult: TeamInvitePageResult{
			Items: []TeamInviteRecord{{
				ID:             inviteID,
				TeamID:         teamID,
				Email:          "owner@example.test",
				TeamRole:       TeamRoleMember,
				Status:         InviteStatusPending,
				DeliveryStatus: InviteDeliverySent,
				ExpiresAt:      expiresAt,
				CreatedAt:      createdAt,
				UpdatedAt:      updatedAt,
			}},
			HasMore: true,
		},
	}
	handler := newReadyHandler(t, repo)

	rec := performTeamRequest(handler, http.MethodPost, teamsPath,
		`{"name":" Research ","idempotencyKey":"create-key"}`,
		true,
	)
	assertTeamStatus(t, rec, http.StatusCreated)
	var created teamDTO
	decodeTeamBody(t, rec, &created)
	if created.ID != teamID || created.CreatedAt != createdAt.UTC().Format(timeFormatRFC3339UTC) {
		t.Fatalf("created team = %#v", created)
	}
	if repo.createTeamInput.Name != "Research" || repo.createTeamInput.IdempotencyKey != "create-key" {
		t.Fatalf("create team input = %#v", repo.createTeamInput)
	}

	rec = performTeamRequest(handler, http.MethodGet, teamsPath, "", true)
	assertTeamStatus(t, rec, http.StatusOK)
	var teamPage apiPageDTO[teamDTO]
	decodeTeamBody(t, rec, &teamPage)
	if len(teamPage.Items) != 1 || teamPage.Items[0].CreatedAt != createdAt.UTC().Format(timeFormatRFC3339UTC) {
		t.Fatalf("team page = %#v", teamPage)
	}
	if teamPage.NextCursor == "" {
		t.Fatal("nextCursor = empty, want opaque cursor")
	}
	if strings.Contains(teamPage.NextCursor, teamID) || strings.Contains(teamPage.NextCursor, createdAt.UTC().Format(timeFormatRFC3339UTC)) {
		t.Fatalf("nextCursor leaked raw pagination fields: %q", teamPage.NextCursor)
	}

	rec = performTeamRequest(handler, http.MethodGet,
		teamsPath+"?cursor="+url.QueryEscape(teamPage.NextCursor)+"&limit=1",
		"",
		true,
	)
	assertTeamStatus(t, rec, http.StatusOK)
	if repo.teamPageInput.Limit != 1 {
		t.Fatalf("list teams limit = %d, want 1", repo.teamPageInput.Limit)
	}
	if repo.teamPageInput.After == nil || repo.teamPageInput.After.ID != teamID || !repo.teamPageInput.After.CreatedAt.Equal(createdAt.UTC()) {
		t.Fatalf("list teams cursor = %#v", repo.teamPageInput.After)
	}

	rec = performTeamRequest(handler, http.MethodGet, teamsPath+"/"+teamID, "", true)
	assertTeamStatus(t, rec, http.StatusOK)
	var gotTeam teamDTO
	decodeTeamBody(t, rec, &gotTeam)
	if gotTeam.UpdatedAt != updatedAt.UTC().Format(timeFormatRFC3339UTC) || gotTeam.MyMembership.JoinedAt != joinedAt.UTC().Format(timeFormatRFC3339UTC) {
		t.Fatalf("get team dto = %#v", gotTeam)
	}

	rec = performTeamRequest(handler, http.MethodPatch, teamsPath+"/"+teamID, `{"name":" Research Ops "}`, true)
	assertTeamStatus(t, rec, http.StatusOK)
	var renamed teamDTO
	decodeTeamBody(t, rec, &renamed)
	if renamed.Name != "Research Ops" || repo.renameTeamInput.Name != "Research Ops" {
		t.Fatalf("rename team dto/input = %#v / %#v", renamed, repo.renameTeamInput)
	}

	rec = performTeamRequest(handler, http.MethodGet, teamsPath+"/"+teamID+"/members", "", true)
	assertTeamStatus(t, rec, http.StatusOK)
	var memberPage apiPageDTO[teamMemberDTO]
	decodeTeamBody(t, rec, &memberPage)
	if len(memberPage.Items) != 1 || memberPage.Items[0].JoinedAt != joinedAt.UTC().Format(timeFormatRFC3339UTC) {
		t.Fatalf("member page = %#v", memberPage)
	}

	rec = performTeamRequest(handler, http.MethodPatch, teamsPath+"/"+teamID+"/members/"+memberUserID, `{"teamRole":"admin"}`, true)
	assertTeamStatus(t, rec, http.StatusOK)
	var updatedMember teamMemberDTO
	decodeTeamBody(t, rec, &updatedMember)
	if updatedMember.TeamRole != TeamRoleAdmin || repo.updateMemberInput.TargetUserID != memberUserID {
		t.Fatalf("updated member dto/input = %#v / %#v", updatedMember, repo.updateMemberInput)
	}

	rec = performTeamRequest(handler, http.MethodPost, teamsPath+"/"+teamID+"/invites",
		`{"email":"OWNER@example.test","teamRole":"member","idempotencyKey":"invite-key"}`,
		true,
	)
	assertTeamStatus(t, rec, http.StatusCreated)
	if strings.Contains(rec.Body.String(), "owner@example.test") {
		t.Fatalf("invite response leaked raw email: %s", rec.Body.String())
	}
	var invite teamInviteDTO
	decodeTeamBody(t, rec, &invite)
	if invite.MaskedEmail != "o***@e***.test" || invite.ExpiresAt != expiresAt.UTC().Format(timeFormatRFC3339UTC) {
		t.Fatalf("invite dto = %#v", invite)
	}
	if repo.createInviteInput.Email != "owner@example.test" || repo.createInviteInput.TeamRole != TeamRoleMember {
		t.Fatalf("create invite input = %#v", repo.createInviteInput)
	}

	rec = performTeamRequest(handler, http.MethodGet, teamsPath+"/"+teamID+"/invites", "", true)
	assertTeamStatus(t, rec, http.StatusOK)
	var invitePage apiPageDTO[teamInviteDTO]
	decodeTeamBody(t, rec, &invitePage)
	if len(invitePage.Items) != 1 || invitePage.Items[0].MaskedEmail != "o***@e***.test" {
		t.Fatalf("invite page = %#v", invitePage)
	}
	if invitePage.NextCursor == "" {
		t.Fatal("invite nextCursor = empty, want opaque cursor")
	}
}

func TestHandlerRejectsStrictJSONAndIdentityHints(t *testing.T) {
	handler := newReadyHandler(t, &handlerRepo{})
	teamID := "44444444-4444-4444-8444-444444444444"
	userID := "55555555-5555-4555-8555-555555555555"

	oversized := `{"name":"` + strings.Repeat("a", maxTeamRequestBytes) + `"}`
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "oversized team body", method: http.MethodPost, path: teamsPath, body: oversized, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PAYLOAD_TOO_LARGE"},
		{name: "team unknown field", method: http.MethodPost, path: teamsPath, body: `{"name":"Research","idempotencyKey":"create-1","bogus":true}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidTeamPayload},
		{name: "team trailing object", method: http.MethodPatch, path: teamsPath + "/" + teamID, body: `{"name":"Research"}{}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidTeamPayload},
		{name: "team role wrong dto", method: http.MethodPost, path: teamsPath, body: `{"name":"Research","idempotencyKey":"create-1","teamRole":"admin"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidTeamPayload},
		{name: "team actor field forbidden", method: http.MethodPost, path: teamsPath, body: `{"name":"Research","idempotencyKey":"create-1","actorUserId":"bad"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
		{name: "team bare role forbidden", method: http.MethodPatch, path: teamsPath + "/" + teamID, body: `{"name":"Research","role":"admin"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
		{name: "team acl forbidden", method: http.MethodPatch, path: teamsPath + "/" + teamID, body: `{"name":"Research","acl":{}}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
		{name: "team revision forbidden", method: http.MethodPatch, path: teamsPath + "/" + teamID, body: `{"name":"Research","membershipRevision":7}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
		{name: "membership unknown field", method: http.MethodPatch, path: teamsPath + "/" + teamID + "/members/" + userID, body: `{"teamRole":"admin","displayName":"x"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidMembershipPayload},
		{name: "membership trailing array", method: http.MethodPatch, path: teamsPath + "/" + teamID + "/members/" + userID, body: `{"teamRole":"admin"}[]`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidMembershipPayload},
		{name: "membership user field forbidden", method: http.MethodPatch, path: teamsPath + "/" + teamID + "/members/" + userID, body: `{"teamRole":"admin","userId":"bad"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
		{name: "invite unknown field", method: http.MethodPost, path: teamsPath + "/" + teamID + "/invites", body: `{"email":"owner@example.test","teamRole":"member","idempotencyKey":"invite-1","bogus":true}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidInvitePayload},
		{name: "invite array body", method: http.MethodPost, path: teamsPath + "/" + teamID + "/invites", body: `[]`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeInvalidInvitePayload},
		{name: "invite team id forbidden", method: http.MethodPost, path: teamsPath + "/" + teamID + "/invites", body: `{"email":"owner@example.test","teamRole":"member","idempotencyKey":"invite-1","teamId":"bad"}`, wantStatus: http.StatusBadRequest, wantCode: ErrorCodeForbiddenIdentityField},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performTeamRequest(handler, tt.method, tt.path, tt.body, true)
			assertTeamStatus(t, rec, tt.wantStatus)
			assertJSONHeaders(t, rec)
			assertTeamErrorCode(t, rec, tt.wantCode)
		})
	}
}

func TestHandlerRejectsInvalidQueryParameters(t *testing.T) {
	handler := newReadyHandler(t, &handlerRepo{})
	teamID := "44444444-4444-4444-8444-444444444444"

	tests := []struct {
		name string
		path string
	}{
		{name: "teams limit parse", path: teamsPath + "?limit=oops"},
		{name: "teams duplicate limit", path: teamsPath + "?limit=1&limit=2"},
		{name: "teams duplicate cursor", path: teamsPath + "?cursor=a&cursor=b"},
		{name: "teams unknown query", path: teamsPath + "?unknown=1"},
		{name: "team route query forbidden", path: teamsPath + "/" + teamID + "?limit=1"},
		{name: "members route unknown query", path: teamsPath + "/" + teamID + "/members?cursor=a&extra=1"},
		{name: "invites route duplicate limit", path: teamsPath + "/" + teamID + "/invites?limit=1&limit=2"},
		{name: "create route query forbidden", path: teamsPath + "?cursor=x"},
		{name: "malformed semicolon query", path: teamsPath + "?limit=1;cursor=x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := http.MethodGet
			body := ""
			if tt.name == "create route query forbidden" {
				method = http.MethodPost
				body = `{"name":"Research","idempotencyKey":"create-1"}`
			}
			rec := performTeamRequest(handler, method, tt.path, body, true)
			assertTeamStatus(t, rec, http.StatusBadRequest)
			assertTeamErrorCode(t, rec, ErrorCodeInvalidTeamPayload)
		})
	}

	for name, path := range map[string]string{
		"teams owner identity": teamsPath + "?ownerId=bad",
		"members acl identity": teamsPath + "/" + teamID +
			"/members?allowedCollectionIds=bad",
	} {
		t.Run(name, func(t *testing.T) {
			rec := performTeamRequest(handler, http.MethodGet, path, "", true)
			assertTeamStatus(t, rec, http.StatusBadRequest)
			assertTeamErrorCode(t, rec, ErrorCodeForbiddenIdentityField)
		})
	}

	rec := performTeamRequest(
		handler,
		http.MethodPost,
		teamsPath+"?sessionId=bad",
		`{"name":"Research","idempotencyKey":"create-1"}`,
		true,
	)
	assertTeamStatus(t, rec, http.StatusBadRequest)
	assertTeamErrorCode(t, rec, ErrorCodeForbiddenIdentityField)
}

func TestHandlerServiceErrorMatrix(t *testing.T) {
	teamID := "44444444-4444-4444-8444-444444444444"
	userID := "55555555-5555-4555-8555-555555555555"
	inviteID := "66666666-6666-4666-8666-666666666666"

	tests := []struct {
		name       string
		handler    http.Handler
		method     string
		path       string
		body       string
		withUser   bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid team id",
			handler:    newReadyHandler(t, &handlerRepo{}),
			method:     http.MethodGet,
			path:       teamsPath + "/bad",
			withUser:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   ErrorCodeInvalidTeamPayload,
		},
		{
			name:       "invalid membership payload",
			handler:    newReadyHandler(t, &handlerRepo{}),
			method:     http.MethodPatch,
			path:       teamsPath + "/" + teamID + "/members/" + userID,
			body:       `{"teamRole":"owner"}`,
			withUser:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   ErrorCodeInvalidMembershipPayload,
		},
		{
			name:       "invalid invite payload",
			handler:    newReadyHandler(t, &handlerRepo{}),
			method:     http.MethodPost,
			path:       teamsPath + "/" + teamID + "/invites",
			body:       `{"email":"not-an-email","teamRole":"member","idempotencyKey":"invite-1"}`,
			withUser:   true,
			wantStatus: http.StatusBadRequest,
			wantCode:   ErrorCodeInvalidInvitePayload,
		},
		{
			name:       "unauthenticated",
			handler:    newReadyHandler(t, &handlerRepo{}),
			method:     http.MethodGet,
			path:       teamsPath + "/" + teamID,
			withUser:   false,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHENTICATED",
		},
		{
			name:       "team admin required",
			handler:    newReadyHandler(t, &handlerRepo{renameTeamErr: ErrTeamAdminRequired}),
			method:     http.MethodPatch,
			path:       teamsPath + "/" + teamID,
			body:       `{"name":"Research"}`,
			withUser:   true,
			wantStatus: http.StatusForbidden,
			wantCode:   "TEAM_ADMIN_REQUIRED",
		},
		{
			name:       "team not found",
			handler:    newReadyHandler(t, &handlerRepo{getTeamErr: ErrTeamNotFound}),
			method:     http.MethodGet,
			path:       teamsPath + "/" + teamID,
			withUser:   true,
			wantStatus: http.StatusNotFound,
			wantCode:   "TEAM_NOT_FOUND",
		},
		{
			name:       "team member not found",
			handler:    newReadyHandler(t, &handlerRepo{removeMemberErr: ErrTeamMemberNotFound}),
			method:     http.MethodDelete,
			path:       teamsPath + "/" + teamID + "/members/" + userID,
			withUser:   true,
			wantStatus: http.StatusNotFound,
			wantCode:   "TEAM_MEMBER_NOT_FOUND",
		},
		{
			name:       "invite not found",
			handler:    newReadyHandler(t, &handlerRepo{revokeInviteErr: ErrInviteNotFound}),
			method:     http.MethodDelete,
			path:       teamsPath + "/" + teamID + "/invites/" + inviteID,
			withUser:   true,
			wantStatus: http.StatusNotFound,
			wantCode:   "INVITE_NOT_FOUND",
		},
		{
			name:       "last team admin",
			handler:    newReadyHandler(t, &handlerRepo{leaveTeamErr: ErrLastTeamAdmin}),
			method:     http.MethodDelete,
			path:       teamsPath + "/" + teamID + "/membership",
			withUser:   true,
			wantStatus: http.StatusConflict,
			wantCode:   "LAST_TEAM_ADMIN",
		},
		{
			name:       "invite conflict",
			handler:    newReadyHandler(t, &handlerRepo{createInviteErr: ErrInviteConflict}),
			method:     http.MethodPost,
			path:       teamsPath + "/" + teamID + "/invites",
			body:       `{"email":"owner@example.test","teamRole":"member","idempotencyKey":"invite-1"}`,
			withUser:   true,
			wantStatus: http.StatusConflict,
			wantCode:   "INVITE_CONFLICT",
		},
		{
			name:       "idempotency conflict",
			handler:    newReadyHandler(t, &handlerRepo{createTeamErr: ErrIdempotencyConflict}),
			method:     http.MethodPost,
			path:       teamsPath,
			body:       `{"name":"Research","idempotencyKey":"create-1"}`,
			withUser:   true,
			wantStatus: http.StatusConflict,
			wantCode:   "IDEMPOTENCY_CONFLICT",
		},
		{
			name:       "invite not active",
			handler:    newReadyHandler(t, &handlerRepo{revokeInviteErr: ErrInviteNotActive}),
			method:     http.MethodDelete,
			path:       teamsPath + "/" + teamID + "/invites/" + inviteID,
			withUser:   true,
			wantStatus: http.StatusGone,
			wantCode:   "INVITE_NOT_ACTIVE",
		},
		{
			name:       "invite delivery unavailable",
			handler:    NewHandler(NewService(&handlerRepo{})),
			method:     http.MethodPost,
			path:       teamsPath + "/" + teamID + "/invites",
			body:       `{"email":"owner@example.test","teamRole":"member","idempotencyKey":"invite-1"}`,
			withUser:   true,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "INVITE_DELIVERY_UNAVAILABLE",
		},
		{
			name:       "database required",
			handler:    NewHandler(NewService(nil)),
			method:     http.MethodGet,
			path:       teamsPath + "/" + teamID,
			withUser:   true,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "DATABASE_REQUIRED",
		},
		{
			name:       "cursor codec required",
			handler:    NewHandler(NewService(&handlerRepo{})),
			method:     http.MethodGet,
			path:       teamsPath,
			withUser:   true,
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "CURSOR_CODEC_REQUIRED",
		},
		{
			name:       "internal error",
			handler:    newReadyHandler(t, &handlerRepo{getTeamErr: errors.New("boom")}),
			method:     http.MethodGet,
			path:       teamsPath + "/" + teamID,
			withUser:   true,
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performTeamRequest(tt.handler, tt.method, tt.path, tt.body, tt.withUser)
			assertTeamStatus(t, rec, tt.wantStatus)
			assertJSONHeaders(t, rec)
			assertTeamErrorCode(t, rec, tt.wantCode)
		})
	}
}

func TestHandlerReturnsNotFoundForUnknownRoutes(t *testing.T) {
	handler := newReadyHandler(t, &handlerRepo{})
	tests := []string{
		"/v1/teamz",
		teamsPath + "/",
		teamsPath + "/44444444-4444-4444-8444-444444444444/unknown",
		teamsPath + "/44444444-4444-4444-8444-444444444444/invites/66666666-6666-4666-8666-666666666666/extra",
	}
	for _, path := range tests {
		rec := performTeamRequest(handler, http.MethodGet, path, "", true)
		assertTeamStatus(t, rec, http.StatusNotFound)
		assertJSONHeaders(t, rec)
		assertTeamErrorCode(t, rec, "NOT_FOUND")
	}
}

func newReadyHandler(t *testing.T, repo *handlerRepo) *Handler {
	t.Helper()
	cursorCodec, err := NewCursorCodec(CursorKeyring{
		ActiveKeyID: "cursor-active",
		Keys: map[string][]byte{
			"cursor-active": repeatedKey('c'),
		},
	})
	if err != nil {
		t.Fatalf("NewCursorCodec() error = %v", err)
	}
	mailCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "mail-active",
		Keys: map[string][]byte{
			"mail-active": repeatedKey('m'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher() error = %v", err)
	}
	service := NewService(repo,
		WithCursorCodec(cursorCodec),
		WithMailCipher(mailCipher),
		WithInviteDeliveryGate(&fakeInviteDeliveryGate{}),
		WithInviteURLBuilder(func(token string) (string, error) {
			return "https://example.test/invites/accept#token=" + token, nil
		}),
		WithTeamServiceClock(func() time.Time { return handlerFixedNow }),
		WithTeamIDGenerator(func() (string, error) {
			return "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", nil
		}),
		WithInviteTokenGenerator(func() (string, error) {
			return testInviteToken('d'), nil
		}),
	)
	return NewHandler(service)
}

func performTeamRequest(handler http.Handler, method string, path string, body string, withUser bool) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if withUser {
		req = req.WithContext(auth.WithUser(req.Context(), auth.User{
			ID:          "11111111-1111-4111-8111-111111111111",
			DisplayName: "Server Owner",
			Role:        "user",
		}))
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func assertTeamStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func assertTeamErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	decodeTeamBody(t, rec, &body)
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%s", body.Error.Code, want, rec.Body.String())
	}
}

func assertJSONHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func decodeTeamBody(t *testing.T, rec *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(destination); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}

type handlerRepo struct {
	createTeamInput   CreateTeamRepositoryInput
	teamPageInput     ListTeamsRepositoryInput
	getTeamInput      TeamLookupInput
	renameTeamInput   RenameTeamRepositoryInput
	listMembersInput  ListMembersRepositoryInput
	updateMemberInput UpdateMemberRepositoryInput
	removeMemberInput RemoveMemberRepositoryInput
	leaveTeamInput    LeaveTeamRepositoryInput
	createInviteInput CreateInviteRepositoryInput
	listInvitesInput  ListInvitesRepositoryInput
	revokeInviteInput RevokeInviteRepositoryInput

	createTeamResult   Team
	listTeamsResult    TeamPageResult
	getTeamResult      Team
	renameTeamResult   Team
	listMembersResult  TeamMemberPageResult
	updateMemberResult TeamMember
	createInviteResult TeamInviteRecord
	listInvitesResult  TeamInvitePageResult

	createTeamErr   error
	listTeamsErr    error
	getTeamErr      error
	renameTeamErr   error
	listMembersErr  error
	updateMemberErr error
	removeMemberErr error
	leaveTeamErr    error
	createInviteErr error
	listInvitesErr  error
	revokeInviteErr error
}

func (r *handlerRepo) CreateTeam(_ context.Context, input CreateTeamRepositoryInput) (Team, error) {
	r.createTeamInput = input
	if r.createTeamErr != nil {
		return Team{}, r.createTeamErr
	}
	if r.createTeamResult.ID != "" {
		return r.createTeamResult, nil
	}
	return Team{
		ID:   input.ID,
		Name: input.Name,
		MyMembership: TeamMembership{
			TeamRole:  TeamRoleAdmin,
			Status:    MembershipStatusActive,
			JoinedAt:  handlerFixedNow,
			UpdatedAt: handlerFixedNow,
		},
		MembershipRevision: 1,
		CreatedAt:          handlerFixedNow,
		UpdatedAt:          handlerFixedNow,
	}, nil
}

func (r *handlerRepo) ListTeams(_ context.Context, input ListTeamsRepositoryInput) (TeamPageResult, error) {
	r.teamPageInput = input
	if r.listTeamsErr != nil {
		return TeamPageResult{}, r.listTeamsErr
	}
	return r.listTeamsResult, nil
}

func (r *handlerRepo) GetTeam(_ context.Context, input TeamLookupInput) (Team, error) {
	r.getTeamInput = input
	if r.getTeamErr != nil {
		return Team{}, r.getTeamErr
	}
	if r.getTeamResult.ID != "" {
		return r.getTeamResult, nil
	}
	return Team{
		ID:   input.TeamID,
		Name: "Research",
		MyMembership: TeamMembership{
			TeamRole:  TeamRoleAdmin,
			Status:    MembershipStatusActive,
			JoinedAt:  handlerFixedNow,
			UpdatedAt: handlerFixedNow,
		},
		MembershipRevision: 1,
		CreatedAt:          handlerFixedNow,
		UpdatedAt:          handlerFixedNow,
	}, nil
}

func (r *handlerRepo) RenameTeam(_ context.Context, input RenameTeamRepositoryInput) (Team, error) {
	r.renameTeamInput = input
	if r.renameTeamErr != nil {
		return Team{}, r.renameTeamErr
	}
	if r.renameTeamResult.ID != "" {
		return r.renameTeamResult, nil
	}
	return Team{
		ID:   input.TeamID,
		Name: input.Name,
		MyMembership: TeamMembership{
			TeamRole:  TeamRoleAdmin,
			Status:    MembershipStatusActive,
			JoinedAt:  handlerFixedNow,
			UpdatedAt: handlerFixedNow,
		},
		MembershipRevision: 2,
		CreatedAt:          handlerFixedNow,
		UpdatedAt:          handlerFixedNow,
	}, nil
}

func (r *handlerRepo) ListMembers(_ context.Context, input ListMembersRepositoryInput) (TeamMemberPageResult, error) {
	r.listMembersInput = input
	if r.listMembersErr != nil {
		return TeamMemberPageResult{}, r.listMembersErr
	}
	return r.listMembersResult, nil
}

func (r *handlerRepo) UpdateMemberRole(_ context.Context, input UpdateMemberRepositoryInput) (TeamMember, error) {
	r.updateMemberInput = input
	if r.updateMemberErr != nil {
		return TeamMember{}, r.updateMemberErr
	}
	if r.updateMemberResult.UserID != "" {
		return r.updateMemberResult, nil
	}
	return TeamMember{
		UserID:      input.TargetUserID,
		DisplayName: "Operator",
		TeamRole:    input.TeamRole,
		Status:      MembershipStatusActive,
		JoinedAt:    handlerFixedNow,
		UpdatedAt:   handlerFixedNow,
	}, nil
}

func (r *handlerRepo) RemoveMember(_ context.Context, input RemoveMemberRepositoryInput) error {
	r.removeMemberInput = input
	return r.removeMemberErr
}

func (r *handlerRepo) LeaveTeam(_ context.Context, input LeaveTeamRepositoryInput) error {
	r.leaveTeamInput = input
	return r.leaveTeamErr
}

func (r *handlerRepo) CreateInvite(_ context.Context, input CreateInviteRepositoryInput) (TeamInviteRecord, error) {
	r.createInviteInput = input
	if r.createInviteErr != nil {
		return TeamInviteRecord{}, r.createInviteErr
	}
	if r.createInviteResult.ID != "" {
		return r.createInviteResult, nil
	}
	createdAt := input.ExpiresAt.Add(-defaultInviteTTL)
	return TeamInviteRecord{
		ID:             input.ID,
		TeamID:         input.TeamID,
		Email:          input.Email,
		TeamRole:       input.TeamRole,
		Status:         InviteStatusPending,
		DeliveryStatus: InviteDeliveryPending,
		ExpiresAt:      input.ExpiresAt,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}, nil
}

func (r *handlerRepo) ListInvites(_ context.Context, input ListInvitesRepositoryInput) (TeamInvitePageResult, error) {
	r.listInvitesInput = input
	if r.listInvitesErr != nil {
		return TeamInvitePageResult{}, r.listInvitesErr
	}
	return r.listInvitesResult, nil
}

func (r *handlerRepo) RevokeInvite(_ context.Context, input RevokeInviteRepositoryInput) error {
	r.revokeInviteInput = input
	return r.revokeInviteErr
}
