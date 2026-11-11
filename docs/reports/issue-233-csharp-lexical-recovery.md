# #233 C# lexical producer 교정

2026-10-26 KST 기준 로컬 구현·검증 기록이다. #279의 primary full CST 대조가 검출한 네 producer 오판을 수정하며, #233 전체 acceptance는 별도로 유지한다.

| 원본 사례 | 교정 전 추가 지시문 byte | 교정 후 전체 지시문 byte | 활성 구문 기대값 |
|---|---:|---|---|
| `directive-string-no-escape` | 25 | `[0]` | `ERROR` |
| `razor-comment-lines` | 9 | `[18,35]` | `ERROR` |
| `dollars-two-quotes-raw` | 37 | `[]` | `ERROR` |
| `nested-string-open-no-runaway` | 62 | `[]` | `ERROR` |

`#nullable`은 활성 상태의 비선택 지시문이므로 활성 source에도 남는다. 네 사례는 지시문 위치가 일치한 뒤 활성 파싱까지 실행하며 실제 `ERROR` 노드를 유지한다. 오류 source를 정상 source로 바꾸거나 comparator가 오류 노드를 제외하도록 수정한 결과가 아니다.

## 원인과 구현

일반 문자열의 token 사이에서 lexer가 줄바꿈을 건너뛰면 다음 줄의 따옴표가 앞 문자열을 닫았다. 닫는 따옴표·escape를 immediate로 바꾸고 C# 줄 경계를 content에서 제외했다. 지시문 문자열의 잘못된 escape 해석과 raw tail의 줄 소유 범위도 별도 scanner 경계로 처리한다.

Razor comment와 짧은 quote의 잘못된 interpolated raw literal을 일반 token 복구에 맡기면 내부 `#`가 새 지시문이 됐다. recovery 전용 lexical 단위가 comment·literal의 실제 경계를 소비하고 parser의 실제 `ERROR` 아래에 남는다. 임시 walker는 중첩 문자열·hole·group·format의 소유자와 오류 상태를 구분하며, 잘못된 `$@$` prefix 뒤의 다음 실제 문자열도 독립된 문자열 상태로 읽는다.

독립 검토에서 정상 verbatim content의 256개 quote를 임시 byte counter가 거부하는 문제와 중복 `@` prefix를 정상 문자열로 수용하는 문제를 추가로 발견했다. 임시 counter를 넓히고 persistent 필드에 저장할 때만 범위를 확인했다. `$@"x"`·`@$"x"`와 doubled quote 254·256개는 원본 full CST를 보존하며 `@$@"x"`에는 실제 오류를 남긴다.

## 독립 기대값과 재구성

설치된 SDK `10.0.401`의 `Microsoft.CodeAnalysis.CSharp.dll`은 19,191,592 byte, SHA256 `12b0428203faae6555858d2ff856cef7450d2764cf00c57e7516587a04885870`이다. 이 DLL과 compiler·runtime·reference·oracle source를 고정한 새 실행에서 기존157개 typed 관측이 checked-in fixture와 정확히 같았다. 새 경계58개와 독립 검토 반례6개도 같은 고정 oracle로 관측했다. 과거 #218 실행의 진위를 소급 인증하는 근거로 사용하지 않는다.

SDK ProductVersion의 source commit `e34a38d2ae1fc26406a317517196e55c68ff83ab`은 공식 저장소에서 확보하지 못했다. 다른 Roslyn commit이나 현재 공개 source는 기전 설명에만 사용하며 해당 DLL의 정확한 source로 간주하지 않는다.

제품 변경은 `src/dev/c2-patches/csharp.json`의 literal patch chain과 `language-sources`·`native-routes`·`reproduction-routes`의 C# identity에 함께 등록한다. 원본 adoption chain을 재구성한 두 workspace의 생성 결과는 동일했다. prototype은 package metadata를 생략해 parser language version이 `0.0.0`이었고, 실제 pinned metadata `0.23.5`로 생성한 결과와 그 상수만 달랐다. 초기 reference 불일치 결과를 보존한 뒤 정본 metadata에 맞는 identity를 등록하고 두 workspace의 결과를 다시 대조했다.

## 검증과 한계

로컬 registered closure의 primary/active 회귀6개와 정상·오류 문자열 경계18개가 통과했다. 네 교정 사례의 위치는 위 표의 독립 anchor와 같으며 활성 `ERROR`가 실제 node로 남는다. 기존 두 control은 primary 오류와 활성 `NO_ERROR` 기대값의 분리를 계속 확인한다.

최종 등록 소스의 157개 원본 source를 기존 baseline 157개와 full CST로 대조했다. baseline과 candidate가 모두 `NO_ERROR`인 88개 source의 full CST 변화는 0개였다. 157개 전체에서 새 지시문 위치 불일치·기존 ERROR 소실도 모두 0개였다. 고정 Roslyn의 오류 관측이 없더라도 두 native가 이미 `ERROR`인 `format-escaped-quote`는 CST가 달라지므로 이 정상 대조 범위에 포함하지 않는다. Roslyn 기준의 모든 정상 source에 대한 CST 동등성 주장이 아니다. 새 ERROR 두 개인 `dollars-at-raw`와 `at-dollar-at-raw`는 고정 Roslyn에도 각각 오류가 있는 잘못된 prefix다. 두 번째 교정을 예상하지 않은 초기 비교 스크립트의 실패를 보존하고 원 관측을 다시 실행하지 않은 별도 disposition으로 근거를 기록했다. 새 경계58개는 지시문 위치52개 일치, 기존4개 gap, 표현 한도 producer 실패2개였다. 단순한 oracle 위치 차이 수는 unknown directive·file-based mode·멈춘 사례를 포함하므로 qualification defect 수나 전체 semantic 지원률로 사용하지 않는다.

현재 trusted inventory는 기존6set/30사례/29비교step을 유지하고 `PASS26·MISMATCH0·NOT_APPLICABLE4`를 요구한다. 실제 CLI의 `TestQualifySymbolRole`이 이 분모와 축·변조 controls를 63.683초에 통과했다. 등록만으로 실행 PASS를 판단하지 않는다. 역사적 네 mismatch와 위치25·9·37·62는 CI 판정기의 독립 음성 fixture에서 유지하며, 과거 BLOCKED를 임의 PASS로 승격하는 변조를 계속 거부한다.

기존 byte 표현을 넘는 raw quote·dollar·frame 수는 producer 실패다. 예를 들어 raw quote·dollar256은 성공한 CST나 C# syntax rejection으로 기록하지 않는다. 임시 count의 폭을 늘린 것과 persistent 표현 한도의 처분을 구분한다. native 실행은 별도 process에서 감독하며 생성4GiB·180초, compile4GiB·120초, 작은 native 관측1GiB·30초와 제한된 출력을 사용했다.

남은 기존 경계에는 raw `#line` filename, raw q3의 closing q−1/q+1, token을 읽는 `#pragma` raw tail이 있다. 정확한 `#pragma """\n#if HIDDEN\n"""\n#undef X\n`는 고정 Roslyn의 지시문 `[0,27]`과 baseline·candidate의 `[0,12]`가 다르며 두 native full CST는 같다. filename 뒤에 multiline comment가 오는 LF·CRLF·CR·NEL·LS·PS 여섯 경계도 baseline·candidate가 모두 정상으로 수용하고 Roslyn은 오류로 판정한다. 이 여섯 사례는 지시문 위치가 같아도 syntax 의미가 같지는 않음을 보여 준다. 이 기존 결함들을 이번 네 사례 교정의 완료 범위에 포함하지 않는다. #233은 계속 열어 두며 후속 producer 교정·전체 acceptance로 연결한다.

독립 정확성 검토에서 새 lexical 경계·edit/revert 시험이 mandatory CI 선택식에 포함되지 않은 문제를 발견했다. 두 시험을 기존 필수 `TestNativePreprocessCST`의 하위 검사로 연결하며 prepared closure와 build를 공유한다. 초기 step에는 fresh counterpart가 없다는 protocol을 따르고, 두 편집 step의 incremental/fresh 12필드 및 원상 복원 source·초기 CST를 대조한다. CI와 같은 `^TestNativePreprocess(Ranges|CST)$` 선택식의 실제 로컬 실행이 13.773초에 통과했고 두 새 하위 검사의 PASS와 skip 부재를 확인했다. 이 실행의 process는 HARD4GiB·정리확인 잔여0이었다. Foundation 전체는 registered anchor와 나머지로 나누어 모두 통과했으며, CI gate 전체 회귀도 통과했다.

첫 PR CI `40297763711`은 세 플랫폼의 `TestQualifySymbolSetLimits`에서 실패했다. 현재 C# grammar 등록을 바꾼 뒤에도 현재·역사 fixture 세 개가 모두 과거 parser/scanner를 가리켰고, 시험이 세 revision 모두에 현재 등록을 적용했다. `REGISTRATION_MISMATCH`는 올바른 거부였다. 앞선 로컬 컴파일과 선택 시험은 kit 전체 실행을 대신하지 못했다.

현재 fixture의 정상 1사례는 최종 등록 closure와 고정 compiler로 실제 CLI를 다시 실행해 얻었다. 실행은 15.661초에 COMPLETED/PASS, HARD4GiB, 정리 후 잔여0이었다. profile SHA256은 `7cdebb07509faf237540f6b97e9e7003c0d0edd50d0bab94e7b35ba1b85a8e36`이며 source는 기존 74byte·SHA256 `aa0c80936108908705d36dc193ea59e5047f0178b7387016769f4844e8d2c695`를 유지한다. profile·result·active raw·primary raw·source 다섯 원문을 함께 채택하고 기존 두 역사 fixture의 전체 bytes는 보존한다. 역사 시험은 저장소 profile의 독립 고정 SHA256을 검사한 뒤 당시 grammar만 사용하며, 같은 과거 기록을 현재 grammar로 평가하면 FAIL인지도 확인한다. 현재 기록의 잘못된 grammar 역시 거부한다. 공유 byte 예산 대조에는 primary raw 크기와 replay 진입 확인을 포함하며 기존 한도·code 변조·deadline·record 검사는 유지한다.

두 번째 PR CI `40303688974`는 producer job이 모두 성공했으나 최종 qualification에서 78칸 중 C# 세 플랫폼이 실패했다. `csharp-issue157-review-line-multiline-comment-r1`의 `#line 10 /* comment\n*/ "file.cs"`가 기대한 `ERROR`를 잃어 기존 2,478개 의무 중 여섯 개가 실패했다. 외부 filename token이 extras comment 뒤의 quote를 받아들이면서 내부 `token.immediate`의 인접 조건을 잃은 회귀였다. 고정 Roslyn과 기존 native는 multiline·inline comment 뒤 filename을 오류로 판정했다. 기대값과 gate를 유지하고 `#line` filename을 내부 immediate token으로 복원했다. scanner는 잘못된 지시문 tail의 recovery만 맡고 `#pragma checksum`의 별도 내부 filename token은 유지한다.

교정 후 등록 C# corpus를 축소 없이 실행했다. S05 361건·S06 390건 모두 COMPLETED/PASS였고 언어 기대값 실패는 0건이다. S06은 API·fact reproduction 390건, 편집 query equality 54건, query 기대값 28건과 dynamic SQL 1건이 PASS다. 기대값이 없는 한 dynamic SQL 사례의 syntax claim은 NOT_CLAIMED다. SVC 69건은 각각 68 PASS·기존 1 BLOCKED이며 기대값 실패는 없었다. runner의 종료 코드 외에 원본 summary의 기대값·claim·API·requirement 결과를 별도로 검사했다. 이 로컬 관측은 세 플랫폼 공식 qualification을 대신하지 않는다.

최종 grammar로 157개 full CST 차분도 다시 통과했다. 추가 filename 경계19개는 지시문 위치19개 모두 고정 Roslyn 관측과 일치했다. mandatory native 시험은 LF·CRLF·CR·NEL·LS·PS comment 경계, 정상 filename·enhanced line 및 multiline·inline comment 삽입과 복귀를 포함한다. 두 새 이력은 정상→ERROR→정상과 incremental/fresh full CST 및 초기 CST 복원을 요구한다. 역사 fixture의 문법·원본 기대값·자원 한도를 변경하지 않는다.

macOS symbol reference 검증은 사용자 결정으로 실행하지 않는다. 기존 mandatory baseline의 플랫폼 gate는 해당 계약을 유지한다. 수정 후 필수 PR·main CI와 전체 #233 acceptance 완료는 아직 주장하지 않는다.

## 2026-11-11 후속 제품 반영: C1–C4 (#233)

통합 후보 `all-v3`는 남아 있던 raw filename·raw quote closing·pragma raw tail·filename 뒤 block comment 경계를 제품 C2 patch chain에 반영하는 기준이다. 아래 계약은 기존 2026-10-26의 #279 네 회귀 결과를 바꾸지 않으며, 그때의 제품 실행 기록과 이번 잔여 교정을 구분한다.

`#line` raw filename은 normal filename의 `string_literal` leaf/span을 보존하면서 raw token을 오류로 남겨야 한다. Roslyn은 `filename-raw`에서 directive hash `[0,27,44]`와 `CS8996 [8,8)`를 관측한다. 내부 `#if HIDDEN`을 지시문으로 노출해서는 안 된다. q3 raw의 q−1 single-line 닫힘은 물리 newline에서 오류 token을 끝내고 다음 실제 directive를 살려야 하며, q−1 multiline은 충분한 닫힘 또는 EOF까지 소유한다. q+1은 literal 경계를 닫은 뒤 남은 quote의 오류를 보존한다. 정상 raw·hole·brace 동작과 persistent frame 제한은 별도 보존 조건이다.

원본157 처분에서 의도한 full CST 변경은 raw 두 건, pragma 두 건과 `trailing-line-comment` 한 건, 총 다섯 건이다. all-v3 당시 네 건에 일반 directive `//` 수용으로 교정된 한 건을 명시 승인했다. Roslyn 기준은 SDK `10.0.401`의 `Microsoft.CodeAnalysis.CSharp.dll` 5.9.0.0, SHA256 `12b0428203faae6555858d2ff856cef7450d2764cf00c57e7516587a04885870`이다.

| 사례 | baseline → 제품 후보 | 고정 Roslyn 근거 |
|---|---|---|
| `raw-single-line-unterminated` | `[]` / ERROR → `[10,20,37]` / ERROR | CS8997·CS1002 `[9,9)`, CS1032 `[11,17)`; 실제 directive line end 20/26/44 |
| `interp-raw-single-line-unterminated` | `[]` / ERROR → `[14]` / ERROR | CS8997·CS1002 `[13,13)`, CS1032 `[15,21)`; 실제 define line end 24 |
| `pragma-raw-takes-lines` | `[0,28]` / ERROR → `[0,38,55]` / NO_ERROR | `errors=[]`; pragma가 `[0,38)`을 소유하고 내부 `#if`를 가림 |
| `inactive-pragma-raw-takes-endif` | `[0,7,35]` / ERROR → `[0,7,57]` / NO_ERROR | `errors=[]`; raw가 `[7,46)`을 소유하고 실제 `#endif` line end는 64 |
| `trailing-line-comment` | `[0,25]` / ERROR → `[0,25]` / NO_ERROR | `#if A // note\nclass X {}\n#endif\n`; `errors=[]`; directive 위치 유지 |

따라서 원본157에서 허용되는 ERROR 소실은 Roslyn에 오류가 없는 두 pragma 사례와 `trailing-line-comment` 한 사례뿐이며, raw 두 사례는 ERROR를 유지한다. 다섯 원본의 source SHA256을 허용 목록으로 고정하고 그 밖의 변화는 거부한다. `#line` raw filename 및 `#line <filename> /* … */` 뒤의 LF·CRLF·CR·NEL·LS·PS 경계는 이 다섯 intentional correction과 혼동하지 않고 각각 별도 보존·오류 대조로 둔다. 후자의 여섯 사례는 Roslyn `CS1025 [19,20)` 및 actual ERROR를 요구한다. `#pragma` raw와 inactive pragma raw는 raw가 소유한 줄 안의 가짜 directive를 숨기고 그 뒤 actual directive만 인식해야 한다.

scratch `all-v3` 대조는 원본157에서 baseline의 `NO_ERROR`였던 88개 full CST 변화 0, 네 intentional CST 변화, 추가로 실행한 별도 경계 입력을 각각 보고했다. 확장 입력은 원본157 분모에 더하지 않는다. 이는 scratch 후보 evidence이며 제품 검증 receipt가 아니다. 이 보고서의 이번 변경은 validation contract를 정리한 기록이고, 제품 local mandatory Go/native·CLI·kit·ci-gate·S05/S06/SVC 시험, full foundation, 두 독립 리뷰 및 PR/main CI는 별도로 완료 기록해야 한다. scratch는 모든 C# input에 대한 Roslyn 동등성, 모든 nested scanner state의 증명, 독립 reconstruction 두 벌을 입증하지 않는다. 기존 oracle mismatch 축과 256 quote/dollar producer failure 경계는 계속 별도 처분한다.

## 2026-11-11 1차 제품 이관 결과 (#233, 아래 후속 교정으로 해소)

제품 C2 literal patch를 두 개의 생성 workspace에서 재구성하고, 등록 전 `prepare-routes.ps1 -RecordOutputs`와 등록된 출력 검사를 각각 실행했다. 두 workspace의 생성물은 모두 `all-v3` identity와 정확히 일치했고 등록 전 수집과 등록 후 검증도 통과했다. 등록된 grammar로 diagnostic 157 사례를 다시 실행한 결과 baseline과 둘 다 `NO_ERROR`였던 88개 full CST 차분은 0이었다. 변경은 의도한 raw 두 사례와 pragma 두 사례뿐이고, ERROR 소실도 Roslyn 오류가 없는 pragma 두 사례에 한정됐다.

1차 이관 시점의 제품 수용은 BLOCKED였다(아래 두 후속 교정으로 해소). `run-routes.ps1 -Routes csharp -Oracle -Symbols`의 사례별 결과는 다음과 같다.

| 경로 | 결과 |
|---|---|
| S05 C# | 361 중 350 PASS, 11 FAIL |
| S06 C# | 390 중 379 PASS, 11 FAIL |
| SVC | 69 중 64 PASS, 4 FAIL, 기존 1 BLOCKED |
| C# symbols | 30 중 25 PASS, 5 registered BLOCKED; 신규 mismatch 없음 |

11개의 C# expectation failure와 SVC 4개 FAIL은 directive 뒤 comment 처리에 모여 있다. 재구성 scanner의 `scan_comment_token`은 `src/scanner.c`에서 `in_directive && !pp_line && !recovering`일 때 external `COMMENT`를 거부한다. 같은 변경에서 internal `comment` 규칙을 제거했으므로 `#define`, `#undef`, `#endif` 뒤의 유효 trailing line comment가 CST에 들어오지 않고 root `has_error` 기대를 깨뜨린다. `#line`의 `pp_line` 경로와 opaque pragma tail은 별도 분기여서 scratch 통과로 이 축이 덮이지 않았다. route summary의 `failures=[]` 및 wrapper 종료 코드 0은 route별 `assessment=FAIL`과 15개 C#/SVC expectation failure를 상쇄하지 않는다.

현행 symbol fixture는 최종 grammar로 CLI를 실제 실행해 다시 기록했다. `qualify-symbols-r4`·`r5`의 원본 bytes도 보존했다. 이번 제품 작업에서는 기준 후보의 `all-v3` scanner identity를 유지하므로 이 보고서에서 추가 scanner 수정을 적용하지 않았으며, trailing directive comment 회귀 때문에 구현을 채택 완료로 판정하지 않는다. 필수 native/Go 회귀와 foundation 전체, 두 관점의 독립 리뷰 및 PR/main CI 결과는 별도 완료 전에 남아 있다.

제품 검증 후속 기록: `gofmt`와 `CGO_ENABLED=0 go vet ./src/...`, CLI의 `TestIncrementalCLISymbolMode`·`TestQualifySymbolRole`(두 top-level 모두 PASS, skip 0), kit의 C# 전처리·현재/역사 symbol fixture·qualification 검사 및 ci-gate는 통과했다. kit의 수동 differential 검사는 환경 미지정 SKIP이며 실행 PASS로 세지 않는다. mandatory native에서는 `TestNativePreprocessRanges`가 PASS이고, `TestNativePreprocessCST`의 기존18 lexical controls·Roslyn residual20개·기존6 및 추가5 edit/revert 이력은 통과했다. 이 parent의 `registered-define-trailing-comment`만 기존 등록의 NO_ERROR 기대값을 실패하므로 mandatory native 전체는 FAIL, skip은 0이다. malformed/zero-active source의 lexical 대조는 full-input tree로 검사하며 symbol-mode의 정당한 BLOCKED를 PASS로 바꾸지 않는다.

current fixture의 profile은 LF bytes를 실행 전에 고정한 뒤 실제 `tsgk incremental`을 새로 실행해 재기록했다. 원본 route의 CRLF profile bytes를 실행 후 바꾸고 result hash만 치환하는 방식은 쓰지 않았다. 두 독립 검토는 lexical/recovery와 evidence/state 관점에서 같은 comment 회귀를 제품 차단 finding으로 처분했다. source-only 결과·로컬 시험은 PR/main CI나 #233 전체 acceptance를 대신하지 않는다.

전체 Go suite의 기본 경로는 HARD4GiB에서 처음 병렬 구간의 memory limit에 도달했고 정리·잔류0을 확인했다. native-routes 전체 pin을 소비하는 네 Small17/Web16 검사에서 stale SHA를 갱신한 뒤 한도는 그대로 `-p 1 -parallel 1`로 재실행했다. 최종 실행은 769.606초·peak969.570MiB·cleanup verified/잔류0이며 native·kit·ci-gate를 포함한 나머지 패키지는 통과했다. CLI의 `TestPublishWindowsShortAliasFinal`은 이 host의 8.3 alias fixture 부재, foundation의 `TestWorkflowPostMergeReuseDecision/unresponsive-native-gh`는 기대한 fallback log 부재로 실패했다. 호스트 volume 설정이나 기존 보호 검사를 바꾸지 않았다. 기본 suite의 prepared 미지정 native SKIP은 별도 mandatory prepared 검사의 C4 FAIL을 대신하지 않는다.

#362 P0 PR #365는 읽기 전용 조회 당시 open/merged=false이고 지정 worktree에는 semantic registry가 없다. P0 merge 뒤 재생성 필요: `go run ./src/dev/query-registry -write`와 foundation registry 검사를 통합 뒤 수행한다. CI·PR/main 통합과 제품 채택은 아직 완료되지 않았다.

## 2026-11-11 directive comment guard 후속 교정 (#233)

`scan_comment_token`의 `in_directive && !pp_line && !recovering` 거부를 `//` 반환 뒤, `/*` 처리 앞에 옮겼다. `#define`·`#undef`·`#endif`를 포함한 directive의 single-line comment를 다시 수용하며, `#line` filename 뒤 block comment의 실제 ERROR 및 다른 directive의 기존 block-comment 오류/recovery 경계는 유지한다. 기존 등록 define 양성 대조에 undef·endif line comment 및 if block comment의 세 대조를 추가했다.

새 scanner는 61,035 bytes, SHA256 `34a93834b32ab7c8169555a5fc52dd61d104e3a8c282e98ecc7b450d86e42903`이다. 두 생성 workspace와 등록 후 normal prepare가 통과했고 parser·grammar.json·node-types 및 공유 header identity는 직전 all-v3와 같다. qualification 및 source-bound pin을 같은 canonical 생성 경로로 갱신했다. current symbol fixture는 LF profile을 실행 전에 고정하고 실제 CLI로 다시 기록했으며 원본 source와 역사 fixture는 보존했다.

제품 사례별 판정은 S05 C# 361/361 PASS, S06 C# 390/390 PASS, S05/S06 SVC 각각 68 PASS·기존 1 BLOCKED·FAIL 0이다. csharp-symbols 6 set·30 case는 25 PASS·등록된 5 BLOCKED이며 등록 outcome과 일치한다. wrapper exit만으로 판정하지 않았다.

다만 이번 brief의 원본157 strict 처분은 FAIL로 남긴다. 새 identity로 scratch424를 실행해 정상88 full CST 변화0, 명시 검사한 C1–C4 관문, q+1 여섯 span, edit40 full CST equality, 상태32 왕복·reset 및 invalid/cap3 거부는 유지했다. 그러나 원본 `trailing-line-comment`의 `#if A // note`도 baseline/all-v3 ERROR에서 NO_ERROR로 바뀌었다. 고정 Roslyn의 errors는 비어 있지만 기존 허용4건 밖이므로 원본157의 full CST 변화는5, ERROR 소실은3이다. 별도 extended `pragma-line-comment`는 오류 유무와 directive 위치를 유지하지만 기존 `preproc_arg [28,35)`가 같은 범위의 named `comment` extra로 바뀐다. free-text pragma/comment의 full CST 보존을 포괄적으로 주장할 수 없으며 이 차분도 별도 처분이 필요하다. 기대값·사례·비교기를 바꾸어 strict4 통과로 만들지 않았다. 따라서 제품 comment 회귀의 해소와 별개로 후보 전체 채택 및 #233 acceptance 완료는 선언하지 않는다. 후속 처분에는 일반 directive `//` 수용과 추가 원본 교정의 관계를 명시해야 한다. 이 기록은 PR/main CI나 새 Roslyn 실행의 근거가 아니다.

## 2026-11-11 pragma `//` 꼬리 CST 보존 후속 교정 (#233)

직전 directive comment 교정에서 정상 `#pragma warning disable 123 // text`의 `preproc_arg [28,35)`가 `comment` extra로 바뀌는 호환성 회귀가 확인됐다. `scan_comment_token`은 `in_directive && pp_pragma_mode && !recovering`이면 선점하지 않고 기존 tail lexer에 맡긴다. define·undef·endif·if는 pragma mode가 아니므로 정상 `//` comment 경로를 유지한다. 복구 호출과 기존 block-comment 보호 조건, scanner 직렬화 형식은 유지한다. mandatory native lexical-boundary에 해당 source의 NO_ERROR와 정확한 `preproc_arg` 범위·named/non-extra 소유권 및 comment 부재 검사를 추가했다.

새 scanner는 61,112 bytes, SHA256 `8ed217a313908371b8941d2f9586aff8750b113785c4a07aa9b86def55ffb36b`이다. 허용5건/ERROR 소실3건 처분은 위 표와 validation 계약을 따른다. 직전 절의 strict4 FAIL은 당시 brief의 실제 결과로 보존하며 새 후보의 결과와 혼동하지 않는다. 이 후속 교정의 최종 로컬 검증 기록은 아래 결과를 따르며, PR/main CI와 #233 전체 acceptance는 별도다.

최종 새 identity 검증에서 원본157 변화는 승인5 source SHA뿐이고 ERROR 소실은 승인3 source SHA뿐이었다. 정상88 full CST 변화0, extended pragma-line-comment baseline full CST 동일, scratch424의22 gate·edit40·상태32왕복과 invalid3거부·별도 newline18은 모두 PASS다. 제품 S05 C#361·S06 C#390 전부 PASS, SVC69는 S05/S06 각각68 PASS·기존1 BLOCKED·FAIL0, symbols30은25 PASS·등록5 BLOCKED로 정확히 일치했다. current fixture는 실행 전 고정한 LF profile을 새 실제 CLI로 기록했고 역사/원본11파일은 보존했다.

gofmt·go vet·mandatory native preprocess/CLI symbols·kit 영향 검사·classification-native·ci-gate는 PASS다. 명시적 capture 환경이 없는 수동 kit5개/native2개는 SKIP이다. 동일 HARD4GiB/300초의 foundation 전체는 기존 `unresponsive-native-gh`만 FAIL(599.2734ms, fallback 로그 부재), top-level58 PASS·SKIP0이었다. 별도 환경 대조에서 mixed-slash `ComSpec`의 `.cmd` 인자 전달이 `--jq` 안의 `|`를 cmd 파이프로 해석해 exit255로 끝나고 hang shim에 진입하지 못하는 것을 확인했다. 자식 ComSpec만 역슬래시로 정규화하면 shim hang과 실제 미수정 leaf의 PASS(1.93초)가 확인됐다. 전체 foundation의 최초 FAIL은 유지하며 시험·workflow·보호 조건을 바꾸지 않았다.

두 독립 STATIC_REVIEW에서 parser/CST와 identity/evidence를 서로 다른 관점으로 교차 확인했고 blocking finding은 없었다. outer receipt509개 모두 HARD·peak_known·cleanup verified·잔류0, timeout/메모리 hit/출력 잘림0이며 최대 Job committed peak는1,488.105MiB다. 이는 로컬 지정 관문의 근거이며 PR/main CI·Linux/macOS/sanitizer·새 Roslyn/실제 semantic capture·#233 전체 acceptance는 미완료다. LF 정상 pragma 회귀와 고정 scratch의 성공을 malformed/inactive/nested·모든 newline/pragma-comment 조합의 보편적 증명으로 확대하지 않는다.


## 2026-11-12 pragma verbatim quote 경계 회귀 교정 (#233)

PR #369의 후속 P2는 `scan_pragma_raw_tail`이 `@"a"""""`의 첫 content quote에서 일반 문자열처럼 닫고 남은 quote를 multiline raw opener로 오인하는 회귀다. 수정 전 실제 native에서 정상30개 중 세 verbatim prefix의 doubled quote가 두 쌍 이상인18개는 다음 `#undef`와 후속 class를 pragma tail로 삼켰다. NO_ERROR만으로는 이 소실을 검출할 수 없었다.

helper는 raw quote run 검사 전에 `@`/`$@`/`@$` prefix와 doubled quote를 넘긴다. 일반 문자열의 backslash escape와 char literal 처리를 보존하고, 기존 first-physical-line 종료·raw tail 내부 directive 차단·두 caller·직렬화 형식과 자원 한도는 유지한다. scanner는61,493 bytes / SHA256 `8d00d37d375fc680d7cb728ee7e3cace7818b52c2591b42b6cafab4852ae51ed`다. 두 생성 workspace의 출력6개는 product3 pin과 같으며 등록 상태의 normal prepare도 PASS다. language/native/qualification scanner pin, qualification/query-slot inventory, P0 registry 원문 pin 및 파생 registry와 source-bound opt-in 시험 pin을 갱신했다. 기존 C# capture ABI의662 symbols(658+4 alias)·26 fields는 그대로다.

새 fixture30개는 정상 pragma/arg/다음 `#undef` span을 검사하고 모두 PASS다. 이전 정상12개 full CST는 같고 소실18개를 복원했다. scratch424는 CST 생성 성공422개와 기존 producer failure2개(exit2)를 구분하며22 gate가 PASS다. 정상 원본88 full CST 차분0, 승인5 source SHA/ERROR 소실3 source SHA 정확한 일치, edit40 equality, 상태32왕복·invalid3거부, 별도 newline18을 유지했다. 새 Roslyn 실행이나 승인 집합 추가는 없다. 새 회귀의 기대값은 C# 명세의 문자열/char 경계와 pragma 물리행에 근거하며 기존 고정 Roslyn 관측과 구분한다.

제품 S05 C#361·S06 C#390은 전부 PASS, SVC69는 각68 PASS·기존1 BLOCKED·FAIL0이며 사례별 expectation 행을 직접 대조했다. symbols6set/30case의25 PASS·등록5 BLOCKED가 정확히 일치한다. current symbol fixture는 실행 전 LF profile을 고정한 실제 CLI로 재기록했고 원본/역사11파일은 그대로다. gofmt·vet·mandatory native preprocess와 CLI symbols·kit 영향 검사·classification-native·ci-gate·queryregistry·foundation 전체는 exit0다. foundation은62 top-level PASS·SKIP0, 수동 환경 없는 kit5개/native2개는 SKIP로 구분했다. product3 환경 진단에 따라 이번 자식 process의 ComSpec만 정상 Windows 역슬래시 경로로 지정했으며 host·시험·workflow를 수정하지 않았다.

outer receipt573개 모두 HARD·peak_known·cleanup verified/잔류0, timeout·memory limit hit·출력 잘림0이다. 최대 Job committed peak는1,274.293MiB이며 nonzero5개는 위 producer failure2와 invalid state3이다. 이 기록은 지정된 로컬 회귀 교정의 근거다. PR/main CI, Linux/macOS/sanitizer, 새 Roslyn 및 실제 query reference capture/승인, 보간 expression 내부의 모든 중첩 문자열·char 조합과 #233 전체 acceptance를 입증하지 않는다.
