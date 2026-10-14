# Fixed-input comparisons for core languages (#198)

[한국어](issue-198-core-language-holdout.md). [#198](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/198) measures syntax acceptance, context differences, recovery, and edit restoration for provisional consumer languages. **One real Go identifier overacceptance was found.** C# conditional branches and T-SQL version/quoted-identifier contexts are separate observations. C/C++ compiler diagnostics are not raw-syntax accuracy. These results do not replace the [26-route support verdict](../specs/platform-support.md).

## Candidate and method

On 2026-10-14, Windows/amd64, the CLI was built from main `cdc0564ce7e072d90aa64bc24bf23606ac4df2d2` using Go1.27.2 and `CGO_ENABLED=0`. Its SHA256 is `ba1a959585d6237d54962df31d86a193a5dc9716aaa748f09a83cba6372b530e`. The module floor and existing CI remain Go1.27.1. A separate full Go suite, vet, and build passed on Go1.27.2; that compatibility check is not a performance comparison.

Inputs, labels, profiles, sources, and oracle contexts were frozen before native execution. There are82 language-case pairs: Go32, C#14, C9, C++9, T-SQL18, representing81 distinct raw byte strings because one C/C++ input is shared. Five cases register two edits: invalid original → closing brace/bracket repair → original restoration. Running an input under multiple SDK versions does not create additional distinct inputs. Consumer priority remains provisional; the paused go-treesitter repository was neither executed nor modified.

Native collection used existing `tsgk-oracle/r2`, `native-query`, `output=tree`, `api=false`, and no queries. Grammar/runtime compilation and execution use separate processes; no Cgo or consumer dependency was added to the core. The prepared source closure from the fixed [native registry](../../src/contracts/native-routes.json) contains107 files:24 across five grammars and83 runtime files. Hashes were checked before and after execution. Existing reference checkouts were preserved.

| Route | Pinned upstream commit | Independent oracle and executed contexts |
|---|---|---|
| Go | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` | Go1.27.2 `go/parser` and `go/scanner`; acceptance, scanner diagnostic counts, and implicit-semicolon offsets match the existing Go1.27.1 observations |
| C# | `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` | .NET SDK10.0.401, Roslyn5.9, runtime10.0.12; `Regular`, C#7.3 and14, preserving each case's symbols |
| C | `b780e47fc780ddc8da13afa35a3f4ed5c157823d` | GCC16.2.0, C99 andC23; fixed compiler contexts |
| C++ | `c009222808634c1014f82438d4883753516a2c24` | G++16.2.0, C++11 andC++23; fixed compiler contexts |
| T-SQL | `8620fbcfca9438e1ff7104835bcb8305aba3bdfa` | Microsoft ScriptDOM180.107.0, assembly18.0.0.0; parser110 and170, initial quoted identifiers false andtrue |

An upstream commit alone does not identify the executed source. Prepared sources include registered adoption/C2 patches. The registry SHA256 is `85589bb6656c5e211d27480ab1ffb3fa7fd22cdd469f5256102a3b111654df08`; the runtime manifest SHA256 is `affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080`. The runtime commit is `659cda7c7f86ebe31cc825dc5da59e9add172dc7`. The registered compiler is the existing UCRT64 GCC16.2.0. No runtime/compiler installation was performed.

## Syntax comparisons and full coverage

Native acceptance means a completed full parse whose root has `has_error=false`; it is distinct from process success or route PASS. The positive class is syntax acceptance: TP=both accept, TN=both reject, FP=only native accepts, FN=only the oracle accepts. Ordinary oracle syntax diagnostics count as invalid. Execution failures, resource limits, and unsupported contexts do not count as TN. Different parsers have no contract requiring identical AST/CST structures, diagnostic counts, or diagnostic locations.

| Route | Inputs | SDK observations | Syntax inputs / observations | TP / TN / FP / FN | Observations retained separately |
|---|---:|---:|---:|---|---|
| Go | 32 | 32 | 32 / 32 | 18 / 13 / 1 / 0 | None |
| C# | 14 | 28 | 8 / 16 | 8 / 8 / 0 / 0 | Different conditional contexts8, language-version probes4 |
| C | 9 | 18 | 0 / 0 | Not applicable | Compiler contexts18 |
| C++ | 9 | 18 | 0 / 0 | Not applicable | Compiler contexts18 |
| T-SQL | 18 | 72 | 17 / 34 | 20 / 12 / 0 / 2 | Initial quoted identifiers=false contexts34, version probes4 |

All82 input pairs and168 SDK observations are retained. The16 C# and34 T-SQL syntax observations run8 and17 inputs respectively under two versions. The eight C# observations under different contexts and34 T-SQL quoted-context observations remain in the full coverage denominator. This table establishes neither whole-language accuracy nor a random sample nor168 independent inputs. Raw-syntax accuracy was not measured for C/C++.

For C# `inactive-broken`, `undef-inactive`, and `nested-directives`, the actual native CST includes the erroneous body of the first non-literal-false branch and retains alternatives as `preproc_skipped_section`. This differs from Roslyn's registered symbols or source `#undef`. `active-broken` selects the same erroneous branch under its registered `ENABLED` context and remains in the syntax comparison. `unclosed-directive` also retains a different branch context because A is undefined in Roslyn. Agreement on rejecting its unterminated directive does not establish accuracy under equivalent contexts. Symbol cases were not excluded as one group, and native was not equated with an empty-symbol compiler. Syntax acceptance of the two version probes through `CSharpSyntaxTree.ParseText`/`GetDiagnostics` is separate from compilation feature diagnostics such as CS8370.

Native's default T-SQL quoted context was checked using the identifier node in `SELECT "name";` and acceptance of the quoted table-name control. Syntax comparisons therefore use initial quoted identifiers=true. ScriptDOM's two rejections of the quoted table-name under false are not counted as FPs for the same native configuration. Acceptance by both parsers also does not prove equivalent literal/identifier interpretation of SELECT. `JSON_OBJECT`, rejected by parser110 and accepted by170, is a separate version probe; the fixed grammar is not claimed to validate version110.

C/C++ header-free inputs used `-std=<registered> -pedantic-errors -fsyntax-only -nostdinc -fdiagnostics-format=sarif-stderr -fmax-errors=16 -`. In C, two undeclared-semantic observations and two older-version feature observations differ from native syntax acceptance; C++ has the same2+2 categories. All eight differences are retained. Compiler acceptance of macro/preprocessor inputs establishes neither raw-source CST equivalence nor source-mapping equivalence.

## Mismatch causes and disposition

### Go identifier: a false positive requiring a fix

The minimal input is the actual U+200C UTF-8 byte sequence represented by `package p\nvar a\u200Cb=1\n` (source SHA256 `f4c1d4d1c64fa8cf249b021db60e0d765d7380c16c346c554fd9cf0175fb80df`). Native accepts without errors; Go1.27.1 and1.27.2 reject it. The pinned grammar uses `[_\p{XID_Start}][_\p{XID_Continue}]*`; its prepared parser's identifier character set actually includes U+200C–U+200D. This differs from the underscore/Unicode Letter/Decimal Digit rule in the [Go specification](https://go.dev/ref/spec#Identifiers).

[Fix Issue #205](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/205) requires correcting the owning C2 patch, regenerating with the pinned generator, updating registry identities, testing Letter/Nd positives and Cf/combining/connector/letter-number negatives, and preserving semicolon/BOM/CRLF/edit and three-platform regressions. This report neither changes the grammar nor closes that defect. The observed FP was not removed by changing a label.

### C# conditional context: a separate implementable contract

The current [conditional-fragment policy](../validation/language-feature-disposition.md) and [#182 report](issue-182-csharp-conditional-fragments.md) do not implement compiler symbol evaluation. Eight observations under four different contexts include six invalid/valid differences from three cases; both observations of `unclosed-directive` reject in both parsers. Actual source and branch selection establish a support boundary, not an accuracy PASS for compiler contexts.

Resolution requires explicit symbol context, ordered `#define`/`#undef`, conditional-expression and nesting evaluation, active projection or included ranges preserving original coordinates, and original/projection/range identities in profile, producer, and replay contracts. Defined/undefined, nested, Unicode/CRLF, fragment, edit/restore, and depth/resource controls must accompany it. This is technically implementable and was not implemented here. The existing fragment policy was not silently changed.

### T-SQL CTE: oracle acceptance differences do not establish a grammar defect

For `SELECT 1\nWITH c AS (SELECT 2 AS x) SELECT x FROM c;`, native rejects while ScriptDOM110 and170 accept under both quoted contexts. The original invalid label is preserved. The table's two FNs are differences **relative to ScriptDOM** under true. The same two differences under false remain separate observations.

Microsoft's current [CTE documentation](https://learn.microsoft.com/en-us/sql/t-sql/queries/with-common-table-expression-transact-sql?view=sql-server-ver17) requires the preceding statement in the same batch to have a semicolon. The repository's historical [#121 engine comparison](issue-121-tsql-engine-validation.md) and [#122](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/122) address that boundary after a preceding SELECT. SQL Server was not executed in this evaluation. The existing grammar guard and label were therefore not relaxed to match ScriptDOM acceptance. An engine comparison of this exact new input and ScriptDOM's internal cause remain unverified; resolving those questions requires an authorized fixed-input check and verified package-to-source mapping. Package signing establishes neither parser accuracy nor equivalence to its declared source commit.

## Recovery, CST integrity, and replay

Before native execution, all five preregistered repairs were accepted in11 SDK/compiler contexts. The preparation's72 additional SDK records comprise11 repairs, six empty-symbol C# observations,51 other controls, and four fresh T-SQL restorations. These are not72 new inputs and are not added to the168 base SDK observations as independent samples. Go/C#/C/C++ oracle restoration reuses existing observations of the original bytes; only the four T-SQL restoration contexts were newly executed.

Native collected82 originals and ten edit steps once. Full CST observations total **102**:82 initial + ten edited incremental + ten fresh. A separate Go verifier established:

- Original source SHA/length and every source version/edit byte coordinate reconstructed by `ApplyEdits` match.
- All12 node fields, structure, digest, node count, depth, and root error state pass; every span's LF row and UTF-8 byte column was independently recomputed from its actual source prefix.
- All ten incremental/fresh pairs match across the full12-field CST. All five original-source/CST restorations match. All five repaired native trees have no errors.
- Ten verifier controls reject duplicate keys, null booleans, extra point elements, ASCII/Unicode case aliases, and digest/source/coordinate/restoration mutations. The coordinate mutation passes structural validation before failing the byte/point check. The restoration mutation keeps both final trees equal and fails comparison with the original.

Valid recovery trees containing error/missing nodes were preserved. SDK diagnostic coordinates retain their existing independent conversion receipts; four differences between Go's original parser line/column and source-prefix coordinates were not forced to agree. Exact agreement between all implicit-semicolon offsets and a native token stream is not claimed.

`kit.VerifyOracleSet` validates all five sets. `oracle-set-r2` replay of unchanged copies and the frozen workload reproduces Go/C#/C/C++ PASS and T-SQL BLOCKED. The `case-source` role belongs only to logger-oriented `native-result-r3`. A first r2 registration using that role failed with `MEMBER_ROLE_UNKNOWN` and was preserved; a separate registration using supported r2 roles was then checked. Source-based edit-point recomputation is not attributed to r2 replay: the separate verifier supplies that evidence.

T-SQL `unclosed-bracket` reports `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1`: no nodes were reused next to full trees including an error tree, limiting route proof. Incremental equality is PASS; actual full-CST comparison and restoration also pass. Set integrity, syntax acceptance, and route proof are distinct claims. #197's separate logger was not applied retroactively to this r2 result. Stronger observation requires evidence newly collected with that producer and an explicit contract consuming those observations.

Evidence is in task-local `artifacts/198-checks` and preserved `artifacts/followup-goal-20261014/198-preflight`. The native collection receipt, `cst-check/result-r4.json`, `sdk-comparison-r3.json`, and `offline-replay-r2/*/result.json` separate collection, coordinates/restoration, SDK accounting, and replay. Artifacts remain untracked under repository policy. These tables alone do not authenticate execution independently. Right-pane Codex reviews are `STATIC_REVIEW` of supplied code/facts, not independent execution checks.

## Unmeasured scope and release implications

This is a small fixed synthetic Windows evaluation. It does not validate accuracy for the other21 routes, whole language specifications, actual consumer corpora, this same holdout on Linux/macOS, query/API semantics, or long-run stability/RSS/throughput/latency. Collection durations include compilation/orchestration and are not benchmarks. ScriptDOM source-commit mapping, new SQL Server execution, and consumer restart were not performed.

#199 source-only0.x release preparation must disclose these observations, unresolved #205, and the C# context boundary. There is no evidence here for whole-grammar accuracy, equal accuracy across three platforms, or a speed advantage over competitors. Tag/Release publication requires separate authority and final-candidate checks. Completing the #198 evaluation and resolving every individual limitation are separate outcomes.
