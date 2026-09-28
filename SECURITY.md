# Security policy

## Current security boundary

The implemented code consists of development foundation checks for this repository. These checks assume a caller-owned tree that is not being modified concurrently. They are not a sandbox for arbitrary grammars or hostile code, and successful CI does not establish that a grammar or native runtime is safe to execute.

The [trust and execution contract](docs/specs/trust-and-execution.md) owns the planned offline, generator, native, and adapter boundaries. Those product capabilities are not implemented yet. Environment cleanup, private caches, and resource caps alone do not isolate malicious native code. Source/tool identities and preserved evidence support traceability; a self-generated fingerprint does not authenticate an input's origin.

## Reporting a concern

Report non-sensitive defects through [GitHub Issues](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues). Include:

- the affected commit or tag and the relevant command;
- OS/architecture and tool versions;
- expected and observed behavior, with a minimal non-sensitive reproduction;
- the affected boundary and impact, if known.

Do not post secrets, credentials, private inputs, personal information, or sensitive exploit details in public Issues or attachments. Private vulnerability reporting, a security email address, and a response SLA are not established by this policy. Before sharing sensitive material, ask the maintainer for a private reporting route using only non-sensitive context; keep the material private until that route is confirmed.

## Support and disclosure limits

This foundation-stage project does not promise a supported-release matrix or response deadline. A report or a passing test is not a guarantee covering every input, platform, or future revision. Keep reports tied to the affected revision and distinguish observed failures from untested risks. Platform and validation claims are defined in the [platform](docs/specs/platform-support.md) and [validation](docs/validation/validation.md) contracts.
