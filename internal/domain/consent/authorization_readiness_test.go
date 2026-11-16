package consent

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type readinessGrantRepository struct {
	ports.UserGrantRepository
	grant *storage.UserGrant
	err   error
}

func (r readinessGrantRepository) FindByPrincipalAndAgent(context.Context, id.Principal, id.AgentID) (*storage.UserGrant, error) {
	return r.grant, r.err
}

type readinessSessionRepository struct {
	ports.UserSessionRepository
	session *storage.UserSession
	err     error
	calls   int
}

func (r *readinessSessionRepository) FindByPrincipalAndService(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
	r.calls++
	return r.session, r.err
}

func TestEvaluateAuthorizationReadiness(t *testing.T) {
	principal := id.NewPrincipal("subject-1")
	agentID, serviceID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
	expired := time.Now().Add(-time.Hour)
	agent := &storage.Agent{ID: agentID}
	grant := &storage.UserGrant{AgentID: agentID, Principal: principal}
	delegated := &storage.Agent{ID: agentID, PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID}}}
	delegatedGrant := &storage.UserGrant{AgentID: agentID, Principal: principal, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{serviceID, serviceID}}}}
	mandatory := &storage.Agent{ID: agentID, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read"}}}}
	shared := &storage.Agent{ID: agentID, PermissionSets: delegated.PermissionSets, ServiceRequirements: mandatory.ServiceRequirements}

	for _, tc := range []struct {
		name      string
		agent     *storage.Agent
		grant     *storage.UserGrant
		grantErr  error
		session   *storage.UserSession
		err       error
		want      ports.AuthorizationReadiness
		wantErr   error
		wantCalls int
	}{
		{name: "active grant", agent: agent, grant: grant, want: ports.AuthorizationReadiness{Status: ports.UserDelegationActive}},
		{name: "nil grant", agent: agent, want: ports.AuthorizationReadiness{Status: ports.UserDelegationMissing, Reason: ports.ReadinessGrantMissing}},
		{name: "grant not found", agent: agent, grantErr: ports.ErrNotFound, want: ports.AuthorizationReadiness{Status: ports.UserDelegationMissing, Reason: ports.ReadinessGrantMissing}},
		{name: "expired grant", agent: agent, grant: &storage.UserGrant{ValidUntil: &expired}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationExpired, Reason: ports.ReadinessGrantExpired}},
		{name: "grant repository failure", agent: agent, grantErr: assert.AnError, want: ports.AuthorizationReadiness{Reason: ports.ReadinessGrantUnavailable}, wantErr: assert.AnError},
		{name: "undeclared permission set", agent: agent, grant: delegatedGrant, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessPermissionSetUndeclared, PermissionSetID: psID}},
		{name: "missing delegated session", agent: delegated, grant: delegatedGrant, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionMissing, ServiceID: serviceID}, wantCalls: 1},
		{name: "delegated session not found", agent: delegated, grant: delegatedGrant, err: ports.ErrNotFound, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionMissing, ServiceID: serviceID}, wantCalls: 1},
		{name: "expired delegated session", agent: delegated, grant: delegatedGrant, session: &storage.UserSession{AccessTokenExpiresAt: &expired}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionExpired, ServiceID: serviceID}, wantCalls: 1},
		{name: "refreshable delegated session", agent: delegated, grant: delegatedGrant, session: &storage.UserSession{AccessTokenExpiresAt: &expired, EncryptedRefreshToken: []byte("encrypted-refresh")}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationActive}, wantCalls: 1},
		{name: "delegated repository failure", agent: delegated, grant: delegatedGrant, err: assert.AnError, want: ports.AuthorizationReadiness{Reason: ports.ReadinessSessionUnavailable, ServiceID: serviceID}, wantErr: assert.AnError, wantCalls: 1},
		{name: "missing mandatory session", agent: mandatory, grant: grant, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionMissing, ServiceID: serviceID}, wantCalls: 1},
		{name: "expired mandatory session", agent: mandatory, grant: grant, session: &storage.UserSession{AccessTokenExpiresAt: &expired}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionExpired, ServiceID: serviceID}, wantCalls: 1},
		{name: "mandatory scopes missing", agent: mandatory, grant: grant, session: &storage.UserSession{Scope: []string{"write"}}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessScopeInsufficient, ServiceID: serviceID}, wantCalls: 1},
		{name: "mandatory repository failure", agent: mandatory, grant: grant, err: assert.AnError, want: ports.AuthorizationReadiness{Reason: ports.ReadinessSessionUnavailable, ServiceID: serviceID}, wantErr: assert.AnError, wantCalls: 1},
		{name: "shared mandatory and delegated session", agent: shared, grant: delegatedGrant, session: &storage.UserSession{Scope: []string{"read", "write"}}, want: ports.AuthorizationReadiness{Status: ports.UserDelegationActive}, wantCalls: 1},
		{name: "optional unselected service", agent: &storage.Agent{ID: agentID, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: serviceID, RequirementType: storage.RequirementTypeOptional}}}, grant: grant, want: ports.AuthorizationReadiness{Status: ports.UserDelegationActive}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := &readinessSessionRepository{session: tc.session, err: tc.err}
			result, err := EvaluateAuthorizationReadiness(context.Background(), principal, tc.agent, readinessGrantRepository{grant: tc.grant, err: tc.grantErr}, sessions)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, result)
			assert.Equal(t, tc.wantCalls, sessions.calls)
		})
	}
}

func TestAuthorizationReadinessFailsClosedWhenUnconfigured(t *testing.T) {
	for _, agent := range []*storage.Agent{nil, {}} {
		result, err := EvaluateAuthorizationReadiness(context.Background(), id.NewPrincipal("subject-1"), agent, readinessGrantRepository{}, &readinessSessionRepository{})
		require.Error(t, err)
		assert.Empty(t, result.Status)
	}
}
