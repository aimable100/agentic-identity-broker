package oauth2session_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func projectedCredentialPair(t *testing.T, clientID, secret string) ports.CredentialFileBinding {
	t.Helper()
	directory := t.TempDir()
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(directory, "identity-input"), ClientSecretFile: filepath.Join(directory, "authentication-input")}
	require.NoError(t, os.Symlink("current/client-id", binding.ClientIDFile))
	require.NoError(t, os.Symlink("current/client-secret", binding.ClientSecretFile))
	require.NoError(t, publishCredentialGeneration(binding, clientID, secret))
	return binding
}

func publishCredentialGeneration(binding ports.CredentialFileBinding, clientID, secret string) error {
	directory := filepath.Dir(binding.ClientIDFile)
	generation, err := os.MkdirTemp(directory, "generation-")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generation, "client-id"), []byte(clientID), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generation, "client-secret"), []byte(secret), 0600); err != nil {
		return err
	}
	link := generation + "-link"
	if err := os.Symlink(filepath.Base(generation), link); err != nil {
		return err
	}
	return os.Rename(link, filepath.Join(directory, "current"))
}

func TestCredentialRotation_BrokerRetriesAndActualRefreshesAcquireFreshPairs(t *testing.T) {
	binding := projectedCredentialPair(t, "rotation-client", "first-secret")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"retry-service": binding})
	provider := h.register(t, model.CredentialSourceFilesystem, "retry-service", "", "")
	before, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Zero(t, before.ClientIdentityRevision)
	principal := id.Principal("retry@example.com")
	flow := h.initiate(t, principal, provider.ID)
	var publishOnce sync.Once
	var publicationError error
	h.endpoint.setOnRequest(func(request credentialTokenRequest) (int, error) {
		if request.secret == "first-secret" {
			publishOnce.Do(func() { publicationError = publishCredentialGeneration(binding, "rotation-client", "retry-secret") })
			return http.StatusInternalServerError, publicationError
		}
		return http.StatusOK, nil
	})
	session := h.callback(t, principal, provider.ID, flow.StateToken)
	identity := id.ClientID("rotation-client")
	assert.Equal(t, &identity, session.UpstreamClientID)
	assert.Zero(t, session.UpstreamClientRevision)
	ids, pairs := h.reader.counts()
	assert.Equal(t, 3, ids, "initiation and callback identity gates never read the secret")
	assert.Equal(t, 2, pairs, "auth-style negotiation may issue multiple requests, but each broker attempt acquires exactly one pair")
	requests := h.endpoint.captured(t)
	require.GreaterOrEqual(t, len(requests), 2)
	assert.Equal(t, "first-secret", requests[0].secret)
	assert.Equal(t, "retry-secret", requests[len(requests)-1].secret)
	for _, request := range requests[:len(requests)-1] {
		assert.Equal(t, "first-secret", request.secret, "negotiation within the first attempt must retain its coherent acquired pair")
	}
	require.NoError(t, publishCredentialGeneration(binding, "rotation-client", "explicit-refresh-secret"))
	_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	require.NoError(t, publishCredentialGeneration(binding, "rotation-client", "automatic-refresh-secret"))
	h.expire(t, principal, provider.ID)
	refreshed, token, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "credential-access", token)
	assert.Equal(t, session.UpstreamClientID, refreshed.UpstreamClientID)
	assert.Equal(t, session.UpstreamClientRevision, refreshed.UpstreamClientRevision, "same-ID secret rotation preserves session establishment evidence")
	requests = h.endpoint.captured(t)
	assert.Equal(t, "explicit-refresh-secret", requests[len(requests)-2].secret)
	assert.Equal(t, "automatic-refresh-secret", requests[len(requests)-1].secret)
	assert.Equal(t, "refresh_token", requests[len(requests)-1].form.Get("grant_type"))
	ids, pairs = h.reader.counts()
	assert.Equal(t, 10, ids, "refresh gates recheck live identity and the explicit refresh summary performs one Display read")
	assert.Equal(t, 4, pairs)
	cachedCount := len(requests)
	_, _, err = h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	ids, pairs = h.reader.counts()
	assert.Equal(t, 11, ids, "even a still-valid token checks the current identity once")
	assert.Equal(t, 4, pairs, "a still-valid token does not constitute an actual refresh")
	assert.Len(t, h.endpoint.captured(t), cachedCount)
	after, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	before.ClientIdentity = &identity
	assert.Zero(t, after.ClientIdentityRevision, "same-ID secret rotation must not increment the service revision")
	assert.Equal(t, before, after)
}

func TestCredentialRotation_ClientIdentityChangeStopsRetryBeforeAuthentication(t *testing.T) {
	binding := projectedCredentialPair(t, "client-A", "secret-A")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"identity-retry": binding})
	provider := h.register(t, model.CredentialSourceFilesystem, "identity-retry", "", "")
	principal := id.Principal("identity-retry@example.com")
	flow := h.initiate(t, principal, provider.ID)
	var publishOnce sync.Once
	var publicationError error
	h.endpoint.setOnRequest(func(request credentialTokenRequest) (int, error) {
		if request.clientID == "client-A" {
			publishOnce.Do(func() { publicationError = publishCredentialGeneration(binding, "client-B", "secret-B") })
			return http.StatusInternalServerError, publicationError
		}
		return http.StatusOK, nil
	})
	result, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{
		ServiceID: provider.ID, Code: "client-A-code", State: flow.StateToken,
	})
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
	assert.Nil(t, result)
	ids, pairs := h.reader.counts()
	assert.Equal(t, 2, ids, "the callback checks live identity before its first exchange attempt")
	assert.Equal(t, 2, pairs, "the broker retry must acquire the changed identity and stop")
	requests := h.endpoint.captured(t)
	require.NotEmpty(t, requests, "the first attempt must reach the provider before publication")
	assert.LessOrEqual(t, len(requests), 2, "only the first attempt and its auth-style negotiation may reach the provider")
	for _, request := range requests {
		assert.Equal(t, "client-A", request.clientID, "A's authorization code must never authenticate as B")
		assert.Equal(t, "secret-A", request.secret)
		assert.Equal(t, "client-A-code", request.form.Get("code"))
	}
	session, err := h.sessions.FindByPrincipalAndService(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	assert.Nil(t, session, "identity mismatch must not create a session")
}

func TestCredentialRotation_IndependentServicesUseCurrentProjectedGenerationsWithoutWrites(t *testing.T) {
	firstBinding := projectedCredentialPair(t, "first-client", "first-secret")
	secondBinding := projectedCredentialPair(t, "second-client", "second-secret")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"first-service": firstBinding, "second-service": secondBinding})
	first := h.register(t, model.CredentialSourceFilesystem, "first-service", "", "")
	second := h.register(t, model.CredentialSourceFilesystem, "second-service", "", "")
	firstBefore, err := h.records.Get(context.Background(), first.ID)
	require.NoError(t, err)
	secondBefore, err := h.records.Get(context.Background(), second.ID)
	require.NoError(t, err)
	assert.Zero(t, firstBefore.ClientIdentityRevision)
	assert.Zero(t, secondBefore.ClientIdentityRevision)
	principal := id.Principal("independent@example.com")
	for _, provider := range []*model.ThirdpartyOAuth2ProviderEntity{first, second} {
		flow := h.initiate(t, principal, provider.ID)
		session := h.callback(t, principal, provider.ID, flow.StateToken)
		assert.Zero(t, session.UpstreamClientRevision)
	}
	require.NoError(t, publishCredentialGeneration(firstBinding, "first-client", "rotated-first-secret"))
	for _, provider := range []*model.ThirdpartyOAuth2ProviderEntity{first, second} {
		flow := h.initiate(t, principal, provider.ID)
		session := h.callback(t, principal, provider.ID, flow.StateToken)
		assert.Zero(t, session.UpstreamClientRevision, "reconnecting with a rotated secret keeps the identity revision")
		_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
		require.NoError(t, err)
	}
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 6)
	assert.Equal(t, "first-secret", requests[0].secret)
	assert.Equal(t, "second-secret", requests[1].secret)
	for _, index := range []int{2, 3} {
		assert.Equal(t, "first-client", requests[index].clientID)
		assert.Equal(t, "rotated-first-secret", requests[index].secret)
	}
	for _, index := range []int{4, 5} {
		assert.Equal(t, "second-client", requests[index].clientID)
		assert.Equal(t, "second-secret", requests[index].secret)
	}
	firstAfter, err := h.records.Get(context.Background(), first.ID)
	require.NoError(t, err)
	secondAfter, err := h.records.Get(context.Background(), second.ID)
	require.NoError(t, err)
	firstIdentity, secondIdentity := id.ClientID("first-client"), id.ClientID("second-client")
	firstBefore.ClientIdentity = &firstIdentity
	secondBefore.ClientIdentity = &secondIdentity
	assert.Zero(t, firstAfter.ClientIdentityRevision, "the rotated service keeps its identity revision")
	assert.Zero(t, secondAfter.ClientIdentityRevision, "the unrelated service keeps its identity revision")
	assert.Equal(t, firstBefore, firstAfter)
	assert.Equal(t, secondBefore, secondAfter)
	ids, pairs := h.reader.counts()
	assert.Equal(t, 18, ids, "each connection and refresh evaluates identity, including one Display read per explicit refresh summary")
	assert.Equal(t, 6, pairs)
}

func TestCredentialRotation_AcquisitionGenerationChangeStopsEveryConsumerWithoutRetry(t *testing.T) {
	for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
		t.Run(operation, func(t *testing.T) {
			binding := credentialPair(t, "generation-client", "generation-secret")
			h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"generation-service": binding})
			provider := h.register(t, model.CredentialSourceFilesystem, "generation-service", "", "")
			principal := id.Principal("generation@example.com")
			established := id.ClientID("generation-client")
			state := h.initiate(t, principal, provider.ID).StateToken
			if operation != "callback" {
				h.seedSession(t, principal, provider.ID, &established)
			}
			h.reader.beforePair = func(attempt int) error {
				if attempt == 1 {
					return model.NewCredentialSourceError(model.CredentialSourceReasonGenerationChanged)
				}
				return nil
			}
			attemptOperation := func() error {
				if operation == "callback" {
					_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "generation-code", State: state})
					return err
				}
				if operation == "explicit refresh" {
					_, err := h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
					return err
				}
				_, _, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
				return err
			}
			assertCredentialSourceReason(t, attemptOperation(), model.CredentialSourceReasonGenerationChanged)
			assert.Empty(t, h.endpoint.captured(t))
			_, pairs := h.reader.counts()
			assert.Equal(t, 1, pairs, "source acquisition errors are not provider retry candidates")
			require.NoError(t, publishCredential(binding.ClientSecretFile, "recovered-secret"))
			require.NoError(t, attemptOperation())
			requests := h.endpoint.captured(t)
			require.Len(t, requests, 1)
			assert.Equal(t, "generation-client", requests[0].clientID)
			assert.Equal(t, "recovered-secret", requests[0].secret)
			_, pairs = h.reader.counts()
			assert.Equal(t, 2, pairs)
		})
	}
}

func TestCredentialRotation_SourceLossAfterSuccessfulUseNeverUsesLastKnownPair(t *testing.T) {
	for _, file := range []string{"client ID", "client secret"} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			t.Run(file+"/"+operation, func(t *testing.T) {
				binding := credentialPair(t, "recovery-client", "initial-secret")
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"recovery-service": binding})
				provider := h.register(t, model.CredentialSourceFilesystem, "recovery-service", "", "")
				principal := id.Principal("recovery@example.com")
				state := h.initiate(t, principal, provider.ID).StateToken
				h.callback(t, principal, provider.ID, state)
				h.expire(t, principal, provider.ID)
				path := binding.ClientIDFile
				if file == "client secret" {
					path = binding.ClientSecretFile
				}
				require.NoError(t, os.Remove(path))
				attemptOperation := func() error {
					if operation == "callback" {
						_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "recovery-code", State: state})
						return err
					}
					if operation == "explicit refresh" {
						_, err := h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
						return err
					}
					_, _, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
					return err
				}
				assertCredentialSourceReason(t, attemptOperation(), model.CredentialSourceReasonNotFound)
				assert.Len(t, h.endpoint.captured(t), 1)
				_, pairs := h.reader.counts()
				failedPairs := 1
				if file == "client secret" {
					failedPairs++
				}
				assert.Equal(t, failedPairs, pairs, "an identity read failure must not acquire a secret")
				require.NoError(t, publishCredential(binding.ClientIDFile, "recovery-client"))
				require.NoError(t, publishCredential(binding.ClientSecretFile, "restored-secret"))
				require.NoError(t, attemptOperation())
				requests := h.endpoint.captured(t)
				require.Len(t, requests, 2)
				assert.Equal(t, "restored-secret", requests[1].secret)
				_, pairs = h.reader.counts()
				assert.Equal(t, failedPairs+1, pairs)
			})
		}
	}
}

func TestCredentialRotation_RetryStopsOnNewAcquisitionErrorBeforeAnotherProviderRequest(t *testing.T) {
	binding := credentialPair(t, "retry-client", "retry-secret")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"failed-retry": binding})
	provider := h.register(t, model.CredentialSourceFilesystem, "failed-retry", "", "")
	principal := id.Principal("failed-retry@example.com")
	state := h.initiate(t, principal, provider.ID).StateToken
	h.endpoint.setOnRequest(func(credentialTokenRequest) (int, error) { return http.StatusInternalServerError, nil })
	h.reader.beforePair = func(attempt int) error {
		if attempt > 1 {
			return model.NewCredentialSourceError(model.CredentialSourceReasonGenerationChanged)
		}
		return nil
	}
	_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "retry-code", State: state})
	assertCredentialSourceReason(t, err, model.CredentialSourceReasonGenerationChanged)
	_, pairs := h.reader.counts()
	assert.Equal(t, 2, pairs)
	requests := h.endpoint.captured(t)
	require.NotEmpty(t, requests)
	assert.LessOrEqual(t, len(requests), 2, "only the first acquired attempt and its auth-style negotiation may reach the provider")
}

func TestCredentialRotation_PublicationAfterValidationUsesOnlyAcquiredPairAndChecksItsIdentity(t *testing.T) {
	for _, acquiredMatches := range []bool{true, false} {
		for _, operation := range []string{"callback", "explicit refresh", "automatic refresh"} {
			name := "matching acquired pair"
			if !acquiredMatches {
				name = "mismatching acquired pair"
			}
			t.Run(name+"/"+operation, func(t *testing.T) {
				acquiredID, nextID := "client-A", "client-B"
				if !acquiredMatches {
					acquiredID, nextID = "client-B", "client-A"
				}
				binding := projectedCredentialPair(t, "client-A", "acquired-secret")
				h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"publication-service": binding})
				provider := h.register(t, model.CredentialSourceFilesystem, "publication-service", "", "")
				principal := id.Principal("publication@example.com")
				established := id.ClientID("client-A")
				state := credentialState(t, h.service, principal, provider.ID, &established)
				if operation != "callback" {
					h.seedSession(t, principal, provider.ID, &established)
				}
				var publication sync.Once
				var publicationError error
				h.reader.beforePair = func(attempt int) error {
					if !acquiredMatches && attempt == 1 {
						return publishCredentialGeneration(binding, acquiredID, "acquired-secret")
					}
					return nil
				}
				h.reader.afterPair = func() error {
					publication.Do(func() { publicationError = publishCredentialGeneration(binding, nextID, "published-secret") })
					return publicationError
				}
				attemptOperation := func() error {
					if operation == "callback" {
						_, err := h.service.HandleCallback(context.Background(), principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "publication-code", State: state})
						return err
					}
					if operation == "explicit refresh" {
						_, err := h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
						return err
					}
					_, _, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
					return err
				}
				err := attemptOperation()
				if acquiredMatches {
					assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
					requests := h.endpoint.captured(t)
					require.Len(t, requests, 1)
					assert.Equal(t, "client-A", requests[0].clientID)
					assert.Equal(t, "acquired-secret", requests[0].secret)
					if operation == "automatic refresh" {
						h.expire(t, principal, provider.ID)
					}
					assertCredentialSourceReason(t, attemptOperation(), model.CredentialSourceReasonIdentityMismatch)
					assert.Len(t, h.endpoint.captured(t), 1)
				} else {
					assertCredentialSourceReason(t, err, model.CredentialSourceReasonIdentityMismatch)
					assert.Empty(t, h.endpoint.captured(t), "a newly matching publication cannot rescue a mismatched acquired pair")
					require.NoError(t, attemptOperation())
					requests := h.endpoint.captured(t)
					require.Len(t, requests, 1)
					assert.Equal(t, "client-A", requests[0].clientID)
					assert.Equal(t, "published-secret", requests[0].secret)
				}
				_, pairs := h.reader.counts()
				if acquiredMatches {
					assert.Equal(t, 1, pairs, "the changed identity blocks the subsequent attempt before pair acquisition")
				} else {
					assert.Equal(t, 2, pairs)
				}
			})
		}
	}
}
