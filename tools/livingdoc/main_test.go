package main

import (
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
)

const page = "# Agents\n\n" +
	"Creating works (`example:agents.create`) and replays are safe (`example:agents.replay`).\n\n" +
	"```http example=\"agents.create\"\n" +
	"PUT /api/agents?external_id=urn%3Ax\n" +
	"    &dry=1 HTTP/1.1\n" +
	"Content-Type: application/json\n" +
	"\n" +
	"{\"display_name\": \"Research Assistant\"}\n" +
	"\n" +
	"HTTP/1.1 201 Created\n" +
	"Location: /api/agents/550e8400-e29b-41d4-a716-446655440000\n" +
	"\n" +
	"{\n" +
	"  \"id\": \"550e8400-e29b-41d4-a716-446655440000\",\n" +
	"  \"created_at\": \"2026-10-30T09:00:00Z\",\n" +
	"  \"updated_at\": \"2026-10-30T09:00:00Z\"\n" +
	"}\n" +
	"```\n\n" +
	"```http\nGET /not-an-example HTTP/1.1\n```\n"

const recordedAgentID = "0b9f4c0e-51c1-4d0b-9a55-7f2f6f0f3c11"

func recorded(agentID string) evidence.Exchange {
	return evidence.Exchange{
		Method: "PUT", Target: "/api/agents?external_id=urn%3Ax&dry=1",
		RequestHeader:  map[string][]string{"Content-Type": {"application/json"}, "X-Remote-User": {"admin"}},
		RequestBody:    `{"display_name":"Research Assistant"}`,
		Status:         201,
		ResponseHeader: map[string][]string{"Location": {"/api/agents/" + recordedAgentID}, "Date": {"now"}},
		ResponseBody:   `{"updated_at":"2026-11-16T10:00:05Z","created_at":"2026-11-16T10:00:00Z","id":"` + agentID + `"}`,
	}
}

func verdictsByKind(t *testing.T, records []evidence.Record) map[string]string {
	t.Helper()
	claims, err := parsePage("agents.md", page)
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}
	if len(claims) != 3 {
		t.Fatalf("expected 3 claims, got %d: %+v", len(claims), claims)
	}
	got := map[string]string{}
	for _, result := range check(claims, records) {
		kind := "prose"
		if result.Claim.Exchange != nil {
			kind = "http"
		}
		got[result.Claim.Example+"/"+kind] = result.Verdict
	}
	return got
}

func TestCheck_MatchesRecordedExchangeDespiteVolatileValues(t *testing.T) {
	got := verdictsByKind(t, []evidence.Record{
		{Example: "agents.create", State: evidence.StatePassed, Exchanges: []evidence.Exchange{
			{Method: "GET", Target: "/api/agents", Status: 200, ResponseBody: "[]"},
			recorded(recordedAgentID),
		}},
		{Example: "agents.replay", State: evidence.StateSkipped},
	})
	want := map[string]string{
		"agents.create/prose": verdictPassed, "agents.create/http": verdictPassed, "agents.replay/prose": verdictNotRun,
	}
	for key, verdict := range want {
		if got[key] != verdict {
			t.Errorf("%s: got %q, want %q", key, got[key], verdict)
		}
	}
}

func TestCheck_ReportsStaleWhenDocumentedIdentityNoLongerHolds(t *testing.T) {
	records := []evidence.Record{{Example: "agents.create", State: evidence.StatePassed, Exchanges: []evidence.Exchange{
		recorded("9d2c7a55-0c1e-4a57-8f43-2f5f7d6f1a20"),
	}}}
	claims, err := parsePage("agents.md", page)
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}
	for _, result := range check(claims, records) {
		if result.Claim.Example != "agents.create" || result.Claim.Exchange == nil {
			continue
		}
		if result.Verdict != verdictStale {
			t.Fatalf("got %q, want %q", result.Verdict, verdictStale)
		}
		if !strings.Contains(result.Detail, "-") || !strings.Contains(result.Detail, "+") {
			t.Fatalf("expected a diff, got %q", result.Detail)
		}
		return
	}
	t.Fatal("no HTTP claim checked")
}

func TestCheck_FailedScenarioAndUnboundExample(t *testing.T) {
	got := verdictsByKind(t, []evidence.Record{
		{Example: "agents.create", State: evidence.StatePassed, Exchanges: []evidence.Exchange{recorded(recordedAgentID)}},
		{Example: "agents.create", Spec: "second scenario", State: evidence.StateFailed},
	})
	if got["agents.create/http"] != verdictFailed || got["agents.create/prose"] != verdictFailed {
		t.Errorf("failed scenario must fail every claim of its example: %v", got)
	}
	if got["agents.replay/prose"] != verdictUnbound {
		t.Errorf("example without evidence must be unbound: %v", got)
	}
}

func TestUndocumented_ListsEvidenceNoPageCites(t *testing.T) {
	claims, err := parsePage("agents.md", page)
	if err != nil {
		t.Fatalf("parsePage: %v", err)
	}
	got := undocumented(claims, []evidence.Record{{Example: "agents.create"}, {Example: "agents.orphan"}})
	if len(got) != 1 || got[0] != "agents.orphan" {
		t.Errorf("got %v", got)
	}
}

func TestParsePage_RejectsExampleBlockWithoutResponse(t *testing.T) {
	_, err := parsePage("bad.md", "```http example=\"x\"\nGET / HTTP/1.1\n```\n")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestFailing_NotRunFailsOnlyWhenStrict(t *testing.T) {
	results := []Result{{Verdict: verdictPassed}, {Verdict: verdictNotRun}}
	if failing(results, false) {
		t.Error("not-run must not fail a lenient check")
	}
	if !failing(results, true) {
		t.Error("not-run must fail a strict check")
	}
	if !failing([]Result{{Verdict: verdictStale}}, false) {
		t.Error("stale must always fail")
	}
}
