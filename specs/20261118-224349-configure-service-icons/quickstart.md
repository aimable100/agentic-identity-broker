# Service Icons: Validation Guide

This guide describes checks after implementation, not current capabilities. See [the contract](contracts/service-icons.md), [model](data-model.md) and [plan](plan.md).

## Prerequisites

Use Go/Node versions in the manifests, just, existing test tools and Playwright browsers. PostgreSQL restart/migration cases require Docker or Podman. Complete API confirmation/publication and ADR/glossary/test-first gates before behavior changes. Use original synthetic artwork, never a required third-party logo or local download.

## Commands

Run focused package/case selections while implementing. After Go/test changes run just check before the matching tests. Existing suite recipes are:

```sh
just check
just test
just web-test
just test-integration-infra
just test-e2e-backend
just test-e2e-docker
just test-e2e-frontend
```

Run just verify before the PR per CONTRIBUTING.md. Record actual results and unavailable prerequisites, not assumed passes.

## Journeys

1. Create Atlas Files with the original inline SVG from spec.md; read/list exact source. Establish consent/grant/session with existing fixtures. Restart against the same PostgreSQL database without reseeding; inspect actual consent/session images.
2. Replace source and obtain genuinely fresh responses. Omission preserves; null clears and restores each original fallback. The other service sharing the same artwork remains unchanged. Existing five-/two-minute caches are not bypassed by merely invoking a cached refetch.
3. Submit malformed XML, empty input, URL/path input, prohibited active constructs and a 65,537-byte SVG. Reject atomically even alongside credentials/metadata edits. Accept a valid exactly-65,536-byte source; cover decoded UTF-8 bytes rather than wire characters.
4. Submit valid visual examples with styles, filters, masks, patterns, text, local reuse, inert media and declarative animation. Check visual fidelity against browser image mode, with equal animation timeline points. No exhaustive drawing vocabulary is required.
5. Include external references in otherwise accepted source, including CSS and animated values. With built SPA in actual supported browsers, observe zero external loads, script execution or interaction; inspect no inline SVG insertion. Explicit script/event/hyperlink/foreignObject constructs still reject upload. Do not confuse accepted source with safe inline/document markup.
6. Without authentication, read every actual delivery path; existing failures must contain no SVG bytes.
7. Cause a deterministic browser image error with valid stored source, retain usable controls/fallback, replace source and observe recovery in the open page without remounting actions. Verify decorative alt/focus behavior and narrow/wide views. Save configured/unset/failed/recovered screenshots under tests/e2e/screenshots/.

## Evidence and limits

Map all twelve scenario IDs to one canonical It each as in plan.md. Record production migration up/down/up, unchanged preexisting rows and real PostgreSQL restart results. Verify actual built-SPA security headers and browser isolation, not only mocked events. No rendering-resource budget or stress suite is required; operator artwork may be expensive. This planning command does not claim runtime verification.
