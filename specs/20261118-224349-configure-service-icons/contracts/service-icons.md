# Proposed Service Icon Contract

Status: designed for stakeholder review, not approval to implement. Incorporate the schemas in `schemas.yaml` into both canonical OpenAPI documents, with endpoint examples and existing authentication/error definitions, before behavior changes. Additive icon fields do not authorize changes to any existing field or API versioning rule.

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

## Normative static SVG profile

Validation uses strict XML tokens and expanded namespace names, not text searching or a browser sanitizer.

- Input must be valid UTF-8 and contain 1–65,536 bytes after JSON decoding. Exactly 65,536 bytes is permitted if the rest of this profile is satisfied. Empty or whitespace-only input is invalid.
- Exactly one root `svg` in `http://www.w3.org/2000/svg`; all descendant elements use that namespace. Outside the root, only whitespace and an optional leading UTF-8 XML declaration are permitted. Reject comments and processing instructions other than that declaration, directives, DOCTYPE/entity declarations, additional roots, and non-whitespace character data. Predefined/numeric XML character references are decoded by the standard parser before checking attribute values; custom entities are never resolved.
- Allowed elements: `svg`, `g`, `path`, `rect`, `circle`, `ellipse`, `line`, `polyline`, `polygon`. Nested `svg` is not allowed. This intentionally excludes text/fonts, `style`, `script`, links, `foreignObject`, `image`, `use`, animation, filters, masks, gradients, and resource definitions.
- The sole namespace declaration allowed is the root default SVG namespace. Reject namespace aliases/rebindings, foreign namespaces and namespaced attributes. This keeps XML namespace handling distinct from browser-specific fallback parsing.
- Common attributes: `fill`, `stroke`, `stroke-width`, `stroke-linecap`, `stroke-linejoin`, `stroke-miterlimit`, `stroke-dasharray`, `stroke-dashoffset`, `fill-rule`, `clip-rule`, `opacity`, `fill-opacity`, `stroke-opacity`, `transform`.
- Root-only attributes: `viewBox`, `width`, `height`, `preserveAspectRatio`, plus the namespace declaration. Element-specific geometry: path `d`, rect `x/y/width/height/rx/ry`, circle `cx/cy/r`, ellipse `cx/cy/rx/ry`, line `x1/y1/x2/y2`, polyline/polygon `points`. Reject all other attributes, including every event handler, `style`, `class`, `href`, `xml:base`, and `src`.
- Validate every decoded attribute value against the rules below; an allowed attribute name alone is insufficient. Reject a value unless the entire value matches its rule. No CSS functions, URL syntax, escapes, custom properties, embedded declarations or resource references are accepted. Only the geometric functions listed for `transform` are permitted. These rules validate a bounded lexical/value profile, not rendered geometry.
- Reject duplicate attributes, including duplicates after namespace expansion. Reject invalid XML names/encoding declarations and unclosed or mismatched tags. Stop on the first failure; no sanitization or network access.

Element/attribute names, enums, units and transform function names are exact and case-sensitive. Paint keywords and ASCII alphabetic named-color tokens alone are ASCII case-insensitive. Preserve accepted bytes; validation must not rewrite case, whitespace, character references or numbers. The restricted profile is intentional, not a promise to accept arbitrary safe SVG exports. `currentColor` is resolved inside the image document and does not inherit the surrounding page's CSS color.

### Attribute-value acceptance rules

Apply these rules after XML character-reference decoding, without changing the stored source. `S` means XML whitespace only: U+0020, U+0009, U+000A or U+000D. Trim leading/trailing `S` for validation only; reject empty values. Other control characters and non-ASCII characters in attribute values are rejected. The root namespace value must equal `http://www.w3.org/2000/svg` exactly, with no trimming. Missing optional attributes use browser defaults; an explicit empty value is never an alias for omission.

`N` is one complete number matching `[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`, whose standard-library float64 conversion succeeds without range error and yields a finite value. Reject NaN, infinity, overflow and underflow reported as a conversion error. A length `L` is `N` followed immediately by no unit, `px` or `%`; no other units or functions are allowed. A list separator is one or more `S`, or a single comma with optional surrounding `S`; reject leading/trailing/repeated commas. Adjacent signed numbers without a separator are permitted only inside path `d` under its lexical rule, not in number/length lists.

| Attribute family | Complete acceptance rule |
|---|---|
| `fill`, `stroke` | `none`, `currentColor`, `transparent`, `#` followed by exactly 3, 4, 6 or 8 hex digits (`[0-9A-Fa-f]`), or one `[A-Za-z]+` named-color token. Alphabetic tokens are intentionally not checked against a browser color-name registry: an unknown name is accepted as inert and may be ignored by the browser. No parentheses or fallback-paint syntax. |
| `x`, `y`, `x1`, `y1`, `x2`, `y2`, `cx`, `cy` | One `L`, including a negative value. |
| `width`, `height`, `rx`, `ry`, `r`, `stroke-width` | One `L` whose numeric value is nonnegative, including zero. This applies to root dimensions and element-specific dimensions alike. |
| `stroke-dashoffset` | One `L`, including a negative value. |
| `stroke-dasharray` | Exact `none`, or a nonempty separator-delimited list of nonnegative `L` values. Odd-length and all-zero lists are accepted; browser rendering semantics apply. |
| `stroke-miterlimit` | One unitless `N` with value at least 1. |
| `opacity`, `fill-opacity`, `stroke-opacity` | One unitless `N` in the inclusive range 0–1; percentages are rejected. |
| `stroke-linecap` | Exactly `butt`, `round` or `square`. |
| `stroke-linejoin` | Exactly `miter`, `round` or `bevel`. |
| `fill-rule`, `clip-rule` | Exactly `nonzero` or `evenodd`. |
| `viewBox` | Exactly four separator-delimited unitless `N` values; the third and fourth must be nonnegative. Zero dimensions are accepted; they can produce no visible drawing. |
| `preserveAspectRatio` | Exact `none`, or one of `xMinYMin`, `xMidYMin`, `xMaxYMin`, `xMinYMid`, `xMidYMid`, `xMaxYMid`, `xMinYMax`, `xMidYMax`, `xMaxYMax`, optionally followed by one or more `S` and exact `meet` or `slice`. No `defer`, commas or suffix after `none`. |
| `points` | An even-length separator-delimited list of at least two unitless `N` values. No units or command letters. |
| `transform` | A nonempty list of `matrix(6 numbers)`, `translate(1 or 2 numbers)`, `scale(1 or 2 numbers)`, `rotate(1 or 3 numbers)`, `skewX(1 number)` or `skewY(1 number)`. Arguments are unitless `N` separated by list separators, with optional `S` just inside parentheses and between the function name and `(`. Separate functions by a list separator; no nesting, units, unknown functions or trailing separator. The parenthesized counts denote argument counts, not literal text. |
| Path `d` | After trimming `S`, must start with `M` or `m`. Scan the rest as command tokens from `MmZzLlHhVvCcSsQqTtAa`, finite unitless `N` tokens, `S` or commas; consume numeric tokens maximally, so exponents are part of a number. Reject any unconsumed character or malformed numeric token. Adjacent commands/numbers and signed numbers are allowed. Deliberately do not validate command operand counts, arc flags, comma placement or path topology; these cannot introduce resource references and the browser owns path interpretation. |

For example, `opacity="0.5"`, `width="24px"`, `viewBox="0 0 24 24"` and `transform="translate(1,2) scale(.5)"` pass. `opacity="2"`, `width="2em"`, `stroke-linecap="inherit"`, `viewBox="0 0 24"`, `points="1,2,3"`, `transform="rotate(1,2)"` and `d="M0 0 calc(1)"` fail. `fill="notacolor"` and `d="M0"` pass their deliberately lexical rules but may render nothing; acceptance does not promise a visible drawing. A blank or partially ignored drawing need not fire an image error. Only a browser image load/decode error invokes the failed-image fallback; malformed XML, prohibited content and values outside this table always fail the write.

Tests must cover every table row's accepted/rejected cases before validator implementation. The 65,536-byte boundary uses permitted ASCII documents; separate oversized well-formed UTF-8 fixtures with multibyte characters prove byte counting and early size rejection even when those characters would also fail the attribute profile. Do not require a valid multibyte attribute value that this profile excludes.

## Browser contract

Use only `<img src="data:image/svg+xml;charset=utf-8,...">` with URI-encoded source and empty `alt`. Never interpolate markup or accept caller-supplied image URLs. No `<object>`, `<embed>`, `<iframe>`, or inline SVG insertion. The surrounding service name stays visible; icon error/loading state cannot disable controls or create a page-level error. Reset state on source changes, preserving unrelated component state.

Broker-served SPA CSP becomes `frame-ancestors 'none'; img-src 'self' data:; object-src 'none'`. Preserve existing security headers. Equivalent deployment policies must permit the selected local data images without permitting remote image origins. Verify the built frontend, not only Vite. No script, font, style, or connection policy relaxation is required. No SVG is served as an active document; manually constructed/copied data URLs are not broker resources. Validation still excludes active content if an authorized user copies the source elsewhere.

## Required security/contract fixtures

Valid rectangle/path/circle, Unicode-safe byte counting, exactly-limit and over-limit documents; omit/null/value transitions; malformed roots/namespaces/duplicates/entities; script/onload/href/style/foreignObject/image/use/animation; mixed-case and character-reference-obfuscated resource references; nested data URLs; and two independent services with identical source. Test invalid input alongside credential and metadata changes for all-or-nothing behavior. Assert response JSON, rendered image mode, absence of remote requests and DOM insertion, unauthenticated rejection, and replacement/removal after fresh data.
