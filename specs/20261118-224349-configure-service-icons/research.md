# Research: Configurable Third-Party Service Icons

## Scope and selection

The setup script selected `specs/20261118-224349-configure-service-icons/` and reported branch `20261118-224349-configure-service-icons`. The spec's `221-configure-service-icons` label is historical; selection comes from `.specify/feature.json`, not the worktree name. This research follows the SVG-only scope correction dated 2026-11-19.

## 1. Delivery through authenticated JSON

**Decision:** Carry the validated SVG string in existing end-user JSON: `iconSvg` on consent services and service requirements; `service_icon_svg` on session summaries and inherited details. Missing source is omitted; consumers also accept null. Convert the string to `data:image/svg+xml;charset=utf-8,` plus `encodeURIComponent(source)` only at the image boundary. No new HTTP image resource, URL input, revision identifier, hashing, or asset store.

**Rationale:** Existing authenticated data access already determines which service metadata a user may see. Embedding needs zero additional requests and cannot expose a publicly addressable broker SVG document. Changed content changes the image source; removal is absence in fresh data. It also avoids credentials on image fetches and cache invalidation protocols. The source remains JSON text if an API response is opened directly; the broker never serves it as an active SVG document.

**Cost:** An occurrence carries at most 65,536 decoded source bytes. Repeated services repeat that data: 100 maximum-sized occurrences carry 6.25 MiB before JSON escaping and transport compression. JSON escaping may increase wire size; URI encoding adds up to three times the UTF-8 byte count in browser memory. Existing pagination and refresh behavior remain unchanged. Do not claim a new latency target or independent image cache benefit. Include representative maximum-sized and repeated-service fixtures during validation.

**Alternatives considered:** A protected image endpoint reduces repeated JSON bytes and supports independent caching, but adds routing, object access decisions, direct-navigation security headers, revision/cache behavior, and a request per uncached image. Blob URLs avoid percent expansion but introduce ownership and revocation lifecycle. Both add machinery unnecessary for optional small service decoration. An external URL, proxy, or public static path violates the spec.
Frontend source research confirmed that a protected endpoint is viable and recommended it for lower list payloads. This design instead chooses embedding to avoid a new access-controlled active-document resource and its caching protocol; the payload tradeoff above is intentional. Reconsidering that tradeoff requires contract review, not an unplanned implementation switch. Existing consent/session API client caches last five/two minutes respectively; receiving genuinely fresh metadata, not simply invoking a cached refetch, is the freshness boundary in the spec.


## 2. Validation is rejection, not sanitization

**Decision:** Use Go's standard `encoding/xml` strict token parser plus a deliberately small static-vector allowlist. Validate in the third-party domain service before any persistence or discovery side effects. Validate UTF-8 and decoded byte length first. Preserve the accepted source exactly; reject unsupported content rather than stripping it. The normative profile is in `contracts/service-icons.md`.

**Rationale:** There is no SVG validation implementation or dependency in the searched Go source or manifests. A restricted XML vocabulary is smaller and more reviewable than accepting all SVG/CSS and attempting to sanitize it. The parser handles XML syntax; the application validates a finite input language, not custom cryptography. Do not use regular expressions to parse XML. There is no resource resolver or network availability check.

**Alternatives considered:** Browser image-mode isolation alone fails SR-005/SR-006. Generic HTML sanitization does not establish an SVG subset and silently modifying unsafe input violates SR-008. Full SVG/CSS support adds unnecessary parsing and resource-reference complexity.

## 3. Image isolation and browser policy

**Decision:** Render only an `<img>` using the encoded validated source. Retain `frame-ancestors 'none'`; add `img-src 'self' data:` and `object-src 'none'` to the broker SPA policy. No wildcard, remote image allowance, or new script/style permissions. Do not add a broad `default-src` change in this feature.

**Rationale:** `internal/adapters/http/handlers/spa.go` currently sets only the frame-ancestors CSP directive. Explicit image policy permits local brand assets and the chosen data representation while disallowing third-party image requests. XML validation additionally rejects every nested resource-bearing construct, including embedded data URLs. MDN's [SVG as an image](https://developer.mozilla.org/en-US/docs/Web/SVG/Guides/SVG_as_an_image) distinguishes image-mode restrictions from direct document embedding. A data source is not a broker endpoint or authentication bypass: access is controlled when JSON is obtained. Authorized clients can retain or copy received content; logout cannot retract bytes already delivered.

**Alternatives considered:** Weakening CSP for remote images is out of scope. Image response sandbox headers are irrelevant without an image endpoint. A global script policy redesign is unrelated.

## 4. Aggregate ownership and update semantics

**Decision:** Extend the existing third-party service aggregate and its storage records with one optional source; keep sessions free of copied icons. Preserve presence independently from null in Admin writes. Existing service repository operations and concurrency controls remain authoritative. Add one nullable PostgreSQL text column using the next unused atomic migration number, and mirror it in memory.

**Rationale:** The service owns the visual identity; identical sources do not share mutable assets. A plain nullable string request field cannot distinguish omitted and explicit null. Session read projections must join or look up the current service, not serialize the creation-time session state.

**Alternatives considered:** A separate icon aggregate, repository, identifier, object storage, or session snapshot introduces an independent lifecycle the spec excludes. Treating omitted input as null breaks existing clients.

## 5. Rendering recovery and accessible fallback

**Decision:** Compose existing design-system primitives through one service-specific icon renderer. Associate failure with the current source, not the component's entire lifetime. Source change or removal resets image load/error state. Preserve each caller's existing initials or name-only fallback, image slot sizing, visible label, and controls. Decorative images have empty alt text and no focusability.

**Rationale:** The existing Avatar primitive stores image-error and load state. A source-keyed image instance can recover without remounting unrelated controls or altering agent/user avatar behavior. Name-only session fallbacks must not acquire a new placeholder. Service-specific composition avoids duplicating the security-sensitive source conversion across views.

**Alternatives considered:** Page reloads fail US3.3; remounting entire cards loses interaction state; globally redesigning avatars expands scope.

## 6. Governance and validation

**Decision:** Planning produces reviewable proposed contracts; implementation remains gated on stakeholder confirmation, canonical OpenAPI updates, glossary/architecture documentation, and acceptance tests written first. No runtime config, Helm changes, token changes, or new telemetry instruments are needed. Record the SVG trust boundary and delivery choice in a timestamp-named ADR before implementation.

**Rationale:** Constitution IV/X require explicit API confirmation, VIII/XIII require TDD and 1:1 acceptance coverage, IX and accepted `004-storage-layer-architecture` require both adapters and real PostgreSQL migration tests. Accepted `035-root-mounted-spa` keeps the existing root browser routes and API namespaces unchanged.

**Alternatives considered:** Marking API approval complete without a review record or changing canonical contracts silently would misrepresent the gate. Generating `tasks.md` here exceeds the planning command, which ends after Phase 1.

## Resolution

No technical clarification remains. API approval is an outstanding governance gate, not an assumed approval. See the plan's post-design gate table; implementation must not start until its blocking conditions are satisfied.

## Source-backed integration findings

Backend research identified existing `ServiceRequest.UnmarshalJSON` presence checks in `internal/adapters/http/handlers/admin/services_handler.go`; reuse them. The actual persisted merge belongs in provider `prepareUpdate`/`Update`, because `ValidateUpdate` validates a copy. The aggregate entity is under `internal/domain/model/`; storage uses `thirdparty_oauth2_services`. Existing row-version locking remains, but icon-only PUT does not acquire a new mandatory client `If-Match` precondition.

`internal/domain/oauth2session/service.go` already resolves providers for list/detail names; carry the current icon through those paths and refresh summaries. `internal/domain/consent/service.go` requirement resolution does not cover permission-set-only services, and `internal/adapters/http/handlers/consent/agent_detail_handler.go` currently omits the documented `available_services` catalog. A deduplicated, agent-referenced redacted catalog is necessary to supply every required identity row without repeating full source in each permission-set scope. The contract explicitly includes this integration correction and its review requirement. Migration coverage can extend `tests/integration/migrations/framework.go` and existing credential-source migration patterns.
