# Documentation map

The current implementation consists of repository foundation checks. Product commands and r0 data contracts are planned specifications, not claims of implementation.

| Task | Canonical owner |
|---|---|
| Purpose and release-candidate scope | [scope](specs/scope.md) |
| Commands, errors, profile fields | [CLI/profile](specs/cli-and-profile.md) |
| Fingerprints, manifests, evidence | [identity/evidence](specs/identity-and-evidence.md) |
| Untrusted inputs, paths/archives, execution authority | [trust/execution](specs/trust-and-execution.md) |
| Ordered CST, queries, adapters | [tree/protocol](specs/tree-and-adapter-protocol.md) |
| OS/arch/capabilities | [platform](specs/platform-support.md) |
| Structure, dependencies, state transitions | [architecture](design/architecture.md), [decision record](design/decisions/0001-core-and-execution.md) |
| Pinned external references and license observations | [provenance](provenance/upstream-sources.md) |
| Checks, review, merge | [validation](validation/validation.md), [workload](validation/workload-matrix.md) |
| Campaign scope, Issues/Milestones | [roadmap](roadmap.md) |
| Observed Session 00 results | [foundation report](reports/session-00-foundation.md) |

Read only the documents relevant to the current task. [AGENTS.md](../AGENTS.md) is the entry point for persistent project rules.

## Task-specific development rules

Ordinary development, fixes, and reviews have no session-prompt prerequisite. The [validation contract](validation/validation.md) owns Issue, task-branch/worktree, and completion requirements. Read the affected code, callers, existing checks, and relevant contracts above. A review-only request ends with findings and evidence; it does not expand into modifications or PR creation.

Use the [Issue template](../.github/ISSUE_TEMPLATE/task.md) to record change-work goals and acceptance criteria, and the [PR template](../.github/PULL_REQUEST_TEMPLATE.md) to identify the criteria satisfied by the actual diff and validation. A partial PR must identify remaining work rather than claim whole-Issue completion. Templates capture intent and evidence; they do not authorize remote posting or replace validation gates.

| Change surface | Read or update together |
|---|---|
| Go implementation or dependencies | Architecture, affected specs, regression tests, and local foundation checks |
| CLI/profile/evidence/adapter contracts | Owning spec, examples, consumers, error paths, and corresponding workload checks |
| Paths/archives or external execution | Trust/execution input boundaries, authority, failure paths, and negative tests |
| Platforms, CI, or validation policy | Platform and validation contracts, workflow, and policy-violation negative tests |
| AGENTS or development rules | Applicable task types, entry conditions, canonical ownership, links, and semantic impact |
| Meaning-preserving documentation typos or formatting | Diff, links, and formatting; the same mandatory CI applies before integration into main |

This map and the canonical documents own task-specific development rules. Do not duplicate them in a separate RULES.md. Foundation/CI checks machine-testable boundaries; review assesses meaning, review applicability, and authority. The AGENTS line limit is a size ceiling, not a quality score.

The shared `development-start` and `pr-review-workflow` skills are optional aids for preparation and PR review. This repository owns its rules and commands; installing skills is not a prerequisite for development or CI. Design command-execution enforcement separately when a concrete prohibited behavior warrants it.

## Campaign material

`docs/prompts/` and `docs/plans/` hold local execution material; `artifacts/` holds retained raw evidence and handoffs; `_ref/` holds pinned references; `.work/` is regenerable workspace. Keep these paths untracked. Public acceptance criteria must be understandable from this map and applicable Issues without local material.
