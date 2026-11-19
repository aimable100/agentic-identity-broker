# Implementation Plan: Configurable Third-Party Service Icons

**Branch**: `20261118-224349-configure-service-icons` | **Date**: 2026-11-19 | **Spec**: [spec.md](spec.md)

**Input**: `specs/20261118-224349-configure-service-icons/spec.md`, including the SVG-only scope correction. Setup selected this directory through `.specify/feature.json`; the older feature-branch label in the spec is not the selection source.

## Summary

Extend the existing third-party service with an optional validated static SVG source. Admin writes distinguish omission, null and replacement. Both storage adapters retain it; authenticated consent and session projections return the current source. The frontend converts it to a data image and renders only through `<img>`, retaining each view's fallback and recovering after source changes. No external URLs, asset endpoint, object store, new configuration or token changes.

Phase 0 research is recorded in [research.md](research.md). Phase 1 defines [data-model.md](data-model.md), [contracts/service-icons.md](contracts/service-icons.md), [contracts/schemas.yaml](contracts/schemas.yaml) and [quickstart.md](quickstart.md). This command does not create `tasks.md` or implement behavior.

## Technical Context

**Language/Version**: Go 1.27.4 (`go.mod`); React 19 and TypeScript 6.0.x (`web/package.json`, `CONTRIBUTING.md`).

**Primary Dependencies**: Existing chi v5 routing, sqlx/pgx PostgreSQL persistence, standard-library `encoding/xml` strict parsing, React design-system primitives and current API clients. No additional dependency is selected.

**Storage**: Existing service aggregate, memory and PostgreSQL records; one nullable `icon_svg` text column with atomic up/down migrations. No session copy or new repository.

**Testing**: Go unit tests, table-driven validation, Ginkgo/Gomega production-bootstrap E2E, real PostgreSQL integration tests, Vitest/React tests and the existing Go Playwright browser harness.

**Target Platform**: Existing broker server and root-mounted browser SPA; production PostgreSQL, process-local memory for development/tests.

**Project Type**: Existing Go/React monorepo, not a new application or bounded context.

**Performance Goals**: Zero icon-related external requests and zero additional image HTTP requests. Validate bounded input once per write; avoid per-session/per-row database lookups added solely for icons. No new latency SLO is asserted.

**Constraints**: At most 65,536 decoded UTF-8 bytes per source; reject unsupported XML rather than sanitize; authenticated JSON access; image-mode rendering only; preserve existing controls, concurrency and fresh-data behavior. Embedding repeats source per occurrence: 100 maximum-size occurrences are 6.25 MiB before JSON escaping/compression. URI encoding increases browser memory; see research for the deliberate tradeoff.

**Scale/Scope**: Admin service create/update/read/list; scoped consent services, permission-set service rows, mandatory/optional requirements, session list/detail headers; twelve acceptance scenarios. No Admin icon editor, provider discovery, live updates, raster images, unrelated avatars, or infrastructure changes.

## Constitution Check

### Before Phase 0

The spec identifies aggregate ownership, source semantics, security requirements, UI scope and all twelve acceptance scenarios. Research can proceed without assuming an API approval or changing existing behavior. Accepted `004-storage-layer-architecture` and `035-root-mounted-spa` remain binding; no deviation is proposed.

### After Phase 1

| Principles / gate | Design result | Before implementation |
|---|---|---|
| I, III: fail-closed/library-first security | Strict standard XML parser, finite static-vector profile, no resource resolution, image-mode isolation and scoped CSP | Implement adversarial tests first; security review the vocabulary and delivery ADR |
| II, V: architecture/domain | Service-owned optional value, no new entity or typed ID | Add Service Icon glossary entry, architecture trust-boundary/performance text and timestamp-named ADR for delivery/profile; obtain acceptance before dependent implementation |
| IV, X: API-first | Proposed OpenAPI schema additions and complete behavioral contract in `contracts/` | **BLOCKED: no stakeholder API confirmation is recorded.** Update both canonical OpenAPI documents, reconcile `logoUrl`, examples and rendered `docs/api/`, then obtain written confirmation |
| VI: hexagonal separation | Domain service enforces validation; existing ports/adapters carry values and projections | No handler-to-storage shortcut or cross-adapter dependency |
| VII: configuration | No new config or toggles | YAML examples and Helm changes are not applicable |
| VIII, XIII: TDD and acceptance | Twelve scenario mappings below; browser/screenshot and restart validation designed | Write meaningful compiling acceptance/unit tests before behavior; container red runs follow the existing infrastructure exception; do not require stored red-run evidence |
| IX: persistence | Existing repositories, both adapters, one nullable service column | Atomic next-number migration, production-chain PostgreSQL apply/down/reapply and restart tests |
| XI: design system | Reuse primitive styling and application-level service icon composition; light theme and existing fallback | Review design-system guidance, narrow/wide browser accessibility and meaningful screenshots |
| XII: builder | No new service/route required | Keep existing construction and injection boundaries; any changed dependency remains builder-owned |
| XIV: reviewable examples | Spec has linked HTTP journeys and editable/rendered Wireloom states | Reconcile omitted end-user representation with the accepted contract before implementation |
| XV: coding standards | No new telemetry instrument is planned | Read LOGGING/OPENTELEMETRY before changing their constructs; never emit source in telemetry; preserve required audit behavior |

**Gate outcome:** Technical design is resolved and Phase 1 artifacts are complete. Implementation is **not authorized**: API confirmation, canonical contract publication, glossary/ADR work and test-first preconditions remain blocking. These are explicit delivery prerequisites, not waived constitution violations. Do not mark them complete without evidence. No unresolved technical clarification or justified deviation is carried forward.

## Project Structure

### Documentation

```text
specs/20261118-224349-configure-service-icons/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── service-icons.md
│   └── schemas.yaml
└── wireframes/
```

`tasks.md` belongs to the subsequent `/speckit-tasks` command.

### Source areas

```text
internal/domain/thirdparty/              # invariant enforcement in existing service
internal/domain/storage/                 # aggregate and session read models
internal/domain/model/                   # third-party provider entity
internal/domain/consent/                 # current service projections
internal/domain/oauth2session/           # session read orchestration where needed
internal/adapters/http/handlers/admin/   # request presence and response mapping
internal/adapters/http/handlers/         # end-user mappings and SPA CSP
internal/adapters/storage/memory/        # service records and projections
internal/adapters/storage/postgres/      # records, explicit SQL and projections
api/admin/openapi.yaml
api/enduser/openapi.yaml
migrations/                             # next unused atomic up/down pair
web/src/components/consent/
web/src/components/sessions/
web/src/pages/
web/src/services/api/
web/src/design-system/components/primitives/Avatar/
tests/e2e/
tests/e2e/frontend/
tests/integration/storage/
docs/api/
```

**Structure decision:** Extend the existing service, request/record conversions and read projections. Keep source-to-image conversion and source-keyed failure state in one service-specific composition; do not redesign agent/user avatars. Existing `Avatar` provides image/initials behavior, but session name-only fallback must remain name-only. A source-keyed image instance can reset errors without remounting controls. Do not introduce a second general-purpose image abstraction or asset subsystem.

Concrete frontend propagation targets are `web/src/types/consent.ts`, `web/src/services/api/consent.ts`, `web/src/services/api/sessions.ts`, and the available-service mapping in `AgentGrantDetailPage.tsx` through `PermissionSetsList.tsx` to `PermissionSetCard.tsx`. Session details are displayed in `TerminationDialog.tsx` via `ThirdPartySessionsPage.tsx`, not a separate details page. Decorate its service identity header only; explanatory/confirmation prose stays text-only. The live requirement path uses `ServiceCard`; do not expand scope to unused requirement components solely because their names match.

Consent detail is cached for five minutes and session reads for two minutes in the existing API clients. A hook `refetch` is not proof of fresh network data. Browser tests must expire/invalidate through supported existing cache APIs or wait through the existing lifetime, and observe a new response before asserting recovery. This feature does not change those cache lifetimes or add live invalidation.

Backend integration must reuse `ServiceRequest.UnmarshalJSON` presence handling and carry icon presence into the actual `prepareUpdate`/`Update` path: `ValidateUpdate` only validates a copy and cannot perform the persisted merge. The aggregate is `internal/domain/model/thirdparty_oauth2_provider.go`; the PostgreSQL table is `thirdparty_oauth2_services`. Clear request-only flags in memory records. Preserve ETag locking, but do not newly require `If-Match` for icon-only writes: the existing client precondition requirement is tied to replacing `protected_resources`.

Permission-set-only services need an explicit catalog: current `AgentDetailData` does not emit the documented `available_services`, and requirement resolution alone omits those services. Populate a deduplicated catalog covering the current agent's requirement and permission-set service IDs in the domain service; expose redacted `AvailableService` entries with optional `icon_svg`, and map that catalog into frontend available-service rows. Do not duplicate full SVG source in permission-set scope entries or expose all unrelated administrative services. This minimum metadata-path correction is required by FR-007/008 and requires API confirmation alongside the new fields.

## Implementation Phase Overview

| Phase | Purpose | Scope |
|---|---|---|
| Setup | Confirm selected feature and read nearest routed instructions | Existing toolchain only |
| Mandatory design preconditions | Review/approve contracts; update glossary/architecture; accept delivery/profile ADR; design migration; write acceptance tests first | No behavior before gates pass |
| Foundation | Domain SVG value validation, presence-aware writes, both storage adapters, current service projections | FR-001–FR-006, FR-013/015, DB/SR requirements |
| Story 1 | Admin lifecycle and current consent/session metadata | US1.1–US1.4 |
| Story 2 | Restricted SVG validation, authenticated delivery, image boundary and CSP | US2.1–US2.4; security is part of initial delivery, not a later toggle |
| Story 3 | All identity surfaces, original fallbacks and source-change recovery | US3.1–US3.4 |
| Mandatory compliance | Focused tests, real PostgreSQL migrations/restart, browser proof/screenshots, docs and full PR gate | All requirements |

Skip pre-feature refactoring: no demonstrated structural blocker. Skip entity scaffolding: no new entity, identifier or repository is introduced. Do not ship intermediate stubs or a partially secured icon path. Detailed dependency ordering belongs in `tasks.md`.

## Testing Strategy

### Acceptance mapping

Use the existing Ginkgo production bootstrap, fixtures, authentication helpers and fresh isolated storage. Proposed test files are `tests/e2e/service_icons_test.go` and `tests/e2e/frontend/service_icons_test.go`; they are design targets, not existing tests. A scenario requiring browser observables can have its canonical `It` in the frontend suite under `tests/e2e/`, avoiding an API-only substitute for UI/security assertions. Supplemental unit/integration tests are not additional acceptance mappings.

| Scenario | Canonical suite | Required observable |
|---|---|---|
| US1.1 | Browser, docker-labeled | Admin create/read/list, real PostgreSQL broker restart without reseeding, then decoded icons beside visible service names in fresh consent/session views |
| US1.2 | Backend | Replacement preserves ID, grants, session and unrelated OAuth settings |
| US1.3 | Browser | Omitted-field update preserves source and displayed icons; explicit null clears source, then fresh consent/session views show each original fallback |
| US1.4 | Browser | URL/path/empty rejection, unchanged service, no resource request |
| US2.1 | Browser, docker-labeled | Exact source survives its own persistent restart journey; img-only rendering, no SVG DOM/script/remote fetch |
| US2.2 | Browser | Fresh data displays replacement, not predecessor |
| US2.3 | Browser | Malformed/oversized/unsafe writes reject atomically; no rejected rendering/execution/fetch |
| US2.4 | Backend | Existing authentication rejects consent and session access without source bytes |
| US3.1 | Browser | Existing and pre-migration services keep each view's fallback and controls |
| US3.2 | Browser | Deterministic image failure shows fallback, no page error, working actions |
| US3.3 | Browser | Source change after error retries successfully without page reload |
| US3.4 | Browser | Narrow/wide readable labels, decorative images, keyboard-operable controls |

Each canonical `It` must reference exactly its scenario and verify the complete journey rather than a single endpoint segment. Use stored production data across restart without reseeding it. Read the frontend harness's supported persistent bootstrap before implementing; use real PostgreSQL for durability rather than pretending memory survives process restart. Tests must compile and fail semantically when behavior is absent; no pending/skipped cases, vacuous assertions or test-only production APIs. No red-run log archive is required.

US1.1 and US1.3 each own their complete visual journey in the browser suite; backend unit/integration checks supplement rather than replace it. US1.1 must inspect rendered icons after the persisted broker restart. US1.3 must inspect rendered icons after omission and the original per-view fallbacks after clearing, observing fresh responses rather than asserting source absence alone. Keep US2.1's separate scenario-specific restart/isolation journey; do not combine multiple acceptance scenarios into one `It`.

### Browser and screenshots

Cover ServiceCard, PermissionSetCard service rows, mandatory/optional requirement rows, SessionCard and session detail headers. Capture `service_icons_{consent,sessions}_{unset,configured,failed,recovered}_{narrow,wide}.png` for meaningful states actually exercised under `tests/e2e/screenshots/`. Assert empty image alt text and no extra focus stops/duplicate announcements. Use deterministic browser image-error injection against valid stored SVG, not unsafe input or an imaginary image HTTP route. Refresh current data through existing hooks; do not reload the page to pass recovery.

Verify the built SPA headers and browser network activity; mocked image events alone do not prove image-mode isolation. URI encoding must preserve Unicode, XML delimiters and fragment characters. Clearing must remove previous metadata rather than leave a stale merged field. Keep existing buttons operable during loading and failures.

### Unit and integration

- Table-driven domain tests: accepted static vectors, exact byte boundary with valid ASCII plus oversized well-formed multibyte UTF-8 fixtures, XML syntax/namespace/attribute adversaries, hidden entity/URL references, and rejection without side effects. Cover every attribute-value rule in `contracts/service-icons.md`, including intentional lexical acceptance of path data/unknown alphabetic paint names; the browser interprets geometry and a blank image is not necessarily a load error.
- Handler tests: field presence/type parsing, null vs omission, existing error envelope/auth/concurrency behavior, exact create/update/read/list representation.
- Memory and PostgreSQL service record parity; independent services sharing identical strings; current consent/session reads after replacement and clearing; no snapshot behavior or new N+1 query pattern.
- Real PostgreSQL production migration chain: pre-existing rows remain absent without timestamp changes; up/down/up preserves unrelated data; configured source survives broker/storage reconnect and restart. Rollback intentionally removes icon metadata only.
- Frontend component tests: write caller-specific consent/session and API-mapping unit tests before mapping/UI integration, in addition to shared image tests before the renderer. Cover original fallback by surface, changed-source recovery, unchanged-source stability, Unicode-safe encoding, decorative semantics and no disruption to control state. US3's later phase proves the existing behavior in the browser; it must not defer baseline unit tests until after integration.
- Scoped CSP regression tests: retain existing headers; allow local/data images, reject remote images and object embedding. Do not weaken script/style controls.

Fixtures include Atlas Files, a second independent service with identical source, agent/permission set/grant/session from existing factories, rectangle and circle documents, absent source, maximum-size valid SVG, prohibited XML corpus, and deterministic browser image failure. Reuse existing fixtures and mock OAuth providers; do not create remote image infrastructure.

### Commands and evidence

[quickstart.md](quickstart.md) provides runnable repository commands and expected journeys. After Go/test changes run `just check`, then the smallest relevant tests; use existing backend, docker, browser and integration recipes for the affected suites. Before PR run `just verify` per `CONTRIBUTING.md`. Planning itself changes no runtime behavior and does not claim those suites have run.

## Complexity Tracking

No constitution deviation is requested. The deliberate payload tradeoff is bounded per icon, documented in research, and avoids a separate asset lifecycle. API approval and other preconditions remain gates, not exceptions. No unrelated cleanup, speculative caching, new telemetry or configuration is included.
