package oauth2session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestResolveClientIdentityCommittedObservations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous *id.ClientID
		observed string
		revision int64
		changed  bool
	}{
		{name: "initialization", observed: "client-A"},
		{name: "same ID", previous: identityPointer("client-A"), observed: "client-A", revision: 2},
		{name: "changed ID", previous: identityPointer("client-A"), observed: "client-B", revision: 3, changed: true},
		{name: "stale replica or rollback", previous: identityPointer("client-B"), observed: "client-A", revision: 4, changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canonical := "first"
			serviceID := id.NewServiceID()
			entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: 7, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: tc.previous, ClientIdentityRevision: tc.revision}
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				t.Fatal("matched observations do not reload configuration")
				return nil, nil
			})
			service.credentialFileReader = identityReader{read: func(path string) (string, error) { assert.Equal(t, "first-file", path); return tc.observed, nil }}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(_ context.Context, got id.ServiceID, value id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				assert.Equal(t, serviceID, got)
				assert.Equal(t, id.ClientID(tc.observed), value)
				assert.Equal(t, ports.ClientIdentityObservation{ServiceVersion: 7, CanonicalID: canonical, Revision: tc.revision}, expected)
				revision := tc.revision
				if tc.changed {
					revision++
				}
				return ports.ClientIdentityState{Identity: identityPointer(tc.observed), Revision: revision}, true, tc.changed, nil
			}})
			state, err := service.resolveClientIdentity(context.Background(), entity)
			require.NoError(t, err)
			assert.Equal(t, identityPointer(tc.observed), state.Identity)
			assert.Equal(t, state.Identity, entity.ClientIdentity)
			assert.Equal(t, state.Revision, entity.ClientIdentityRevision)
			assert.Equal(t, int64(7), entity.Version)
			if tc.changed {
				var event map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &event), "one increment must emit exactly one JSON record")
				assert.Equal(t, "session.oauth2.client_identity_changed", event["event"])
				assert.Equal(t, float64(tc.revision+1), event["client_identity_revision"])
				assert.Equal(t, "INFO", event["level"])
				assert.Equal(t, serviceID.String(), event["service_id"])
				assert.Equal(t, "filesystem_observation", event["channel"])
				assert.NotContains(t, event, "operator_principal")
				assert.NotContains(t, event, "principal")
			} else {
				assert.Empty(t, logs.String())
			}
			assert.NotContains(t, logs.String(), tc.observed)
			assert.NotContains(t, logs.String(), "first-file")
		})
	}
}

func TestIdentityDisplayFallbackUsesReloadedEvidence(t *testing.T) {
	serviceID := id.NewServiceID()
	canonical := "first"
	gets, reads := 0, 0
	service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		gets++
		revision := int64(0)
		if gets > 1 {
			revision = 2
		}
		return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: int64(gets), CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("current"), ClientIdentityRevision: revision}, nil
	})
	service.credentialFileReader = identityReader{read: func(string) (string, error) {
		reads++
		if reads > 1 {
			return "", model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
		}
		return "obsolete", nil
	}}
	service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
		return ports.ClientIdentityState{}, false, false, nil
	}})
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID, UpstreamClientID: identityPointer("current")}}, ports.UsabilityDisplay)
	require.NoError(t, err)
	assert.True(t, result[serviceID], "fallback must not revive a session using evidence from the rejected configuration")
	assert.Equal(t, 2, gets)
	assert.Equal(t, 2, reads)
	assert.NotContains(t, logs.String(), "session.oauth2.client_identity_changed")
	assert.NotContains(t, logs.String(), "obsolete")
}

func TestIdentityEvaluatorLoadsEachServiceOnce(t *testing.T) {
	serviceID := id.NewServiceID()
	gets := 0
	service, _ := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		gets++
		return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, ClientID: "current", CredentialSource: model.CredentialSourceStored, Secret: model.NewEncryptedSecret([]byte("encrypted-secret"))}, nil
	})
	result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{nil, {ServiceID: serviceID, UpstreamClientID: identityPointer("current")}, {ServiceID: serviceID, UpstreamClientID: identityPointer("old")}}, ports.UsabilityGate)
	require.NoError(t, err)
	assert.Equal(t, 1, gets)
	assert.True(t, result[serviceID])
}

func TestIdentityEvaluatorUnavailableService(t *testing.T) {
	for _, mode := range []ports.UsabilityMode{ports.UsabilityGate, ports.UsabilityDisplay} {
		t.Run(fmt.Sprintf("mode=%d", mode), func(t *testing.T) {
			serviceID := id.NewServiceID()
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return nil, ports.ErrNotFound
			})
			result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID}}, mode)
			if mode == ports.UsabilityGate {
				assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.True(t, result[serviceID])
				assert.Contains(t, logs.String(), `"reason":"read_failed"`)
			}
		})
	}
}

func TestClientIdentityObservationContentionStopsAfterEightFreshAttempts(t *testing.T) {
	for _, mode := range []ports.UsabilityMode{ports.UsabilityGate, ports.UsabilityDisplay} {
		t.Run(fmt.Sprintf("mode=%d", mode), func(t *testing.T) {
			serviceID := id.NewServiceID()
			canonical := "first"
			gets, reads, observations := 0, 0, 0
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				gets++
				return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: int64(gets), CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("persisted-identity-sentinel"), ClientIdentityRevision: 3}, nil
			})
			service.credentialFileReader = identityReader{read: func(string) (string, error) { reads++; return fmt.Sprintf("obsolete-file-value-%d", reads), nil }}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(_ context.Context, _ id.ServiceID, observed id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				observations++
				assert.Equal(t, fmt.Sprintf("obsolete-file-value-%d", observations), observed.String())
				assert.Equal(t, int64(observations), expected.ServiceVersion)
				return ports.ClientIdentityState{Identity: identityPointer("uncommitted-identity-sentinel"), Revision: 100}, false, true, nil
			}})
			result, err := service.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: serviceID, UpstreamClientID: identityPointer("persisted-identity-sentinel"), UpstreamClientRevision: 2}}, mode)
			if mode == ports.UsabilityGate {
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, model.CredentialSourceReasonGenerationChanged, source.Reason)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.True(t, result[serviceID], "persisted revision evidence still invalidates the session")
			}
			assert.Equal(t, 8, observations)
			assert.Equal(t, 8, reads)
			assert.Equal(t, 9, gets, "each rejected snapshot must be replaced, including fallback evidence")
			assert.NotContains(t, logs.String(), "session.oauth2.client_identity_changed")
			for _, forbidden := range []string{"obsolete-file-value", "uncommitted-identity-sentinel", "persisted-identity-sentinel", "first-file"} {
				assert.NotContains(t, logs.String(), forbidden)
			}
		})
	}
}

func TestClientIdentityFilesystemBatchSharesOneReadAndObservation(t *testing.T) {
	serviceID := id.NewServiceID()
	canonical := "first"
	gets, reads, observations := 0, 0, 0
	service, _ := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
		gets++
		return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, Version: 7, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("current"), ClientIdentityRevision: 3}, nil
	})
	service.credentialFileReader = identityReader{read: func(string) (string, error) { reads++; return "current", nil }}
	service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
		observations++
		return ports.ClientIdentityState{Identity: identityPointer("current"), Revision: 3}, true, false, nil
	}})
	sessions := []*storage.UserSession{
		{ServiceID: serviceID, UpstreamClientID: identityPointer("current"), UpstreamClientRevision: 3},
		{ServiceID: serviceID, UpstreamClientID: identityPointer("current"), UpstreamClientRevision: 2},
		{ServiceID: serviceID, UpstreamClientID: identityPointer("old"), UpstreamClientRevision: 3},
	}
	got, err := service.RequiresReauthentication(context.Background(), sessions, ports.UsabilityGate)
	require.NoError(t, err)
	assert.True(t, got[serviceID])
	assert.Equal(t, 1, gets)
	assert.Equal(t, 1, reads)
	assert.Equal(t, 1, observations)
}

func TestClientIdentityLostOrFailedObservationNeverAuditsAChange(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed=%t", failed), func(t *testing.T) {
			serviceID := id.NewServiceID()
			canonical := "first"
			entity := &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonical, CredentialSource: model.CredentialSourceFilesystem, ClientIdentity: identityPointer("previous"), ClientIdentityRevision: 3}
			service, logs := identityService(t, func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				return &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, ClientID: "current", CredentialSource: model.CredentialSourceStored, ClientIdentityRevision: 3}, nil
			})
			service.credentialFileReader = identityReader{read: func(string) (string, error) { return "obsolete", nil }}
			service.WithClientIdentityRepository(identityObservationRepository{observe: func(context.Context, id.ServiceID, id.ClientID, ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
				if failed {
					return ports.ClientIdentityState{Identity: identityPointer("obsolete"), Revision: 4}, true, true, model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed)
				}
				return ports.ClientIdentityState{Identity: identityPointer("obsolete"), Revision: 4}, false, true, nil
			}})
			_, err := service.resolveClientIdentity(context.Background(), entity)
			if failed {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Zero(t, strings.Count(logs.String(), "session.oauth2.client_identity_changed"))
			assert.Empty(t, logs.String(), "a returning helper does not own the failed use")
		})
	}
}
