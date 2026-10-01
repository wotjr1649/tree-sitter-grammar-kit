# PREPARE-06 C# r5와 로컬 보존 한도의 좁은 추가 승인

2026-10-02 실제 사용자 답변 `두 exact 효과 승인`은 human 11,379 bytes /
`cced4a06c71eacd8c28e855ddfd20c92d7c82bb2cfe7bc8d29313302dd857390`과
machine 50,057 bytes / `5d66340b2e0559bedc827e9fc3fc48f1d4af549e37b1cd2d6af5c9a089b488a7`에 결속한다.
[r5 projection](../../src/dev/prepare-p05/remedy-csharp-r5.json)은 LF를 CRLF로 복원하여
승인 원문의 terminal CRLF까지 size/SHA를 확인한다. 아래 R2 원문과 이전 6 GiB 승인·소비·실패는 역사 기록으로 유지한다.

`csharp-r5` / `csharp-candidate-r5`는 같은 `9150f7d56bb47f1a809fa23623f1ba1413e93fa9`의
r4 `grammar.js` 62,851 bytes / `b6946fac28ee1e470aa3155f5a3b8deddfd6cd0c6c47ac52c587e3e69a683e5e`를
별도 r5 copy에 복사한다. `method_declaration.returns`와 `object_creation_expression.type`의
기존 `prec(2, alias('var', $.identifier))`를 각각 한 번 `prec.dynamic(2, prec(2, alias('var', $.identifier)))`로 감싼다.
결과는 62,885 bytes / `da01e654f8cf2fc2dd5665615a54c726eaefb4f445e5fd7b90f002407c64ebe7`이어야 한다.
r4 copy의 scanner(r1에서 승인된 bytes 유지) 23,949 bytes / `164f51f2c55c08244791cabe5621b57a51d33e3813462ab39ebf67cad4b79227`,
도구·runtime·npm·image·입력·기대·비교기를 유지한다. 이 수정은 시험할 가설이며 지원 성공이나 최종 provider 채택이 아니다.

G1/B1/X27/edit5와 기존 owned G1/B1/X1·isolation8을 별도 fresh dispatch에서 수행한다.
r4와 같은 27개 입력/5개 edit의 원문 size/SHA·feature·positive/negative·구조 기대를 전수 대조한다.
`pinned-tsql-r1`, `trixie-r1` 및 위 r5 human subject가 필요하며 옛 R2 subject로 r5를 실행할 수 없다.
r1~r5의 변경 없는 `src/parser.c` 다섯 identity는 lossless source-copy r3 schema로 보관한다.
기존 r4의 정확한 네 identity/r2 schema는 유지한다. source object는 고유 SHA별 한 개로 보관하고
모든 원래 path/copy proof와 decoded·compressed identity를 확인한다. 원 capture와 runner 원본은 보존한다.
C# 펼침192 MiB·outer/inner64 MiB·회수/collector120초·일반 파일64 MiB는 유지한다.

새 승인은 kit 준비 증거의 누적 로컬 보존 상한만 앞으로 8 GiB (`8589934592` bytes)로 높인다.
이 상한은 로컬 kit의 `.work`·`artifacts`·`docs/plans`·`docs/prompts` 실제 파일 합계에 적용한다.
로컬 finite-batch/dispatch guard와 회수 helper가 승인 subject를 대조하고 실행 전 공간을 예약한다.
hosted workflow/collector의 runner 한도와 별개이며, 해당 로컬 guard의 검증·독립 리뷰 전에는 native dispatch하지 않는다.
회수 전에 실제 보존량 + outer 실제 크기 + inner 최대64 MiB + stage expanded 한도 + 기록64 MiB를 예약하고
C# r5는 승인 machine의 추가64 MiB 여유를 포함한 469,762,048 bytes를 실행 전에 예약한다.
MSSQL의 기존 예약은 402,653,184 bytes다. manifest 확인 후 실제 보존량을 다시 대조한다.
runner8 GiB, PG generation-only6 GiB, 다른 native4 GiB 및 모든 개별 실행·격리 한도는 그대로다.
과거 한도 초과·소비량·UNKNOWN을 새 상한으로 소급 수정하지 않는다.

# PREPARE-06 후속 exact R2 실행 계약

2026-10-01 실제 사용자 답변 `R2 exact 효과·기대 정정 승인`은 human
22,330 bytes / `a68d0717a75b6b769f3ea44eef4591c49bfedfc14f578abfc65657c43bffd1f2`,
machine 235,604 bytes / `d6781d9736d1871488563ac458d22f97e78a0311194d417370e24a0fa01763d4`,
신규 원문 subject 16,877 bytes / `9fc51da6cd0ad60d6e28cefed4f37b2a3c5f651bccd9eacca61ef3494d41e603`에 결속한다.
승인 기록은 `20261001-prepare-06/followup-exact-authorization-accepted-r2.json`이다.
tracked `remedy-followup-r2.json`의 LF projection은 승인된 machine의 CRLF 원문과 terminal LF를 복원해 전체 size/SHA를 대조한다.

## C# 후보 원본의 손실 없는 보관

`csharp-r4` collector는 r1~r4 후보에 복사된 변경 없는 `src/parser.c`를 SHA-256별 `source-objects/<sha256>.c.gz` 한 개로 보관한다. `records/lossless-csharp-sources.json`의 r2 schema는 정확한 네 원래 path·uncompressed bytes/SHA·candidate copy proof와 compressed bytes/SHA를 연결한다. runner의 원본 파일을 삭제하거나 변경하지 않으며, 원래 identity는 압축을 푼 bytes다. consumer는 copy proof·64 MiB 파일 한도를 확인하고, decoder와 별도로 압축 stream 전체 bytes를 읽어 SHA를 검증한다. 이 대조는 덧붙인 bytes/member의 identity 변경도 거부한다. decoder 자체가 임의 gzip의 single-member EOF를 증명한다는 주장은 하지 않는다. decode 전후와 최종 manifest의 compressed identity를 각각 결속한다.

펼침 budget은 ZIP manifest의 물리적 bytes + 고유 source object의 decoded bytes + metadata reserve8 MiB로 계산한다. 같은 SHA의 네 원래 경로는 논리 identity로 기록하며 중복 materialize하지 않는다. C#192 MiB, outer/inner64 MiB, 총 회수120초, local6 GiB 및 collector120초 한도는 유지한다. 원 capture/raw·case·expectation·scanner·grammar·generated-C export의 identity도 유지한다. 자체 검사는 네 path 각각의 누락·추가 path·압축 identity/manifest drift·tail/member 변경·실제64 MiB×4의 고유 decoded256 MiB 거부·NUL/CRLF·collision·deadline을 포함한다. upstream 실행 전 실패로 후보 bytes가 없다면 NOT_APPLICABLE, 일부만 있으면 NOT_VERIFIED로 보존하며 지원 PASS로 승격하지 않는다.

## 후속 stage 실행 범위

| stage | producer | G/B/X/edit | source/image receive | native memory |
|---|---|---|---|---|
| `csharp-r4` | `csharp-candidate-r4` | 1/1/27/5 | 1 GiB | 4 GiB |
| `pg-legacy-g6-r1` | `postgresql-legacy-candidate-r1-g6` | 1/1/12/1 | 1.5 GiB | 이 producer의 generation만 6 GiB; B/X 및 owned/diagnostic은 4 GiB |
| `mssql-patch-r1` | `mssql-candidate-r1` | 1/1/41/1 | 1 GiB | 4 GiB |

C#은 r1→r2→r3를 보존하며 r4 conflict 한 행만 별도 copy에 추가한다. PG는 같은 legacy patch를 재구성하고 유효한 baseline4를 재실행하지 않는다. MSSQL은 원본 C/JSON 두 producer를 보존하며 별도 네 파일 patch와 24 ESM module/42 literal import의 G1만 수행한다. source/dependency/scanner/tool identity는 G/B/X 전에 각각 확인한다. module metadata와 고정 입력만 읽으며 package/lifecycle/install/application 실행은 없다. 기존 26 route·256행·A/B 및 exact3 입력/기대/receipt는 유지한다.

별도 `P05-MSSQL-BRACKET-COUNTEREXAMPLE-r1`은 기존20byte `e463eaca…`를 단일 delimited identifier byte[7,18)로 요구한다. `P05-MSSQL-BRACKET-UNTERMINATED-r1`은 실제 닫는 `]`를 제거한19byte `65434d11…`의 오류를 요구한다. 공식 identifier 규칙에 따른 이 좁은 기대 amendment를 채택했으며, 옛 negative 기대와 결과는 역사 증거로 보존한다. `SELECT  FROM t;`의 기존 negative/recovery 의무와 temporal/graph 구조 의무는 유지한다. 신규12개의 원문·source/member/hash·error window와 기존68개를 검사하고 stage별 frozen 입력만 materialize한다.

PG G 시작 전 host `MemAvailable`과 실제 cgroup2 membership/mount에서 확인한 모든 부모의 가용 한도가 8 GiB 이상이어야 한다. UNKNOWN/부족이면 G를 시작하지 않는다. 같은 생성 container의 Docker inspect와 `memory.max=6442450944`, before/after `memory.events`를 기록한다. 다른 operation으로 예외를 확대하거나 자동 fallback하지 않는다. 기존 전체 isolation8·owned G→B→회수→fresh-container X·capture·cleanup을 통과해야 upstream을 실행한다.

PG memory counter는 동일 container의 원문·command·hash와 전후 단조 증가를 대조한다. `oom`/`oom_kill`/`oom_group_kill` 증가면 원 exit/termination을 유지해 OOM으로 기록하고 B/X를 시작하지 않는다. `max` 증가만으로 OOM을 주장하지 않는다. 이전 patch와 현재 patch의 machine provenance도 구분하며 C# r3와 PG legacy는 이전 exact3 subject, C# r4와 MSSQL 네 파일은 새 R2 subject에 연결한다.

[Linux kernel의 memory interface 정의](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#memory-interface-files)에 따라 실제 cgroup2 hierarchy root에는 `memory.current`/`memory.max`가 없다. mount root `/`, root 경로 `/sys/fs/cgroup`, 가용 memory controller와 두 interface 부재가 모두 관측된 경우만 root limit를 NOT_APPLICABLE로 기록한다. 이는 usage0 또는 임의 unlimited 관측이 아니다. 다른 부모의 미확인·부족 값은 계속 거부하며 host 가용8 GiB 조건은 유지한다.

개별 G300/B120/X10초·CPU1/PIDs64/tmpfs2 GiB·input64 KiB/output8 MiB·network none/read-only/user65534/cap-drop/no-new-privileges는 유지한다. HTTP52/각120초/총600초, 일반 file64 MiB/PG exact LFS100 MiB, runner8 GiB, artifact256 MiB/7일, packed outer/inner64 MiB·회수120초·C#/MS 펼침192 MiB/PG256 MiB는 유지하며 local retained cap만6 GiB로 승인했다. 예약·poll·완료·실패·summary·collector는 같은 stage의 counter와 한도를 사용한다. 과거 UNKNOWN과 실패의 한도를 소급 수정하지 않는다.

collector는 workflow가 전달한 stage/subject를 summary 및 전체 frozen case/expectation/edit ledger와 비교하고, 시도 횟수의 상한·관측된 command/outcome·출력 hash·cleanup·acquisition 완료/실패 counter를 확인한다. G 실패로 B/X가 NOT_RUN인 근거는 그대로 보관하며 quota를 실제 수행 수로 표시하지 않는다. 결속 실패도 bounded 원본 artifact를 보존하되 성공한 생성물 export나 지원 PASS로 승격하지 않는다. collection의 export/hash/manifest/ZIP 전체에는 별도의 총120초 clock을 적용한다. 로컬 회수는 download 전에 현재 retained bytes와 outer 실제 크기·inner 최대64 MiB·stage별 최대 expanded·기록64 MiB를6 GiB 안에 예약하고, 실제 manifest를 읽은 뒤 다시 확인한다. 이는 hosted runner8 GiB를6 GiB로 변경하지 않는다.

R2 helper 검사와 실제 native 실행 결과는 각각의 immutable receipt로 판정한다. 이 계약 변경 시점에 R2 native와 최종 provider 채택은 **NOT_RUN/NOT_ADOPTED**다. 새 코드의 독립 리뷰·필수 CI·actual merge/postmerge 뒤 현재 main의 fresh dispatch에서 `pinned-tsql-r1`, `trixie-r1`, 위 human subject를 명시한다. S01/MASTER·업무 corpus/.svc 제품·S08 전체78셀은 시작하지 않는다.

# P05 source closure 실행 순서

소유: PREPARE [#20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20), S01 readiness 전. [고정 후보 등록부](../../src/contracts/language-sources.json), [feature disposition](language-feature-disposition.md), [위험별 case](source-feature-feasibility.md)를 함께 사용한다. 초기 T-SQL G1/B2/X56, C# G1/B2/X20, TS/TSX G2/B4/X16, Swift G1/B1/X3의95개 실행 결과와 당시 PG16개 미실행은 보존된 역사적 관측이다. 후속 original/r1/r2 및 실제 PG/SQL 결과는 아래 run별로 구분한다. 기존 26-route/256행 채택은 scope 근거이며 source 지원 근거가 아니다.

## 네 단계와 완료 근거

| 단계 | 입력과 완료 근거 | 입력 확인 시점 |
|---|---|---|
| A: artifact inspection | 선택 subdir의 grammar.json/node-types.json/parser.c, scanner/shared/query/license/metadata를 commit+file blob/size와 대조하고 SHA-256 기록 | 취득·정적 검사 전, S01 identity 전 |
| G: generation | JSON 경로 또는 JS의 직접·간접 imports와 helper/npm grammar, CLI/Node identity, 실제 명령/생성물/limits/result | 해당 PREPARE generation 또는 S04 reproduction 전 |
| B: build | parser.c/scanner 및 recursive quoted includes, 독립 runtime/header, compiler/linker/libc와 probe bytes, EXE hash/exit | 해당 PREPARE build 또는 S05 전 |
| X: execution | EXE/library closure, 원본 input/edit bytes, 독립 검토한 syntax·구조·negative 기대, 격리 preflight, 실제 출력/종료/cleanup | 각 PREPARE parse/edit 또는 S05/S06 case 전 |

A가 닫혀도 G/B/X가 닫혔다고 기록하지 않는다. 기존 generated parser와 동일 grammar source를 재생성한 parser는 별도 identity다. Swift의 없는 parser.c는 실제 generation 결과가 있어야 B로 진행한다. 파일 이름·README·몇 개 성공 예제로 전수 지원을 주장하지 않는다. 다음 표의 common 경로는 각 고정 repository 기준이다.

## 26 route의 입력 경로와 남은 확인

`root`는 registry의 `.` selector다. 각 행의 `src/`는 그 selector 아래다. A에는 해당 query/metadata/license를 더한다. B에는 각 source가 실제 include하는 헤더와 별도 runtime을 더한다. 표의 closure는 **사용 전 검토 대상**이며 미실행 연산의 PASS가 아니다.

| Route | A / G의 고정 입력 | B의 native 입력 | X와 남은 검증 |
|---|---|---|---|
| csharp | root grammar.js, src/grammar.json, node-types | parser.c, scanner.c, tree_sitter headers | P05-CS-ID/DIRECTIVE; 넓은 선언 gap 유지 |
| go | root grammar.js/json, node-types | parser.c, headers; scanner 없음 | P05 native NOT_RUN; go-V18/V27은 담당 단계 전 case 고정 |
| python | root grammar.js/json, node-types | parser.c, scanner.c, headers | indent/f-string/t-string state; 사용 전 scanner closure |
| javascript | root grammar.js/json, node-types | parser.c, scanner.c, headers | script/module/ASI/regexp case; JSX와 route별 기록 |
| jsx | JavaScript와 동일 source pin | 같은 parser/scanner, 별도 profile | JSX mode 전환·태그/문자열 case |
| typescript | typescript/grammar.js/json; common/define-grammar.js; npm JS0.23.1 | typescript/src/parser.c/scanner.c, common/scanner.h, headers | P05-TS-DEFER; 원본/재생성 대응과 dependency closure |
| tsx | tsx/grammar.js/json; 같은 common/npm pin | tsx/src/parser.c/scanner.c, common/scanner.h, headers | P05-TSX-AMBIGUITY/DEFER; TS producer로 대체 금지 |
| java | root grammar.js/json, node-types | parser.c, headers; scanner 없음 | legacy8/compact source/module import/constructor case |
| kotlin | root grammar.js/json, node-types | parser.c, scanner.c, headers | 2.4 context/field/@all; stable와 experimental 분리 |
| c | root grammar.js/json, node-types | parser.c, headers; scanner 없음 | legacy/C23 preprocessor/declarator source |
| cpp | root grammar.js/json; npm C0.24.1 | parser.c, scanner.c, headers | C dependency는 G용; B에 npm lifecycle 불필요 |
| rust | root grammar.js/json, node-types | parser.c, scanner.c, headers | token-tree/edition/let-chain case |
| swift | root grammar.js/json; baseline parser.c 부재 | 실제 재생성 parser/header + 고정 scanner.c의 B1 완료 | P05-SWIFT-GENERATION; legacy 수용과 inline/module selector 실패 구분 |
| dart | root grammar.js/json, node-types | parser.c, scanner.c, headers | tree_sitter submodule은 관련 binding/test 실행 전 필요성 판단 |
| php | php/grammar.js/json; common/define-grammar.js | php/src parser/scanner, common/scanner.h, headers | mixed PHP source; php_only 결과 자동 대체 금지 |
| ruby | root grammar.js/json, node-types | parser.c, scanner.c, headers | heredoc/state/newline/edit case |
| r | root grammar.js/json, node-types | parser.c, scanner.c, headers | pipe/lambda/placeholder precedence case |
| bash | root grammar.js/json, node-types | parser.c, scanner.c, headers | shell source만 parse; shell application 실행 없음 |
| powershell | root grammar.js/json, node-types | parser.c, scanner.c, headers | 5.1/7.6 source 합집합; command mode case |
| html | root grammar.js/json, node-types | parser.c, scanner.c, headers | raw-text/foreign/fragment recovery |
| css | root grammar.js/json, node-types | parser.c, scanner.c, headers | 채택된 module 계열별 syntax; 단일 smoke로 축소 금지 |
| json | root grammar.js/json, node-types | parser.c, headers; scanner 없음 | strict negative/edit; JSONC는 대체 근거 아님 |
| yaml | root grammar.js/json; 선택 root schema | root parser/scanner/headers | 다른 schema 및 test-suite submodule은 별도 입력/효과 |
| xml | xml/grammar.js/json; common/common.mjs | xml parser/scanner, common/scanner.h, headers | XML1.1/DTD 경계; 별도 dtd parser 자동 포함 금지 |
| tsql | root grammar.js/json; grammar/*.js 및 functions/*.js | parser.c, headers; scanner 없음 | P05-TSQL-CASE/STATEMENTS/GO와 identifier·필수 modern 절 |
| postgresql-sql | postgres/grammar.js/json, node-types | 원133byte pointer와 별도 승인·취득한 exact LFS object, scanner/headers | run36796853218 baseline8 실행, WITH OIDS 실패/구조7개 관측; noopt state overflow로 재생성8개 NOT_RUN, 과거 exit137 원인 미확정 |

TS/TSX의 고정 lockfile은 `tree-sitter-javascript@0.23.1`을, C++은 `tree-sitter-c@0.24.1`을 가리킨다. range나 현재 language-route pin으로 대체하지 않고 lock integrity 및 실제 package bytes를 확인한다. grammar 상속에 불필요한 binding install script는 실행하지 않는다. PostgreSQL의 grammar.json→C 생성과 upstream PostgreSQL grammar→Tree-sitter converter는 다른 closure다. 후자의 원본 PG pin·converter 도구·변환 script가 확인되지 않으면 그 재현은 미실행으로 남긴다.

## 고정 JS closure의 실제 import 분류

run36651074932는 C# G1/B2/X20 및 큰 archive 회수 후 TS closure 검사에서 중단됐다. `common/define-grammar.js` SHA-256 `c2ac6894e0db6164f56dc339788d9da2da60ce1f81d888e6b74a74fc81db818f`의 두 `'import'` grammar 토큰과 `// foo: import('x').y.z;` 설명 주석, npm JS0.23.1 `grammar.js` SHA-256 `230e330dd914d94297e5e58d53942cd5debf9c069852da93b6a6b1daae1967dc`의 세 `'import'` 토큰을 실행 import로 오인한 helper 실패다. 실제 추가 dependency나 TS 구문 실패 근거가 아니다.

원문 archive와 selected-file SHA를 대조하고 해당 두 파일의 정확한 SHA 및 여섯 match 위치/값만 비실행 관측으로 결속한다. source text를 삭제·정규화하거나 실행 bytes를 수정하지 않는다. 다른 digest/위치/추가 import, dynamic require·eval·별칭 loader·등록 밖 dependency와 root 탈출은 계속 거부한다. 자체 literal closure와 해당 반례를 실제 helper로 검사하고, 고정 원문 closure는 별도 로컬 정적 검사 및 새 hosted generation으로 확인한다. arbitrary JavaScript의 모든 동작을 증명한다는 주장은 유지하지 않는다.

run36657324824에서 수정한 TS/TSX closure와 G2/B4/X16을 실제 실행했다. 두 producer 모두 `import defer`는 실패했고 legacy/ambiguity 대조는 수용했다. Swift 고정 grammar SHA-256 `e798585e0b27886fce7fc540b3e246073bd6bedd2d7d18c63c5832b6a148db2b`의 `"import"` 토큰(char50974)도 같은 검사에서 module import로 오인됐다. 추가 dependency나 Swift syntax 실패가 아니다. 같은 exact SHA와 단일 occurrence만 shared 검사에 결속하고 원문을 보존한다. 빈/틀린 digest·위치·추가 occurrence 거부, 고정 원문 정적 closure와 실제 hosted generation은 서로 다른 검증이다.

수정 후 run36660558049에서 Swift G1/B1/X3를 실제 수행했다. 고정 source와 scanner를 유지한 regeneration producer에서 legacy generic function과 등록 edit는 통과했고 inline array/module selector는 ERROR였다. baseline parser 부재를 실패로 세지 않는다. 전체 격리8개·자체 G/B/X·회수/새 container 실행·container21개 종료/PID0/제거를 확인했다. 이 구문 실패는 앞선 helper 오류의 재발이 아니다.

## PostgreSQL artifact와 generation 차단

같은 run의 PG generation은 고정 JSON/CLI/ABI15/4GiB에서92.265초 후 exit137, stdout/stderr0bytes였다. kill 원인을 입증할 memory event가 없어 OOM이라고 확정하지 않는다. baseline `postgres/src/parser.c`는133bytes의 Git LFS pointer이며 compiler가 첫 `version https://git-lfs.github.com/spec/v1` 행을 거부했다. pointer의 SHA-256은 `9b714c169bc14fb13b23374f344fe1bd20b7db4a07546b2b671d644fe374bcc3`, 실제 object는 SHA-256 `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`/97,664,793bytes다.

selected pointer bytes의 정합성은 실제 generated C artifact 확보를 뜻하지 않는다. LFS object provider·취득과 기존64MiB보다 큰 단일 파일 한도는 별도 승인 대상이다. generation 실패와16개 미실행 producer/등록 PG edit를 보존하고 PG9.6/18 구문 지원을 실패나 PASS로 재분류하지 않는다. 같은 실패의 무변경 반복, memory cap 증가, source replacement는 하지 않는다.

## 승인 후 실행 순서와 멈춤 조건

1. 현재 main, effect 승인, 누적 ledger, exact file allowlist와 case pack을 대조한다. source 취득/추출은 허용한 새 목적지에만 수행하고 기존 checkout을 변경하지 않는다.
2. 26행의 A 입력을 검증한다. import/include closure가 목록 밖 입력을 요구하면 그 연산을 멈추고 누락 경로/bytes/검증 시점을 기록한다.
3. 실제 native 격리를 검증한다. env 제거만으로 host filesystem/network 격리를 입증하지 않는다. tool/OS/digest·경계와 회수 절차의 독립 리뷰 후 실행한다.
4. C#/TSQL/TS/TSX/PG 기존 artifact baseline을 보존한다. Swift 및 승인된 동일-source regeneration은 별도 producer로 build한다. source 대응이 불일치하면 원래 실패를 덮지 않는다.
5. 등록 P05 case family의 구체 원문, 구조 사실, negative/recovery/edit를 검토·고정하고 제한 실행한다. official syntax에서 기대를 얻으며 candidate tree를 golden으로 복사하지 않는다.
6. 각 결과를 upstream 선언/정적 관찰/재현 실패/검증된 remedy로 구별한다. 실제 gap에 대해 적합한 기존 후보를 비교한 뒤 exact replacement/patch/추가 효과 승인을 요청한다. 승인 없는 grammar patch 또는 required syntax 재분류는 하지 않는다.
7. 현재 P01~P14, 실제 PR/CI/merge/post-merge, tracking과 immutable checkpoint를 대조한다. 알려진 필수 gap이나 누락 권한/능력이 남으면 exact blocker를 기록하고 #20을 OPEN으로 유지한다.

source·도구·입력·출력·예산·cleanup은 각 실행 receipt에 결속한다. 미래 S05/S06의 제품 native producer나 S08의 78-cell qualification을 여기서 완료했다고 표시하지 않는다.

### 지속 승인과 유한 작업 배치

현재 사용자가 동일 PREPARE의 계속된 개발·검증·CI·고정 입력 실행을 명시 승인한 경우, 과거 총시간·job 횟수의 중단 조건은 그 승인 범위에서 새 배치로 갱신한다. 실제 메시지와 기존 effect subject를 새 승인 기록에 연결하고, 배치마다 목적·입력·작업 횟수·시간/CI/전송/저장 상한과 시작 소비를 먼저 기록한다. 과거 소비와 실패, 불확실 시간은 이월하며 사용자 idle·측정 시간·보수적 reserve·청구 금액을 구분한다. 동일 결정적 실패의 무변경 재실행, 무한/null 한도, 승인되지 않은 후보 교체·grammar patch·scope 축소는 허용하지 않는다. 개별 timeout·memory·output·격리·cleanup 제한은 유지하고 변경이 필요하면 측정 근거와 독립 리뷰를 먼저 확보한다. S01/MASTER 권한은 별도다.

### 별도 승인된 exact remedy 경로

후속 [remedy workflow](../../.github/workflows/prepare-p05-remedy.yml)는 명시 `pinned-tsql-r1`/`trixie-r1`과 별도 remedy subject를 요구한다. subject는 승인 제안의 identity이며 실제 사용자 acceptance·현재 main·유한 batch를 caller가 먼저 확인한다. [literal patch](../../src/dev/prepare-p05/remedy-patches.json), [추가 회귀](../../src/dev/prepare-p05/remedy-cases.json), [독립 source 사실](../../src/dev/prepare-p05/remedy-fact-oracles.json), [추가 공급 입력](../../src/dev/prepare-p05/remedy-sources.json)을 hash로 결속하며 원57입력/6edit와 실패를 유지한다. 제안 파일의 역사적 PROPOSED 상태를 실행 성공이나 채택으로 해석하지 않는다.

승인 원본 JSON은 그대로 보존하고 tracked delivery는 기존 UTF-8/LF 정책에 맞게 문서 바깥 whitespace만 LF로 기록한다. 별도 delivery SHA와 parsed 값 동일성을 대조하며 literal patch·grammar 결과 SHA·input UTF-8/CRLF bytes는 변경하지 않는다. 자체 supervisor 회귀는 필수 Go 도구로 만든 출력 전용 fixture의8MiB를 기존2초 안에 정확히 회수한다. compiler30초/CGO0/offline/task cache, fixture source/compiler/EXE hash와 cleanup을 기록하며 upstream grammar 실행과 구분한다.

supervisor는 ready stream의 완료 read를 즉시 drain하되 매 read의 time/network/output cap을 유지한다. 자체 output-limit와 timeout 반례는 partial 출력/종료/cleanup을 실제 확인한다. 각 fixture build1/execute3은 별도 소유한 검증 소비이며 원 upstream case 또는 제품 native qualification으로 세지 않는다.

`patch-r1`은 별도 scratch의 C#/TS/TSX/Swift 네 파일에 exact literal patch만 적용한다. 원본 producer에는 추가32입력만, 후보에는 원21+추가32입력을 계획한다(G5/B8/X85, producer edit16). `sql-pg-r1`은 Derek 고정30파일의 **평가만** 수행하고 TSQL28+negative2, PG exact LFS baseline8 및 옵션 변경 regeneration8을 계획한다(G2/B3/X46, edit3). SQL 채택/교체/patch와 다음 grammar patch는 자동 승인되지 않는다. PG object는97,664,793bytes/SHA-256과100MiB 단일 파일 예외를 대조하며 원 pointer를 보존한다. PG generation의 `--disable-optimizations` 및 같은 container의 `memory.events` 전후 관측은 기존 exit137의 무변경 반복과 구별한다.

기존 supervisor·회수·전체8개 isolation·자체 G/B/X·새 container의 EXE/ELF/hash 확인·cleanup과 개별 limits를 재사용한다. 각 stage의 더 작은 G/B/X 상한을 적용하고 quoted include/ABI를 build 전에 검사한다. 실패한 producer는 후속 NOT_RUN을 남기며 독립 producer는 안전한 한도 안에서 계속한다. 원 probe/comparator와 구문 기대는 변경하지 않는다. negative의 ERROR도 원 exit2로 보존하므로 job의 성공/실패 표시는 최종 지원 판정을 대신하지 않는다. 후보별 required syntax·구조·negative·legacy/recovery/edit 검증과 독립 리뷰를 통과하기 전 kit source 등록부에 채택하지 않는다.

## 실제 archive 취득 경계

고정 T-SQL archive `443d2bc774f1d779af7dcabcc99160fb24da96e6`에는 `bindings/c/tree-sitter-TSQL.h`와 `tree-sitter-tsql.h` 등 대소문자 충돌 세 쌍이 있다. 실제 두 번째 준비 run은 이를 거부했고 native를 실행하지 않았다. 거부한 archive의 추출과 충돌 검사는 유지한다.

별도 `pinned-tsql-r1` 취득 profile은 원 pin의 등록된 충돌 없는21개 regular file만 정확한 raw URL로 받아 Git blob/size와 대조하고 원본 bytes를 보존하는 제안이다. 이는 별도 source 취득 효과·HTTP 횟수·수동 job 승인이 있어야 실행한다. 기존 archive profile과 source/case/grammar 기대는 바꾸지 않는다. 고정 grammar의 이름은 `TSQL`이므로 C entry-point 식별자는 대소문자를 보존한다. archive 거부는 구문 미지원 재현이나 후보 교체 근거가 아니다.

workflow와 직접 취득 진입점은 `approval.ps1`의 profile별 subject 검사를 거친다. 새 profile은 기존 B reference만으로 실행할 수 없다. 별도 제안의 immutable hash는21개 URL·input SHA·HTTP52/누적55·추가 job1/누적3·누적 전송/CI 예산을 결속한다. hash 값은 승인 증거가 아니며 실행 담당자가 실제 사용자 acceptance와 exact main/dispatch receipt를 함께 확인한다. 빈 값·기존 B 값·다른 profile 조합의 거부는 기존 foundation self-check에서 검사한다.

## tmpfs 회수 capability

run36592527846은 별도 승인을 받은 raw 취득으로 등록474파일과 CLI/image를 확인했으나 owned preflight4회 후 중단했다. `/work/proof` 생성과 `docker cp`는 exit0이었지만 회수 tar에는 빈 `./`만 있었다. generation/build/parse는0회이며 네 container cleanup은 확인했다. [Docker 공식 문서](https://docs.docker.com/reference/cli/docker/container/cp/#corner-cases)는 tmpfs를 `docker cp`로 회수할 수 없다고 명시한다.

`quiescent-tar-r1`은 별도 실행 승인 대상이다. pause 상태에서 inspect의 host PID와 `docker top -eo pid,args`의 유일한 `/bin/sleep infinity` PID를 대조한다. 실제 baseline의 두 PID는3253으로 같았다. 불일치/다른 process가 있으면 거부한다. 그 뒤 unpause하여 동일 immutable image의 tar만 실행하고 다시 pause·동일 PID 단독 상태를 확인한다. 원 native를 재실행하지 않으며 network/source/root/cgroup/tmpfs 한도는 유지한다. 첫 tar 사용 전 version/hash를 diagnostic에 기록하고 기존 raw byte 상한과 regular-file/path/link/collision/size 검사를 적용한다. 실제 tmpfs 회수와 전 격리 preflight를 통과해야 upstream generation을 시작할 수 있다. 이 수정의 정적 리뷰와 foundation CI는 해당 native capability의 성공을 대신하지 않는다.

preflight/diagnostic의 stdout+stderr 합산 상한은1 MiB로 supervisor에 전달하며 결과 archive 상한과 각각 적용한다. process-limit probe의 JavaScript 원문은 task 변수에 보관하고 PowerShell의 read-only 자동 변수 `PID`에 대입하지 않는다. 이 두 경로는 이전 run에서 아직 도달하지 않은 실행 경계로, 실제 hosted 성공을 미리 주장하지 않는다.

회수 canary는 `preserved`와 NUL·CRLF의12 bytes를 `/work`에 쓰고 원 container를 종료하기 전에 회수한다. host의 안전한 tar 추출 뒤 원문 bytes/hash를 확인하며, 정적 self-check는 NUL 삭제·CRLF 변경·같은 길이의 내용 변경을 거부한다. build의 `probe`는 archive mode0755만 허용하고 추출 파일에 같은 mode를 적용·검증한다. 각 등록 case의 새로운 container에서 read-only 입력의 mode와 SHA를 다시 대조한 뒤 실행하며, 불일치는 exit74로 보존한다. canary나 mode 확인만으로 grammar 지원 또는 실제 다음 container 실행 성공을 주장하지 않는다.

회수의 frozen-state 조회와 cleanup의 state 조회는 서로 다른 immutable 명령 이름을 사용한다. lifecycle 회귀 검사는 실제 `Freeze`/`StopContainer`/`Record` 함수에 owned Docker 응답을 공급해 이름 충돌을 검출하며, 실제 Docker 회수·격리·cleanup 성공은 hosted 결과로 별도 확인한다.

## Toolchain loader 경계와 별도 image subject

run36606981293/attempt1은 PR #29의 actual main3099b294e8b8bf960c21a0a25c454d2647082410에서 등록474파일, canary12bytes/NUL/CRLF 회수, owned isolation8개와 container9개의 종료·PID0·제거를 확인했다. 이후 고정 CLI0.27.0은 기존 bookworm image의 libc에서 `GLIBC_2.39 not found`로 실패했다. 따라서 toolchain preflight는 미완료이며 upstream G/B/X0, 원문57개와 edit6개는 NOT_RUN이다. 이 loader 실패를 grammar 지원 실패로 분류하지 않는다. timeout 반례의 실제 출력은0bytes였지만 direct 호출의 output 상한이 기본8MiB였음도 확인했다. 이 호출을 계약의1MiB로 명시 교정하며 이전 receipt는 보존한다.

`bookworm-r1`은 원래 `inputs.json` image identity를 보존한다. 제안된 `trixie-r1`은 같은 Node24.21.0의 linux/amd64 image `node@sha256:98ad2493de85738f55c11fe22e8586caf1fd917b7a8075c57ab9c55116e06492`와 compressed440,298,459bytes를 별도 subject에 결속한다. **새 image의 실제 사용자 승인 전 pull·native 실행은 불가**하다. helper 통합이나 subject 문자열 존재는 승인이 아니다. source/case 원본은 변경하지 않으며 기존 1GiB 취득 상한에서 선택 image 크기를 미리 예약한다. generation/build/header/capture/probe 기록은 선택한 실제 image와 tool bytes를 가리킨다.

승인 후 새 image에서 전체 isolation/capture와 CLI/tool identity를 다시 검증해야 한다. 현재 metadata와 공식 libc 계열만으로 loader 호환성이나 grammar 결과를 PASS로 표시하지 않는다. self-check는 기존 image 동일성, 새 image의 정확한 binding, 빈/이전/다른 profile subject 거부를 검사하며 image를 pull하거나 실행하지 않는다.

## GLIBC 진단과 자체 수직 control

호환성 진단은 전체 source 취득 뒤 선택 image의 실제 digest/platform/storage와 container 설정을 확인한 상태에서 수행한다. 이 helper는 CLI만 먼저 취득하는 경로가 없어 일괄 취득을 유지한다. `tool-environment`는 libc/Node/GCC/linker identity와 CLI ELF header·interpreter·NEEDED·required versions 및 libc/libm/libgcc/loader provider versions·SHA를 CLI 실행과 별도로 보존한다. 이어 `cli-version`과 `cli-generate-help`를 각각 제한 실행한다. 이전 loader 실패가 환경 진단의 출력을 가리지 않도록 한다. 실제 회수·격리8개를 새 image에서 반복한 뒤 upstream generation을 시작한다.

등록57원문과 별도로 소유한 작은 scannerless grammar와 `OWNED-P05-VERTICAL` 입력을 task scratch에 고정한다. 같은 CLI의 ABI15 generation1회, 독립 runtime/GCC build1회, 회수한0755/SHA executable의 새 container parse/edit1회로 수직 경로를 확인한다. 구조 기대는 declaration/name/body identifier의 byte ranges와 brace 손상 오류·복구/incremental/fresh 동일성이다. 이는 product S04/S05/S06 구현이나 upstream57개 지원 근거가 아니다. 자체 G/B/X1회씩은 별도 counter에, 회수는 기존169회 shared cap 안에 기록한다. 나머지 generation6/build11/execution128/diagnostic16 및 개별 timeout/memory/output/transfer/storage 한도는 유지한다.

각 build는 같은 image의 readelf로 실제 ELF/loader/NEEDED/version 원문을 회수한다. 허용한 x86-64 ELF64·image loader·libc closure와 executable mode/hash를 확인한 후 새 container에서 실행한다. 모든57개 input의 baseline/재생성 variant는 final case ledger에 남으며 미실행은 dependency blocker를 명시한다. syntax/구조/negative/recovery/edit의 최종 지원 판정은 해당 원문·tree·feature 사실의 실제 평가가 필요하다.

upstream `grammar.json`의 rule key는 대소문자를 구별한다. grammar entry-point 이름은 case-sensitive hashtable로 읽고 기존 C identifier 제한을 적용한다. run36644794802는 새 image의 CLI/libc·격리8개·자체 G/B/X control을 통과한 뒤 T-SQL의 `AS`/`as`를 기본 PowerShell JSON object로 읽다가 중단했다. upstream G/B/X0이며 grammar 구문 실패가 아니다. 원본 source를 고치거나 key를 정규화하지 않고 reader를 교정하고 case-distinct key와 잘못된 entry-point 이름을 회귀 검사한다.

원격 command supervisor는 준비된 stdout/stderr를 drain하고 다음 read task 중 하나가 완료될 때까지 최대20ms만 기다린다. 출력 준비와 무관하게 매65,536bytes read마다20ms를 기다리던 이전 동작은 run36646415814에서 C# generation exit0 뒤 회수10초 동안32,305,152bytes만 읽고 timeout됐다. partial tar·명령·cleanup77개와 T-SQL의 이미 실행된56개 producer 결과를 보존한다. 회수10초·archive/output·storage 한도를 늘리지 않으며 큰 회수의 완료는 새 native 관측으로 확인한다.

동일 T-SQL source의 이미 완료한56개 producer 관측은 `tsql_prior_evidence_subject`의 고정 immutable receipt에 결속해 재실행하지 않을 수 있다. caller는 실제 run36646415814·원57input projection·같은 CLI/runtime/Trixie·28원문/2producer edit·46syntax 실패와 원출력을 먼저 검증한다. subject는 실행 허가나 지원 PASS가 아니다. 원본 전체57case ledger를 유지하며 현재 통합 helper의 prior 상태는 `NOT_REEXECUTED_PRIOR_OBSERVED_RESULTS_RETAINED`다. 앞선 run36651074932의 이전 `NOT_REEXECUTED_PRIOR_OBSERVED_FAILURES_RETAINED` 원 ledger도 보존한다. T-SQL만 이월한 그 run은 나머지5route/55producer를 계획했으며 C#20개 완료 후 중단됐다. 과거 결과와 새 결과를 함께 평가하고 required scope 실패를 유지한다. source/tool/image/input이 달라지면 이 이월은 적용할 수 없다.

run36651074932에서 C# G1/B2/X20 및 두 producer의 edit를 완료했다. 생성 archive32,522,240bytes는 수정한 supervisor가0.148초에 정확히 회수했고 container40개의 종료·PID0·제거를 확인했다. 등록된 다섯 `#:` directive는 각각 두 producer에서 실패했으며 좁은 identifier/shebang/raw string 성공은 넓은 declared gap을 해소하지 않는다.

후속 실행은 `csharp_prior_evidence_subject`의 고정 receipt에도 같은 source/input/CLI/runtime/image 검증을 적용한다. TSQL56행/2edit와 C#20행/2edit를 원 관측으로 보존하고 TS/TSX/PG/Swift의 나머지35producer/7edit를 새로 실행한다. 전체111행의 prior 상태는 `NOT_REEXECUTED_PRIOR_OBSERVED_RESULTS_RETAINED`, 현재 raw/exit는 null이며 어느 prior subject도 scope PASS나 권한이 아니다. 기본 빈 subject는 원래 경로를 유지하고 잘못된 subject/legacy image는 효과 전에 거부한다.

run36657324824의 결과는 `ts_pg_stage_evidence_subject`로 별도 immutable 관측에 결속한다. TS/TSX16행/4edit는 실행된 관측으로, PG16행/2개 producer edit는 `NOT_REEXECUTED_PRIOR_STAGE_BLOCKER_RETAINED`로 이월한다. PG syntax는 NOT_RUN이다. caller는 해당32행·원 input projection·같은 source/CLI/runtime/Trixie·원출력과 단계 차단을 먼저 대조한다. C#/TSQL까지 포함하면 prior108행 중92행이 실행됐고16행은 미실행이다. 남은 Swift3행/1edit와 전체111행/57원문/6edit 등록을 유지한다. 새 run의 성공 exit도 prior 필수 실패나 PG blocker를 해소하지 않는다.

run36660558049의 Swift 관측까지 합친 당시 집계는49개 원문/95개 producer와5개 원 edit/9개 producer edit 실행, PG8개 원문/16개 producer와1개 원 edit/2개 producer edit 미실행이었다.95행 중62행에 원본 ERROR/MISSING,33행에 원본 오류 부재를 관측했다. 당시 T-SQL lowercase configuration 분류 차이와 broad C# 선언 gap도 보존한다. 이 역사적 집계는 P05 scope PASS나 S08의78-cell qualification 완료가 아니다. 후속 run36796853218에서 PG baseline8개를 실제 실행했으며 WITH OIDS 실패와 나머지7개 fact 관측, noopt 생성 실패로 인한 재생성8개 NOT_RUN은 아래 해당 run 결과를 따른다.

remedy 실행에서 확인한 producer별 import/include·ABI·entry-point·ELF closure 실패는 해당 producer의 전체 dependent row를 구체적 이유와 함께 `NOT_RUN`으로 남기고 독립 producer를 계속한다. generation/build의 exit 및 resource termination도 별도로 연결한다. 개별 native 한도 실패 뒤에는 원 partial output을 보존하고 실제 container cleanup을 검증한 뒤에만 진행한다. 승인·bytes identity·공유 한도·격리·capture·cleanup 오류와 예상하지 않은 예외는 전체 실행을 중단한다. 자체 input closure 반례는 한 producer의 실패가 다른 row를 바꾸지 않는지, 공유 안전 실패가 전파되는지 검사한다.

승인 oracle의 mutable primary URL은 역사적 관측으로 보존한다. 같은 사실의 재검토는 campaign cutoff 이전의 [C# source revision](https://github.com/dotnet/docs/blob/489ab432482b3e4d50ab952d3a41cfc9b9a245c8/docs/csharp/language-reference/preprocessor-directives.md)(21,832bytes, SHA-256 `131523b253a9ac3a60577e27ad1ba480f247b7102dbd6884008a2878ff2a9f23`)과 [Swift SE-0452 revision](https://github.com/swiftlang/swift-evolution/blob/7f16311d11a01b95fd48b6e33d22f03dfcb2de9c/proposals/0452-integer-generic-parameters.md)(24,688bytes, SHA-256 `e337038a95fdc85e270667908079563ebf729f481822ec699f3ba9df76c7fdac`)에 결속한다. 공개 원 bytes와 Git blob/SHA를 별도 receipt에 보존한다. C# directive의 행 경계·compiler ignore와 SDK 검증 제외, Swift6.2의 signed integer generic argument와 semantic binding 분리는 기존 기대를 유지하며 native 지원을 입증하지 않는다.

### Docker daemon 사전 조회 순서

`docker version`과 `docker info --format '{{json .SecurityOptions}}'`를 source/image 취득 전에 각각10초/1MiB로 조회한다. 실제 builtin seccomp 응답이 없으면 취득·container·G/B/X를 시작하지 않는다. command-level receipt와 raw stdout/stderr를 보존하고 timeout을 grammar 실패와 분리한다. Docker 내부 원인과 client/plugin/server 단계는 별도 근거가 없으면 `NOT_VERIFIED`다. 조기 host 조회의 시간과 관측 traffic도 기존 combined acquisition600초/1GiB accounting에 보수적으로 포함한다. 이 순서 보완은 성공 응답을 보장하지 않는다. 응답을 바꾸는 fallback·자동 retry·시간 확대 없이 기존 전체 isolation/capture/cleanup 검사와 native 한도를 유지한다.

### 실제 remedy 실행과 취득 실패의 분리

[run36741763343](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36741763343)은 main `ee92eae8cd1f7a16f5e00834023da1d44c991536`의 `patch-r1`에서 upstream G5/B8/X85, producer edit16, 자체 G/B/X 각1, isolation8, capture120을 실행했다. exact 생성·빌드 결과를 새 container에서 실행했고121개 container의 종료/PID0/제거와2054개 host command의 root 종료를 확인했다. 조기 Docker security 조회는 성공했으며 앞선 timeout의 원인이 확정됐다는 뜻은 아니다.

r1 후보는 C# directive9개와 제한 Swift14개 source facts를 수용했지만 C# `F(await)` 호출 구조 및 named type `var` 분류가 충족되지 않았고 TS/TSX default binding `defer`2개가 ERROR였다. 모든 원본/r1 raw와 original57/6·추가34개 expectation을 보존한다.16개 edit는 damaged/restored incremental=fresh와 원 tree 복구가 일치했지만 broad ERROR의 손상 범위를 좁은 복구로 과장하거나 damaged 후속 AST가 항상 유지됐다고 주장하지 않는다. 이 관측만으로 후보 채택·whole feature support·제품 qualification을 표시하지 않는다.

[run36745186550](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36745186550)은 같은 main의 `sql-pg-r1`에서 SQL 고정30파일과 PG LFS97,664,793bytes/SHA `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`를 취득·검증했다. HTTP51회/source124,365,296bytes 후 image pull이 기존 combined1GiB guard의 `DOWNLOAD_LIMIT`로 중단됐다. upstream·자체 G/B/X, isolation, capture는 모두0이며 SQL/PG46행/3producer edit는 NOT_RUN이다. native container0과 host command4개의 root 종료를 확인했다. partial Docker pull의 daemon quiescence까지 검증됐다고 표시하지 않는다.

이 revision은 실패 final 수신 counter를 기록하지 않아 초과량은 미관측이다. 알려진 값은 guard threshold1,073,741,824bytes 초과이며 실제 전송 upper/partial image payload/청구량으로 환산하지 않는다. 새 기록 helper는 acquisition 중 각 command의 host root 종료 뒤 `acquisition_network` snapshot과 최종 `download-budget-incomplete`를 남긴다. counter가 unavailable/시작값보다 감소하면 수치 PASS를 만들지 않고 `NOT_VERIFIED`로 보존하며 정상 command도 실패로 처리한다. 취득 완료도 같은 단일 snapshot이 OBSERVED/비음수/1GiB 이내이고 record를 남긴 뒤에만 acquiring을 해제한다. 기존 TIMEOUT/DOWNLOAD_LIMIT/UNKNOWN_CLEANUP 사유를 counter 관측 실패로 덮지 않는다. 이 교정은 이전 미관측 counter를 복원하거나 cap을 늘리지 않는다. 자체 controlled-counter 중단·unavailable·regression과 완료 전환 반례는 기록 경로의 검증이며 실제 host traffic/daemon 종료 증거와 구분한다.

이전 checkpoint에서 후속 C#/TS/TSX exact 추가 patch와 SQLPG-only1.5GiB cap은 미승인 결정으로 남았다. 2026-10-01 실제 사용자 통합 지시가 human12553bytes/SHA `a72b87c3dfe6561855749b64cce03bdaa5d7c231948f42dfa6ef41ce84a7747e`와 machine31295bytes/SHA `a216d31a0242ac161291e3f00cacf721d602dcdf7f76d8e86ef5a8bf161f5ba2`의 A/B를 명시 승인했다. 현재 일반1GiB guard·52HTTP/600초·image1회·개별 native/격리/회수/저장 한도와 SQL 평가만의 범위를 유지한다. 승인된 r1 owned-fixture/취득 효과나 같은 범위의 지속 개발/CI 승인을 새 grammar patch·개별cap 확대·SQL 교체로 해석하지 않는다. 같은 결정적 실패를 무변경 dispatch하지 않는다.

## A/B r2 실행 계약

`patch-r2`는 exact r1→r2 C#/TS/TSX 별도 후보만 G3/B3/X39·7producer edit로 실행하며 original/r1/Swift를 무변경 재실행하지 않는다. r1 scanner·고정 npm/runtime/CLI·input/expectation을 유지하고 r2 source의 literal import occurrence도 exact SHA/offset으로 검증한다. `sql-pg-r2`는 동일 SQL30행과 PG baseline/noopt16행의 G2/B3/X46·3edit만 실행한다. 두 stage는 새 reviewed/merged main dispatch의 `pinned-tsql-r1`·`trixie-r1`·정확한 새 subject에 결속한다.

source/image receive-counter는 `approval.ps1`이 효과 전에 선택한다. 새 `sql-pg-r2`만1,610,612,736bytes이고 다른 stage/일반경로는1,073,741,824bytes다. source+image 예약·polling·완료 snapshot·실패 snapshot·요약이 동일 stage bound를 쓰며 unknown/regressed/negative/over-cap 거부를 유지한다. HTTP52/600초·일반파일64MiB·PG exact100MiB 예외와 기존 native memory/시간/output/isolation/capture/cleanup 한도는 변하지 않는다. 과거 실패의 미기록 counter·UNKNOWN에 새 cap을 소급 적용하지 않는다.

실제 G/B/X와 등록 syntax·구조·negative·recovery·edit를 별도 판정한다. job 성공·ERROR 부재·같은 tree 복구만으로 요구 지원을 주장하지 않으며 candidate 시험은 최종 provider 채택이 아니다. 새 [NET461 업무 계약](net461-workload.md)은 별도 case/owner/effect 경로이며 A/B 입력 집합에 포함하지 않는다.

## A/B 관측과 B 미실행 SQL 재개

[A run36795440494](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36795440494)은 G3/B2/X12·edit2다. C# r2 generation은 `new var`의 `_reserved_identifier`/`implicit_type`/`object_creation_expression` conflict로 exit1이므로 C#27개는 NOT_RUN이다. TypeScript4/TSX8은 등록 positive 구조10개·negative 거부2개와 edit2의 incremental/fresh 및 원본 복원을 확인했다. 이 한정 관측은 전체 feature 지원이나 최종 후보 채택이 아니다. 새 C# grammar patch는 정확한 추가 결정이 필요하다.

[B run36796853218](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36796853218)은 G1/B1/X8·edit1이다. SQL은 native 전에 `Unreviewed SQL loader/evaluation`으로30개 NOT_RUN이다. static source 대조 결과 기존 PowerShell `-match`가 소문자 `function` 선언·SQL 문자열을 대문자 JavaScript `Function`으로 잘못 검사했다. `-cmatch`는 JavaScript 이름의 case sensitivity를 보존하며 `require`/`createRequire`/`eval`/`Function` 거부, 고정 source hash, literal 상대 ESM closure, root 제한과 모든 격리를 유지한다. 일반 function/SQL keyword 허용과 실제 loader/evaluation·동적/외부 import·변경/미등록 bytes 거부를 자체 반례로 검증한다.

`sql-only-r2`는 승인 B에서 아직 native0인 동일 Derek source30파일·TSQL30개·edit1의 재개다. G1/B1/X30 상한, `pinned-tsql-r1`/`trixie-r1`과 동일 A/B subject를 사용한다. PG LFS 취득·PG 생성/빌드/실행은 하지 않으며 이미 회수한 B의 별도 PG 결과에 연결한다. combined source/image counter는 일반1GiB이고 SQLPG-only1.5GiB를 다른 경로에 적용하지 않는다. baseline/실패 원문·기대값·검사기는 유지한다.

B의 PG LFS parser97,664,793bytes/SHA `a9090d5082ae5c23892d05aa59e61476f9bd39ad634228f2046024debdf815b5`는 실제 native baseline이다. baseline8개 중 legacy `WITH OIDS`는 required syntax ERROR이며 나머지7개는 quoted identifier/dollar string/ON CONFLICT 및 네 PG18 case의 등록 구조를 별도 raw 대조로 확인했다. noopt 재생성은 state387,042가 ABI/parser16-bit 최대65,535를 초과해 exit1이며8개 NOT_RUN이다. 과거 optimized exit137과 이번 명시적 state overflow를 구분하고 같은 noopt 생성을 반복하지 않는다. 각 remedy의 채택/추가 patch/생성 옵션 결정은 실패와 회귀 범위를 묶어 별도로 확인한다.

## 다음 exact 3개 효과의 실행 경계

2026-10-01 실제 사용자 답변은 human14,590bytes/SHA `dba0d5409f845fdcd1c2a0f373bac5cf90edf3b21033d5d060f59ced06dcd9ef`와 machine204,857bytes/SHA `80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824`의 정확한 세 효과를 승인했다. `remedy-exact-r1.json`은 원문 CRLF와 terminal LF를 재구성해 검증하는 공개 LF projection이다. subject 일치는 대상 식별이며 실제 사용자 권한·코드 정확성·지원 PASS를 대신하지 않는다.

| dispatch stage | 실행 대상 | 새 upstream 상한 | source/image counter |
|---|---|---|---|
| `csharp-r3` | C#9150f7d의 r1→r2를 별도 copy에 재구성한 뒤 conflicts 한 행 추가. r1 scanner와 원래27개 입력/기대 유지 | G1/B1/X27/edit5 | 1,073,741,824bytes |
| `pg-legacy-r1` | PG59d0d8c의 별도 copy에서 `OptWith`에 `WITH OIDS` 한 대안만 추가하고 최적화 생성. candidate 기존8+새4와 exact LFS baseline 새4 비교 | G1/B2/X16/edit1 | 1,610,612,736bytes |
| `mssql-evaluate-r1` | meloncholera8620fbc/tree4db801d의 고정36 regular 파일27,954,992bytes. 원본 C와 고정JSON 재생성 producer를 frozen SQL30에 각각 대조 | G1/B2/X60/edit2 | 1,073,741,824bytes |

PG 새4개는 `WITHOUT OIDS`·`WITH (fillfactor = 70)` positive와 `WITH;`·중복 `WITH OIDS OIDS;` negative다. old A/B 행·원문·기대는 변경하지 않는다. 실패한 PG noopt의 무변경 재시도, MSSQL JavaScript module 실행·package 설치·lifecycle·후보 patch·provider 채택은 포함하지 않는다. 아직 취득하지 않은 MSSQL SHA-256은 사용 전 고정 Git blob/size를 검증하고 실제 SHA-256을 기록한다.

새 단계도 fresh reviewed/merged main의 `pinned-tsql-r1`/`trixie-r1`/정확한 subject를 사용한다. tool bytes·전체8개 isolation·owned G→B→회수→새 container X를 먼저 확인하며 기존 개별 시간·4GiB memory·8MiB output·64KiB input·격리·capture·cleanup을 유지한다. native 단계 count와 동일 bound의 source/image 예약·polling·완료·실패 기록을 모두 검증한다. 이전 UNKNOWN과 실패에 새 한도를 소급하지 않는다.

각 successful upstream G의 실제 `parser.c`는 원래 capture 출력과 함께 손실 없는 `parser.c.gz`로 별도 보존하고, 원 bytes/size/SHA와 build-input 기록에 연결한다. gzip의 압축 bytes를 producer identity로 사용하지 않는다. PG 실제 LFS bytes도 보존한다. 새 단계의 packed64MiB·펼침C#/MSSQL192MiB/PG256MiB와 기존 host256MiB 한도를 초과하면 원문을 자르지 않고 실패를 보존한다. local 누적4GiB와 회수 예약은 dispatch 전에 별도로 대조한다.

생성 C export는 성공한 generation/input/capture 기록과 raw stdout/stderr의 size/SHA를 먼저 대조한다. build-input 기록이 있으면 그 parser identity도 같아야 하며, 기록이 없으면 build를 NOT_RUN으로 명시한다. 자체 fixture는 누락 command·변경 capture·다른 build parser를 거부하고 NUL/CRLF gzip roundtrip을 확인한다. Unix의 이 fixture는 fresh 숫자 suffix와 검증한 task root에서만 생성하고 finally에서 정리한다. 새 source/image counter는 승인 machine의 해당 stage 값과 직접 비교하며 다른 stage 값을 거부한다. local receiver의 120초는 download·outer/inner 확장·manifest/hash 검증을 합한 시간이다.

성공한 upstream generation이0개여도 `generated-artifacts.json`은 `[]`로 보존하고 기존 실패·summary·raw를 artifact로 묶는다. Unix 자체 반례는 실패 generation만 있는 실제 collector 경로에서 빈 목록과 package 생성을 확인하며, 이 packaging 성공을 generation/native 지원 성공으로 바꾸지 않는다.

새 empty-export root가 이미 존재하면 cleanup 후보에서 제외하고 실패한다. 충돌한 기존 디렉터리를 이 self-test가 생성한 상태로 취급하지 않는다.

새 효과의 승인·helper 검사·job conclusion·오류 부재는 syntax/구조/negative/recovery/edit 지원의 근거가 아니다. actual producer 결과와 최종6 route source 채택이 충족되기 전에는 PREPARATION_READY로 판정하지 않는다. NET461 업무 corpus와 `.svc` 제품, S08 전체78셀은 별도 NOT_RUN이다.

## SQL-only 실행 후 source 처분

[run36803412644](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36803412644)은 reviewed/merged main `b640b0ad3f2c092eed3c782a9824058368e49139`의 fresh `sql-only-r2` dispatch다. exact DerekStride97614d0 source·고정 runtime/CLI·ABI15로 G1/B1/X30/edit1을 수행했다. generated parser41,602,006bytes/SHA `36ffce6999124e9762054f9bfe2cd3778db05288feb33a4e150dc17df2e73026`와 실행 probe11,395,032bytes/SHA `c13d9ecf141548cab10d95901a88f29eb8529342eea0770ca2c3d1320798eebc`를 사용 전 대조했다. 회수한 evidence는 명령·입력·출력·생성물/EXE identity를 보존하며 생성물 전체 파일의 장기 보관이나 타 플랫폼 재생성은 주장하지 않는다.

전체8개 isolation과 owned vertical control,48개 exact container의 종료/PID0/제거,813개 host command root 종료를 확인했다. 실제 source HTTP50회/26,700,503bytes, source/image receive-counter949,981,821bytes는 해당1GiB threshold 이내였다. daemon quiescence는 NOT_PROVEN이며 과거 UNKNOWN/overshoot를 소급 복원하지 않는다.

[등록 구문·구조·negative/recovery 판정](source-feature-feasibility.md#prepare-06의-실제-ab-및-sql-결과)은 필수 syntax 실패13개, positive 구조11개, 당시 frozen negative 기대와 일치한 거부2개, 구조 실패2개, mapping 미해결2개다. 거부2개 중 실제 SELECT 괄호 negative1개와 공식 구문에 반하는 bracket 기대1개를 구별하며 과거 원 기대·집계를 보존한다. BARE의 등록 손상 입력은 오류 없이 수용해 edit support가 실패했다. 같은 후보의 무변경 재실행은 해결책이 아니며 적합한 기존 후보의 정적 비교 뒤 정확한 새 취득/시험 효과만 추가 결정한다. 후보 시험 성공을 최종 source 채택으로 표시하지 않는다. C#/PG의 남은 generation·legacy gap, source 재구성·최종 채택과 P01~P14 판정은 실제 후속 checkpoint에 연결한다.

SQL ESM 준비 검사는 pinned bytes와 literal import closure를 먼저 확인하고 loader/evaluation 단어를 거부한다. `meloncholera/tree-sitter-mssql@8620fbc`의 `grammar/statements/create-function.js`만 2,539 bytes / SHA-256 `f395d3e20195f86c3e3902f0ca05f32e0b6df242ecc744e54732ca71d8161b4a`, UTF-16 index1289의 `require`1회가 검토된 주석 원문임을 구별한다. 같은 path·전체 hash·bytes·단일 위치가 모두 일치해야 하며 주석 제거/임의 단어 허용은 없다. public MIT 원문 fixture의 정상 검사와 변경 bytes·위치·추가 loader·다른 path 거부를 self-check한다. 이는 static 입력 검사 보완이며 해당 후보 JS 실행·grammar 수정·provider 채택은 별도 exact 승인 전 NOT_RUN이다.

## exact3 종료 후 단계별 재개 조건

[실제 세 run과 기대값 충돌](source-feature-feasibility.md#승인된-exact3의-실제-관측과-기대값-충돌)은 A/G/B/X를 분리한다. C# r3는 source/scanner의 승인 hash 대조 후 G conflict로 B/X27이 NOT_RUN이다. PG는 실제 LFS baseline B/X4와 legacy candidate의 G OOM/X12 NOT_RUN을 구분한다. MSSQL은 고정36파일의 commit/blob/size와 취득한 SHA를 확인하고, 원본 C의 B/X30 및 JSON-only 재생성 G/B/X30을 각각 수행했다. MSSQL의 JS module 실행·수정·최종 provider 채택은 이 JSON-only 시험에 포함되지 않는다.

각 failed job도 실제 command/input/tool/raw/한도와 cleanup의 근거다. probe exit2는 등록 syntax/edit의 결과이며 invocation/IO/runtime/bounds 실패 exit64~71이나 resource kill과 구별한다. 관측 row는 원 registered edit·command label·actual ledger state/dependency·command 원문·probe identity에 결속하고, negative 거부의 PASS를 positive syntax support로 표시하지 않는다. materialization이 없는 NOT_RUN 입력은 frozen identity로만 기록하며 실제 file read나 parse를 주장하지 않는다.

공식 구문과 frozen 기대가 충돌하면 해당 expectation의 정확한 원문/primary 근거/영향을 별도 검토·채택한다. 허용 구문을 grammar에서 제거해 기대에 맞추거나 다른 유효 input으로 손상 의무를 대신하지 않는다. 기존 receipt/input/기대는 immutable로 보존하고, 새 revision에는 기존과 새 판정의 applicability를 명시한다. unresolved expectation은 P05의 evidence-integrity 잔여 조건이며 승인된 26-route scope 자체를 미채택으로 되돌리지 않는다.

세 run의 회수는 download/확장/manifest/hash 총120초 안에 완료됐고 전체8개 isolation, owned 수직 경로, 성공한 generation artifact의 lossless export 및 모든 생성 container의 종료/PID0/제거와 host root 종료를 검증했다. upstream 생성 artifact는 MSSQL1개이며 C#/PG candidate는 G 실패로 export 대상이 없다. `pg-legacy-r1`은 실제 채택한 exact3 human subject SHA-256 `dba0d5409f845fdcd1c2a0f373bac5cf90edf3b21033d5d060f59ced06dcd9ef`와 machine subject `80a69edf69a32263fc92efe3b5363583a6ee4b28e281752e33453333af617824`가 명시한1,610,612,736-byte source/image receive-counter 예외를 사용했다. C#/MSSQL은 일반1,073,741,824-byte 한도이며 예약·polling·완료/실패·summary의 stage mapping은 동일하다. 과거 실패에 새 한도를 소급하지 않는다. 저장4 GiB를 유지하며 daemon quiescence와 역사적 미기록 network는 NOT_PROVEN/UNKNOWN이다. C#/PG의 추가 연산은 새 exact remedy·개별 한도 결정 전 NOT_RUN으로 남기고 같은 deterministic 실패를 반복하지 않는다.
