# Grammar support and verification scope

[한국어](grammar-support.md) · [English](grammar-support.en.md)

This user guide explains grammar-kit's purpose and design, the version ranges of registered grammars, and what their tests establish. It creates no new support contract. Product responsibilities belong to [scope](specs/scope.md), structure to [architecture](design/architecture.md), support assessments to [platform](specs/platform-support.md), and syntax requirements to [feature scope](validation/language-feature-scope.md) and [disposition](validation/language-feature-disposition.md).

## Purpose and design rationale

`tree-sitter-grammar-kit` is a Go CLI (`tsgk`) and a small public offline Go API (`src/kit`) for inspecting and comparing files, generated artifacts, parse structure, queries, runtime APIs, and their verification evidence when adopting or updating Tree-sitter grammars. Its first users are maintainers adopting or updating grammars for `go-treesitter`. Go conversion, scanner porting, runtime integration, and final adoption remain the consumer's responsibilities; ordinary application parsing does not require running the kit each time.

A successful compilation or a tree without `ERROR` is insufficient to establish update safety. Changed declaration fields or source spans can break consumer analysis, and incremental parsing can differ from fresh parsing. Overacceptance of invalid syntax also requires separate negative cases.

The CLI and API share the same offline core. The core inspects and compares with `CGO_ENABLED=0`; generators and the native C driver run in separate processes subject to the runner's time, output, memory, and cleanup checks. This keeps file inspection independent of a native toolchain and separates the failure boundaries of execution checks. The runner is not a complete sandbox for arbitrary malicious native code.

| Verification stage | Operations | What it checks |
|---|---|---|
| Inputs and file sets | `inspect`, `identity`, `corpus`, `verify` | What was inspected and which bytes or files differ from the trusted baseline |
| Static structure | `schema check`, `schema diff` | Changes to node, field, and reference contracts, and review risks |
| Artifact reproduction | `reproduce` | Whether results from independent workspaces match using a pinned generator |
| Native behavior | `incremental`, `oracle record` | Whether tree, edit/fresh, capture, fact, and API observations match registered expectations |
| Evidence and aggregate assessment | `replay`, `evidence verify`, `qualify` | Whether evidence is complete, identity matches, and requirements are satisfied |

A `verify` PASS means the selected set matches caller-trusted expected data. A fingerprint does not authenticate provenance, and a `schema diff` does not automatically establish a defect or SemVer decision. Ordered CST preserves parent relationships, order, fields, flags, and original byte/point spans; captures preserve order and duplicates. Sorting, deduplication, or span adjustment must not hide differences before comparison.

The kit links source, tools, inputs, policy, comparator, and execution identity, while separating `execution_status`, `evidence_mode`, and `assessment`. Execution completion does not imply a requirement PASS, and replay is not a new native execution. Qualification's `mechanism` assesses the kit's checking machinery; `requirement` assesses the grammar's syntax requirements. Successfully detecting a grammar failure does not turn that failure into grammar support. Details belong to [identity/evidence](specs/identity-and-evidence.md) and [tree/protocol](specs/tree-and-adapter-protocol.md).

## Meaning of support status and versions

This guide's analysis baseline is main `679d7dd` (2026-10-12). The adopted language scope has a cutoff of `2026-09-29`; the upper bounds below are not described as the latest external releases today. The qualification recorded in the [platform contract](specs/platform-support.md) and [#188 report](reports/issue-188-corpus-cst-api.md) is PASS for 26 routes × 3 platforms = 78 cells on Windows amd64, Linux amd64, and macOS arm64, with 2,478 obligations (826 per platform). Writing this guide does not constitute a native rerun.

Offline `inspect`, `identity`, `verify`, `schema`, and their public APIs are VERIFIED. `corpus`, `reproduce`, `incremental`, `oracle record`, `replay`, `evidence verify`, and `qualify` are experimental and carry no compatibility promise. SUPPORTED for the 26 routes assesses registered candidates and cases; it is not a complete conformance certification for arbitrary input.

A language version defines the syntax scope to check; a grammar revision identifies parser source. Tests connect cases in the [qualification inventory](../src/contracts/qualification-c1.json) with version-specific syntax requirements. They do not establish that every compiler, SDK, or interpreter version was installed and executed, or that every minor/patch release was tested individually. Basic syntax and change families are tested together; gaps between numbers in the grouped test column do not imply that the entire intervening version is unsupported.

## The 26 required routes

The table summarizes [feature scope](validation/language-feature-scope.md), [disposition](validation/language-feature-disposition.md), and the [source registry](../src/contracts/language-sources.json). The test column lists registered syntax families, not versions of individually executed tools. Every route's support assessment is limited to the registered cases and three-platform scope described above.

| Grammar / route | Adopted syntax scope | Registered version test families (including basic syntax) | Base grammar commit (12 characters) |
|---|---|---|---|
| C# / `csharp` | 7.3 / net461 source → 14 / net10.0 source | 7.3·8·9·10·11·12·13·14, 14 file-based directive | `9150f7d56bb4` |
| Go / `go` | 1.17 → 1.27 | 1.18~1.27 change families, 1.27 items | `2346a3ab1bb3` |
| Python / `python` | 3.8 → 3.14 | 3.8·3.9·3.10·3.11·3.12·3.13·3.14 | `26855eabccb1` |
| JavaScript / `javascript` | ES5.1 → ECMAScript 2026 | ES2015, 2016~2019, 2020~2021, 2022~2025 families, script/Annex B | `58404d8cf191` |
| JSX / `jsx` | classic JSX → pinned JSX specification | Adopted syntax including element·attribute·expression; specification revision pinned without a numeric language version | `58404d8cf191` |
| TypeScript / `typescript` | 2.8 → 7.0 | 2.8~2.9, 3.0~3.9, 4.0~4.4, 4.5~4.9, 5.0~5.4, 5.5~7.0 | `75b3874edb2d` |
| TSX / `tsx` | TS 2.8 + JSX → TS 7.0 + JSX | Basic TSX, 2.9~7.0, type parameter/JSX boundaries | `75b3874edb2d` |
| Java / `java` | Java SE 8 → 27 | 9~11, 14~17, 21~22, 25~27 change families | `e10607b45ff7` |
| Kotlin / `kotlin` | 1.3 → 2.4 language line | 1.4~1.9, 2.0~2.3, 2.4 stable syntax families | `3dea6dfa9c01` |
| C / `c` | C99 → C23 | C11/C17, C23·preprocessor, adopted legacy syntax | `b780e47fc780` |
| C++ / `cpp` | C++11 → C++23 | C++14·17·20·23, adopted legacy syntax | `c00922280863` |
| Rust / `rust` | Edition 2015 → 2024 / stable syntax through rustc 1.98.1 | Change families from 2018 onward, through 2024/1.98.1, edition boundaries | `77a3747266f4` |
| Swift / `swift` | 5.0 → 6.4 | 5.0~5.4, 5.5~5.8, 5.9~6.1, 6.1·6.2, 6.3~6.4 | `35245fbfee2f` |
| Dart / `dart` | 2.12 → 3.13 | 2.13~2.19, 3.0~3.3, 3.6~3.9, 3.10~3.13, legacy | `be07cf7118d3` |
| PHP / `php` | 5.6 → 8.5 | 5.6~7.1, 7.2~7.4, 8.0~8.1, 8.2~8.3, 8.4~8.5, legacy | `92b5271b60be` |
| Ruby / `ruby` | 2.7 → 4.0 | 2.7~3.0, 3.1~3.4, 4.0 | `ad907a69da0c` |
| R / `r` | 3.6 → 4.6 | 4.0~4.1, 4.2~4.6 | `58a22794466c` |
| Bash / `bash` | 3.2 → 5.3 | 4.x~5.2, 5.3 | `a06c2e4415e9` |
| PowerShell / `powershell` | Windows PowerShell 5.1 syntax → PowerShell 7.6 syntax | 5.1 legacy, 6.0~7.6 | `e7bd348c49fd` |
| HTML / `html` | HTML5 REC 2014 → Living Standard 2026-09-28 | Adopted syntax including document·fragment·raw-text; specification revision pinned | `73a3947324f6` |
| CSS / `css` | CSS2.1 → Syntax Level3 + adopted Snapshot2026 modules | Basic syntax and 30 module rows (selectors·cascade·color·fonts·flexbox·grid, etc.) | `dda5cfc5722c` |
| JSON / `json` | RFC8259 / ECMA-404 2nd edition | Strict JSON·negative·structure | `254c42a64764` |
| YAML / `yaml` | 1.1 → 1.2.2 | directive·flow·block, 1.1/1.2 syntax boundaries | `a1c4812a73ec` |
| XML / `xml` | 1.0 5th edition + 1.1 2nd edition | namespace·DOCTYPE, 1.0/1.1 boundaries | `5000ae8f22d1` |
| T-SQL / `tsql` | SQL Server2012/compat110 → SQL Server2025/compat170 syntax | Basic syntax, 2016~2019, 2022~2025, batch·context boundaries | `8620fbcfca94` |
| PostgreSQL SQL / `postgresql-sql` | 9.6 → 18 | 10~12, 13~15, 16~17, 18, legacy | `59d0d8cd7506` |

The commit column identifies the base upstream revision. Actual candidates also include each route's patch chain, regenerated artifacts, and runtime patches; the commit alone does not establish reproduction of the same parser. JSX uses the JavaScript grammar, while TypeScript/TSX use the `typescript`/`tsx` directories of the same repository. The [native routes](../src/contracts/native-routes.json) and [runtime manifest](../src/drivers/native-c/runtime-manifest.json) pin the actual closure. Registered native inputs are regenerated with Tree-sitter CLI 0.27.0/ABI15.

A union of syntax from multiple versions differs from a compiler that selects a version. The C# row's net461/net10.0 entries identify source profiles and do not guarantee .NET runtime execution. Accepting adopted T-SQL syntax from 2012~2025 does not mean every construct is valid under compatibility110. Some legacy interpretations of C++ `module`/`import` and comma subscripts, and PHP legacy tags and `#[`, are excluded under the [version-conflict dispositions](validation/language-feature-disposition.md), because the same bytes have different interpretations across versions.

T-SQL also has a separate comparison against SQL Server2025 Express LocalDB17.0.1000.7. Checks run at compatibility110·170 do not substitute for execution on an actual SQL Server2012 engine or establish whole-application execution compatibility. The [engine comparison report](reports/issue-126-expanded-validation.md) owns the actual execution scope and unrun checks.

## Additional formats and reference status

| Target | Status | Scope and reason |
|---|---|---|
| WCF `.svc` | Registered cases PASS on three platforms | `SVC-SERVICEHOST-r2` directives·inline C#·recovery·edits·registered references; a composite format using C#, not an additional language route |
| NET461 large C# workload | Executed and verified on Windows | Windows business workload profile; Linux·macOS NOT_APPLICABLE |
| `.csproj`·`.resx`·`.config`·`.wsdl`·`.xsd`, etc. | XML source syntax observations | No guarantee of MSBuild, resource, XSD/WSDL-specific semantics or execution |
| BrightScript grammar v0.1.4 | maintained reference, NOT_RUN | No native route/workload registered in the campaign |
| BrightScript grammar v0.1.2 | historical replay target, NOT_RUN | Verification bundle not prepared; cannot serve as NEW_RUN evidence |
| Cooklang | static metadata audit, NOT_RUN | Only S00 static metadata observations; no native execution |
| Owned scannerless/stateful fixtures | Separate native tests | Kit machinery and scanner validation; not a list of supported user languages |

BrightScript v0.1.x identifies releases of the grammar project, not language versions. Additional role status belongs to the [qualification inventory](../src/contracts/qualification-c1.json), business formats to the [NET461 workload](validation/net461-workload.md), and reference revisions to [provenance](provenance/upstream-sources.md). Reading a file as XML does not establish dedicated XSD/XSLT language support.

## Why other grammars and extensions are outside the support scope

The current campaign fixes its scope at 26 routes. Lua·Elixir·Scala·Haskell·Zig·OCaml·Julia·Objective-C·MySQL·SQLite·Oracle SQL, among others, are not required routes. No individual technical rejection rationale is registered for each of them; absence from the registry does not mean implementation is impossible. Additional support requires source closure, specification/version scope, expectations, positive/negative/recovery/edit/query cases, and evidence for each platform.

The following are representative exclusions from [disposition](validation/language-feature-disposition.md). The complete list follows each route's EXT rows and version-conflict dispositions. A candidate grammar's lack of support is not itself grounds for excluding adopted required syntax.

| Excluded target | Reason |
|---|---|
| Razor·separate `.csx` hosts·Vue/Svelte templates·ERB·Blade | Composite formats, hosts, or template languages distinct from ordinary source |
| JSONC/JSON5·Sass/Less·Helm/Jinja YAML | Extensions distinct from strict JSON, adopted CSS modules, or YAML |
| zsh/fish/ksh-specific syntax | Shell syntax distinct from Bash |
| PL/pgSQL·psql backslash commands | Languages or client syntax separate from PostgreSQL SQL |
| Azure/Fabric/Synapse-specific SQL | Outside the adopted SQL Server T-SQL scope |
| Python2·notebook magic·pre-2.12 Dart | Outside adopted versions or modes |
| GNU/MS C/C++ extensions·C++26 draft | Outside adopted standards and stable version scope |
| Java preview·Rust nightly·Kotlin experimental, etc. | Outside adopted stable syntax |
| XQuery·DTD-only routes·XSD/XSLT-specific semantics | Separate languages or validation responsibilities beyond XML source syntax |

Later fixes added cases for SQLCMD `:r`/`:setvar` that check batch and CTE boundaries in surrounding T-SQL. Include-file execution and variable substitution remain out of scope; this does not establish complete SQLCMD execution support. Distinguish the preparation-time EXT list from later implementation and follow the boundaries in the [#122·#123 report](reports/issue-122-123-tsql-boundaries.md).

Type and name resolution, macro/proc-macro execution, DB object existence and SQL execution, DOM/rendering, external XML entity retrieval, and MSBuild/NuGet/WCF activation are responsibilities beyond syntax interpretation. A real corpus's CST/API PASS, source without ERROR, and successful application build/runtime are separate outcomes. The [#188 report](reports/issue-188-corpus-cst-api.md) distinguishes structural/API validation of error trees from source and dialect boundaries.

Offline `inspect`, `identity`, `verify`, and `schema` are not restricted to the 26 routes. They can inspect other grammars that use supported file/schema formats. Such inspection does not replace qualification of that language's syntax support.

## Updating this guide

When support scope, versions, source, or status change, first check the owning canonical contracts and registries and update both language versions together. Do not automatically carry historical source-registry preparation states or release-time limitations forward as current native success or failure. Bind support assessments to candidate, run, and platform evidence, and distinguish local documentation checks from native/cross-platform reruns. [Validation](validation/validation.md) owns verification, review, and integration procedures.
