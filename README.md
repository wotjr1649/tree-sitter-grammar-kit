# tree-sitter-grammar-kit

A Go project for a planned CGO-free CLI, `tsgk`, and a small public offline Go API that share grammar inspection, comparison, and evidence checks. The first use case is grammar adoption and updates; consumer conversion, scanners, runtime, and adoption remain separate responsibilities.

## Status and scope

Implemented, each with its owning contract and observed session report (see the [documentation map](docs/README.md)). In release v0.2.0, the offline core is **VERIFIED** on Windows amd64, Linux amd64 and macOS arm64, and the 26-route qualification verdict is **SUPPORTED** on all three for the registered cases; it does not certify arbitrary inputs. The other areas are **experimental**: they work as their reports show, but they carry no compatibility promise. See the [platform contract](docs/specs/platform-support.md) for the evidence and the known limitations.

| Area | CLI | Public API (`src/kit`) | Release status | Notes |
|---|---|---|---|---|
| Inventory and identity | `inspect`, `identity`, `corpus` | `Inspect`, `Identity`, `Corpus` | VERIFIED (`corpus`: experimental) | offline; a fingerprint identifies bytes, it does not authenticate a source |
| Strict verification | `verify` | `Verify` | VERIFIED | exact-set check against a caller-trusted expected document; ZIP inspected without extraction |
| Node schema | `schema check`, `schema diff` | `SchemaCheck`, `SchemaDiff` | VERIFIED | static `node-types.json` facts and review risks, not runtime trees |
| Reproduction | `reproduce` | profile parsing only | experimental | runs a pinned generator in two workspaces (EXEC_GENERATOR) |
| Native parse/edit and query records | `incremental`, `oracle record` | comparators, profile parsing, record-set verification | experimental | build and run the pinned runtime with a host compiler (BUILD_NATIVE, EXEC_NATIVE); not part of the offline API |
| Evidence replay | `replay`, `evidence verify` | `Replay`, `VerifyEvidence`, `CompareGates` | experimental | registered data-only reducers; integrity, not authenticity |
| Qualification | `qualify` | `Qualify`, `ParseQualificationInventory` | experimental | aggregates one candidate's three host runs into the 26 route x 3 platform cells; read-only |

A grammar update workflow uses only the offline commands: take the baseline snapshot's `identity` as the expected document, `verify` the candidate against it (changed files are reported as FAIL), and `schema diff` the two `node-types.json` files for review risks. Nothing is adopted automatically.

Campaign qualification requires [26 syntax routes](docs/validation/language-feature-scope.md) on Windows amd64, Linux amd64 and macOS arm64 with the registered legacy and modern features. `tsgk qualify` reports each cell on two axes, the kit mechanism and the grammar requirement coverage; the support claim is SUPPORTED only when every cell and every executed extra-role row passes and the kit gate passes. The registered cases do not yet cover every required feature row and case kind, and adopted routes carry known grammar gaps, so the current support claim is **BLOCKED**: the verified offline core and the experimental evidence functions are usable on their own, but no route, version range or platform is claimed as fully supported. The [platform contract](docs/specs/platform-support.md) separates foundation success, native execution and cross-platform semantic parity. Experimental and campaign-internal protocols (`tsgk-native/r2`, the qualification inventory, CI helpers under `src/dev/`) may change. No tag, Release or package has been published, and no Go consumer (including `go-treesitter`) has adopted the kit.

## Verify this checkout

Use Go 1.27.1, Git, and PowerShell 7 (`pwsh`). Core checks use `CGO_ENABLED=0`; Node, Python and WSL are not prerequisites, and native tests skip locally without a C compiler. The only external Go dependency is the pinned `golang.org/x/sys` used by the process runner; download it first as the [validation contract](docs/validation/validation.md) describes.

From the repository root in PowerShell 7:

```powershell
$env:CGO_ENABLED = '0'
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
go test ./src/... -count=1 -v -timeout 120s
if ($LASTEXITCODE -ne 0) { throw 'foundation tests failed' }
```

This runs development checks, not grammar validation. Follow the [validation contract](docs/validation/validation.md) for formatting, vet, build, review, and integration requirements. Windows PowerShell 5 is not supported for project scripts.

## Repository and evidence

| Location | Purpose |
|---|---|
| `go.mod` | Root Go module |
| `src/kit/` | Public offline API |
| `src/cmd/tsgk/` | CLI over the same API |
| `src/testdata/consumer/` | External-consumer module template used by tests |
| `src/internal/foundation/` | Implemented development checks and tests |
| `src/contracts/` | Registries (routes, sources, fact queries, qualification inventory) and contract examples |
| `docs/specs/`, `docs/design/` | Specifications and architectural decisions |
| `docs/provenance/`, `docs/validation/` | Reference identities and validation contracts |
| `docs/reports/` | Results bound to named revisions and dates |

Local prompts, plans, raw evidence, reference checkouts, and scratch work under `docs/prompts/`, `docs/plans/`, `artifacts/`, `_ref/`, and `.work/` stay out of Git. Public contracts and acceptance criteria must remain understandable without them.

Native tools and downstream runtimes belong in separate processes; they are not core CGO dependencies. A fingerprint alone does not authenticate a source, and the project does not provide a sandbox for arbitrary grammar code. Read the [trust contract](docs/specs/trust-and-execution.md) and [security policy](SECURITY.md).

## Contributing and documentation

Start with the [documentation map](docs/README.md) and [AGENTS.md](AGENTS.md). Ordinary development has no session-prompt prerequisite. Track changes in an Issue and a dedicated task branch; reuse matching work and choose a worktree when isolation is needed. Local completion, main integration, and workspace cleanup have separate conditions in the [validation contract](docs/validation/validation.md).

Use the [Issue tracker](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues) for non-sensitive defects and proposed work. Report observed behavior, the relevant revision, and reproducible acceptance criteria. Follow [SECURITY.md](SECURITY.md) for sensitive findings.

## License

MIT. See [LICENSE](LICENSE).
