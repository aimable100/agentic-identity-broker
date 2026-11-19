# Service Icons: Validation Guide

This guide validates the proposed feature after implementation. It does not claim that the current broker implements it. See [the contract](contracts/service-icons.md) for field names and the SVG profile, and [the model](data-model.md) for lifecycle semantics.

## Prerequisites

- Install the Go version in `go.mod`, Node/npm per `web/package.json`, `just`, and the repository test tools: `just deps`, `just install-tools`, `just web-install`.
- Browser tests need the Playwright browsers required by the existing frontend harness. PostgreSQL durability/migration tests need Docker or Podman; follow `tests/integration/storage/README.md`.
- Complete the plan's blocking design gates, especially canonical OpenAPI publication and stakeholder approval, before implementing behavior.
- Use synthetic principals, service credentials and SVG documents. Never paste real tokens or source documents into logs or trace attributes.

## Automated checks

Run from repository root after the implementation has been written test-first:

```sh
just check
just test
just web-test
just test-integration-infra
just test-e2e-backend
just test-e2e-docker
just test-e2e-frontend
```

These are existing recipes, not new feature-specific commands. Use focused package/test selections while developing, then run `just verify` before the PR as required by `CONTRIBUTING.md`. The container-backed restart scenario belongs in the docker-labeled suite and must not silently use memory storage. Screenshots go to `tests/e2e/screenshots/`; enable the harness's screenshot capture setting when running browser tests.

Expected results: all twelve acceptance scenarios have one corresponding Ginkgo `It` under `tests/e2e/`, browser assertions cover all meaningful visual states, and production PostgreSQL migration apply/down/reapply tests pass. No pending/skipped cases or placeholder assertions. Compile tests before implementing behavior; local container red runs may be omitted only under the constitution's infrastructure exception. Do not require archived red-run evidence.

Canonical US1.1 and US1.3 cases belong in `tests/e2e/frontend/service_icons_test.go`: US1.1 must show decoded icons beside names after a real PostgreSQL broker restart without reseeding; US1.3 must show retained icons after omission and each view's original fallback after explicit null and fresh responses. Backend projection assertions are supplemental, not substitutes for those visual outcomes. Write consent/session caller and API-mapping unit tests before their integrations, not in the later US3 verification phase.

## End-to-end journeys

### Configure, persist, replace (US1.1, US1.2, US2.1, US2.2)

1. Through the existing Admin client/harness, create Atlas Files using Journey 1's complete service body from `spec.md`, including the permitted rectangle SVG. Use the existing trusted-proxy authentication mechanism, not a public unauthenticated listener.
2. Assert 201 and exact source on create, detail and list. Set up the existing permission-set, grant and OAuth2 session journey for Alice and Data Analysis Assistant.
3. With PostgreSQL configured by the existing test bootstrap, close the broker and reopen it against the same database. Read again through Admin and authenticated end-user HTTP, without reseeding the service.
4. Open `/agents/{agent-id}` and `/sessions` in the browser. Inspect service cards, permission-set rows, mandatory/optional requirements, session cards, and the session details surface. They show an image beside the readable service name.
5. Replace the document with Journey 1's circle SVG using the existing full PUT and its actual concurrency precondition. Refresh data without changing the service ID, credentials, grants or session lifecycle. All affected images show the current source.

### Preserve, clear, reject (US1.3, US1.4, US2.3, US2.4)

1. Submit an otherwise valid full PUT without `icon_svg`; confirm its previous source remains on subsequent reads. Then send explicit null; confirm Admin and end-user reads omit the source and all views return to their prior fallback.
2. Restore a valid document. Submit URL/path, empty/whitespace, malformed XML, wrong namespace, DOCTYPE/entity, script, event handler, foreign content, animation, styling and external-reference fixtures. Include credential/metadata changes in rejected writes; compare all stored fields after each rejection.
3. Test permitted ASCII documents at exactly 65,536 decoded UTF-8 bytes and 65,537 bytes. The first is accepted if its XML/profile is valid; the latter returns 400. Separately use oversized well-formed multibyte UTF-8 fixtures to distinguish byte count from character count and exercise size rejection before profile validation; the attribute profile does not promise valid Unicode values. Do not measure JSON wire length. Exercise each numeric/unit/range/list/enum/transform rule and the deliberately lexical path/paint rules in the contract; a valid but blank drawing need not raise an image load error.
4. Without authentication, call the same consent and session routes used above. Assert their existing failure statuses and no source bytes. There is no icon URL to probe.
5. Record browser requests and monitor a synthetic remote target: accepted rendering and rejected source submission must trigger no icon-related request to another origin. Assert no SVG children appear in the HTML DOM and no injected script executes.

### Fallback, recovery, accessibility (US3.1–US3.4)

1. Load services with no metadata, including a pre-migration service. Confirm original initials/placeholders or name-only headers, plus working connection, consent, refresh and termination controls.
2. Cause a deterministic browser image error for the configured data source through the browser test harness, without bypassing server validation or persisting an unsafe document. Confirm no broken-image marker or page-level error.
3. While the page remains open, replace the source through Admin. Trigger the existing data refresh. Assert a new image load attempt succeeds and the failure state does not suppress the replacement. Clear the source and confirm removal in the same open view.
4. Test narrow and wide viewports and keyboard navigation. Images are decorative with empty alt text; service names remain accessible once, controls remain operable, and no extra focus stop is introduced. Capture configured, unset, failed and recovered states for consent and sessions with descriptive filenames.
5. Repeat a replacement for one of two services sharing identical SVG content; the other must remain unchanged. Exercise repeated maximum-sized sources to expose serialization/rendering problems without introducing a new benchmark target.

## Completion evidence

Report actual test commands and outcomes, migration durability results, and browser screenshot paths. Verify the built SPA response preserves its security headers and includes the exact image/object policy in the contract. API definitions and examples in `api/admin/openapi.yaml`, `api/enduser/openapi.yaml` and rendered `docs/api/` must agree with the accepted design. No additional runtime configuration, icon endpoint or remote asset deployment is required.
