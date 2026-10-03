# Session 05 — Incremental Verification 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S05`, Issue #7, Milestone 6, branch `session/05-incremental`(base main `5aee01d`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* **`tsgk-native/r1` driver**(`src/drivers/native-c/driver.c`): 계약은 [tree/protocol](../specs/tree-and-adapter-protocol.md)의 `S05 구현`이다. 4-byte 길이 frame, `single`·`batch` 모드, 고정 key 순서의 canonical JSON 요청, 표준 base64 원본·replacement, 요청 단위 한도, 모든 edit를 bytes에 먼저 적용해 보는 사전 검사, encoding별 경계·point 계산, parser A의 incremental parse와 parser B의 fresh parse, 반복 cursor preorder 직렬화(full·summary·record), `tsgk-tree-digest/r1`, 선언 구조 순회(S03 선언 node 종류와 locator, query 없음), 등록 지점 partial tree, 60초 progress callback 취소(`PARSE_TIME_LIMIT`), allocator hook(`ALLOCATION_LIMIT`), node·depth·출력 상한, step별 included range를 구현했다. route 계측은 tree에서 관측한 `edit_has_changes`·`reused_nodes`·`fresh_reused_nodes`이고 자기 보고 flag가 아니다. 시험 전용 fault define(`TSGK_FAULT_*`)은 build identity에 들어간다.
* **kit 공개 offline API**: 하나뿐인 tree 비교 `kit.CompareTrees`, `TreeDigest`, `ValidateTree`, edit 검사 `ApplyEdits`(경계·point), cp949 표(`CP949Pair`·`CP949Rune`), `ParseIncrementalProfile`, `.svc` directive 관측 `ObserveServiceHost`. 계약은 [공개 API](../specs/public-go-api.md)의 `S05 함수`다. S01 encoding 판정은 고정한 WHATWG `index-euc-kr`로 AMBIGUOUS(`AMBIGUOUS_ENCODING`)와 표 밖 쌍(`CP949_UNMAPPED`)을 완성했다([identity/evidence](../specs/identity-and-evidence.md)).
* **CLI `tsgk incremental`**과 profile `tsgk-incremental/r1`(`src/internal/native`): driver build(runtime manifest·grammar hash 대조, shim 생성, 단계별 compile/link를 S04 runner로 실행, build identity), 실행 직전 executable hash 재확인, 사례별 driver process, response 검증, 세 claim 판정, `tsgk-tree/r1`·`tsgk-tree-summary/r1`·`tsgk-svc-composite/r1` 결과, 비공개 corpus batch(500 파일·256 MiB per process, 재대기열, trailing bytes 거부). 계약은 [CLI/profile](../specs/cli-and-profile.md)의 `S05 구현`, [플랫폼](../specs/platform-support.md)의 `S05 native build`다.
* **등록부와 사례**: [`native-routes.json`](../../src/contracts/native-routes.json)(26 route의 build 입력: 고정 commit upstream 파일, 채택 6 route의 patch chain·adoption hash·재생성 출력), route마다 feature·recovery 사례 2개(`src/testdata/native/routes`), 채택 route `known_gaps`와 `postgresql-sql-B04` gap 사례 42개(`gaps`), NET461 C#·XML·SVC 사례 43개(`n461`), 동적 SQL 위치 fixture(`dynamic-sql`, S06용 기대 사실 26개·비사실 11개), 합성 대용량 C# fixture 3개([`native-large-fixtures.json`](../../src/contracts/native-large-fixtures.json)), owned scannerless·stateful scanner fixture(`src/testdata/native/plain`, `stateful`).
* **CI**: foundation job이 고정 runtime source와 host의 기존 compiler로 owned native 시험을 세 OS에서 실행하고, 이어 `native prepare (ubuntu-24.04)`가 26 route 입력을 준비·재생성하고 `native routes (<os>)` 세 job이 route 사례와 대용량 fixture를 실행한다. Linux job은 sanitizer build로 owned 시험을 한 번 더 돈다([validation](../validation/validation.md)).

제외: query 실행(S06), Go consumer adapter, native library embedding, scanner 번역, 장기 실행 server, 성능 최적화, 다른 저장소 변경.

## 도구 identity

Windows 로컬 실행은 Go 1.27.1, PowerShell 7.6.6, 기존 MSYS2 UCRT64 GCC 16.2.0(`gcc.exe` sha256 `75e87953…`, 설치·변경 없음), runtime `tree-sitter/tree-sitter@659cda7c`(내장 manifest 83개 파일 대조)를 썼다. 채택 route 재생성은 S04에 등록된 tree-sitter 0.27.0(`9fbc4f28…`)과 Node 24.21.0(공식 `win-x64/node.exe`와 같은 hash)으로 한다. hosted runner의 compiler는 `select-compiler.ps1`이 고르고 경로·hash·version을 CI 로그와 route 결과에 남긴다(아직 실행 전). WHATWG `index-euc-kr`은 identifier `1d97134c…`, sha256 `89af20dd…`, 671621 bytes로 받아 `src/kit/data`에 고지와 함께 고정했다.

## 관측 revision과 로컬 검사

구현 commit `b07fc5b`, `4f94433`, `7d192bc`, `5189eee`, `4b73cdc`, `0685eaf`, `9c047c1`, `fb8934a`, 보고서 `861e215`, 리뷰 수정 `e81fa10`, `58f079b`, `02577a1`에서 Windows amd64로 [validation](../validation/validation.md)의 명령 블록(고정 module 취득 → `GOPROXY=off`·`-mod=readonly`, `gofmt`, `go vet`(windows·`GOOS=linux`·`GOOS=darwin`), `go test ./src/... -timeout 300s`, `go build`, `git diff --check`)을 `TSGK_NATIVE_RUNTIME`·`TSGK_NATIVE_CC`를 준 상태로 실행해 모두 통과했다. 한 번은 기존 S01 시험 `TestCorpusLimits/WALL_LIMIT`(wall 1ns)이 native 시험과 동시에 돌 때 실패했고 단독 3회는 통과했다. 시간 의존 시험의 기존 불안정으로 기록하며 이번 변경과 무관하다.

| 검사 | 결과 |
|---|---|
| owned native 시험(`src/internal/native`, `TestIncrementalCLI`) | 통과. A01 삽입·삭제·교체 3 step 모두 incremental = fresh, route 증명. A02 손상 중간 상태와 복구. A04·A03 fault build(`OMIT_EDIT`, `OMIT_OLD_TREE`, `FRESH_WITH_OLD`, `STEP1_FLAG`) 모두 검출, 중간 step 불일치는 마지막 step이 같아도 step 1로 보고. A08 stateful scanner 통과, 직렬화 결함 build(`TSGK_SCANNER_FAULT`)는 incremental ≠ fresh로 검출 |
| 경계·encoding(A05·A06·A16·A17) | EOF 삽입, zero-width MISSING, 부모·자식 같은 범위 보존, 빈 source 삽입은 equality PASS·route FAIL(재사용할 node 없음을 정직하게 보고). BOM·CRLF·NUL·잘못된 UTF-8, UTF-16LE/BE(surrogate 포함), CP949 문자열과 identifier가 원본 bytes·byte point로 왕복. 홀수 UTF-16·cp949 분할·UTF-8 문자 분할 edit는 kit와 driver 모두 거부, 잘못된 byte 옆 경계는 받음 |
| edit 거부(A07) | 범위 밖·역전·old 불일치·길이 불일치·빈 edit를 kit와 driver가 같은 code로 거부, driver는 step 0개 |
| frame(A09) | 짧은 머리·짧은 payload·48 MiB 초과·single의 추가 frame/byte·공백 삽입·trailing JSON·protocol revision·base64·0 한도·tree가 아닌 edit·5 edit·locator·encoding·range 위반을 `INVALID_REQUEST`로 거부. 응답 쪽 truncation·trailing byte·unknown/중복 member·`complete:false`·exit 불일치·step 누락·digest 위조·node graph 위조·source 전송 불일치를 거부. batch의 trailing byte는 그 batch 전체를 받지 않음 |
| 한도·취소(A10·A13·A19) | node·depth·출력 상한 `RESOURCE_LIMIT`, `parse_ms` 1ms progress 취소 `PARSE_TIME_LIMIT`, allocator hook 2048 bytes `ALLOCATION_LIMIT`, null tree fault `FAILED`/`PARSE_NULL`(빈 tree로 바꾸지 않음), caller 취소 `CANCELLED`와 cleanup verified, 같은 요청 10회 별도 process 동일 digest. 중첩 12000은 완료(max depth ≥ 10000), 100001은 `DEPTH_LIMIT` |
| identity(A11) | define·parser bytes 변경은 새 build identity, 옛 hash의 parser·runtime 변경·compiler hash 불일치·제한 밖 symbol은 실행 전 거부, 변조한 executable은 `EXECUTABLE_MISMATCH`로 실행하지 않음 |
| targeted mutant(A12) | 21/21 검출(`mutants-fb8934a.json`, `mutants-58f079b.json`, 최종 code `02577a1`과 code가 같은 `81a0538`에서 `mutants-81a0538.json`). 마지막 step만 비교, 정렬·중복 제거 후 비교, 누락 flag 기본값, route 재사용·fresh 독립 확인 제거, trailing byte·digest·exit·step 수 검사 제거, edit old 검사 제거, Unicode scalar column, cp949 AMBIGUOUS 무시, progress 취소·allocator 상한·depth 상한 제거, cp949 decode 폭, 홀수 UTF-16 수용, summary gate 무시, CR 정규화, batch trailing 무시. 첫 실행(`4b73cdc`)은 18/21이었다: 한 mutant는 컴파일되지 않았고(검출 실패 아님), allocator mutant는 realloc 경로가 같은 한도를 지켜 동등했으며, chunk 크기 mutant는 runtime이 chunk 경계를 이어 읽어 동등했다. 앞의 둘은 의미 있는 형태로 고쳤고 마지막은 decode 폭 mutant로 바꿨다. decode 폭을 관측하려고 owned plain grammar의 identifier를 Unicode 문자로 넓혀 parser를 다시 생성했다 |
| 공개 offline closure(A15) | `TestOfflineClosure` 통과: 공개 package closure에 runner·`os/exec`·network가 없다. native 도구가 없으면 owned native 시험은 skip이고 기존 CLI/API 시험은 그대로 통과한다(CI는 `TSGK_NATIVE_REQUIRED=1`) |
| cp949 표(A16) | 내장 표 identity·고지 시험, driver header가 표의 기계 변환인지 시험, AMBIGUOUS·표 밖 쌍·선언 판정 시험 |
| 등록부·fixture | 26 route registry가 adoption hash와 재현 기준 parser를 가리키는지, 대용량 fixture를 Go에서 독립 재생성해 크기·sha256이 같은지, 사례 파일 형식을 시험 |

## 26 route (A14·A22, Windows 로컬)

`prepare-routes.ps1`이 S01이 결속한 source에서 26 route 입력을 만들었다. 채택 6 route는 patch chain을 적용해 모든 adoption hash가 일치했고, typescript는 `tsgk reproduce`로 다시 생성해 6개 출력이 모두 등록 기준과 같았다(PASS, npm `tree-sitter-javascript@0.23.1` 632551 bytes 다운로드). 나머지 5개 채택 route는 S04 출력을 같은 hash로 재사용했다. 최종 후보 `02577a1`의 `run-routes.ps1` 결과(`routes-windows-02577a1.json`; `fb8934a`, `e81fa10` 결과도 보존):

* 26 route 모두 GCC로 build됐다. build 이식성 patch는 필요 없었다(parser compile 최장은 PostgreSQL: `fb8934a` 6.9초, `e81fa10` 5.2초, host 부하가 있던 `02577a1` 14.3초).
* 사례 137개(route 기본 52, gap 42, NET461 C#·XML 21, SVC 22) 모두 `COMPLETED`: PASS 119, FAIL 10, BLOCKED 8. edit가 있는 모든 사례에서 incremental equality와 route가 PASS다. FAIL은 아래 grammar gap 10건이다.
* SVC 22건 중 진단 없는 directive만 있거나 C# inline을 parse한 14건이 PASS다. BLOCKED 8건: inline 경계·언어를 해석하지 못함(`SVC_INLINE_UNRESOLVED` 4건: directive 중복, directive 뒤 남는 class, `%>` 없음, Language 생략), VB(`SVC_INLINE_UNSUPPORTED`), `%>`를 지운 중간 step(`SVC_INLINE_NOT_PARSED`), 닫히지 않은 quote 진단(`SVC_DIRECTIVE_DIAGNOSTICS` 2건). 이 형식은 inline이 없는 step의 incremental 비교를 하지 않는다(한계).

grammar gap 처분(kit가 충실히 보고한 결과, kit 결함 아님, S08로 넘김):

| route | 사례 | 관측 | 처분 |
|---|---|---|---|
| csharp | `csharp-gap-pattern-r1`, `csharp-gap-query-r1` | 비동기 밖 문맥 키워드 `async`·`await`를 형식·상수 pattern·query range 변수로 쓴 유효 C#을 ERROR로 파싱 | grammar gap(upstream 선언 한계, `known_gaps`의 등록 27사례 밖 항목) |
| typescript | `using`·`await using`, `accessor`와 decorator, import attributes, `export type *` | TS 5.0–5.3 유효 구문을 ERROR로 파싱 | grammar gap(V59 밖 최신 구문) |
| tsx | `<T>` generic arrow, 닫는 tag 이름 불일치 | TSX에서 오류인 입력을 오류 없이 받음 | grammar gap(관대한 수용) |
| swift | 빈 macro 인자, 빈 `repeat each` | 오류인 입력을 오류 없이 받음 | grammar gap(관대한 수용) |

## 대용량·깊은 중첩(A18·A19, Windows 로컬)

`real-world-source-r2`, C# route:

| fixture | bytes | 결과 | parse | process wall | job commit 최대 |
|---|---|---|---|---|---|
| `cs-large-22m` | 21997719 | `COMPLETED`, summary(`DESCENDANT_LIMIT`, 11463151 node), errors 0, 선언 PASS | 10.5초(`02577a1`: 31.2초) | 21.6초(72.7초) | 3.33 GB |
| `cs-large-8m-errors` | 7999000 | `COMPLETED`, summary, errors 2257(목록 1000건 상한, truncated), 선언 FAIL(기대값) | 4.0초(7.0초) | 8.0초(14.7초) | 1.23 GB |
| `cs-large-32mib-errors` | 33554432 | `RESOURCE_LIMIT`(`MEMORY_LIMIT`, Job Object hard cap 4 GiB) | — | 13.5초(36.8초) | 4.30 GB |

시간은 `e81fa10` 실행 값이고 괄호는 같은 입력의 최종 후보 실행 값이다. 최종 실행 동안 host에 다른 부하가 있어 시간만 늘었고 상태·node 수·errors 수·선언 판정은 같았다(시간은 host 관측값이며 비교 대상이 아니다). route summary의 대용량 항목에는 digest를 기록하지 않는다. 32 MiB C#은 이 grammar에서 4 GiB 안에 parse되지 않는다. S05-A18에 따라 이 결과를 그대로 기록하고 상한은 올리지 않는다. 값을 올릴지는 사용자 결정이다. 48 MiB를 넘는 encoded 요청은 실행 전 `REQUEST_TOO_LARGE`, driver frame 머리는 `FRAME_TOO_LARGE`다. 세 host 측정은 `native routes` job이 같은 fixture로 한다(실행 전). 깊은 중첩은 owned grammar로 foundation의 세 OS 시험에 들어 있다.

## 비공개 corpus(A20, Windows 로컬, 개수만)

`NET461-PHASE2-LOCAL-r1`을 clean 후보 `81a0538`에서 `run-corpus.ps1`로 실행했다(11분 32초, 실행 wall 3600초 안, 호출마다 남은 시간을 `--run-wall`로 줌). `81a0538`은 최종 code `02577a1`에 문서 2개만 더한 commit이라 build가 같다. `fb8934a`, `58f079b`의 실행도 같은 개수였다. 경로·이름·내용이 담긴 기록은 추적하지 않는 로컬 artifacts에만 있다.

* inventory 21451 record: route 있는 비`PRESENCE_ONLY` 17957, `PRESENCE_ONLY` 133, route 없음 3494. encoding: UTF-8(BOM 13371, 검증 1146), UTF-16LE(BOM) 2789, CP949(검증) 651. route 없는 파일 중 AMBIGUOUS 2, NUL_WITHOUT_BOM 1292.
* route별 batch: csharp 6624 파일·14 process, tsql 6150·13, xml 5163·11. svc 20 파일은 모두 inline 없는 directive라 driver process 없이 관측만 했다. 17957 파일 모두 `COMPLETED`, `NOT_RUN` 0, 치명 frame 0, kit 결함 0.
* `has_error`: csharp 48(그중 자동 생성 파일 45), tsql 181, xml 1. svc는 tree를 parse하지 않았으므로 관측값이 없다. 이 230개 ERROR 파일의 개별 처분은 S08 판정이다. 표본 진단에서 tsql 오류는 한글 식별자(별칭·변수·열 이름) 위치였고 decode된 글자는 정확했다. UTF-8 파일에서도 같은 형태가 나와 encoding이 아니라 grammar의 비ASCII 식별자 미지원으로 본다.
* full tree gate: 517개 파일이 descendant 50000을 넘었고 모두 완료됐다(보존은 record만).
* svc 20개는 모두 directive와 CodeBehind 관측, inline 없음(`inline: ABSENT`)이다.
* `02577a1`의 첫 corpus 실행은 csharp 단계 중 host 메모리 부족으로 Claude Code가 중단시켰다. 남은 process가 없음을 확인했고 부분 출력은 보존했다(`private-corpus-02577a1-attempt-killed-low-memory`). 메모리가 회복된 뒤 orchestrator 요청으로 `81a0538`에서 한 번 다시 실행한 것이 위 결과다.
* `58f079b`와 비교하면 17957 record 모두 실행 상태·판정·code·`has_error`·node 수·digest·errors·출력 형식·SVC coverage·encoding이 같다. svc 20개는 모두 `%>`가 있고 진단이 없는 directive라 바뀐 SVC 판정에서도 PASS다. 공개 summary에서 달라진 것은 commit, wall(host 부하 차이), 관측하지 않은 `has_error` 키 표기(`svc:` → `svc:null`), 네 group의 `executable_sha256`이다. `build_identity`는 네 group 모두 같다. 같은 identity로 다시 build하면 Windows PE bytes가 달라진다(platform-support S05 native build).
* helper 결과 취합 결함으로 네 번의 시도가 완전한 기록을 남기지 못했다: `0685eaf`·`9c047c1`은 parse를 마친 뒤 strict mode 속성 접근 오류로 멈췄고, `e81fa10`은 주석 처리 실수로 `has_error`를 기록하지 않았다. 모두 보존했다(`private-corpus-*-attempt*`). parse 자체의 결과 개수는 시도마다 같았다.

## 분리 context 리뷰와 처분

`5aee01d..861e215`에 대한 분리 context 리뷰(EXECUTED_REVIEW, 별도 general-purpose subagent, Windows에서 전체 `go test`와 native frame 시험 실행; sanitizer·세 OS CI는 미실행; GitHub 승인 아님)는 BLOCKER 1, MATERIAL 3, MINOR 6, NOTE 5를 보고했다(`artifacts/.../session-05/review-r1.json`). 처분은 `e81fa10`이다.

* BLOCKER R1-B1: driver가 included range의 끝을 검사하기 전에 point를 계산해 source 밖을 읽었다 → 범위 검사를 먼저 하고, 4000000000 끝 범위 거부 시험을 더했다.
* MATERIAL R1-M1: 선언 64개일 때 계산 표시 bit와 64번째 선언 bit가 겹쳤다 → 별도 계산 표시, `TestSixtyFourDeclarations`.
* MATERIAL R1-M2: 빈 buffer를 `memcpy(NULL, 0)`으로 넘겨 UBSan에서 멈출 수 있었다 → 길이 0이면 복사하지 않는다.
* MATERIAL R1-M3: parse하지 않은 `.svc` inline(Language 생략·VB·directive 없음·일부 step만 C#)이 PASS였다 → directive만 있는 source만 PASS, 나머지는 code를 붙여 BLOCKED. 계약 문구와 시험을 고쳤다.
* MINOR R1-m1~m6: batch 배열 null과 parse하지 않은 step의 `has_error` 기록, cleanup 실패를 다른 상태보다 먼저 판정, partial tree 최하위 node의 field, 응답 파일 이름 충돌과 쓰기 실패 exit, 응답 code와 첫 미완료 tree의 일치, corpus 전체 wall(`--run-wall`).
* NOTE R1-n1~n4: locator 구분자 일치, protocol 오류 시 frame buffer 해제, route helper의 BLOCKED claim·대용량 build 실패 집계, build identity와 executable bytes 관계(같은 identity로 다시 build하면 Windows PE bytes가 달라질 수 있어 executable hash를 build마다 기록) — 모두 반영했다.

재리뷰 r2(`861e215..52ef70c`, EXECUTED, `review-r2.json`)는 R1의 13건을 RESOLVED로 확인했다. 남은 R1-M3에서 `%>` 없는 `.svc`가 PASS인 경로를 MATERIAL R2-M1로, 줄인 run wall을 결과에 남기지 않는 점을 MINOR로, NOTE 2건을 보고했다. 처분 `02577a1`: `%>`가 없으면 inline `UNRESOLVED`와 `SVC_INLINE_UNRESOLVED`, directive 진단이 있으면 관측 전용 판정에서 `SVC_DIRECTIVE_DIAGNOSTICS`(BLOCKED), 줄인 run wall을 결과 operation과 policy identity에 기록, 관측하지 않은 `has_error`를 null로 표기. 재리뷰 r3(`52ef70c..02577a1`, STATIC: host 메모리 부족으로 시험은 실행하지 않음)은 R2 4건을 모두 RESOLVED로 확인했고 BLOCKER·MATERIAL은 0이다. NOTE 3건은 다음과 같이 처분했다: C# inline을 parse한 사례에는 directive 진단을 assessment에 반영하지 않는다는 범위를 계약에 명시했다. 관측기가 이름 아닌 문자·빈 값을 진단하지 않아 `n461-svc-multiline-n-r1` 음성 사례가 PASS인 점은 한계로 남긴다. `58f079b` 공개 summary의 `svc:` 키는 수정 전 산출물이며, 최종 실행 `private-corpus-81a0538`에서는 `svc:null`이다.

재리뷰 r4(`02577a1..57e4afb`, EXECUTED, `review-r4.json`)는 최종 증거와 맞지 않는 보고서 수치 3건(PostgreSQL compile 시간의 출처, 32 MiB 최종 wall 누락, corpus summary 차이에서 `executable_sha256` 누락)과 확인할 수 없는 digest 서술을 보고했고 모두 고쳤다. SVC 판정 code가 여럿 해당할 때의 순서는 계약에 정하지 않았고 구현 순서를 기록한다: 앞 step부터 보며, 각 step 안에서 directive 없음(`SVC_DIRECTIVE_ABSENT`), `%>` 없음·inline 해석 실패(`SVC_INLINE_UNRESOLVED`), 지원하지 않는 언어(`SVC_INLINE_UNSUPPORTED`), 일부 step만 C# inline(`SVC_INLINE_NOT_PARSED`) 순으로 처음 맞는 code를 낸다. 모든 step을 본 뒤에야 기대값(`SVC_EXPECTATION_UNASSESSABLE`), 진단(`SVC_DIRECTIVE_DIAGNOSTICS`)을 본다. 그래서 step 0에 C# inline이 있고 step 1에 `%>`가 없는 `n461-svc-terminator-r-r1`은 `SVC_INLINE_NOT_PARSED`다. 재리뷰 r5(`57e4afb..415d44a`, EXECUTED)는 r4 정정 4건을 RESOLVED로 확인했고, 이 순서 서술과 문단 구성을 고치게 했다.

## PR CI 1회차 실패와 수정

PR #64의 CI run 37120183769(head `4a52224`)에서 foundation ubuntu-24.04만 실패했다(orchestrator 보고: Windows·macOS foundation 성공, native prepare·routes는 `needs`로 건너뜀). workflow의 foundation native 준비 단계는 세 OS 모두 `TSGK_NATIVE_REQUIRED=1`을 주므로, 성공한 두 job에서는 owned native 시험이 skip 없이 실행됐다. CI 로그에서 관측한 것은 `TSGK_NATIVE_CC=/usr/bin/gcc`, 단계 실행 전(`wall_ms` 0, build null)의 `TOOL_MISSING`, 원인이 빠진 CLI finding, nil build를 역참조한 시험 helper다. 원인은 소거법으로 추론했다. 수정 전 코드에서 build 없이 `TOOL_MISSING`을 내는 곳은 compiler identity 확인 하나뿐이고, 이 확인은 일반 파일이 아닌 경로(link)를 거부한다. Ubuntu의 `/usr/bin/gcc`는 보통 버전별 gcc를 가리키는 link이고, Windows MSYS2 `gcc.exe`와 macOS `/usr/bin/clang`은 일반 파일이라 그 host에서는 드러나지 않았다. kit 결함이다.

수정 `483a640`:

* compiler 경로의 link를 해석해 실제 파일의 bytes를 대조하고 그 파일을 실행한다. 해석할 수 없는 link는 계속 `TOOL_MISSING`이다.
* build 실패 finding에 원인 error와 실패한 단계의 stderr 끝을 담는다.
* helper는 nil build에서 `t.Fatalf`로 실패한다.
* `TestBuildCompilerLink`를 추가했다. 로컬에서 수정을 되돌리면 CI와 같은 code `TOOL_MISSING`로 실패하고, 원인은 `not a regular file`로 나온다.

단언, `TSGK_NATIVE_REQUIRED` 관문, 상한은 바꾸지 않았다. 로컬 Windows에서 전체 검사가 통과했다. Linux 실행은 다음 CI에서 확인한다.

재리뷰 r7(`4a52224..483a640`, EXECUTED, `review-r7.json`)의 결론은 BLOCKER·MATERIAL·MINOR 0, NOTE 4다.

* finding 메시지에 host 경로가 들어갈 수 있다. 지금은 로컬 결과에만 쓰고 공개 summary에는 code만 옮긴다.
* 호출 이름에 의존하는 wrapper는 link로 지정하지 않는다고 위 계약에 적었다.
* 새 시험은 compiler를 실제로 실행하지 않는다. Linux CI build는 link로 지정한 compiler로 build할 수 있음을 보여 주지만, 해석한 경로로 실행했는지는 간접적으로만 확인한다.
* 다음 CI의 위험은 Linux sanitizer 단계의 시간 예산이다.

## PR CI 2회차 실패와 Linux 전용 경로 점검

CI run 37121192095(head `0271e06`)에서 compiler link 수정은 효과가 있었다. Linux에서 CLI 시험과 대부분의 native 시험이 실제 build까지 통과했다(native package 47.8초). 남은 실패는 `TestBuildIdentity`와 `TestBuildCompilerLink` 두 개였고, 둘 다 `BUILD_START_FAILED: MEMORY_HARD_CAP_UNSUPPORTED: posix-process-group backend memory is SAMPLED`였다. 두 시험만 build 요청에 위임된 cgroup parent를 넘기지 않았다. 계약(platform-support, cli-and-profile)상 Windows·Linux는 hard memory backend가 없으면 실행 전에 거부하므로 제품 동작은 맞고, 결함은 시험에 있었다.

수정 `95bde45`:

* 시험 build 요청을 `testBuildRequest` 하나로 모았다. 모든 시험 build가 같은 cgroup parent와 같은 sanitizer 모드를 쓴다. 이전에는 `TestBuildIdentity`의 요청에 sanitizer 설정도 빠져 있었다. 그래서 sanitizer 단계에서 "parser가 바뀌면 identity도 바뀐다"는 비교가 처음부터 다른 build끼리 이뤄졌다.
* Linux에서만 드러나는 경로를 정적으로 점검하다 CI helper script의 종료 코드 결함을 찾았다. `run-routes.ps1`은 실패가 없으면 `exit` 없이 끝났다. 그러면 호출한 workflow의 `$LASTEXITCODE`에 마지막 tsgk exit이 남는다(대용량 32 MiB의 예상된 `RESOURCE_LIMIT`이면 3). 결국 실패 0건이어도 routes job이 세 OS 모두에서 실패했을 것이다. `run-routes.ps1`과 `prepare-routes.ps1`은 이제 `exit 0`으로 끝난다. 수정 뒤 csharp route를 workflow와 같은 방식으로 호출해 확인했다. 마지막 tsgk는 exit 3이었고 호출자의 `$LASTEXITCODE`는 0이었다.
* `TestHostSettingsReachCI`를 추가했다. 시험 build가 `testBuildRequest`를 우회하거나 두 script가 `exit 0`으로 끝나지 않으면 실패한다. 두 음성 대조로 시험이 잡는 것을 확인했다.
* Linux sanitizer 단계에서 `vm.mmap_rnd_bits=28`을 설정한다. 높은 ASLR 엔트로피에서 오래된 sanitizer runtime이 shadow memory를 배치하지 못하는 문제가 알려져 있다(actions/runner-images#9515). kernel 전역 설정이므로 시험 뒤(실패해도) 원래 값으로 되돌려 같은 job의 route 단계는 원래 엔트로피에서 돈다.

재리뷰 r10(`0271e06..198f6c1`, EXECUTED)은 위 수정을 확인했고, 보고서가 아직 이름 붙이지 않은 Linux 전용 경로 하나를 MATERIAL로 지적했다. route helper가 compiler bytes를 `Get-Item`의 `Length`로 기록하는데, 이것은 link 자체의 크기다. 반면 hash는 `Get-FileHash`가 link를 따라가 실제 파일로 계산한다. 그래서 Linux의 `/usr/bin/gcc`(link)로는 모든 route build가 `TOOL_IDENTITY_MISMATCH`로 거부됐을 것이다. 처분:

* `select-compiler.ps1`은 해석한 실제 경로를 돌려준다.
* `run-routes.ps1`·`run-corpus.ps1`은 link면 해석한 파일로 hash와 크기를 함께 기록한다.
* Windows에서 symlink compiler로 json route를 실행해 확인했다(Windows도 link의 `Length`는 0이다). 수정 전 helper는 `TOOL_BYTES_INVALID`로 실패했고(caller exit 1), 수정 후에는 PASS(caller exit 0)였다. `TestHostSettingsReachCI`는 세 helper가 link를 해석하는지도 확인한다.
* sanitizer 단계의 `mmap_rnd_bits` 적용 범위 서술을 바로잡았다(MINOR).

정적으로 점검했지만 Linux 없이는 확인할 수 없는 CI 위험:

* sanitizer 단계의 ASan/UBSan 실행(LSan 누수 판정 포함)과 시간 예산. sanitizer 없는 Linux native package가 47.8초였고, 상한은 go test 420초와 step 8분이다.
* native-prepare의 codeload archive 경로. 26 route archive 추출, patch chain, tree-sitter·node 내려받기와 실행 권한, 6개 route 재생성이 해당한다. 로컬 prepare는 S01 source를 재사용해 이 경로를 실행하지 않았다. runtime archive 경로만 세 OS foundation에서 통과했다.
* Linux 대용량 fixture의 cgroup 메모리. cgroup은 page cache도 센다. 22m fixture가 4 GiB에 닿으면 측정 결과가 달라질 수 있지만 route failure로 세지는 않는다.
* macOS 대용량 fixture. sampled 메모리이고 runner RAM은 7 GB다.

## PR CI 3회차 실패와 workflow 방식 재현

CI run 37122458270(head `e220b82`)에서는 foundation이 세 OS 모두 통과했고 native prepare(ubuntu)도 약 7분에 통과했다. native routes는 세 OS 모두 route 실행 전에 멈췄다.

* windows-2025·macos-15: `select-compiler.ps1:14`에서 `InvalidOperation`이 났다. `& $cc --version | Select-Object -First 1`은 첫 줄을 받으면 pipeline을 멈추므로 `$LASTEXITCODE`가 설정되지 않는다. 새 step shell에는 이전 값도 없으므로 StrictMode가 그 변수 읽기를 거부한다. sanitizer step(Linux)에서는 앞의 `sudo` 명령이 이미 값을 설정해 두어서 드러나지 않았다. `run-routes.ps1`·`run-corpus.ps1`도 같은 형태였다. 거기서는 앞의 `go build` 값이 남아 있어 `--version` 실패를 이전 값으로 읽었다.
* ubuntu sanitizer step: runner 사용자가 root 전용 파일인 `/proc/sys/vm/mmap_rnd_bits`를 직접 읽으려다 실패했다.

로컬에서 workflow의 step 본문을 그대로 꺼내 GitHub runner와 같은 형태(`$ErrorActionPreference = 'stop'`, 끝의 `exit $LASTEXITCODE`)로 감쌌다. 그다음 RUNNER_TEMP 같은 디렉터리와 runner 변수를 준 새 `pwsh -NoProfile` process에서 실행했다. 수정 전 HEAD에서 CI와 같은 오류(`'$LASTEXITCODE' 변수가 설정되지 않음`, step exit 1)를 재현했다.

수정 `b025247`:

* 세 helper는 `--version` 출력을 모두 받은 뒤 첫 줄을 고른다.
* sanitizer step은 `sudo sysctl -n`으로 값을 읽고 형식을 확인한다. 복원은 finally에 그대로 둔다.
* `select-compiler.ps1`에 시험용 `-Candidates`를 더했다.
* 회귀 guard를 두 개 추가했다.
  * `TestSelectCompilerFreshShell`: 새 pwsh process에서 일반 compiler와 그 link로 `select-compiler.ps1`을 실행하고, 해석한 실제 파일이 돌아오는지 확인한다.
  * `TestCIScriptPatterns`: helper의 잘린 native pipeline과 workflow의 `/proc/sys` 직접 읽기를 거부한다.
  * 수정 전 script·workflow로 되돌리면 두 시험 모두 실패한다.

수정 뒤 같은 방식으로 Windows routes step 전체를 실행했다(`-Large` 포함). step exit 0, 사례 137개(PASS 119, FAIL 10, BLOCKED 8), failures 0이었다. compiler 후보만 symlink로 바꾸고 3개 route(json, go, csharp와 csharp-svc)를 실행한 변형도 step exit 0이었고, 해석한 실제 파일을 썼다. foundation step은 CI에서 세 OS 모두 통과했으므로 다시 실행하지 않았다.

Linux·macOS route step의 나머지 줄을 정적으로 점검한 결과:

* Linux route step은 sanitizer step과 다른 이름의 cgroup을 만든다. 이 생성 방식(`sudo mkdir`·`chown`·`echo $PID`, 위임 뒤 사용자 쓰기)은 foundation에서 통과했다.
* `select-compiler`는 `/usr/bin/gcc`를 해석한 경로를 돌려준다.
* run-routes는 외부 도구로 go, compiler, tsgk만 쓰므로 GNU와 BSD 도구 차이는 없다. 경로는 모두 `Join-Path`로 만든다.
* macOS `/usr/bin/clang`은 일반 파일 shim이다. build 환경은 `DEVELOPER_DIR`·`SDKROOT`가 있으면 넘기며, foundation macOS native suite가 이 경로(link가 아닌 경로)로 통과했다. 이 shim은 xcrun을 거쳐 호출 이름으로 도구를 고르므로 link로 지정하지 않는다(platform-support의 wrapper 규칙). 그래서 `TestSelectCompilerFreshShell`의 link는 대상과 같은 basename을 쓴다(재리뷰 r12의 MATERIAL. 다른 이름이면 macOS foundation에서 실패할 수 있었다).
* root 권한이 필요한 읽기는 이제 `sysctl`뿐이고 `sudo`로 한다.

push 전 후속(`936eae8`):

* darwin의 linked 사례는 `/usr/bin/clang` shim 대신 `xcrun --find clang`이 돌려주는 실제 clang binary를 link한다. 기대값도 그 실제 파일이다. xcrun이 경로를 주지 못하면 그 하위 사례만 건너뛴다.
* 일반 사례와 linked 사례는 subtest로 나눴다. 기대 경로는 문자열로 비교하지 않는다. 대신 같은 파일인지(`os.SameFile`)와 돌려받은 경로 자체가 link가 아닌지를 본다. PowerShell은 경로 중간의 디렉터리 link(예: `Xcode.app` 별칭)를 남기고 Go는 해석하므로, 문자열로 비교하면 같은 파일이 다른 경로로 보일 수 있다(재리뷰 r14 MINOR). link를 해석하지 않는 script로 되돌리면 linked 사례가 실패한다.
* platform-support에는 wrapper·shim compiler를 link 대상으로 지원하지 않는다고 적었다.
* routes step harness를 CI job env(`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`)로 다시 실행했다. 로컬 `TSGK_*` 변수는 뺐다. 결과는 step exit 0, 사례 137개(PASS 119, FAIL 10, BLOCKED 8), failures 0이었다.

Linux 없이는 확인할 수 없는 위험:

* Linux sanitizer step의 ASan/UBSan/LSan 실행과 420초 예산. 아직 한 번도 끝까지 실행되지 않았다.
* `sudo sysctl -w`와 복원.
* 세 OS의 26 route와 대용량 fixture 실측. Linux cgroup은 page cache를 세고, macOS는 7 GB에 sampled 메모리다.

## acceptance 연결

A01 `TestIncrementalSequence`; A02 `TestMalformedThenRepair`; A03·A04 `TestFaultControlsDetected`; A05 `TestEdgeStructures`, `TestCompareTrees`; A06·A16·A17 `TestEncodingsAndPoints`, `TestApplyEdits`, `TestEncodingSteps`, `TestCP949Table`, `TestCP949TableHeader`; A07 `TestEditRejections`; A08 `TestStatefulScanner`; A09 `TestFrames`, `TestBatchTrailingBytes`; A18 선언 상한 `TestSixtyFourDeclarations`; A10·A13·A19 `TestLimitsAndCancellation`, `TestBatchRecords`; A11 `TestBuildIdentity`; A12 위 mutant; A13 sanitizer는 CI Linux 단계; A14·A22 위 26 route와 `TestNativeRoutesRegistry`, `TestRouteCaseFiles`; A15 `TestOfflineClosure`; A18 `TestSummaryGate`, `TestLargeFixtureIdentity`와 위 대용량 표; A20 위 corpus와 `TestBatchRecords`; A21 `src/testdata/native/dynamic-sql/expected.json`; NET461 SVC는 `TestObserveServiceHost`, `TestObserveServiceHostUTF16`, `TestSvcComposite`와 SVC 사례.

## 남은 일과 한계

* 세 OS CI(foundation native 단계, `native prepare`, `native routes` 세 job, Linux sanitizer)는 PR 단계에서 실행한다. Linux·macOS build·실행, hosted compiler identity, hosted 대용량 측정, Linux 재생성 6 route는 아직 관측하지 않았다. Windows hosted runner에 MinGW나 LLVM이 없으면 compiler를 설치하지 않고 멈춘다.
* sanitizer(ASan/UBSan) 진단은 Windows GCC에서 쓸 수 없어 로컬에서 실행하지 않았다(Linux CI 단계).
* route 재사용 수는 하한이다. `TSNode.id`가 부모 안 subtree slot이라 다시 만든 부모 바로 아래에서 재사용된 leaf는 세지 않는다. 그래서 바뀌지 않는 문맥이 없는 edit(빈 source 삽입)는 route를 증명하지 못한다.
* SVC는 directive 관측과 inline C# included range parse까지다. 관측기는 다섯 진단만 내므로 N461-SVC-MULTILINE 음성 사례(남는 `;`, 빈 값)는 진단 없이 PASS다. C# inline을 parse한 사례의 directive 진단은 기록만 하고 assessment에 반영하지 않는다. inline이 사라지는 step이 있는 사례는 비교하지 않고 `BLOCKED`다. CodeBehind 파일은 찾거나 읽지 않는다(`NOT_RESOLVED`).
* 32 MiB C#은 4 GiB 안에 완료되지 않는다(위 표). driver allocator header 16 bytes도 commit 사용량에 포함된다.
* grammar gap 10건과 corpus ERROR 파일 230개는 S08 처분 대상이다. kit 결함으로 분류한 것은 없다.
* S06 query, dynamic SQL 위치 추출, svc directive/segment query 관측은 범위 밖이다.
