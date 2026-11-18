# End-User API

The end-user API serves OAuth2 authorization, token, and metadata requests. The broker serves it on the end-user listener.

The canonical contract is [End-User API 2.0.0](../../api/enduser/openapi.yaml), upgraded from 1.2.0 on the existing routes. Request and response shapes remain unchanged. The approved behavior adds client-identity invalidation to the existing reauthentication rules. The related [Admin API 3.0.0](../../api/admin/openapi.yaml) cutover changes sweep result classification, not its routes or shapes.

## Session reauthentication and client identity

A service client-ID change invalidates every affected user's existing connection on its next use. This applies to stored IDs changed through the Admin API and filesystem IDs changed through credential publication. Restoring the previous client ID does not restore an invalidated connection. Each user must reconnect under the current identity. Same-ID secret rotation does not require reauthentication. Existing grants and unrelated service connections remain unchanged.

### Session fields

The existing fields apply to session lists, session details, and manual-refresh summaries:

| Field | Meaning |
| --- | --- |
| `requires_reauthentication` | True after a recorded terminal refresh rejection, access expiry without a stored refresh token, or unusable client identity or revision. |
| `is_expired` | Deprecated alias with exactly the same value as `requires_reauthentication`. It is not an independent access-expiry flag. |
| `access_token_expired` | Whether the recorded access-token expiry passed. Unknown expiry does not establish permanent validity. |
| `has_refresh_token` | Whether the broker stores a refresh token, including a rejected token. It does not establish refresh usability or lifetime. |

A recorded rejection blocks issuance even with an unexpired access token. An expired access token remains refreshable with a stored, not-rejected refresh token and usable identity. The broker never predicts refresh-token expiry.

For example, a synthetic Reports Production session can require sign-in before its access token expires:

```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "service_id": "880e8400-e29b-41d4-a716-446655440010",
  "service_display_name": "Reports Production",
  "token_type": "Bearer",
  "scope": ["reports:read"],
  "initiated_at": "2026-11-04T08:00:00Z",
  "requires_reauthentication": true,
  "is_expired": true,
  "access_token_expired": false,
  "has_refresh_token": true,
  "dependent_agent_count": 1,
  "is_encrypted": true
}
```

Consent detail reports `connectionStatus: not_connected` for this service. Such a session cannot satisfy a mandatory service when the user submits a grant. Consent and session displays read the live client-ID file, never the secret file. If source resolution fails, these displays fall back to persisted identity evidence. A display response never authorizes token release.

### Browser authorization and reconnect

In local and hybrid mode, the next relevant `GET /oauth2/authorize` request checks the service connections included in the existing grant. If an included connection requires reauthentication, the broker returns the existing consent redirect:

```http
HTTP/1.1 302 Found
Location: https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000?session_token=SYNTHETIC_SEALED_AUTHORIZATION_CONTEXT
```

An existing grant does not bypass this check. The broker does not send a proactive notification or navigate an already open page. The existing service sign-in action starts authorization under the current identity. Provider login and consent remain under provider control. A successful reconnect restores only that user's service connection.

A callback from authorization started under an older identity or revision fails before code exchange. This also applies after the operator restores the previous client ID. Pre-upgrade callback state without an explicit identity or revision must restart sign-in, even at revision 0. The legacy stored-session exception does not apply to callback state. The browser receives the existing `302` to `/sessions?error=callback_failed&error_description=...`. A failed callback preserves the existing session.

Token requests cannot navigate a browser. The [token-exchange recovery example](../reference/token-exchange.md#400-invalid_grant) shows the service-specific recovery link and the return to `/sessions`. Direct recovery does not redirect the user to agent consent.

### Manual refresh

`POST /api/third-party/{serviceId}/session/refresh` returns `409 refresh_unavailable` for missing required identity, a mismatched client ID, or a superseded revision. The broker makes no provider request and leaves stored tokens unchanged:

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{"error":"refresh_unavailable","message":"no valid refresh token available for this session"}
```

This replaces the previous identity-mismatch `500` outcome. The same `refresh_unavailable` code still covers sessions without a stored refresh token. A recorded or newly observed terminal provider rejection retains `409 refresh_rejected`. Identity handling does not replace that rejection outcome. An unreadable client-ID file remains a `500` failure, not a reconnect diagnosis.

### Admin sweep compatibility

In Admin API 3.0.0, `POST /api/sessions/sweep` counts identity-invalid eligible candidates as `failed`, not `refreshed`, in real and dry runs. Missing required identity, a mismatched ID, a superseded revision, or an unreadable identity cannot count as refresh success. Identity-invalid candidates make no provider request. With one due identity-invalid candidate, a completed sweep returns `200 OK` with `failed: 1`, `refreshed: 0`, and `total_evaluated: 1`.

A dry run compares the live identity with persisted revision evidence without provider calls or observation/session writes. A differing file value can count as failed without a persisted revision increment. Other candidates that would refresh still count as `refreshed` in a dry run. Candidate selection and the exclusion of provider-rejected sessions remain unchanged. Sessions without a recorded access expiry are not sweep candidates, but identity enforcement still applies on their next use.

See [the service-management guide](../guides/manage-agents-and-services.md) for the change procedure and guarded rolling-upgrade requirements.

## CIMD Client Authentication

In local or hybrid mode with CIMD enabled, a pre-registered CIMD client can authenticate at the token endpoint with `private_key_jwt`. The client signs a short-lived assertion with a key from its metadata document. The broker requires a fresh assertion for each authorization-code exchange and refresh request.

The assertion uses `RS256` or `ES256`, and its algorithm must match the client's metadata document. It must include `kid`, `jti`, `iss`, `sub`, `aud`, and `exp`. Both `iss` and `sub` must equal the CIMD URL. The `jti` is single-use per client.

For an authorization-code exchange, send the assertion fields with the existing grant fields:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=https%3A%2F%2Fchatgpt.com%2Foauth%2FEBkIgu5McFbd%2Fclient.json
&code=AUTHORIZATION_CODE
&redirect_uri=https%3A%2F%2Fchatgpt.com%2Fconnector%2Foauth%2FEBkIgu5McFbd
&code_verifier=PKCE_VERIFIER
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=SIGNED_CLIENT_ASSERTION
```

For refresh, use `grant_type=refresh_token`, `refresh_token`, `client_id`, `client_assertion_type`, and a new `client_assertion`. Do not send `client_secret` or an `Authorization` header with an assertion.

Malformed parameter combinations return `400 invalid_request`. Authentication failures return `401 invalid_client` with a stable description. A document or key-set dependency failure returns `server_error`. A failed request does not consume an authorization code or rotate a refresh token.

See the [token endpoint contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/051-private-key-jwt-auth/contracts/token-endpoint.md) for the complete assertion profile and response rules.

## Agent-Subject Token Exchange

In local mode, a privileged client can request a token for a registered agent acting for itself. The [canonical OpenAPI](../../api/enduser/openapi.yaml) defines the contract. Use the complete [agent-subject configuration](../../examples/config/impersonation-agent-subject.yaml) and [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance).

Classification is rule-local: the raw string `sub` must exactly equal the audience-selected target's canonical UUID, and the subject must be signed and verified or, for an unsigned (`verification: none`) subject, the rule's predicate must bind `subject_token.sub`. Everything else is human. There is no request category field. Aliases and alternate UUID spellings never select agent issuance, and a different registered agent's UUID is rejected without falling back to human delegation. The target must exist and existing scope limits still apply. Every credential validation, principal extraction and CEL authorization must succeed. The actor and client assertion always require signatures.

| `allowed_subject_categories` | Human | Signed exact-target subject |
| --- | --- | --- |
| Omitted or `[human]` | Active delegation required | Ineligible, even with same-text human delegation |
| `[agent]` | Ineligible | No human delegation required |
| `[human, agent]` (either order) | Active delegation required | No human delegation required |

Category-ineligible rules fall through. Missing/expired delegation for an authorized human is terminal, with the established consent `error_uri`; no automatic agent retry is allowed. Unsigned-subject rules may list `agent`, but unsigned subjects remain human and that entry has no effect.

### Request and signed response

This example uses registered target `research-agent` with UUID `550e8400-e29b-41d4-a716-446655440000`. Its rule explicitly allows `agent` and authorizes `gateway-prod`, `research-runner` and the target UUID in CEL. The three markers below represent externally signed ES256 credentials with issuer `https://idp.example.com`, audience `https://broker.example.com/impersonation` and valid lifetimes. Their subjects are respectively the gateway, runner and target UUID. Line breaks in the form body are for display only.

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded
Accept: application/json

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2Fresearch-agent
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=SYNTHETIC_CLIENT_JWS
&actor_token_type=urn:ietf:params:oauth:token-type:jwt
&actor_token=SYNTHETIC_ACTOR_JWS
&subject_token_type=urn:ietf:params:oauth:token-type:jwt
&subject_token=SYNTHETIC_AGENT_JWS

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store
Pragma: no-cache

{"access_token":"SYNTHETIC_BROKER_AGENT_JWS","token_type":"Bearer","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","expires_in":3600}
```

Decoded access-token claims (authorization-relevant excerpt; normal `jti` may also be present):

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "aud": "https://policy.example.com",
  "scope": "",
  "iat": 1793005200,
  "exp": 1793008800,
  "act": {"iss": "https://idp.example.com", "sub": "research-runner"}
}
```

The broker creates no human grant/session and initiates no consent interaction. It retains `act`, even if actor and subject identities happen to be equal. Reserved `sub` and `agent_id` cannot be overridden by local policy. No category claim is added. Subject email extraction is skipped and no credential-derived email is supplied; output email remains policy-owned. Issuer, signing, lifetime, scope and audience policy keep their normal semantics.

### Denials

If no rule accepts the category (including signed exact-target equality on a human-only rule with or without a grant), the response is the generic existing envelope, without consent recovery:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"impersonation not permitted"}
```

For an otherwise valid third-party exchange using the issued token as `subject_token`, resource `https://api.thirdparty.example.com`, and a fresh valid third-party client assertion, default detection rejects before agent extraction, authorization or human grant/session/provider access. A matching compliant override has the same result:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"agent-subject tokens cannot be exchanged for third-party service tokens"}
```

There is no `error_uri`: human consent cannot recover this denial. Default detection is the guarded equality of `sub` and `agent_id`; it applies regardless of issuer or issuance date. Runtime detection errors, timeouts and dynamic non-boolean output instead return `500 server_error`, also without `error_uri` or human-state access. A configured detector fully replaces the default. Operators must retain broker agent-token matching; a non-matching override that references `sub` (a detector without `subject_token.sub`, such as `false`, fails startup) removes categorical rejection and same-text human-credential isolation. Existing checks may then consult human grants/sessions or return consent errors; this is not support for third-party agent identity or guaranteed exchange success.

See the [exchange concepts](../concepts/token-exchange.md#agent-subject-boundary), [diagnostic reference](../reference/token-exchange.md#agent-subject-boundary), and [detector example](../../examples/config/token-exchange.yaml).

### Approval endpoints

The same detector guards the approval endpoints that authenticate a subject token (ADR 045 decision 6a). An agent-subject token presented to `POST /api/approvals` or `POST /api/approvals/{id}/consume` is rejected before any approval is read or written:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"forbidden","message":"agent-subject tokens cannot authenticate approval requests"}
```

A detection failure returns `503 service_unavailable`. `GET /api/approvals` authenticates only the client assertion and is unaffected.

The [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance), [concepts](../concepts/token-exchange.md#agent-subject-boundary), [field reference](../reference/token-exchange.md#agent-subject-boundary) and [example configurations](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/examples/config/README.md#impersonation-agent-subjectyaml--signed-self-agent-issuance) describe the same contract.
