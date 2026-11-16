---
name: living-docs
description: "Write, extend, or repair a living-documentation page under docs/specification/. Use when asked for living documentation, specification by example pages, or a capability page; when `just living-docs` reports stale, failed, unbound, or not-run examples; or when a change alters behavior that a page under docs/specification/ describes."
alwaysApply: false
---

# Living documentation

A page under `docs/specification/` describes one capability as it behaves today, in free prose with
concrete examples. `just living-docs` checks every example against the acceptance suite. You write
and maintain the prose. You never decide that an example is correct: only recorded evidence does.

## How the check works

- A scenario in `tests/e2e/` carries `livingdoc.Example("<capability>.<topic>.<case>")`.
- With `LIVING_DOCS_EVIDENCE_DIR` set, the suite records the scenario's state and every HTTP
  exchange of its `bootstrap.TestServer` clients to that directory.
- A page cites an example in two ways. An inline marker in code font, `example:<id>`, says the
  preceding claim is proven by that scenario. A fenced `http` block with `example="<id>"` in its
  info string shows one request and its response.
- The documentation site renders a marker as a badge that links to the scenario, and an example
  block as a request and response card (`assets/docusaurus/src/remark/living-docs.mjs`). The badge
  shows the verdict from `test-results/living-docs/status.json` when `just living-docs` ran before
  the site build. Put a marker after the punctuation of the sentence it proves, not in parentheses.
- `tools/livingdoc` gives each citation a verdict:

| Verdict | Meaning | What to do |
|---|---|---|
| passed | A scenario with this example passed. A shown exchange equals a recorded one. | Nothing |
| stale | The shown exchange equals no recorded one. The report has a diff. | Decide which side is right. See "Repair". |
| failed | A scenario with this example failed. | Fix the product or the scenario. Do not edit the page to hide it. |
| unbound | No scenario carries this example. | Bind an existing scenario, or remove the claim. |
| not run | The scenario was filtered out, usually because it needs Docker. | Run `just living-docs -strict` where Docker is available. |

An exchange block compares the request line, the headers that the block shows, the status code,
and both bodies. JSON compares by value. Timestamps compare by shape. UUIDs compare by identity
pattern: the same UUID must appear in the same places, but its value is free.

## Authority of sources

| Source | Authority |
|---|---|
| Acceptance scenarios and recorded evidence | What the broker does |
| `specs/*/spec.md`, accepted ADRs | Intent, reasons, and words that stakeholders agreed |
| `ARCHITECTURE.md` glossary | Vocabulary. Use its terms and no synonyms. |
| `api/*/openapi.yaml` | Contract |

Specs are deltas. Read every spec that touches the capability in directory order; a later spec
overrides an earlier one. When sources disagree, report the disagreement to the user. Do not
average it away in prose.

## Write a page

1. Find the capability in `specs/capability-map.md`. It lists the specs to read and the parts of
   them that later specs replaced. If the capability is new, name it as a job of a person or
   system, not as an endpoint or a feature number, and add it to the map.
   Read the specs, ADRs, contracts, and scenarios for it.
2. Select key examples. A page needs the few examples that let a reader predict the rest. It is
   not a list of every scenario. Leave edge-case matrices in the tests.
3. For each example, find the one scenario that proves it and add `livingdoc.Example(...)` to its
   `It`. Do not write a new scenario only to feed a page; a missing scenario is a gap under
   Constitution Principle XIII and belongs in a spec.
4. Run `just living-docs`. Read the recorded exchanges in `test-results/living-docs/evidence/`.
5. Write the page for a reader who knows the domain but not the code: cast, situation, action,
   observable result, and the reason for the rule. Give state that HTTP does not show, such as
   seeded data, in prose.
6. Copy exchanges from the evidence. Replace generated UUIDs and timestamps with stable synthetic
   values. Keep equal UUIDs equal. Show only headers that matter to the reader.
7. Put an inline marker after each claim that a scenario proves. A sentence without a marker or
   a block is commentary; do not state untested behavior as fact.
8. Run `just living-docs` until every citation is passed or, without Docker, not run.

Follow `.agents/skills/simple-english/SKILL.md` for the prose. Use synthetic identities only.
Never put a credential in a page; the recorder redacts `Authorization` and cookie headers, but
bodies are recorded as sent.

## Repair

For a stale exchange, read the diff and find the change that caused it.

- The behavior changed on purpose (a spec or ADR says so): update the block and every sentence
  that depends on it.
- The behavior changed by accident: the page found a regression. Report it. Do not edit the page.
- Only the scenario changed and no longer performs the exchange: bind the example to the scenario
  that does, or restore the exchange.

When a pull request changes behavior that a page describes, update the page in the same pull
request.

## Review a page

List each factual sentence that has neither a marker nor an exchange block and that a reader
would take as a guarantee. Offer to bind it or to soften it. This review is advice; it does not
replace the check.
