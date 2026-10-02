# 기능별 검증 workload

추가 [NET461 업무 workload/format 등록부](net461-workload.md)의 파일 역할·case 종류·Session 책임은 기존 Session에 연결한다. 업무/.svc 결과는 기존 26 route·78개 요약 칸과 별도로 결속하고 현재 full corpus·svc 제품 기능·S08 qualification은 NOT_RUN이다. 원본/encoding/mapping·안전한 참조·후속 검증의 준비 계약을 현재 PREPARE에서 확인하며 미래 검증 전체를 새 pre-S01 gate로 만들지 않는다.

실제 test는 해당 Session 구현과 함께 추가한다. 아래는 계획이며 PASS 기록이 아니다. 모든 Session은 [공통 gate](validation.md)를 적용하고 미지원 capability를 BLOCKED/UNSUPPORTED로 구분한다.

| Session / Issue | 정상·negative·mutant·경계 검증 | 플랫폼/완료 범위 |
|---|---|---|
| 00 / [#2](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/2) | 잘못된 module/경계·누락 문서·broken link·긴 AGENTS·tracked local artifact를 검출한다. source-only가 상위 Git을 탐색하지 않는지 확인한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 01 / [#3](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/3) | 오프라인·target 불변·같은 bytes의 결정성, 파일 변조/잘못된 expected identity/unknown layout/drive·UNC·traversal·link·size 초과를 검사한다. 언어별 하드코딩 mutant를 거부한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 02 / [#4](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/4) | 중복 key·큰 정수·NaN·case/device/ADS/trailing-dot 충돌·symlink/junction·ZIP duplicate/overlap/zip-bomb·정상 macOS root alias를 시험한다. 경계 직전/직후와 검증 삭제 mutant가 실패해야 한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 03 / [#5](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/5) | required/multiple 변경, missing/duplicate node, named/anonymous type, supertype 참조, key-order만 달라진 정상 예시와 planted structural difference를 검출한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 04 / [#6](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/6) | source 불변, stale/poisoned cache, 다른 generator/옵션, nonzero exit, output flood, timeout/cancel, child/grandchild cleanup을 시험한다. 미지원 backend cap은 BLOCKED로 종료한다. | 세 OS 필수 등록 범위; 필수 capability 미지원은 진행 BLOCKED |
| 05 / [#7](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/7) | CRLF/NUL/BOM·잘못된 UTF-8·EOF·zero-width·byte boundary·다중 edit·취소·중간 malformed와 planted incremental mismatch를 시험한다. fresh final 한 번만 비교하는 mutant를 검출한다. | 세 OS 필수 등록 범위; 필수 capability 미지원은 진행 BLOCKED |
| 06 / [#8](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/8) | scanner 누락, ABI 불일치, 잘못된 state serialization, incomplete/null tree, ERROR 내부 구조, duplicate range/capture, child cleanup과 source identity mutant를 검출한다. | 세 OS 필수 등록 범위; 필수 capability 미지원은 진행 BLOCKED |
| 07 / [#9](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/9) | 숫자 경계·signed zero·max winner·threshold 근처·누락/중복 record·바뀐 comparator/policy/source를 대조한다. archive verifier 없이 재판정할 수 없는 데이터는 RECORDED_NOT_RECOMPUTED로 남긴다. | 세 OS 필수 등록 범위; 필수 capability 미지원은 진행 BLOCKED |
| 08 / [#10](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/10) | planted cross-platform difference, missing OS artifact, 다른 attempt/identity 혼합, scannerless와 scanner 경로, OS별 resource metric, normalization으로 차이를 숨기는 mutant를 검출한다. | 세 OS 필수 등록 범위; 필수 capability 미지원은 진행 BLOCKED |

작은 자체 scannerless와 stateful-scanner fixture를 독립·오프라인 회귀 기준으로 둔다. 외부 대형 grammar 전체나 verification ZIP을 testdata에 vendor하지 않는다. BrightScript stable identity와 historical replay, Cooklang audit 목적은 [provenance](../provenance/upstream-sources.md)에서 분리한다. malformed byte/JSON/archive fixture는 의도적인 negative 입력이며 문서 링크 검사 대상이 아니다.

## Campaign 01 채택 수용 조건

아래 영문 조건은 제공된 실행 specification의 판정 조건을 그대로 채택한 것이다. 각 Session 표의 마지막 추가 행(S01-A15~A19, S02-A15~A16, S03-A15~A18, S04-A16~A18, S05-A16~A22, S06-A15~A17, S07-A15~A16, S08-A16~A17)은 2026-10-03 PREPARE 보완에서 사용자가 채택한 실사용 source·비공개 corpus·진행 규칙 조건이다. 모든 행은 구현 전 요구사항이며 PASS 증거가 아니다. 세션/Issue/branch/플랫폼 기계 연결은 [campaign 정의](../../src/contracts/campaign-01.json), 언어 범위는 [feature owner](language-feature-scope.md)를 따른다. 각 세션은 등록된 26개 route의 자기 단계 검사군을 소유하며 4개 fixture 역할을 구분한다.

### S01

| ID | Concrete case | Required outcome |
|---|---|---|
| S01-A01 | CLI and external API inspect the same fixed root | Same semantic inventory/findings without requiring native tools |
| S01-A02 | Valid owned and registered upstream layouts, including multi-grammar/shared files | Correct explicit selection and observed closure; no language-name switch in common core |
| S01-A03 | Missing metadata or unknown layout | Observation with limitations, not fabricated qualification or crash |
| S01-A04 | Same bytes scanned twice in differently located roots | Same content identity under same relative selection/policy |
| S01-A05 | Change one byte, path, role, selected membership or required mode | Appropriate identity change; independent expected fixture, not self-derived golden |
| S01-A06 | BOM/CRLF/NUL/non-ASCII fixture | Raw hash/size preserved exactly |
| S01-A07 | Traversal, child symlink/junction/reparse, special file, root outside authority | Rejected before unauthorized content is read |
| S01-A08 | File/count/total/depth/result boundary and one-over, source read failure/change | Typed invalid/limited/I/O result, no partial PASS, bounded allocation |
| S01-A09 | Cancellation during a sizeable scan | Bounded cancellation, no input/output corruption |
| S01-A10 | Output already exists, inside input, link, publication collision | Existing data preserved; no-clobber failure |
| S01-A11 | Run product with executable discovery/network unavailable | Offline operation still works and invokes no target code/process |
| S01-A12 | API direct call, concurrent independent roots where supported | Same guards as CLI; documented ownership and no process-global mutation |
| S01-A13 | Targeted mutant skips a file, returns fixed digest, normalizes bytes, or hard-codes one grammar | Corresponding meaningful test fails for the intended reason |
| S01-A14 | Three real OS/arch core/API lanes | Current-candidate tests/build/vet/CGO closure and source-only checks pass; no zero-test pass |
| S01-A15 | Encoding detection fixtures: UTF-32 BOM, UTF-8/UTF-16LE/UTF-16BE BOM with valid and invalid content, BOM-less NUL, ASCII, strict UTF-8, a cp949 declaration and per-file `utf-8`/`cp949` declarations supplied as API/CLI input (strict profile parsing is S02) | Steps 1–6 of the [real-world source policy](net461-workload.md) in order; per-file declarations replace only steps 4–5; detected encoding and its source are bound to identity; outcomes that need the cp949 table are `assessment=UNRESOLVED` with finding `ENCODING_TABLE_REQUIRED` until S05, never guessed |
| S01-A16 | UTF-16 input with odd length, unpaired surrogate or U+0000 | Typed BLOCKED before any parse input is produced; raw hash/size still recorded |
| S01-A17 | Six adopted routes from `language-sources.json` `adoption` (upstream commit, patch subjects, patched-file hashes; T-SQL meloncholera identity) | Recorded as inventory/identity inputs and checked against the registry; S01 does not fabricate or re-derive patched trees (S04) |
| S01-A18 | Private local corpus `NET461-PHASE2-LOCAL-r1` through operation `private-corpus-local` | Every non-build file gets an N461 role or `UNCLASSIFIED` (role axis only; routes come from the extension table); credential-like files and vendor binaries are `PRESENCE_ONLY` (existence and size, content never read); build outputs excluded; declared `.csproj` membership observed with conditions/imports/wildcards `UNRESOLVED` and missing items `NOT_FOUND`; detected encoding and its source (steps 1–4 in S01; table-dependent outcomes completed in S05), newline class, size class, generated marker and content-duplicate groups recorded; public outputs carry counts and judgements only |
| S01-A19 | Corpus at and one over each `private-corpus-local` limit, exercised with injected reduced limits and count/byte accounting tests; a corpus file over 32 MiB | At the limit COMPLETED, one over typed `RESOURCE_LIMIT` with no partial PASS; `pg-large-source-r1` does not apply to `private-corpus-local` |

### S02

| ID | Concrete case | Required outcome |
|---|---|---|
| S02-A01 | Independently prepared expected set equals actual | Exact-set PASS with selected-set identity, CLI/API agreement |
| S02-A02 | Remove/add/rename/alter one member | Specific finding; optional absence follows predeclared policy; no generic false success |
| S02-A03 | Duplicate key/path, escaped duplicate, case-mismatched key, trailing object | Strict rejection before use |
| S02-A04 | Huge integer, type confusion, null, fraction/exponent, overflow, invalid encoding | No lossy round-trip or zero/default substitution |
| S02-A05 | ZIP valid fixtures incl empty member and supported normal layouts | Correct bounded inventory/hash; no needless extraction |
| S02-A06 | Local/central inconsistency, duplicate or overlapping entries, CRC/truncated stream | Failure, no completed verified inventory |
| S02-A07 | Path traversal, drive/UNC/device/ADS, case/trailing/Unicode collision, symlink/junction | No unauthorized read/write and useful typed finding |
| S02-A08 | Entry/depth/per-file/total/decompressed/nested budget one-under/equal/one-over | Limit correctly enforced during streaming; no decompression bomb allocation |
| S02-A09 | Manifest inside archive tries to define its own trust/exclusions | Cannot become trusted expected automatically |
| S02-A10 | Authorized extraction with existing output, redirected child or forced write failure | Original preserved; partial classified; no automatic destructive cleanup |
| S02-A11 | Direct public API invocation bypassing CLI | Same path/JSON/size rules and errors apply |
| S02-A12 | Mutants remove exact-set, duplicate detection, streamed limit or no-clobber check | Intended negative tests fail meaningfully |
| S02-A13 | Original S01 fixtures and 26 inventory/profile selections | Regression preserved; missing capability distinct from empty success |
| S02-A14 | Three OS actual core/API lanes, no native tools | Same portable contract, real platform path behavior recorded |
| S02-A15 | Patch subjects, the 24 T-SQL ESM modules and the npm closure supplied as profile inputs | Treated as untrusted data with explicit bounds; no execution and no limit relaxation through profile fields |
| S02-A16 | Strict profile with encoding declarations and operation limits, including unknown/duplicate declarations and limits above the caller-supplied operation limits or the tracked `private-corpus-local` limits | Rejected with typed errors; a profile cannot raise limits above those bounds |

### S03

| ID | Concrete case | Required outcome |
|---|---|---|
| S03-A01 | Same schema with reordered object keys and declared unordered alternatives | Equivalent static result and deterministic difference report |
| S03-A02 | Same type spelling with named true/false | Distinct identity; no collision |
| S03-A03 | Node/field addition, removal or identity change | Correct before/after directional findings |
| S03-A04 | Required/multiple transition and added/removed alternatives | Exact field-level difference, not silently additive/safe |
| S03-A05 | Supertype/subtype and children-contract change | Correct supported graph/shape interpretation |
| S03-A06 | Invalid type, duplicate node/field/member, unsupported revision or broken required reference | Typed invalid-schema/unsupported outcome, no partial PASS |
| S03-A07 | Missing/empty/null fields and zero-node malformed input | Follow adopted distinctions; never default unknown to valid empty |
| S03-A08 | Large/deep schema at limits and cancellation | Bounded graph walk/memory/output and typed cancellation/limit |
| S03-A09 | Reverse baseline/candidate | Add/remove correctly inverted; raw source identities remain separate |
| S03-A10 | Public API and CLI with no parser.c/compiler/network | Same semantic result; runtime parser never called |
| S03-A11 | Mutant ignores named, required, multiple, subtype or a removed alternative | Focused fixture detects the intended defect |
| S03-A12 | Mutant sorts/deduplicates an unrelated ordered-tree fixture | Cross-module regression rejects scope leakage |
| S03-A13 | Registered 26 route schema checks plus owned change controls | Coverage table with real outcomes, not one grammar's PASS multiplied |
| S03-A14 | Three OS core/API/source-only/external-consumer validation | Current-candidate evidence and no added CGO/consumer runtime dependency |
| S03-A15 | Declaration node types and fact mapping per adopted grammar: C# type/member declarations, SQL `CREATE` objects, static `EXEC` targets, C# `CommandText` literals, dynamic SQL sites | Versioned mapping from the schema; each fact kind maps to named node types/fields or is `UNSUPPORTED` with a reason |
| S03-A16 | Large-input summary schema and tree envelope encoding fields | Summary carries canonical tree digest, capped ERROR/MISSING list, declaration structure result and registered-point partial trees; `input` carries detected encoding and its source; no text extracted from XML values |
| S03-A17 | Dynamic SQL kinds: `EXEC(...)`, `EXEC(...) AT`, normalized `sp_executesql`, C# command sites, `EXEC @module_var`, six argument kinds | Closed mapping with `EXEC @module_var` non-dynamic and known misses (`AT DATA_SOURCE`, `WITH RESULT SETS`, batch-first call without `EXEC`) reported |
| S03-A18 | Out-of-scope constructs that `known_gaps` assigns to S03: C# async/var/await identifiers and file-based directives, T-SQL beyond registered B01..B05/V16/V22 rows, PostgreSQL 9.6–18 checkpoints | Structure contract written or explicitly dispositioned for each listed item; no unbounded full-language claim |

### S04

| ID | Concrete case | Required outcome |
|---|---|---|
| S04-A01 | Two clean workspaces, fixed source/tool/options | Both produce independently; registered output comparison explicit |
| S04-A02 | Reference artifact deliberately stale/changed/missing | Correct mismatch or missing-claim finding, not auto regeneration into source |
| S04-A03 | Same JSON but modified JS helper/lock/dependency closure | JS proof cannot be claimed by a JSON-only run |
| S04-A04 | Changed CLI/options/header/dependency/cache | New identity or rejection; stale/poisoned cache not trusted |
| S04-A05 | Target source before/after and publication destination | Source unchanged; no-clobber output; failed partial retained as failure |
| S04-A06 | Tool missing, wrong executable/ABI, denied capability | Block before disallowed launch, no automatic installer/fallback |
| S04-A07 | Nonzero exit, invalid output, truncated stream, output flood | Typed failure/limit, no forged output success |
| S04-A08 | Timeout/cancel with child/grandchild and pipe-holding helper | Verified cleanup within declared limits or blocked backend, not just parent exit |
| S04-A09 | Tool succeeds but evidence write/cleanup fails | Result not reported as fully completed success |
| S04-A10 | Literal spaces/quotes/Unicode in authorized paths and safe root aliases | Correct args/path behavior on actual OS, no shell interpretation |
| S04-A11 | One-over/equal input/time/output/storage limits and budget consumption | Enforced finite policy; no auto doubling |
| S04-A12 | Mutants reuse first workspace/cache, skip source check or only kill parent | Corresponding experiment detects the defect |
| S04-A13 | Three OS native-ready supervisor controls | Actual host/tool capability receipts; unsupported controls explicit |
| S04-A14 | Prior offline public API with no tools on PATH | Still usable; no runner/network import/effect leaks into offline use |
| S04-A15 | Registered 26-route reproduction coverage | No route silently omitted; claims match the actually selected source path |
| S04-A16 | Runner side of the real-world resource policy: 90 s process wall per single parse request, 300 s per edit request with at most 4 edits, batch process wall 3600 s, 4 GiB memory, 5 s termination grace | Linux cgroup and Windows Job Object (`golang.org/x/sys`) enforce hard caps; macOS uses sampled termination and is labelled non-strict; OOM and sampled kill map to `RESOURCE_LIMIT`; the status-mapping protocol is tested with an owned helper (driver-side progress callback and allocator hook are S05) |
| S04-A17 | Patch chain r1→r5 and regeneration for adopted routes (Swift regeneration, PostgreSQL optimized 6 GiB, TypeScript npm and T-SQL ESM closures) | Reconstructed patched sources match the adoption hashes and regenerated outputs match the generated-artifact hashes retained in the PREPARE native evidence, or the mismatch is recorded; the PostgreSQL no-optimization regeneration gap is recorded as an upstream tool gap; no silent baseline replacement |
| S04-A18 | Build-portability patch under the pre-authorization; Windows/macOS tree-sitter CLI 0.27.0 and Node 24.21.0 digests | Only compiler options, platform shims or build scripts change, with separate review, three-OS CI and no regression; generator runs stay BLOCKED until the host tool digests are recorded; if no build-portability patch is needed the patch part is `NOT_APPLICABLE` with the registered reason `no patch needed` |

### S05

| ID | Concrete case | Required outcome |
|---|---|---|
| S05-A01 | Insert/delete/replace sequence on valid source | Each intermediate incremental tree equals an independent fresh tree |
| S05-A02 | Malformed middle state followed by repair | Compare both states; no requirement that the malformed state has no ERROR |
| S05-A03 | Two-or-more edits where only an intermediate result is wrong | Mismatch detected at that step, even if the final tree agrees |
| S05-A04 | Instrumented owned control omits ts_tree_edit or old-tree argument | Route test detects loss of actual incremental path |
| S05-A05 | EOF, empty insertion, zero-width/missing nodes, same-span siblings | Public order/ranges/flags preserved; no span dedup |
| S05-A06 | CRLF/BOM/NUL/non-ASCII/invalid UTF-8 under declared encoding | Transport exact; boundary policy and point calculations correctly applied |
| S05-A07 | Negative/out-of-range/overflow/inconsistent length/prefix/suffix edits | Rejected before native state mutation; no source truncation |
| S05-A08 | Scannerless and owned stateful external-scanner edits | State-dependent behavior checked; deliberate serialization defect is detected |
| S05-A09 | Malformed/oversized/truncated/extra request or response frames | Bounded failure, never a complete successful run |
| S05-A10 | Null tree, node/depth cap, parse timeout, cancellation, output flood | Correct partial/noncomplete status and verified cleanup |
| S05-A11 | Changed parser/scanner/runtime/compiler/executable or source bytes | New identity or rejection; stale executable not reused |
| S05-A12 | Mutants compare only final state, sort/dedup nodes, replace bytes or default unsupported fields | Targeted tests fail for intended cause |
| S05-A13 | Driver allocation/lifetime error paths, consecutive requests in separate processes | No known leak/double-free/use-after-free path; use approved diagnostics if available, record unrun diagnostics |
| S05-A14 | Registered 26 route edit/feature cases on three hosts | Real candidate-bound results, not compiler version or previous corpus success |
| S05-A15 | Prior offline CLI/API in tool-free environment | Native feature doesn't become a mandatory core runtime dependency |
| S05-A16 | cp949 decode with WHATWG `index-euc-kr` (identifier `1d97134cbf187263585bc8f593ca4196654ed4c7a673f5672eaad4f5d9fdc4ba`, file sha256 `89af20dd867c84cefb710b1790229786cfef2bf11916361a210d81b90381e267`) after Go strict pre-validation | Table pinned with its licence notice (CC BY 4.0; BSD-3-Clause for incorporated source); valid files decode; Windows-only and invalid sequences BLOCKED; AMBIGUOUS completed for cp949-declared profiles; `read` returns the whole remaining buffer |
| S05-A17 | Edits at odd UTF-16 offsets or splitting a cp949 two-byte character | Rejected; valid edits keep incremental/fresh agreement |
| S05-A18 | `real-world-source-r2` on synthetic large fixtures (sizes fixed by fixture identity, up to 32 MiB) on three hosts | Full tree only when descendant_count ≤ 50000 and output ≤ 16 MiB, else summary with digest, capped ERROR/MISSING, declaration structure check over S03 declaration node types (no query) and registered-point partial trees; the driver cancels at 60 s per parse through the progress callback and maps allocation failure through the runtime allocator hook to `RESOURCE_LIMIT`; S04-A16 limits apply; a hosted shortfall is recorded as this row's result without blocking S05 closure, and raising a value is a user decision |
| S05-A19 | Nesting depth ≥ 10000 and one input over the 100000 depth cap on three hosts | First completes without stack failure; second ends `RESOURCE_LIMIT` |
| S05-A20 | Private corpus encoding paths and large-file profile run locally on Windows | Every routed, non-`PRESENCE_ONLY` file gets a detected encoding and source or a typed BLOCKED reason; AMBIGUOUS count reported; large files pass through the r2 gate with a recorded status; kit defects 0 or recorded as blocking; runs use the batch request (500 files/256 MiB per process, 60 s per file) and record candidate commit, clean tree, host facts and tool identities with per-file records; public outputs carry counts and judgements only |
| S05-A21 | Owned dynamic SQL fixtures for every closed construct and argument kind | Registered with expected location facts for S06 |
| S05-A22 | Native build of 26 routes on three hosts; out-of-scope constructs in `known_gaps` and `postgresql-sql-B04` (0 registered rows at adoption) | Build failures fixed under the build-portability pre-authorization or recorded; fixture/golden and recovery/edit cases registered for each listed gap; grammar gaps dispositioned and carried to S08 without blocking S06; T-SQL window amendment kept applicable |

### S06

| ID | Concrete case | Required outcome |
|---|---|---|
| S06-A01 | S05 requests run against the extended driver | Preserved documented semantics; revision mismatch fails clearly |
| S06-A02 | Record fresh and edited tree with full closure identity | Exact input/build/runtime linkage, not stale producer reuse |
| S06-A03 | Valid query with repeated captures, ties and multiple patterns | Original ordered stream/duplicates/names/node mapping preserved |
| S06-A04 | Invalid query and unsupported predicate/directive | Accurate error/unsupported state; no fabricated empty success |
| S06-A05 | Query limit/cancel after partial matches | Partial records distinct, cleanup verified, not COMPLETED/PASS |
| S06-A06 | Same-span or zero-width nodes with different identity | Capture mapping doesn't collapse them |
| S06-A07 | Field/child/cursor/registered API facts | Match actual public tree contract, including ERROR/MISSING/extra nodes |
| S06-A08 | Missing/wrong scanner, ABI/header/executable or grammar symbol | Clear failure before unsupported interpretation |
| S06-A09 | Truncated artifact, missing footer/member, record count mismatch | Reference publication/verification rejects incomplete set |
| S06-A10 | Existing output or concurrent collision/write failure | No user artifact overwrite; no completed reference receipt |
| S06-A11 | Mutant sorts/deduplicates captures, defaults missing field, omits scanner or identity | Intended negative controls fail |
| S06-A12 | Extended producer with all S05 edit controls | No regression and no alternate duplicate engine |
| S06-A13 | Registered 26 route native/query/API cases on three hosts | Actual compatible cohort results with scoped unsupported reporting |
| S06-A14 | Tool-free offline API and dependency audit | No new runtime/FFI requirement for offline consumers |
| S06-A15 | Versioned fact query pack with expected-capture fixtures | Captures match expectations under the comparator; pack identity recorded; reproduces the S05 declaration structure facts exactly, including large-input summary fixtures through `native-query-large` |
| S06-A16 | Dynamic SQL location facts (construct, argument kind, original byte/point range, variable name; `AS USER` and pass-through excluded; C# heuristic labelled) | Matches the S05 fixtures; known misses reported, never silently absent |
| S06-A17 | Fact pack on large inputs through `native-query-large` (32 MiB input, 25000000 nodes, 90 s, 16 MiB output, 1000000 captures) | Inputs within the limits complete; one input over a limit ends typed `RESOURCE_LIMIT`, no silent truncation |

### S07

| ID | Concrete case | Required outcome |
|---|---|---|
| S07-A01 | Complete supported evidence set with independent expected inventory | Verified references/raw/assessment, with exact evidence mode |
| S07-A02 | Missing/extra/duplicate/unused record or bundle member | Reject incomplete or ambiguous consumption |
| S07-A03 | Change source/tool/input/query/policy/comparator/protocol/run/attempt | Stale or mixed result rejected; no summary-only hash bypass |
| S07-A04 | Unknown reducer/schema or historical gate not recomputed | Explicit unsupported/recorded state, never replayed success |
| S07-A05 | Cycle, traversal, alias/redirect, oversized/nested archive | Bounded shared guard rejection; no unsafe extraction |
| S07-A06 | Numeric boundary/signed-zero/threshold/winner controls for approved reducers | Correct exact or narrowly scoped policy; no global tolerance |
| S07-A07 | Historical FAIL and new policy/fix PASS | Both preserved and attributable; no relabeling as one run |
| S07-A08 | Authorized unchanged-dependency carry-forward and changed-dependency negative | Only the exact adopted relation succeeds |
| S07-A09 | Final qualification requires NEW_RUN but supplied only replayed/historical rows | Eligibility rejected even if integrity checks pass |
| S07-A10 | Data-only product replay with tool/target execution unavailable | Works for supported reducers; invokes no archive code |
| S07-A11 | Large raw stream/cancel/storage write failure | Bounded memory/output, truthful partial/failure status |
| S07-A12 | Mutants omit unused records, trust PASS label, default unknown to success or round numbers | Targeted controls detect each intended defect |
| S07-A13 | S01–S06 records and selected historical subset | No duplicate model or silent schema rewrite; differences documented |
| S07-A14 | Three OS common core/reducer lanes | Deterministic semantic result under same reducer, host observations retained |
| S07-A15 | Private local corpus workload registration, including S05 local raw recorded earlier | Local-run identity (candidate commit, clean tree, host facts, tool identities) defined; private paths/names/content only in untracked local records; public evidence carries counts and judgements only; local raw replays |
| S07-A16 | Gitignored PREPARE native evidence for the `native_evidence` run IDs in `language-sources.json` `adoption`, and SQL Server runtime `tsql-R01` | Replay connected for each listed run whose raw is present; absent raw is `RECORDED_NOT_RECOMPUTED`; `tsql-R01` stays NOT_RUN |

### S08

| ID | Concrete case | Required outcome |
|---|---|---|
| S08-A01 | Expected route×OS inventory | Exactly 78 distinct mandatory summary cells plus separately named extra-role rows |
| S08-A02 | Each cell's registered feature/fixture/check inventory | No endpoint-smoke substitution or unrun mandatory feature marked pass |
| S08-A03 | Current-candidate native execution on all 3 hosts | Real source/tool/input/profile/run-bound evidence, not cross-build alone |
| S08-A04 | Delete one OS row/artifact or supply duplicate row | Aggregation fails completeness |
| S08-A05 | Mix source/attempt/workload/query/comparator or wrong architecture | Reject cohort incompatibility |
| S08-A06 | Plant a real tree/capture/range/ERROR difference | Detected, not normalized away |
| S08-A07 | Change only allowed host observations | Semantic comparison unaffected; original host facts retained |
| S08-A08 | Historical known defect detected, required mainstream defect detected | First can pass its registered detector test; second still blocks support target |
| S08-A09 | Supply replay/old-SHA success instead of required new native execution | Eligibility rejected |
| S08-A10 | Public Go module and tool-free offline CLI workflows | Same semantics/guards; no runtime dependency on private paths/SDKs |
| S08-A11 | Controlled cancellation/output/storage/cleanup failure | Correct unsuccessful/partial status and safe preserved checkpoint |
| S08-A12 | Mutants drop failed route, manufacture missing cell, collapse SQL dialects or relabel history | Targeted aggregation controls fail |
| S08-A13 | Full current core/API and S04–S07 affected regressions | Actual three-OS gates pass at the verified candidate |
| S08-A14 | Final review, actual merge and post-merge/tracking | Exact identities, no unresolved material findings or premature closure |
| S08-A15 | User-facing support/release language | Only registered scope claimed; publication and Go adoption explicitly not performed |
| S08-A16 | Full private corpus on local Windows via `private-corpus-local` (wall 2 h, local storage 2 GiB) with the corpus route table | Every routed file counted by `execution_status` COMPLETED (has_error split), CANCELLED, RESOURCE_LIMIT or FAILED, or `assessment=BLOCKED` (encoding/policy, not run); unrouted and `PRESENCE_ONLY` files counted separately; kit defects 0; every ERROR file (COMPLETED with has_error) dispositioned as grammar gap, source damage or unsupported, or listed as `UNDISPOSITIONED` for the user; every CANCELLED, RESOURCE_LIMIT or FAILED file classified as limit, environment or kit defect, with unclassified FAILED counted as a kit defect; separate row from the 78 cells; no ratio threshold |
| S08-A17 | NET461 workload across three hosts | Hosted hosts use OWNED_FIXTURE only (synthetic large and deep-nesting fixtures); the private corpus never leaves the local host; mandatory mainstream grammar gaps block the support claim |
