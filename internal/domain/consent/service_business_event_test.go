package consent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type consentEventAppender func(context.Context, *model.BusinessEvent) error

func (f consentEventAppender) Append(ctx context.Context, event *model.BusinessEvent) error {
	return f(ctx, event)
}

func TestConsentRevocationBusinessEvent(t *testing.T) {
	for _, outcome := range []string{"success", "cleanup failure", "delete failure", "not found"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := t.Context()
			grants := memory.NewUserGrantRepository()
			validUntil := time.Now().Add(time.Hour).UTC()
			grant := &storage.UserGrant{
				ID: id.NewGrantID(), Principal: id.NewPrincipal("resource-owner"), AgentID: id.NewAgentID(),
				ValidUntil: &validUntil, UpdatedAt: validUntil.Add(-time.Hour),
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
				}},
			}
			if outcome != "not found" {
				require.NoError(t, grants.Create(ctx, grant))
			}
			var grantRepo ports.UserGrantRepository = grants
			var refreshRepo ports.RefreshTokenSessionRepository = memory.NewRefreshTokenSessionStore()
			failure := errors.New("storage unavailable")
			if outcome == "cleanup failure" {
				refreshRepo = failingConsentRefreshRepo{refreshRepo, failure}
			}
			if outcome == "delete failure" {
				grantRepo = failingConsentDeleteRepo{grants, failure}
			}
			var events []*model.BusinessEvent
			appender := consentEventAppender(func(ctx context.Context, event *model.BusinessEvent) error {
				_, err := grants.FindByPrincipalAndAgent(ctx, grant.Principal, grant.AgentID)
				require.ErrorIs(t, err, ports.ErrNotFound, "audit emission must follow committed deletion")
				events = append(events, event.Copy())
				return nil
			})
			var operational bytes.Buffer
			base := slog.NewJSONHandler(&operational, nil)
			fallback := telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(base))
			ledger := telemetry.NewLedgerHandler(appender, fallback, time.Second)
			logger := slog.New(telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(telemetry.NewMultiHandler(base, ledger))))
			service := consent.NewService(nil, nil, grantRepo, nil, refreshRepo, nil, logger)
			err := service.RevokeConsentForPrincipal(ctx, grant.Principal, grant.AgentID)
			switch outcome {
			case "not found":
				require.ErrorIs(t, err, consent.ErrGrantNotFound)
				assert.Empty(t, events)
				return
			case "delete failure":
				require.ErrorIs(t, err, failure)
				assert.Empty(t, events)
				return
			case "cleanup failure":
				require.ErrorIs(t, err, failure)
			default:
				require.NoError(t, err)
			}
			require.Len(t, events, 1)
			event := events[0]
			assert.Equal(t, "consent.grant.revoked", event.Event)
			require.NotNil(t, event.Principal)
			assert.Equal(t, grant.Principal.String(), *event.Principal)
			encoded, err := json.Marshal(event.Attributes)
			require.NoError(t, err)
			var attributes map[string]any
			require.NoError(t, json.Unmarshal(encoded, &attributes))
			assert.Equal(t, grant.ID.String(), attributes["grant_id"])
			assert.Equal(t, grant.AgentID.String(), attributes["agent_id"])
			assert.Equal(t, validUntil.Format(time.RFC3339Nano), attributes["valid_until"])
			assert.Equal(t, grant.UpdatedAt.Format(time.RFC3339Nano), attributes["updated_at"])
			assert.Equal(t, []any{map[string]any{
				"permission_set_id":    grant.GrantedPermissionSets[0].PermissionSetID.String(),
				"included_service_ids": []any{grant.GrantedPermissionSets[0].IncludedServiceIDs[0].String()},
			}}, attributes["granted_permission_sets"])
			require.IsType(t, "", attributes["revoked_at"])
			revokedAt, err := time.Parse(time.RFC3339Nano, attributes["revoked_at"].(string))
			require.NoError(t, err)
			assert.Equal(t, revokedAt.UTC().Format(time.RFC3339Nano), attributes["revoked_at"])
			if outcome == "cleanup failure" {
				assert.Equal(t, "failed", attributes["refresh_cleanup"])
			} else {
				assert.NotContains(t, attributes, "refresh_cleanup")
			}
			require.ErrorIs(t, service.RevokeConsentForPrincipal(ctx, grant.Principal, grant.AgentID), consent.ErrGrantNotFound)
			assert.Len(t, events, 1, "an unchanged retry must emit no event")
		})
	}
}
