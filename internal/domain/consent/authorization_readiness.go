package consent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// EvaluateAuthorizationReadiness checks the consent prerequisites shared by browser
// authorization and inbound broker-token minting, without decrypting or refreshing tokens.
func EvaluateAuthorizationReadiness(ctx context.Context, principal id.Principal, agent *storage.Agent, grants ports.UserGrantRepository, sessions ports.UserSessionRepository) (ports.AuthorizationReadiness, error) {
	if agent == nil || agent.ID.IsZero() || grants == nil || sessions == nil {
		return ports.AuthorizationReadiness{}, fmt.Errorf("authorization readiness is not configured")
	}
	grant, err := grants.FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return ports.AuthorizationReadiness{Reason: ports.ReadinessGrantUnavailable}, fmt.Errorf("check authorization grant: %w", err)
	}
	if errors.Is(err, ports.ErrNotFound) || grant == nil {
		return ports.AuthorizationReadiness{Status: ports.UserDelegationMissing, Reason: ports.ReadinessGrantMissing}, nil
	}
	if !grant.IsActive() {
		return ports.AuthorizationReadiness{Status: ports.UserDelegationExpired, Reason: ports.ReadinessGrantExpired}, nil
	}
	for _, entry := range grant.GrantedPermissionSets {
		if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
			return declared.PermissionSetID == entry.PermissionSetID
		}) {
			return ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessPermissionSetUndeclared, PermissionSetID: entry.PermissionSetID}, nil
		}
	}

	checked := make(map[id.ServiceID]*storage.UserSession)
	checkSession := func(serviceID id.ServiceID) (*storage.UserSession, ports.AuthorizationReadiness, error) {
		session, ok := checked[serviceID]
		if !ok {
			var err error
			session, err = sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			if err != nil && !errors.Is(err, ports.ErrNotFound) {
				return nil, ports.AuthorizationReadiness{Reason: ports.ReadinessSessionUnavailable, ServiceID: serviceID}, fmt.Errorf("check authorization session: %w", err)
			}
			if errors.Is(err, ports.ErrNotFound) {
				session = nil
			}
			checked[serviceID] = session
		}
		if session == nil {
			return nil, ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionMissing, ServiceID: serviceID}, nil
		}
		if session.IsExpired() {
			return nil, ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessSessionExpired, ServiceID: serviceID}, nil
		}
		return session, ports.AuthorizationReadiness{Status: ports.UserDelegationActive}, nil
	}
	for _, entry := range grant.GrantedPermissionSets {
		for _, serviceID := range entry.IncludedServiceIDs {
			_, result, err := checkSession(serviceID)
			if err != nil || result.Status != ports.UserDelegationActive {
				return result, err
			}
		}
	}
	for _, requirement := range agent.ServiceRequirements {
		if !requirement.IsMandatory() {
			continue
		}
		session, result, err := checkSession(requirement.ServiceID)
		if err != nil || result.Status != ports.UserDelegationActive {
			return result, err
		}
		for _, requiredScope := range requirement.RequiredScopes {
			if !slices.Contains(session.Scope, requiredScope) {
				return ports.AuthorizationReadiness{Status: ports.UserDelegationConsentRequired, Reason: ports.ReadinessScopeInsufficient, ServiceID: requirement.ServiceID}, nil
			}
		}
	}
	return ports.AuthorizationReadiness{Status: ports.UserDelegationActive}, nil
}

// VerifyAuthorizationReadiness evaluates the caller's resolved agent without another agent read.
func (s *Service) VerifyAuthorizationReadiness(ctx context.Context, principal id.Principal, agent *storage.Agent) (ports.AuthorizationReadiness, error) {
	return EvaluateAuthorizationReadiness(ctx, principal, agent, s.grantRepo, s.sessionRepo)
}
