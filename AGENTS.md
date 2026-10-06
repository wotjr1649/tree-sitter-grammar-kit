# AGENTS.md

## Project and entry point
- This project provides a Go CLI and small public offline Go API for grammar, generated-artifact, and runtime verification evidence. The offline core (inspect, identity, verify, schema) is verified; other areas are experimental; the 26-route qualification verdict was SUPPORTED on all three platforms as of main `8038e8f`; see [platform support](docs/specs/platform-support.md). Specifications do not imply completed product features.
- Start ordinary development, fixes, and reviews from the request and affected code and checks. Find task-specific development rules through the [documentation map](docs/README.md).
- Apply campaign gates only when the current request or an applicable authoritative assignment explicitly selects campaign scope. Filenames, local artifacts, and incidental Issue references alone do not select it; ordinary work has no session-prompt prerequisite.

## Boundaries
- Keep product and validation implementation under `src/` and the Go module at the root. Build and test the core with `CGO_ENABLED=0`.
- Native tools and downstream runtimes run in separate processes. Do not add `import "C"` or a `go-treesitter` dependency to the core.
- Keep `docs/prompts/`, `docs/plans/`, `artifacts/`, `_ref/`, and `.work/` untracked. Preserve untracked evidence.
- Do not modify existing reference checkouts or other repositories. Prepare new pinned references only within the current authorization.
- Execute PowerShell scripts only with PowerShell 7 `pwsh`. Do not use Windows PowerShell 5 or a `powershell.exe` fallback.
- Write skills and instructions in English. Write plans, progress reports, questions, new explanatory documents, handoffs, and final responses in Korean; preserve code, commands, paths, and identifiers.

## Changes and completion
- Link change work, including documentation fixes, to an Issue and a dedicated task branch. Analysis-only and read-only reviews do not require new tracking or branches.
- Update the owning canonical contract and relevant checks in the same work unit as a behavior or contract change. Use the documentation map to select the task's reading and validation scope.
- Local completion and integration into main are separate. Follow the [validation and merge contract](docs/validation/validation.md) for impact-based checks/review and record unrun checks.
- Integration into main requires a PR, mandatory CI, and finding disposition. Campaign-specific gates apply only to the corresponding campaign work.
