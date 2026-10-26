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
