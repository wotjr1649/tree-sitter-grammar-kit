# #205 Go identifier categories and Unicode 17 alignment

This report records local checks of the #205 fix candidate. The syntax oracle is Go 1.27.2 (`windows/amd64`, Unicode 17.0.0); it does not perform type checking. Three-platform CI and integration into actual main have not been verified. The [#198 report](issue-198-core-language-holdout.en.md) retains its historical observations of the earlier candidate.

## Cause and change

The Go grammar used `XID_Start/XID_Continue`. Go identifiers instead use `L/_` initially and `L/Nd/_` afterward. These sets differ. In addition, the pinned tree-sitter 0.27.0 generator uses regex-syntax 0.8.11 Unicode 16.0 tables. Changing only the categories would still omit Unicode 17 characters required by the latest Go 1.27 lexical scope.

The [shared identifier C2 patch](../../src/dev/c2-patches/go.json) combines `L/Nd` with the Unicode 17 additions. Its 875-byte rule represents exactly the same target set as a 22,248-byte full literal. Type, field, package and label aliases share this rule. The semicolon scanner, runtime, core API and Go module minimum of 1.27.1 are unchanged. The [Go B01 contract](../validation/language-feature-disposition.md#go) owns the scope.

The relevant tables in Go [1.27.1](https://github.com/golang/go/blob/go1.27.1/src/unicode/tables.go), [1.27.2](https://github.com/golang/go/blob/go1.27.2/src/unicode/tables.go), and the installed SDK have the same Git blob identity. This does not claim that all earlier Go versions share these Unicode tables. A generator update requires another full-set comparison, including over-acceptance from expanded properties and the supplement.

## Observations

| Check | Local result |
|---|---|
| Go Unicode 17 target predicates | 2,228,224 comparisons: two predicates over numeric 0..10FFFF |
| Generated lexer start set | 145,673 code points / 685 intervals; extra 0, missing 0 against target |
| Generated lexer continuation set | 146,443 code points / 737 intervals; extra 0, missing 0 against target |
| tree-sitter 0.27.0 / Node 24.21.0 / ABI 15 | 6 outputs equal across two workspaces; registered-reference reproduction PASS |
| Windows native | 54 base cases and 57 including query/API PASS; no requirement/API findings |
| Strict CST | 89 records across two cohorts, 167 full trees, 39 fresh comparisons, 11 source/CST restores, 10 negative controls PASS |
| SDK | 63 matching labels: 31 new regression source-step observations and 32 original holdout inputs |
| Go 1.27.2 / CGO_ENABLED=0 | Full `./src/...` suite, vet and build PASS |

Initial output discovery explicitly used `ABSENT` references, so its assessment is `BLOCKED`; only generator and determinism claims are PASS. After registering the actual 6 hashes in all three registries, a separate normal reproduction confirmed reference-match and JS reproduction PASS. Generated C was not edited manually. The qualification inventory was regenerated with the existing `TestQualificationInventory` writer and retains 26 routes and 78 cells.

| Fixed syntax input cohort | Before TP/TN/FP/FN | Fix candidate TP/TN/FP/FN |
|---|---|---|
| Original #198 Go 32 | 18 / 13 / 1 / 0 | 18 / 14 / 0 / 0 |
| Separate #205 identifier diagnostic 16 | 3 / 1 / 7 / 5 | 8 / 8 / 0 / 0 |

Acceptance means a completed full CST with root `has_error=false`; absence of `ERROR` or missing nodes is insufficient. These are fixed syntax inputs, not independent samples of the entire language, and do not establish an overall accuracy percentage.

The 23 new registered originals include the 16 diagnostic inputs plus U+200C, Greek/Hangul/CJK/Nd, Unicode 17 supplement interval endpoints, type/field/package/label aliases, BOM/CRLF/semicolon insertion and four valid→invalid→restore sequences. The 23 originals have counts of TP 14 / TN 9 / FP 0 / FN 0. The 17 positive tree-kind expectations added after native collection were checked separately against the corresponding recorded full CSTs. Local native collection was not repeated solely for these assertions; final CI must use the registered expectations.

## Limits and completion

The SDK helper limits each source to 8,192 UTF-8 bytes and serialized input to 1 MiB. The native operation limits each case to 65,536 bytes. These small fixed inputs and set comparisons do not test cap-sized inputs, actual program parsing of every code point, other grammars, throughput, memory or large-file performance. Local totals include neither Go SDK 1.27.1 execution nor Linux/macOS native execution.

The first Go suite failed because the qualification inventory was stale and the isolated PATH omitted `git` and `pwsh`. The full suite, vet and build passed after updating the inventory and PATH. The first native helper stopped during CLI build because its isolated HOME could not find the module cache; the next execution passed with the existing pinned module cache explicitly selected. Failed records remain preserved. No dependencies were downloaded, and checks or limits were not relaxed.

Completion still requires independent context-review finding disposition, final-candidate mandatory CI on three platforms, a PR, and actual-merge-SHA main CI/internal evidence verification. STATIC review does not replace independent execution or formal GitHub approval. Release publication and consumer development resumption are separate effects.
