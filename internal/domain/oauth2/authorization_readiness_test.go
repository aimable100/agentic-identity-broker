package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationReadinessLogsDependencyFailure(t *testing.T) {
	agentID, serviceID := id.NewAgentID(), id.NewServiceID()
	agents := NewMockAgentRepository()
	require.NoError(t, agents.Create(context.Background(), &storage.Agent{ID: agentID, RedirectURIs: []string{"https://client.example.com/callback"}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory}}}))
	principal := id.NewPrincipal("subject-1")
	grants := NewMockGrantRepository()
	require.NoError(t, grants.Create(context.Background(), &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: principal}))
	sessions := NewMockSessionRepository()
	sessions.findFunc = func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
		return nil, assert.AnError
	}
	var logs bytes.Buffer
	svc := NewAuthorizationService(grants, sessions, NewAgentClientResolver(agents, nil), &OAuth2Config{ModeStrategy: NewLocalModeStrategy()}, slog.New(slog.NewJSONHandler(&logs, nil)), newTestSessionTokenService())
	decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{ClientID: id.ClientID(agentID.String()), RedirectURI: "https://client.example.com/callback", State: "state-sentinel"}, principal)
	require.NoError(t, err)
	assert.Equal(t, "server_error", decision.ErrorCode)
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
	assert.Equal(t, "ERROR", record["level"])
	assert.Equal(t, string(ports.ReadinessSessionUnavailable), record["reason"])
	assert.Equal(t, serviceID.String(), record["service_id"])
	assert.Contains(t, record["error"], assert.AnError.Error())
	assert.NotContains(t, logs.String(), "state-sentinel")
}

func TestAuthorizationReadinessLogsDenialReason(t *testing.T) {
	principal := id.NewPrincipal("subject-1")
	agentID, serviceID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
	for _, tc := range []struct {
		name    string
		grant   *storage.UserGrant
		session *storage.UserSession
		reason  ports.AuthorizationReadinessReason
		key     string
		value   string
	}{
		{name: "undeclared permission set", grant: &storage.UserGrant{GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID}}}, reason: ports.ReadinessPermissionSetUndeclared, key: "permission_set_id", value: psID.String()},
		{name: "missing service", grant: &storage.UserGrant{}, reason: ports.ReadinessSessionMissing, key: "service_id", value: serviceID.String()},
		{name: "missing scopes", grant: &storage.UserGrant{}, session: &storage.UserSession{Scope: []string{"write"}, EncryptedAccessToken: []byte("access-credential-sentinel")}, reason: ports.ReadinessScopeInsufficient, key: "service_id", value: serviceID.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agents := NewMockAgentRepository()
			require.NoError(t, agents.Create(context.Background(), &storage.Agent{ID: agentID, RedirectURIs: []string{"https://client.example.com/callback"}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read"}}}}))
			grants := NewMockGrantRepository()
			tc.grant.ID, tc.grant.AgentID, tc.grant.Principal = id.NewGrantID(), agentID, principal
			require.NoError(t, grants.Create(context.Background(), tc.grant))
			sessions := NewMockSessionRepository()
			sessions.findFunc = func(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
				return tc.session, nil
			}
			var logs bytes.Buffer
			svc := NewAuthorizationService(grants, sessions, NewAgentClientResolver(agents, nil), &OAuth2Config{ModeStrategy: NewLocalModeStrategy(), PublicURL: "https://broker.example.com"}, slog.New(slog.NewJSONHandler(&logs, nil)), newTestSessionTokenService())
			decision, err := svc.HandleAuthorization(context.Background(), &ports.AuthorizationRequest{ClientID: id.ClientID(agentID.String()), RedirectURI: "https://client.example.com/callback", OriginalURL: "https://broker.example.com/oauth2/authorize?state=state-sentinel"}, principal)
			require.NoError(t, err)
			assert.Equal(t, "redirect_to_consent", decision.Action)
			var record map[string]any
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
			assert.Equal(t, "WARN", record["level"])
			assert.Equal(t, "oauth2.authorization.consent_required", record["event"])
			assert.Equal(t, string(tc.reason), record["reason"])
			assert.Equal(t, tc.value, record[tc.key])
			assert.NotContains(t, logs.String(), "access-credential-sentinel")
			assert.NotContains(t, logs.String(), "state-sentinel")
		})
	}
}

func TestAuthorizationErrorRedirectFailureDoesNotLogURL(t *testing.T) {
	var logs bytes.Buffer
	svc := NewAuthorizationService(NewMockGrantRepository(), NewMockSessionRepository(), nil, &OAuth2Config{ModeStrategy: NewLocalModeStrategy()}, slog.New(slog.NewJSONHandler(&logs, nil)), newTestSessionTokenService())
	decision := svc.authorizationErrorDecision(context.Background(), &ports.AuthorizationRequest{RedirectURI: "https://client.example.com/%zz?secret=url-sentinel", State: "state-sentinel"}, "server error")
	assert.Equal(t, "server_error", decision.ErrorCode)
	assert.Empty(t, decision.RedirectURL)
	var record map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record))
	assert.Equal(t, "ERROR", record["level"])
	assert.Equal(t, "invalid_redirect_uri", record["error_kind"])
	assert.NotContains(t, logs.String(), "url-sentinel")
	assert.NotContains(t, logs.String(), "state-sentinel")
}
