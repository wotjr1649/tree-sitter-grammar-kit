# P05 source closure 실행 순서

소유: PREPARE [#20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20), S01 readiness 전. [고정 후보 등록부](../../src/contracts/language-sources.json), [feature disposition](language-feature-disposition.md), [위험별 case](source-feature-feasibility.md)를 함께 사용한다. 현재 upstream G/B/X는 **NOT_RUN**이며 아래 취득·격리·회수 관측과 구분한다. 기존 26-route/256행 채택은 scope 근거다.

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
| swift | root grammar.js/json; parser.c 부재 | **generation 뒤** parser/header + 고정 scanner.c | P05-SWIFT-GENERATION; 생성 실패와 syntax 실패 분리 |
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
| postgresql-sql | postgres/grammar.js/json, node-types | postgres parser/scanner/headers | P05-PG-LEGACY/18; PG19 기반 선언은 미지원 선언 아님 |

TS/TSX의 고정 lockfile은 `tree-sitter-javascript@0.23.1`을, C++은 `tree-sitter-c@0.24.1`을 가리킨다. range나 현재 language-route pin으로 대체하지 않고 lock integrity 및 실제 package bytes를 확인한다. grammar 상속에 불필요한 binding install script는 실행하지 않는다. PostgreSQL의 grammar.json→C 생성과 upstream PostgreSQL grammar→Tree-sitter converter는 다른 closure다. 후자의 원본 PG pin·converter 도구·변환 script가 확인되지 않으면 그 재현은 미실행으로 남긴다.

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

호환성 진단은 전체 source 취득 뒤 선택 image의 실제 digest/platform/storage와 container 설정을 확인한 상태에서 수행한다. 이 helper는 CLI만 먼저 취득하는 경로가 없어 일괄 취득을 유지한다. `tool-environment`는 libc/Node/GCC/linker identity와 CLI ELF header·interpreter·NEEDED·required versions 및 libc provider versions를 CLI 실행과 별도로 보존한다. 이어 `cli-version`과 `cli-generate-help`를 각각 제한 실행한다. 이전 loader 실패가 환경 진단의 출력을 가리지 않도록 한다. 실제 회수·격리8개를 새 image에서 반복한 뒤 upstream generation을 시작한다.

등록57원문과 별도로 소유한 작은 scannerless grammar와 `OWNED-P05-VERTICAL` 입력을 task scratch에 고정한다. 같은 CLI의 ABI15 generation1회, 독립 runtime/GCC build1회, 회수한0755/SHA executable의 새 container parse/edit1회로 수직 경로를 확인한다. 구조 기대는 declaration/name/body identifier의 byte ranges와 brace 손상 오류·복구/incremental/fresh 동일성이다. 이는 product S04/S05/S06 구현이나 upstream57개 지원 근거가 아니다. 자체 G/B/X1회씩은 별도 counter에, 회수는 기존169회 shared cap 안에 기록한다. 나머지 generation6/build11/execution128/diagnostic16 및 개별 timeout/memory/output/transfer/storage 한도는 유지한다.

각 build는 같은 image의 readelf로 실제 ELF/loader/NEEDED/version 원문을 회수한다. 허용한 x86-64 ELF64·image loader·libc closure와 executable mode/hash를 확인한 후 새 container에서 실행한다. 모든57개 input의 baseline/재생성 variant는 final case ledger에 남으며 미실행은 dependency blocker를 명시한다. syntax/구조/negative/recovery/edit의 최종 지원 판정은 해당 원문·tree·feature 사실의 실제 평가가 필요하다.

upstream `grammar.json`의 rule key는 대소문자를 구별한다. grammar entry-point 이름은 case-sensitive hashtable로 읽고 기존 C identifier 제한을 적용한다. run36644794802는 새 image의 CLI/libc·격리8개·자체 G/B/X control을 통과한 뒤 T-SQL의 `AS`/`as`를 기본 PowerShell JSON object로 읽다가 중단했다. upstream G/B/X0이며 grammar 구문 실패가 아니다. 원본 source를 고치거나 key를 정규화하지 않고 reader를 교정하고 case-distinct key와 잘못된 entry-point 이름을 회귀 검사한다.
