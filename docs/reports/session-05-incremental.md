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
| targeted mutant(A12) | 21/21 검출(`mutants-fb8934a.json`, 리뷰 수정 뒤 `mutants-58f079b.json`도 21/21; 마지막 수정 `02577a1`은 SVC 판정·policy 문구만 바꿨고 이 후보의 mutant 재실행은 하지 않았다). 마지막 step만 비교, 정렬·중복 제거 후 비교, 누락 flag 기본값, route 재사용·fresh 독립 확인 제거, trailing byte·digest·exit·step 수 검사 제거, edit old 검사 제거, Unicode scalar column, cp949 AMBIGUOUS 무시, progress 취소·allocator 상한·depth 상한 제거, cp949 decode 폭, 홀수 UTF-16 수용, summary gate 무시, CR 정규화, batch trailing 무시. 첫 실행(`4b73cdc`)은 18/21이었다: 한 mutant는 컴파일되지 않았고(검출 실패 아님), allocator mutant는 realloc 경로가 같은 한도를 지켜 동등했으며, chunk 크기 mutant는 runtime이 chunk 경계를 이어 읽어 동등했다. 앞의 둘은 의미 있는 형태로 고쳤고 마지막은 decode 폭 mutant로 바꿨다. decode 폭을 관측하려고 owned plain grammar의 identifier를 Unicode 문자로 넓혀 parser를 다시 생성했다 |
| 공개 offline closure(A15) | `TestOfflineClosure` 통과: 공개 package closure에 runner·`os/exec`·network가 없다. native 도구가 없으면 owned native 시험은 skip이고 기존 CLI/API 시험은 그대로 통과한다(CI는 `TSGK_NATIVE_REQUIRED=1`) |
| cp949 표(A16) | 내장 표 identity·고지 시험, driver header가 표의 기계 변환인지 시험, AMBIGUOUS·표 밖 쌍·선언 판정 시험 |
| 등록부·fixture | 26 route registry가 adoption hash와 재현 기준 parser를 가리키는지, 대용량 fixture를 Go에서 독립 재생성해 크기·sha256이 같은지, 사례 파일 형식을 시험 |

## 26 route (A14·A22, Windows 로컬)

`prepare-routes.ps1`이 S01이 결속한 source에서 26 route 입력을 만들었다. 채택 6 route는 patch chain을 적용해 모든 adoption hash가 일치했고, typescript는 `tsgk reproduce`로 다시 생성해 6개 출력이 모두 등록 기준과 같았다(PASS, npm `tree-sitter-javascript@0.23.1` 632551 bytes 다운로드). 나머지 5개 채택 route는 S04 출력을 같은 hash로 재사용했다. 최종 후보 `02577a1`의 `run-routes.ps1` 결과(`routes-windows-02577a1.json`; `fb8934a`, `e81fa10` 결과도 보존):

* 26 route 모두 GCC로 build됐다. build 이식성 patch는 필요 없었다(parser compile 최장 PostgreSQL 6.9초).
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
| `cs-large-32mib-errors` | 33554432 | `RESOURCE_LIMIT`(`MEMORY_LIMIT`, Job Object hard cap 4 GiB) | — | 13.5초 | 4.30 GB |

시간은 `e81fa10` 실행 값이고 괄호는 같은 입력의 최종 후보 실행 값이다. 최종 실행 동안 host에 다른 부하가 있어 시간만 늘었고 상태·node 수·digest 판정은 같았다(시간은 host 관측값이며 비교 대상이 아니다). 32 MiB C#은 이 grammar에서 4 GiB 안에 parse되지 않는다. S05-A18에 따라 이 결과를 그대로 기록하고 상한은 올리지 않는다. 값을 올릴지는 사용자 결정이다. 48 MiB를 넘는 encoded 요청은 실행 전 `REQUEST_TOO_LARGE`, driver frame 머리는 `FRAME_TOO_LARGE`다. 세 host 측정은 `native routes` job이 같은 fixture로 한다(실행 전). 깊은 중첩은 owned grammar로 foundation의 세 OS 시험에 들어 있다.

## 비공개 corpus(A20, Windows 로컬, 개수만)

`NET461-PHASE2-LOCAL-r1`을 clean 후보 `58f079b`에서 `run-corpus.ps1`로 실행했다(6분 34초, 실행 wall 3600초 안, 호출마다 남은 시간을 `--run-wall`로 줌). 리뷰 수정 전 `fb8934a`의 실행도 같은 개수였다. 경로·이름·내용이 담긴 기록은 추적하지 않는 로컬 artifacts에만 있다.

* inventory 21451 record: route 있는 비`PRESENCE_ONLY` 17957, `PRESENCE_ONLY` 133, route 없음 3494. encoding: UTF-8(BOM 13371, 검증 1146), UTF-16LE(BOM) 2789, CP949(검증) 651. route 없는 파일 중 AMBIGUOUS 2, NUL_WITHOUT_BOM 1292.
* route별 batch: csharp 6624 파일·14 process, tsql 6150·13, xml 5163·11. svc 20 파일은 모두 inline 없는 directive라 driver process 없이 관측만 했다. 17957 파일 모두 `COMPLETED`, `NOT_RUN` 0, 치명 frame 0, kit 결함 0.
* `has_error`: csharp 48(그중 자동 생성 파일 45), tsql 181, xml 1. svc는 tree를 parse하지 않았으므로 관측값이 없다. 이 230개 ERROR 파일의 개별 처분은 S08 판정이다. 표본 진단에서 tsql 오류는 한글 식별자(별칭·변수·열 이름) 위치였고 decode된 글자는 정확했다. UTF-8 파일에서도 같은 형태가 나와 encoding이 아니라 grammar의 비ASCII 식별자 미지원으로 본다.
* full tree gate: 517개 파일이 descendant 50000을 넘었고 모두 완료됐다(보존은 record만).
* svc 20개는 모두 directive와 CodeBehind 관측, inline 없음(`inline: ABSENT`)이다.
* 최종 후보 `02577a1`의 corpus 실행은 csharp 단계 중 host 메모리 부족으로 Claude Code가 중단시켰다. 지시대로 다시 시작하지 않았고 남은 process가 없음을 확인했으며 부분 출력은 보존했다(`private-corpus-02577a1-attempt-killed-low-memory`). 따라서 완료된 corpus 실행은 `58f079b`가 마지막이다. `58f079b`와 `02577a1` 사이에는 parse 경로 변경이 없지만, SVC 판정(진단·`%>` 없음은 BLOCKED)과 policy identity 문구가 바뀌었다. corpus의 svc 20개가 새 판정에서도 PASS인지는 관측하지 않았다.
* helper 결과 취합 결함으로 네 번의 시도가 완전한 기록을 남기지 못했다: `0685eaf`·`9c047c1`은 parse를 마친 뒤 strict mode 속성 접근 오류로 멈췄고, `e81fa10`은 주석 처리 실수로 `has_error`를 기록하지 않았다. 모두 보존했다(`private-corpus-*-attempt*`). parse 자체의 결과 개수는 시도마다 같았다.

## 분리 context 리뷰와 처분

`5aee01d..861e215`에 대한 분리 context 리뷰(EXECUTED_REVIEW, 별도 general-purpose subagent, Windows에서 전체 `go test`와 native frame 시험 실행; sanitizer·세 OS CI는 미실행; GitHub 승인 아님)는 BLOCKER 1, MATERIAL 3, MINOR 6, NOTE 5를 보고했다(`artifacts/.../session-05/review-r1.json`). 처분은 `e81fa10`이다.

* BLOCKER R1-B1: driver가 included range의 끝을 검사하기 전에 point를 계산해 source 밖을 읽었다 → 범위 검사를 먼저 하고, 4000000000 끝 범위 거부 시험을 더했다.
* MATERIAL R1-M1: 선언 64개일 때 계산 표시 bit와 64번째 선언 bit가 겹쳤다 → 별도 계산 표시, `TestSixtyFourDeclarations`.
* MATERIAL R1-M2: 빈 buffer를 `memcpy(NULL, 0)`으로 넘겨 UBSan에서 멈출 수 있었다 → 길이 0이면 복사하지 않는다.
* MATERIAL R1-M3: parse하지 않은 `.svc` inline(Language 생략·VB·directive 없음·일부 step만 C#)이 PASS였다 → directive만 있는 source만 PASS, 나머지는 code를 붙여 BLOCKED. 계약 문구와 시험을 고쳤다.
* MINOR R1-m1~m6: batch 배열 null과 parse하지 않은 step의 `has_error` 기록, cleanup 실패를 다른 상태보다 먼저 판정, partial tree 최하위 node의 field, 응답 파일 이름 충돌과 쓰기 실패 exit, 응답 code와 첫 미완료 tree의 일치, corpus 전체 wall(`--run-wall`).
* NOTE R1-n1~n4: locator 구분자 일치, protocol 오류 시 frame buffer 해제, route helper의 BLOCKED claim·대용량 build 실패 집계, build identity와 executable bytes 관계(같은 identity로 다시 build하면 Windows PE bytes가 달라질 수 있어 executable hash를 build마다 기록) — 모두 반영했다.

재리뷰 r2(`861e215..52ef70c`, EXECUTED, `review-r2.json`)는 R1의 13건을 RESOLVED로 확인했다. 남은 R1-M3에서 `%>` 없는 `.svc`가 PASS인 경로를 MATERIAL R2-M1로, 줄인 run wall을 결과에 남기지 않는 점을 MINOR로, NOTE 2건을 보고했다. 처분 `02577a1`: `%>`가 없으면 inline `UNRESOLVED`와 `SVC_INLINE_UNRESOLVED`, directive 진단이 있으면 관측 전용 판정에서 `SVC_DIRECTIVE_DIAGNOSTICS`(BLOCKED), 줄인 run wall을 결과 operation과 policy identity에 기록, 관측하지 않은 `has_error`를 null로 표기. 재리뷰 r3(`52ef70c..02577a1`, STATIC: host 메모리 부족으로 시험은 실행하지 않음)은 R2 4건을 모두 RESOLVED로 확인했고 BLOCKER·MATERIAL은 0이다. NOTE 3건은 다음과 같이 처분했다: C# inline을 parse한 사례에는 directive 진단을 assessment에 반영하지 않는다는 범위를 계약에 명시했다. 관측기가 이름 아닌 문자·빈 값을 진단하지 않아 `n461-svc-multiline-n-r1` 음성 사례가 PASS인 점은 한계로 남긴다. `58f079b` 공개 summary의 `svc:` 키는 수정 전 산출물이라 다시 만들지 않았다.

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
