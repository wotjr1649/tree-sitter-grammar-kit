# tree-sitter-grammar-kit

A Go project for a planned CGO-free CLI, `tsgk`, and a small public offline Go API that share grammar inspection, comparison, and evidence checks. The first use case is grammar adoption and updates; consumer conversion, scanners, runtime, and adoption remain separate responsibilities.

## Status and scope

Session 01 implements offline `tsgk inspect` (bounded zero-config inventory of a grammar snapshot without running its code), `tsgk identity` (versioned file manifest and set fingerprint with detected encodings), `tsgk corpus` (private local corpus inventory), and the same operations in the public `src/kit` API. A fingerprint identifies bytes; it does not authenticate a source or qualify a grammar. Repository foundation checks validate the source/module layout, canonical documentation links, Git tracking/ignore policy, and the CGO-free core boundary.

Verification and schema commands are planned; generator, native runtime, and adapter execution are also unimplemented. The [public API contract](docs/specs/public-go-api.md) selects `src/kit` in the existing root module. Specifications describe intended contracts, not available features. See the [scope](docs/specs/scope.md) and [roadmap](docs/roadmap.md).

Campaign qualification requires [26 syntax routes](docs/validation/language-feature-scope.md) on three platforms, with registered legacy and modern features. This is a development acceptance scope, not a claim that the current product or every candidate grammar supports them. Users can use verified offline operations independently; they need not install all grammars or invoke the kit in their application runtime.

Foundation CI runs on Windows amd64, Linux amd64, and macOS Apple Silicon arm64. Its success does not establish product/native support or cross-platform semantic parity. The [platform contract](docs/specs/platform-support.md) defines those separate claims. Versioned [foundation results](docs/reports/session-00-foundation.md) describe their own tested revision; use the [Foundation workflow](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/workflows/foundation.yml) for later runs.

## Verify this checkout

Use Go 1.27.1, Git, and PowerShell 7 (`pwsh`). Core checks use `CGO_ENABLED=0`; Node, Python, a C compiler, and WSL are not prerequisites. The core currently has no external Go dependencies.

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
| `src/kit/` | Public offline API (inspect, identity, corpus) |
| `src/cmd/tsgk/` | CLI over the same API |
| `src/testdata/consumer/` | External-consumer module template used by tests |
| `src/internal/foundation/` | Implemented development checks and tests |
| `src/contracts/examples/` | Planned contract examples |
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
