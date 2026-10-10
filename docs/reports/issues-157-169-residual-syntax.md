# C#·T-SQL 잔여 구문·증거 경계 보완 (#157–#166·#168–#171)

기준은 main `e18a819c74715d4cb756305822d71caff717c60f`다. 공개 합성 입력으로 재현한 결함을 원인별 Issue로 분리하고 C2 patch, canonical 계약, native fixture와 generated identity를 함께 갱신했다. Go core는 offline API 경계를 유지한다.

| Issue | 원인과 수정 | 검증에서 구분한 경계 |
|---|---|---|
| [#157](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/157) | 전처리 메시지와 지시문 token의 물리적 줄 경계를 고정 | slash·backslash 메시지 뒤 declaration과 comment의 CST 범위를 보존; recovery·EOF·Unicode newline·raw string 대조 |
| [#158](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/158) | GO의 line guard, 같은 줄 repeat count, 일반 GO identifier 경로 분리 | 다음 줄 숫자는 count가 아님; qualified/delimited 이름과 일반 field/function 경로 보존 |
| [#159](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/159) | IN/VALUES 목록의 필수 element와 scalar 괄호의 단일 expression | CHANGETABLE의 두 구문 구분; grouping tuple·빈 grand-total set은 GROUP BY 문맥에 한정 |
| [#160](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/160) | PP 조건식에서 일반 literal과 verbatim symbol 제거 | Boolean literal·conditional symbol·허용 operator의 우선순위와 EOF 보존 |
| [#161](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/161) | define/undef의 conditional symbol 규칙 분리 | raw true/false와 @symbol 거부; 유효한 Unicode escape spelling 보존 |
| [#162](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/162) | #line 전용 decimal token과 raw quoted filename | enhanced #line 유지; filename의 backslash를 일반 string escape로 해석하지 않음 |
| [#163](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/163) | 빈/일반 pragma text와 known pragma token 경계 | 정상 warning ID를 개별 node로 보존; warning-only compiler 진단을 fatal grammar 오류로 바꾸지 않음 |
| [#164](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/164) | C# command API 비교에 lexical identifier 정규화 적용 | @·유효한 Unicode escape·formatting character만 정규화; 원문 source span은 유지 |
| [#165](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/165) | block comment의 모든 nesting level에서 closing mark 요구 | EOF에서 닫히지 않은 주석을 성공한 extra로 반환하지 않음 |
| [#166](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/166) | CHANGETABLE의 CHANGES/VERSION 인자를 전용 규칙으로 분리 | sync version의 일반 expression 과잉수용 제거; VERSION column 목록과 value 목록 분리; 두 form의 optional FORCESEEK 보존 |
| [#168](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/168) | ROLLUP/CUBE 내부 tuple을 non-empty 전용 규칙으로 분리 | 함수의 빈 tuple 거부; 직접 GROUP BY ()와 GROUPING SETS(())의 정상 grand-total set 보존 |
| [#169](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/169) | 회귀 입력을 긴 정상 source 뒤에 반복 배치해 evidence 읽기 상한 초과 | 기존 353개 history와 모든 기존 source/edit/expect prefix를 보존하고 동일한 107개 추가 입력을 작은 최종 NO_ERROR source 뒤에 재배치; raw/hash 검증과 512 MiB 상한 유지 |
| [#170](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/170) | 오류 history에 변경되지 않는 정상 subtree가 없어 incremental E 의무의 reuse 미관측 | 11개 history 앞에 보호된 정상 sibling을 추가; 원래 모든 source state는 suffix로 보존하고 ERROR 기대값·기존 main edits·E coverage·reuse guard 유지 |

## Unicode 공백 추가 리뷰 (#171)

PR #167의 최종 gate 전 추가 리뷰에서 T-SQL external `_line_space`가 원래 extras의 Unicode 공백을 잃은 결함을 확인했다. [#171](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/171)은 고정 code-point predicate와 keyword boundary를 함께 수정한다. Locale에 따라 결과가 달라지는 C library 분류는 사용하지 않는다. GO의 CR/LF 물리적 줄 구분과 scanner serialization 형식은 유지한다. Scanner만 변경하므로 grammar.js·생성기 입력과 등록된 parser/node-types/header 6개 출력은 동일하다.

SQL Server 2025 LocalDB 17.0.1000.7 PARSEONLY에서 `SELECT 0; SELECT<character>1;`로 30개 code point를 대조했다. 앞 문장은 batch-first implicit EXEC 오인을 방지하며 `SELECT 0; SELECTX1;` 음성도 거부됐다. 공백 26개는 수용, U+180E·U+200C·U+200D·중간 U+FEFF 4개는 오류 102였다. 기존 공개 대조 39개에 공백 30개·QUOTED_IDENTIFIER 9개를 추가해 compatibility level 110·170에서 각 78개, 총 156개 PARSEONLY 대조가 모두 기대값과 일치했다. 임시 DB와 파일은 제거했다. 실제 SQL Server 2012 엔진 검증은 아니다. 집중 native 51개는 모두 PASS다. Unicode SELECT, GO indentation/tail/count, QUOTED_IDENTIFIER OFF/ON, 같은 줄 GO 음성과 UTF-16LE 2개를 포함하며 API·fact reproduction도 모두 PASS다. QUOTED_IDENTIFIER OFF의 double-quoted text는 literal, ON의 이름은 identifier로 실제 CST에 나타난다.

기존 353개 gap history의 모든 source/edit/expect prefix를 보존하며 작은 정상 prefix 뒤에 12개 추가 상태를 6개 history에 배치했다. Unicode SELECT의 정확한 query 범위, OFF literal과 ON identifier의 정확한 source anchor를 등록한다. 최대 4 edits, case 수와 모든 검증 상한을 유지한다. 전체 새 scanner의 native·qualification 및 main 결과는 PR의 실제 검증 기록으로 결속한다.

같은 리뷰의 C# NBSP 지적은 수정 없이 처분했다. Pinned runtime의 `get_column`은 byte extent와 별개로 decoded code point 단위 column을 반환한다. Native 6개(ASCII·NBSP·EM SPACE·NARROW NBSP·UTF-16LE와 같은 줄 directive 음성)가 모두 PASS여서 scanner의 `horizontal++`와 일치한다. Byte 길이로 변경하지 않는다.

## 회귀 fixture 비용

최초 PR CI `37990648300`의 12개 job 중 11개는 성공했으나 qualification은 `RESOURCE_LIMIT:TOTAL_BYTES_LIMIT`로 중단됐다. Windows archive의 raw와 normalized record 합계는 `555,286,805` bytes로 host별 `536,870,912` bytes 상한을 넘었다. 긴 normal-siblings source 뒤의 신규 상태가 큰 CST/API evidence를 반복한 것이 원인이었다. 상한이나 검증을 완화하지 않고 추가 입력의 배치를 수정했다.

107개 public 입력은 같은 source bytes와 구문/구조 기대값을 유지한다. #169 단계에서는 작은 source를 가진 기존 history 54개에서 마지막 NO_ERROR 상태 뒤에 독립 GO batch로 붙이고, unique find/replace와 전체 source의 anchor 위치를 재생성했다. 기존 ERROR 상태를 포함한 모든 원래 history prefix, 최대 4 edits, 전체 case 수와 C1 의무를 보존했다. 신규 상태에서 반복하는 prefix는 총 `119,691` bytes에서 `4,911` bytes로 줄었다. 그 배치의 native S05/S06 전체 검증은 완료됐고 kit failure와 API finding은 0이다. 기존 Windows run의 변하지 않은 필수 member에 당시 T-SQL native 결과를 합산한 크기 예측은 `531,892,569` bytes로, 상한보다 `4,978,343` bytes 작았다. 실제 후속 CI에서는 읽기 상한을 통과했지만, 아래 incremental E 의무가 BLOCKED여서 전체 지원 판정은 통과하지 않았다.

## Incremental 의무의 실제 관측

PR head `246f510`의 [CI 38000320983](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38000320983)은 12개 job 모두 성공했다. 그러나 내려받은 qualification JSON은 `mechanism_gate=PASS`, `completeness=PASS`와 별도로 `support_claim=BLOCKED`였다. T-SQL B01~B05의 E 의무가 세 OS에서 각각 BLOCKED여서 75/78칸과 2,463/2,478 의무만 PASS였다. 구문·API·비교 실패는 0이다. 기준 main `e18a819c`의 실제 qualification은 78칸과 2,478 의무 모두 PASS였다. Job success를 지원 판정으로 대체하지 않는다.

11개 오류 history의 source 앞에 `SELECT NULL;\nSELECT NULL;\nGO\n` 29 bytes를 추가해 변경되지 않는 정상 sibling을 명시한다. 원래 source와 모든 mutation state는 각 단계의 suffix로 byte 단위 보존하며, 원래 ERROR/NO_ERROR·contains·declaration·anchor 의미와 기존 main의 edit 표현을 유지한다. Task가 추가한 edit 5개는 `;` 검색이 prefix와 충돌하지 않도록 변경되지 않는 주변 문자를 함께 포함하는 동등한 find/replace로 확장했다. Anchor occurrence와 C1 byte range는 실제 새 source에서 다시 결속한다. GO는 원래 batch의 시작 위치와 QUOTED_IDENTIFIER setting 문맥을 보존한다. Grammar, runtime, guard, coverage, case 수와 상한은 변경하지 않는다.

같은 parser/runtime의 집중 native 실험은 이 11개 history 모두에서 구문 기대값, fresh/incremental equality와 실제 incremental reuse가 PASS임을 확인했다. 실제 converter를 거친 보완 후 전체 T-SQL 실행도 S05 993개와 S06 1,000개 모두 PASS다. S06의 API·fact reproduction 1,000개는 각각 전부 PASS다. 편집 history 359개의 incremental equality·reuse·query equality는 PASS이고, 편집 없는 641개는 해당 비교를 NOT_CLAIMED로 유지한다. Kit failure·API finding·reuse BLOCKED는 0이다. 원래 형태에서 관측하지 못한 reuse를 PASS로 재표시하지 않고, 새 보호 sibling을 가진 입력의 실제 관측으로 판정한다. 최종 head의 세 OS qualification 및 actual main CI를 완료 gate로 유지한다.

## 재현과 검증

Tree-sitter `0.27.0`, Node `24.21.0`, runtime commit `659cda7c7f86ebe31cc825dc5da59e9add172dc7`과 등록된 runtime patch를 사용한다. 두 grammar의 독립 workspace 생성, deterministic 비교와 등록한 6개 출력의 reference match가 모두 PASS다. Windows amd64의 GCC `16.2.0`에서 공개 C# 161개 및 T-SQL 109개 집중 대조의 구문 기대값, API 일치와 fact reproduction이 모두 PASS다. C# mandatory-body recovery 5개와 ordinary identifier의 정확한 CST 대조 2개도 추가로 PASS다. 기대값에는 ERROR 여부뿐 아니라 후속 declaration, comment, identifier와 warning ID의 정확한 source 범위를 포함한다.

C# S05 전체 292개와 S06 전체 321개가 PASS다. #170 보완 전 T-SQL S05 993개는 PASS 982개와 오류 tree의 reuse BLOCKED 11개였으며, 이 관측 한계를 위 E 의무 문제로 추적했다. C# SVC 전체 행의 assessment는 기존 관측 한계로 BLOCKED였지만, 22개 case는 PASS 14개와 BLOCKED 8개(SVC_INLINE_UNRESOLVED 4, SVC_DIRECTIVE_DIAGNOSTICS 2, SVC_INLINE_NOT_PARSED 1, SVC_INLINE_UNSUPPORTED 1)로 구성됐다. BLOCKED를 PASS로 집계하지 않는다. T-SQL context의 공개 source 726개는 새 parser로 다시 파싱해 실제 CST snapshot을 갱신했고, 기존 source identity·syntax 기대값·engine 관측·context 진단 코드를 보존했다.

#170 보완 전 T-SQL S06 1,000개는 PASS 989개와 같은 reuse BLOCKED 11개였다. 999개의 구문 기대값은 모두 PASS이며, syntax expectation을 주장하지 않는 dynamic SQL support 1개는 fact 기대값이 PASS다. 1,000개 전체 API claim은 PASS이며 kit failure와 API finding은 0이다. 오류 tree의 같은 관측 한계가 S05/S06에 두 번 기록되는 것을 22개 새 parser 결함으로 합산하지 않는다.

SQL Server 2025 LocalDB `17.0.1000.7`의 compatibility level `110`·`170`에서 공개 구문 대조 39개씩, 총 78개가 기대값과 일치했다. 검증은 `PARSEONLY`이며 임시 DB와 파일은 제거했다. 이는 실제 SQL Server 2012 엔진 검증이 아니다. [CHANGETABLE의 공식 구문](https://learn.microsoft.com/en-us/sql/relational-databases/system-functions/changetable-transact-sql?view=sql-server-ver17)에 맞춰 sync version과 column/value 위치를 구분하며, bigint 범위·catalog binding·alias의 오류 22104는 engine 의미 영역으로 분리한다.

등록한 large fixture 3개와 large Oracle의 fact reproduction도 PASS다. 이번 host의 22MiB·8MiB·32MiB parse 관측은 각각 `14,608`·`5,604`·`23,540`ms다. 일반 C# `word` lexer를 복원하고 PP Boolean/pragma keyword를 directive 전용 external token으로 분리하여 prototype의 32MiB 시간 초과를 해소했다. 60초·memory·출력 상한은 유지했다. 별도 over-capture 대조는 의도한 `RESOURCE_LIMIT:OUTPUT_LIMIT` BLOCKED였다. summary Oracle은 API를 활성화하지 않으므로 전체 node API를 별도 audit으로 검증했다. 등록한 large 입력의 33,110,006개 node와 2,185,260,882개 필수 API 검사가 PASS이며 필수 차이는 0이다. 9개 fault 대조도 PASS다. Small malformed/zero-width 입력의 기존 position-navigation 관측 4개는 정책상 별도 진단으로 남으며 필수 API 실패와 혼동하지 않는다.

`CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`에서 전체 `go test ./src/... -count=1 -timeout 300s`, `go vet ./src/...`, `go build ./src/...`, `go mod verify`, gofmt와 diff 검사가 PASS다. 독립 STATIC_REVIEW에서 PP, T-SQL, core/facts와 fixture/identity 등록을 검토했다. pragma prefix, coverage와 문서 등록 지적을 보완한 뒤 미해결 BLOCKER/MATERIAL은 0이다. 문서 경로 정리 후 foundation 재검사도 PASS다.

이 보고서는 공개 합성 입력에 대한 로컬 검증을 기록한다. 전체 node large API audit의 PASS receipt를 확보했으며 최종 head의 세 OS qualification과 병합 후 main CI를 완료 판정 gate로 삼는다. 아직 실행하지 않은 gate를 로컬 PASS로 대체하지 않으며, 해당 판정·producer identity·최종 diff와 finding 처분은 PR의 검증 기록에 결속한다. STATIC_REVIEW는 실행한 검사를 대신하지 않는다.

## 판정의 범위

[C# lexical specification](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure)과 Roslyn `5.0.0.0`의 공개 합성 대조를 사용한다. 불완전한 `#pragma checksum`/warning이 compiler warning만 내는 경우에는 prefix와 opaque tail을 보존한다. Raw #line filename은 `string_literal` leaf이며 일반 string의 escape/content child를 만들지 않는다. Inactive section은 명세의 opaque 처리 규칙을 따른다.

[GO](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/sql-server-utilities-statements-go?view=sql-server-ver17)는 client command다. 닫힌 trailing block comment가 물리적 줄을 넘으면 뒤 SQL은 다음 batch에 속한다. Decimal count의 실행 의미와 특정 SQLCMD 구현의 명령 처리 성공은 parser의 수용 기준과 구분한다. [중첩 block comment](https://learn.microsoft.com/en-us/sql/t-sql/language-elements/slash-star-comment-transact-sql)는 모든 closing mark를 요구한다. 기존 [함수 target 경계](issue-122-123-tsql-boundaries.md)는 별도 문맥이며 GO 일반 이름을 허용한다고 그 경계를 넓히지 않는다.

이 보고서와 `SUPPORTED`는 등록한 입력·문법 mode·검증 정책에 대한 근거다. 임의의 모든 C#·T-SQL에서 결함이 없다는 증명, compiler 의미 분석, catalog/type/name binding 또는 실행 성공 판정은 아니다. 실제 검사하지 않은 입력과 버전은 미검증으로 남는다. 제공된 로컬 corpus의 경로·source·개별 결과는 저장소에 등록하거나 외부로 전송하지 않는다.
