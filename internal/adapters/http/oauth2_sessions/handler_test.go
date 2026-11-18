package oauth2_sessions

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshSessionIdentityDenials(t *testing.T) {
	for _, tc := range []struct {
		name       string
		identity   *id.ClientID
		rejected   bool
		unreadable bool
		status     int
		code       string
	}{
		{name: "missing identity", status: http.StatusConflict, code: "refresh_unavailable"},
		{name: "mismatched identity", identity: new(id.ClientID("previous-client")), status: http.StatusConflict, code: "refresh_unavailable"},
		{name: "superseded revision", identity: new(id.ClientID("current-client")), status: http.StatusConflict, code: "refresh_unavailable"},
		{name: "rejection takes precedence", identity: new(id.ClientID("previous-client")), rejected: true, status: http.StatusConflict, code: "refresh_rejected"},
		{name: "unavailable source", unreadable: true, status: http.StatusInternalServerError, code: "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var providerCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				providerCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"unexpected-new-token","token_type":"Bearer","expires_in":3600}`))
			}))
			t.Cleanup(upstream.Close)
			logger := slog.New(slog.DiscardHandler)
			encryptor := testutil.NewTestEncryptionAdapter(t)
			providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := &model.ThirdpartyOAuth2ProviderEntity{
				ID: id.NewServiceID(), DisplayName: "Reports", ClientID: "previous-client",
				TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone, Secret: model.NewAbsentSecret(),
				IssuerURI: "https://reports.example.test", Endpoints: model.OAuth2Endpoints{TokenEndpoint: upstream.URL},
			}
			if tc.unreadable {
				canonicalID := "reports"
				provider.CanonicalID = &canonicalID
				provider.CredentialSource = model.CredentialSourceFilesystem
				provider.ClientID = ""
				provider.TokenEndpointAuthMethod = ""
			}
			require.NoError(t, providers.Create(ctx, provider))
			if !tc.unreadable {
				provider.ClientID = "current-client"
				_, err := providers.Update(ctx, provider, nil)
				require.NoError(t, err)
			}
			sessions := memory.NewInMemoryUserSessionRepository()
			encryptionContext := domainencryption.NewServiceBranchKeySubject(provider.ID).EncryptionContext()
			access, err := encryptor.Encrypt(ctx, []byte("old-access-token"), encryptionContext)
			require.NoError(t, err)
			refresh, err := encryptor.Encrypt(ctx, []byte("old-refresh-token"), encryptionContext)
			require.NoError(t, err)
			now := time.Now()
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: "alice@example.test", ServiceID: provider.ID,
				UpstreamClientID: tc.identity, CallbackID: "established", TokenType: "Bearer",
				EncryptedAccessToken: access, EncryptedRefreshToken: refresh,
				EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
				InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
			}
			if tc.rejected {
				session.RefreshRejectedAt = &now
			}
			_, err = sessions.CreateIfAbsent(ctx, session)
			require.NoError(t, err)
			providerService := thirdparty.NewThirdpartyOAuth2ProviderService(providers, encryptor, &encryptionnoop.BranchKeyManager{}, nil, true, logger)
			service := oauth2session.NewOAuth2SessionService(providerService, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(), encryptor, upstream.Client(), nil, oauth2session.DefaultConfig(), logger).WithClientIdentityRepository(providers)
			router := chi.NewRouter()
			router.Post("/api/third-party/{serviceId}/session/refresh", NewHandler(service, logger).RefreshSession)
			req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+provider.ID.String()+"/session/refresh", nil)
			req = req.WithContext(principal.WithPrincipal(ctx, session.Principal))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body["error"])
			assert.NotContains(t, body, "access_token")
			assert.Empty(t, response.Header().Get("WWW-Authenticate"))
			assert.Zero(t, providerCalls.Load())
			persisted, err := sessions.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, session, persisted, "denial must preserve establishment evidence and encrypted credentials")
		})
	}
}
