# Proposed Service Icon Contract

Status: designed for stakeholder review, not approval to implement. Incorporate the schemas in `schemas.yaml` into both canonical OpenAPI documents, with endpoint examples and existing authentication/error definitions, before behavior changes. Additive icon fields do not authorize changes to any existing field or API versioning rule.

**Approved design — 2026-11-19**: Browser image mode is the primary rendering security boundary. Upload checks reject malformed/oversized source and named active constructs, not every possible unsafe CSS/reference/animation behavior. Rendering-resource budgets are explicitly out of scope for operator-provided artwork. API field/delivery approval and ADR acceptance remain separate implementation gates.

## Admin API

Existing `POST /api/services`, `PUT /api/services/{service-id}`, and their detail/list reads gain `icon_svg`. Create/update input is optional and nullable. A non-null value must satisfy the SVG profile below. Omission creates no icon or preserves an existing icon; null creates no icon or clears an existing icon. Admin responses omit the field when absent and return the accepted source exactly when present. A JSON number, object, array, empty string, whitespace-only string, URL, or path is invalid.

Successful writes retain existing 201/200 status, response envelope, authorization, and concurrency requirements. Invalid icon input returns existing Admin 400 validation errors, for example `{"error":"validation failed","message":"icon_svg contains prohibited content"}`. Size and malformed-document failures use the existing validation error envelope without echoing the document. No partial update, discovery fetch, or referenced-resource fetch occurs for an invalid icon. Existing authentication, not-found, and concurrency error behavior is unchanged.

## End-user API

| Existing schema | New property | Consumers |
|---|---|---|
| `ThirdpartyService` | `iconSvg` | Scoped consent service identities |
| `ServiceRequirementForUser` | `iconSvg` | Mandatory and optional service requirements |
| `UserSessionSummary` | `service_icon_svg` | Session list and inherited `SessionStatus` details |
| `AvailableService` | `icon_svg` | Deduplicated `available_services` catalog for permission-set-only service rows |

Example consent service excerpt:

```json
{"serviceId":"880e8400-e29b-41d4-a716-446655440010","displayName":"Atlas Files","iconSvg":"<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M4 4h16v16H4z\"/></svg>","scopes":[{"value":"files.read","description":"Read files"}]}
```

Session list/detail use the same source under `service_icon_svg`. Requirement entries use `iconSvg` beside their existing `serviceId` and `serviceName`. These extend the existing authenticated consent and `/api/third-party/sessions` responses; no separate endpoint is added. Detail responses and any refresh response returning a session summary use the same projection.

The server omits absent source. Clients accept omission or null and replace their prior metadata rather than merging a cleared field back from stale state. A new source replaces the previous image; identical source is identical content. No revision token, image cache lifetime, or obsolete-resource status is needed. Existing JSON caching/authentication rules apply; do not introduce shared/public caching. Unauthenticated requests receive the existing route's authentication failure and no icon bytes.

The current handler does not emit the already-documented `available_services` catalog. Emit one redacted entry per distinct service referenced by this agent's requirements or assigned permission sets, using `id`, `display_name` and optional `icon_svg`. Example: `{"id":"880e8400-e29b-41d4-a716-446655440010","display_name":"Atlas Files"}` for an absent icon. Resolve this catalog in the domain service, map it through the API client, and use it for permission-set identity rows. Do not place source documents on individual permission-set scope records. The catalog's agent-referenced scope and addition to actual responses require explicit review; do not assume the existing documentation authorizes an unrestricted service listing.

`ThirdpartyService.logoUrl` already appears in canonical documentation. It is not an operator icon source. Do not remove or change its released semantics without explicit approval and the applicable versioning decision; the feature renderer consumes only `iconSvg`. Reconcile its documented role and actual implementation during API review, rather than silently turning it into an alias or remote fallback.

## Upload validation and rendering boundary

The stakeholder selected image-mode isolation during planning. Upload validation is deliberately limited to well-formed input and named active constructs, not complete SVG/CSS capability detection. Accepted source is not safe for arbitrary inline/document use.

1. Measure after JSON decoding: valid UTF-8, nonempty, at most 65,536 bytes. Parse strict XML using standard encoding/xml without resource resolution. Require one root `svg` in the SVG namespace, properly nested elements and no trailing non-whitespace content. Reject malformed XML and duplicate attributes; accept ordinary comments, harmless metadata, aliases and visual content.
2. Reject XML directives, DTD/entity declarations and processing instructions except a single initial UTF-8 XML declaration. Never install an external resolver or custom entity table.
3. For every parsed element, reject local names `script`, `a` and `foreignObject`, compared ASCII case-insensitively regardless of namespace. Reject event-handler attributes whose local names start with ASCII-insensitive `on`, including namespaced forms. Check root and descendants equally.
4. For decoded attribute values, reject a leading `javascript:` or `vbscript:` scheme after removing ASCII control/space characters and comparing ASCII-insensitively. This explicit check does not parse CSS or certify arbitrary attribute/animated values. The browser remains the primary boundary for those capabilities.
5. Do not implement an SVG geometry/value vocabulary, CSS sanitizer, reference graph, animation analyzer, media decoder or rendering-cost model. Preserve accepted bytes exactly. No third-party artwork, hash or local download is required.

External references in otherwise valid source can be accepted. The broker never fetches them; browser SVG image mode prevents their loads. Styles, local reuse, media and animation follow browser image-mode semantics. Tests must pair explicit rejections with harmless visual positives and must observe network/execution behavior in actual supported browsers.

Rendering-resource budgets and stress tests are out of scope by explicit stakeholder instruction. Input bounds are not a claim about browser CPU/GPU, memory, filter work or animation lifetime.

## Browser contract

Use only `<img src="data:image/svg+xml;charset=utf-8,...">` with URI-encoded exact source and empty alt text. Never interpolate SVG markup or permit object/embed/iframe/document delivery. Do not expose a navigable SVG endpoint. JSON authentication controls receipt of bytes, not arbitrary reuse of bytes an authorized client copies.

Preserve existing security headers and propose `frame-ancestors 'none'; img-src 'self' data:; object-src 'none'` for the built SPA. This policy requires API/ADR review of its SPA-wide effects. No new script/style permission is needed. Do not claim the parent CSP independently provides the SVG image-mode guarantees; verify the actual built image context in supported browsers.

Source-key only the image load/error state. Keep visible service names, original caller fallbacks and working controls during failure, replacement and clearing. Existing data refresh/cache lifetimes remain unchanged.

## Required security and contract cases

Original visual examples must cover styles, text, effects, local reuse, embedded media and declarative animation. Named active constructs, malformed XML, directives and over-limit source must fail atomically, including when submitted with credential/metadata changes. Namespace and character-reference variants exercise the actual named checks.

Accepted external references in attributes, CSS and animated values must remain inert when rendered as img; verify network and execution observables rather than expecting complete upload detection. Verify exact source through both storage adapters and restart, authentication, omit/null/value transitions, independent services, visual fidelity and fresh-source recovery. No renderer-resource stress suite or third-party asset is required.
