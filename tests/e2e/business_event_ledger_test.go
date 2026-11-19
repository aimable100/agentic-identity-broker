package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

func businessEventConfig(connectionURL string, enabled bool) *ports.Config {
	cfg := fixtures.DefaultOAuth2Config()
	cfg.Storage = ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: connectionURL},
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
	}
	cfg.BusinessEvents = ports.BusinessEventsConfig{
		Enabled:      enabled,
		WriteTimeout: time.Second,
		Retention: ports.BusinessEventsRetentionConfig{
			Period:    2160 * time.Hour,
			Interval:  time.Hour,
			BatchSize: 1000,
		},
	}
	fixtures.SetTraceResponseEnabled(cfg, true)
	return cfg
}

func committedBusinessEvents(db *sql.DB) []model.BusinessEvent {
	rows, err := db.QueryContext(context.Background(), `
		SELECT id, event, occurred_at, recorded_at, principal, actor, trace_id, attributes
		FROM business_events ORDER BY recorded_at, id`)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { ExpectWithOffset(1, rows.Close()).To(Succeed()) }()
	events := make([]model.BusinessEvent, 0)
	for rows.Next() {
		var event model.BusinessEvent
		var traceID sql.NullString
		var attributes []byte
		ExpectWithOffset(1, rows.Scan(&event.ID, &event.Event, &event.OccurredAt, &event.RecordedAt,
			&event.Principal, &event.Actor, &traceID, &attributes)).To(Succeed())
		event.TraceID = traceID.String
		ExpectWithOffset(1, json.Unmarshal(attributes, &event.Attributes)).To(Succeed())
		events = append(events, event)
	}
	ExpectWithOffset(1, rows.Err()).NotTo(HaveOccurred())
	return events
}

func businessEventCounterValues(reader *sdkmetric.ManualReader) map[string]float64 {
	var metrics metricdata.ResourceMetrics
	ExpectWithOffset(1, reader.Collect(context.Background(), &metrics)).To(Succeed())
	values := make(map[string]float64)
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			key := scope.Scope.Name + "/" + metric.Name
			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if data.IsMonotonic {
					for _, point := range data.DataPoints {
						values[key] += float64(point.Value)
					}
				}
			case metricdata.Sum[float64]:
				if data.IsMonotonic {
					for _, point := range data.DataPoints {
						values[key] += point.Value
					}
				}
			}
		}
	}
	return values
}

func createBusinessEventGrant(client *http.Client, server *bootstrap.TestServer, agentID id.AgentID) id.GrantID {
	req, err := http.NewRequest(http.MethodPost, server.BaseURL()+"/api/consent/agents/"+agentID.String()+"/grants",
		psJSON(map[string]any{"granted_permission_sets": map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
		}}))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remote-User", fixtures.DefaultPrincipal().String())
	resp, err := client.Do(req)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { ExpectWithOffset(1, resp.Body.Close()).To(Succeed()) }()
	body, err := io.ReadAll(resp.Body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusCreated), "%s", body)
	var grant struct {
		Data struct {
			ID id.GrantID `json:"id"`
		} `json:"data"`
	}
	ExpectWithOffset(1, json.Unmarshal(body, &grant)).To(Succeed())
	ExpectWithOffset(1, grant.Data.ID).NotTo(BeZero())
	return grant.Data.ID
}

var _ = Describe("Business event ledger", Label("docker"), func() {
	var (
		postgres *bootstrap.PostgresFixture
		db       *sql.DB
		store    *storageadapter.Adapter
		server   *bootstrap.TestServer
		cfg      *ports.Config
		logger   *slog.Logger
		logs     *bootstrap.BufferedLogCapture
		reader   *sdkmetric.ManualReader
		client   *http.Client
		agent    *storagedomain.Agent
		grantID  id.GrantID
		failing  bool
	)

	revoke := func(status int) string {
		resp, err := server.DirectRequest(http.MethodDelete, "/api/consent/agents/"+agent.ID.String()+"/grants",
			fixtures.DefaultPrincipal().String(), nil, nil)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		defer func() { ExpectWithOffset(1, resp.Body.Close()).To(Succeed()) }()
		_, err = io.Copy(io.Discard, resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		ExpectWithOffset(1, resp.StatusCode).To(Equal(status))
		traceID := requestSecurityTraceID(resp)
		ExpectWithOffset(1, traceID).To(MatchRegexp(`^[0-9a-f]{32}$`))
		return traceID
	}

	BeforeEach(func() {
		postgres, db, store, server = nil, nil, nil, nil
		failing = false
		Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed(), "ledger acceptance journeys require Docker or Podman")
		var err error
		postgres, err = bootstrap.NewPostgresFixture(context.Background())
		Expect(err).NotTo(HaveOccurred())
		db, err = sql.Open("pgx", postgres.ConnectionURL)
		Expect(err).NotTo(HaveOccurred())
		cfg = businessEventConfig(postgres.ConnectionURL, true)
		logger, logs = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
		store, err = storageadapter.NewAdapter(&cfg.Storage)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), store, id.NewPrincipal(fixtures.DefaultPrincipal().String()))).To(Succeed())
		agent = fixtures.ValidAgent()
		Expect(store.Agents().Create(context.Background(), agent)).To(Succeed())
		client = &http.Client{Timeout: 10 * time.Second}

		previous := otel.GetMeterProvider()
		reader = sdkmetric.NewManualReader()
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		otel.SetMeterProvider(provider)
		DeferCleanup(func() {
			otel.SetMeterProvider(previous)
			Expect(provider.Shutdown(context.Background())).To(Succeed())
		})
	})

	JustBeforeEach(func() {
		application, err := bootstrap.NewServerFactory(cfg, logger).BuildApp(store)
		Expect(err).NotTo(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		grantID = createBusinessEventGrant(client, server, agent.ID)
		Expect(committedBusinessEvents(db)).To(BeEmpty(), "ordinary grant creation is not a business event")
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if client != nil {
			client.CloseIdleConnections()
		}
		if failing && db != nil {
			_, err := db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS reject_business_event ON business_events")
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), "DROP FUNCTION IF EXISTS reject_business_event_insert()")
			Expect(err).NotTo(HaveOccurred())
		}
		if store != nil {
			Expect(store.Close(context.Background())).To(Succeed())
		}
		if db != nil {
			Expect(db.Close()).To(Succeed())
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
		}
	})

	Context("when the ledger is enabled and Alice has created a grant", func() {
		// Scenario US1.1 from specs/048-business-event-ledger/spec.md (User Story 1)
		It("stores exactly one consent.grant.revoked event with request and resource identities", func() {
			traceID := revoke(http.StatusNoContent)
			events := committedBusinessEvents(db)
			Expect(events).To(HaveLen(1))
			Expect(events).To(matchers.HaveBusinessEvent("consent.grant.revoked", fixtures.DefaultPrincipal().String()))
			Expect(events[0].ID).NotTo(BeZero())
			Expect(events[0].OccurredAt).NotTo(BeZero())
			Expect(events[0].RecordedAt).NotTo(BeZero())
			Expect(events[0].Actor).To(HaveValue(Equal(fixtures.DefaultPrincipal().String())))
			Expect(events[0].TraceID).To(Equal(traceID))
			Expect(events[0].Attributes).To(HaveKeyWithValue("agent_id", agent.ID.String()))
			Expect(events[0].Attributes).To(HaveKeyWithValue("grant_id", grantID.String()))
		})
	})

	Context("when the ledger is enabled and Alice has already revoked her grant", func() {
		var original []model.BusinessEvent
		JustBeforeEach(func() {
			revoke(http.StatusNoContent)
			original = committedBusinessEvents(db)
			Expect(original).To(HaveLen(1))
		})

		// Scenario US1.2 from specs/048-business-event-ledger/spec.md (User Story 1)
		It("keeps exactly one event when a second revocation returns 404", func() {
			revoke(http.StatusNotFound)
			Expect(committedBusinessEvents(db)).To(Equal(original))
			Expect(proactiveEvents(logs, "consent.grant.revoked")).To(HaveLen(1))
		})
	})

	Context("when the ledger is disabled", func() {
		BeforeEach(func() { cfg.BusinessEvents.Enabled = false })

		// Scenario US1.3 from specs/048-business-event-ledger/spec.md (User Story 1)
		It("stores no event and still logs the revocation when the ledger is disabled", func() {
			traceID := revoke(http.StatusNoContent)
			Expect(committedBusinessEvents(db)).To(BeEmpty())
			records := proactiveEvents(logs, "consent.grant.revoked")
			Expect(records).To(HaveLen(1))
			Expect(records[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(records[0]).To(HaveKeyWithValue("grant_id", grantID.String()))
		})
	})

	Context("when the ledger is enabled and PostgreSQL rejects event inserts", func() {
		BeforeEach(func() {
			failing = true
			_, err := db.ExecContext(context.Background(), `
				CREATE FUNCTION reject_business_event_insert() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'ledger insert rejected'; END; $$`)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.ExecContext(context.Background(), `
				CREATE TRIGGER reject_business_event BEFORE INSERT ON business_events
				FOR EACH ROW EXECUTE FUNCTION reject_business_event_insert()`)
			Expect(err).NotTo(HaveOccurred())
		})

		// Scenario US1.4 from specs/048-business-event-ledger/spec.md (User Story 1)
		It("returns 204 and reports exactly one failed write without losing the audit record", func() {
			before := businessEventCounterValues(reader)["business_events/business_events.write_failures"]
			traceID := revoke(http.StatusNoContent)
			Expect(committedBusinessEvents(db)).To(BeEmpty())
			Expect(businessEventCounterValues(reader)["business_events/business_events.write_failures"] - before).To(Equal(float64(1)))
			audit := proactiveEvents(logs, "consent.grant.revoked")
			failed := proactiveEvents(logs, "business_event.ledger.write_failed")
			Expect(audit).To(HaveLen(1))
			Expect(failed).To(HaveLen(1))
			Expect(audit[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(failed[0]).To(HaveKeyWithValue("trace_id", traceID))
			Expect(failed[0]).To(HaveKeyWithValue("level", "ERROR"))
			Expect(failed[0]).To(HaveKeyWithValue("failed_event", "consent.grant.revoked"))
		})
	})

	Context("when one replica runs retention with an expired and a fresh event", func() {
		BeforeEach(func() {
			cfg.BusinessEvents.Retention.Period = 24 * time.Hour
			cfg.BusinessEvents.Retention.Interval = time.Second
		})

		// Scenario US2.1 from specs/048-business-event-ledger/spec.md (User Story 2)
		It("deletes only the expired event without a deletion log or counter", func() {
			revoke(http.StatusNoContent)
			first := committedBusinessEvents(db)
			Expect(first).To(HaveLen(1))
			secondGrant := createBusinessEventGrant(client, server, agent.ID)
			Expect(secondGrant).NotTo(Equal(grantID))
			revoke(http.StatusNoContent)
			both := committedBusinessEvents(db)
			Expect(both).To(HaveLen(2), "both real revocations must be committed before either event is aged")
			Expect(both[0].ID).To(Equal(first[0].ID))
			Expect(both[1].Attributes).To(HaveKeyWithValue("grant_id", secondGrant.String()))
			counters := businessEventCounterValues(reader)
			logs.Reset()
			result, err := db.ExecContext(context.Background(), `
				UPDATE business_events SET recorded_at = now() - interval '2 days' WHERE id = $1`, first[0].ID)
			Expect(err).NotTo(HaveOccurred())
			changed, err := result.RowsAffected()
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(Equal(int64(1)))
			Eventually(func() []model.BusinessEvent { return committedBusinessEvents(db) }, 5*time.Second, 50*time.Millisecond).
				Should(Equal(both[1:]))
			records, err := logs.Records()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(BeEmpty(), "successful retention must not log deletion")
			Expect(businessEventCounterValues(reader)).To(Equal(counters), "successful retention must not increment a counter")
		})
	})
})
