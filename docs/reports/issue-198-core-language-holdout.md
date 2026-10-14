# 핵심 언어의 고정 입력 대조 (#198)

[English](issue-198-core-language-holdout.en.md). [#198](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/198)은 소비자 후보 언어의 구문 수용, 문맥 차이, 오류 회복과 편집 복원을 측정한다. **Go에서 실제 식별자 과잉 수용 1건을 발견했다.** C# 조건부 분기와 T-SQL 버전·quoted identifier 문맥은 별도 관측이며, C/C++ compiler 진단을 raw syntax 정확도로 세지 않는다. 이 결과는 기존 [26-route 지원 판정](../specs/platform-support.md)을 대체하지 않는다.

## 후보와 방법

2026-10-14, Windows/amd64에서 main `cdc0564ce7e072d90aa64bc24bf23606ac4df2d2`의 CLI를 Go1.27.2·`CGO_ENABLED=0`으로 빌드했다. CLI SHA256은 `ba1a959585d6237d54962df31d86a193a5dc9716aaa748f09a83cba6372b530e`이다. module floor와 기존 CI의 Go1.27.1은 유지했다. Go1.27.2의 별도 전체 Go suite·vet·build 호환성 검사도 통과했으며 성능 비교는 아니다.

입력·label·profile·source·oracle 문맥을 native 실행 전에 고정했다. 82개 language-case pair는 Go32, C#14, C9, C++9, T-SQL18이며 raw source bytes의 서로 다른 값은81개다. C/C++의 한 입력이 중복된다. 새 입력 중 5개에는 invalid 원본 → 닫는 brace/bracket 보완 → 원본 복원의 두 edit를 등록했다. SDK 버전별 관측을 새 입력 수로 세지 않는다. 소비자 도입 우선순위는 provisional이며 paused go-treesitter 저장소는 실행·수정하지 않았다.

native는 기존 `tsgk-oracle/r2`, `native-query`, `output=tree`, `api=false`, 빈 query 목록을 사용했다. grammar·runtime을 별도 process에서 컴파일·실행하며 core에 Cgo 또는 소비자 의존성을 넣지 않았다. 고정 [native registry](../../src/contracts/native-routes.json)의 준비된 source 107개(5 grammar24개와 runtime83개)를 실행 전후 hash 대조했다. 기존 reference checkout은 수정하지 않았다.

| route | 고정 upstream commit | 독립 oracle와 실제 문맥 |
|---|---|---|
| Go | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` | Go1.27.2 `go/parser`·`go/scanner`; 기존 Go1.27.1 결과와 수용·scanner 진단 수·implicit semicolon offset 일치 |
| C# | `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` | .NET SDK10.0.401, Roslyn5.9, runtime10.0.12; `Regular`, C#7.3와14, 사례별 symbols 보존 |
| C | `b780e47fc780ddc8da13afa35a3f4ed5c157823d` | GCC16.2.0, C99와C23; 고정 compiler context |
| C++ | `c009222808634c1014f82438d4883753516a2c24` | G++16.2.0, C++11과C++23; 고정 compiler context |
| T-SQL | `8620fbcfca9438e1ff7104835bcb8305aba3bdfa` | Microsoft ScriptDOM180.107.0, assembly18.0.0.0; parser110과170, initial quoted identifiers false와true |

upstream commit만으로 실행 source를 식별하지 않는다. 등록 adoption/C2 patch를 적용한 준비물의 registry SHA256은 `85589bb6656c5e211d27480ab1ffb3fa7fd22cdd469f5256102a3b111654df08`, runtime manifest SHA256은 `affb30b637b0ef6204382d7a75c17c92e7a67ee6a182654b5b1c43ddfa369080`이다. runtime commit은 `659cda7c7f86ebe31cc825dc5da59e9add172dc7`이다. 등록 compiler는 기존 UCRT64 GCC16.2.0이며 새 runtime·compiler를 설치하지 않았다.

## 구문 대조와 전체 coverage

positive는 구문 수용이다. native 수용은 완료된 full parse의 root `has_error=false`로 집계하며 process 성공이나 route PASS와 구분한다. TP=양쪽 수용, TN=양쪽 거부, FP=native만 수용, FN=oracle만 수용이다. oracle의 정상 구문 오류는 invalid로 센다. 실행 실패·resource limit·지원하지 않는 문맥을 TN으로 세지 않는다. 서로 다른 parser의 AST/CST·오류 수·진단 위치가 같다는 계약은 없다.

| route | 입력 | SDK 관측 | 구문 비교 입력 / 관측 | TP / TN / FP / FN | 별도로 보존한 관측 |
|---|---:|---:|---:|---|---|
| Go | 32 | 32 | 32 / 32 | 18 / 13 / 1 / 0 | 없음 |
| C# | 14 | 28 | 8 / 16 | 8 / 8 / 0 / 0 | 다른 조건부 문맥8, language-version probe4 |
| C | 9 | 18 | 0 / 0 | 해당 없음 | compiler context18 |
| C++ | 9 | 18 | 0 / 0 | 해당 없음 | compiler context18 |
| T-SQL | 18 | 72 | 17 / 34 | 20 / 12 / 0 / 2 | initial quoted identifiers=false 문맥34, version probe4 |

합계82개 입력 pair와168개 SDK 관측을 모두 보존했다. C#의16개와 T-SQL의34개는 각 입력을 두 버전에서 비교한 수이며 각각8개와17개 입력이다. C#의 다른 문맥8개와 T-SQL quoted 문맥34개도 전체 분모에서 제거하지 않는다. 구문 표는 언어 전체 정확도·무작위 표본·독립 입력168개의 근거가 아니다. C/C++는 raw syntax 정확도를 측정하지 않았다.

C# `inactive-broken`, `undef-inactive`, `nested-directives`의 실제 native CST는 각각 첫 non-literal-false branch의 잘못된 body를 포함하고 대안 branch를 `preproc_skipped_section`으로 남겼다. Roslyn의 지정 symbols 또는 source `#undef`에 따른 선택과 다르다. `active-broken`은 지정 `ENABLED` 문맥에서 같은 잘못된 branch를 선택하므로 구문 비교에 남겼다. `unclosed-directive`도 undefined A와 native의 branch 선택이 달라 문맥 관측에 보존했다. 양쪽의 미종료 directive 거부 일치를 같은 문맥의 정확도로 세지 않았다. symbol 사례를 일괄 제외하거나 native를 empty-symbol compiler와 동등하다고 보지 않았다. 두 version probe의 `CSharpSyntaxTree.ParseText`/`GetDiagnostics` 기반 구문 수용과 별도 compilation의 CS8370 등 feature diagnostics도 분리했다.

T-SQL의 기본 native quoted 문맥은 실제 `SELECT "name";`에서 identifier node와 quoted table-name 수용으로 확인했다. 따라서 initial quoted identifiers=true를 구문 비교 문맥으로 사용한다. false 문맥에서 quoted table-name을 거부한 ScriptDOM 관측2개를 같은 native 설정의 FP로 합치지 않는다. 양쪽 모두 수용한 SELECT의 literal/identifier 의미가 같다는 주장도 하지 않는다. `JSON_OBJECT`는 parser110 거부·170 수용의 version probe로 분리하며 고정 grammar가 version110 validator라고 주장하지 않는다.

C/C++는 `-std=<registered> -pedantic-errors -fsyntax-only -nostdinc -fdiagnostics-format=sarif-stderr -fmax-errors=16 -` 문맥에서 header-free 입력을 검사했다. C에서는 undeclared semantic2개와 구버전 feature2개, C++에서도 같은 분류의2+2개가 native 구문 수용과 달랐다. 총8개 차이를 보존한다. macro/preprocessor 입력의 compiler 수용은 raw source CST나 source mapping 일치를 증명하지 않는다.

## 불일치의 원인과 처분

### Go identifier: 수정이 필요한 FP

최소 입력은 `package p\nvar a\u200Cb=1\n` (source SHA256 `f4c1d4d1c64fa8cf249b021db60e0d765d7380c16c346c554fd9cf0175fb80df`)의 실제 U+200C UTF-8 bytes다. native는 오류 없이 수용하고 Go1.27.1·1.27.2는 거부했다. 고정 grammar의 identifier 규칙은 `[_\p{XID_Start}][_\p{XID_Continue}]*`이며 준비된 parser의 identifier 문자 집합에 U+200C~U+200D가 실제 포함된다. [Go 명세](https://go.dev/ref/spec#Identifiers)의 underscore/Unicode Letter/Decimal Digit 규칙과 다르다.

[수정 Issue #205](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/205)에서 owning C2 patch의 문자 범위를 보정하고 고정 generator 재생성·registry identity 갱신, Letter/Nd 정상군과 Cf/combining/connector/letter-number 반례, semicolon·BOM·CRLF·edit 및 세 플랫폼 회귀를 요구한다. 이 보고서에서 grammar를 수정하거나 결함을 닫지 않았다. 발견된1개 FP를 label 변경으로 없애지 않는다.

### C# conditional context: 구현 가능한 별도 계약

현재 [조건부 fragment 정책](../validation/language-feature-disposition.md)과 [#182 보고서](issue-182-csharp-conditional-fragments.md)은 compiler symbol evaluator가 아니다. 네 다른 문맥의8개 관측 중 세 사례에서 여섯 번의 invalid/valid 차이가 관측됐고 `unclosed-directive`의 두 관측은 양쪽 모두 거부했다. 이는 원문·branch 선택을 확인한 지원 경계이며 compiler 문맥의 정확도 PASS가 아니다.

해결에는 explicit symbol context, 순서가 있는 `#define`/`#undef`와 조건식·중첩 평가, 원문 좌표를 보존한 active projection 또는 included ranges, 원문·projection·range identity와 profile/producer/replay 계약이 필요하다. defined/undefined·nested·Unicode/CRLF·fragment·edit/restore 및 depth/resource 반례를 함께 검증해야 한다. 기술적으로 해결 불가능한 제약이 아니며 이 평가에서는 구현하지 않았다. 기존 fragment 정책을 조용히 바꾸지 않는다.

### T-SQL CTE: oracle 수용 차이를 grammar 결함으로 단정하지 않음

`SELECT 1\nWITH c AS (SELECT 2 AS x) SELECT x FROM c;`에서 native는 거부하고 ScriptDOM110·170은 두 quoted 문맥 모두 수용했다. 초기 label은 invalid이며 그대로 보존했다. 구문 표의 FN2개는 true 문맥의 **ScriptDOM 대비 차이**다. false 문맥의 같은 차이2개도 별도 관측에 남겼다.

현재 Microsoft [CTE 문서](https://learn.microsoft.com/en-us/sql/t-sql/queries/with-common-table-expression-transact-sql?view=sql-server-ver17)는 같은 batch의 앞선 문장에 semicolon을 요구한다. 저장소의 과거 [#121 엔진 대조](issue-121-tsql-engine-validation.md)와 [#122](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/122)는 앞선 SELECT 뒤 이 경계를 다뤘다. 이번에 SQL Server를 실행한 것은 아니다. 따라서 ScriptDOM 수용에 맞추려고 기존 grammar guard나 label을 완화하지 않았다. 정확한 새 입력의 엔진 대조 또는 ScriptDOM 내부 원인 규명은 미검증이며, 필요하면 별도 권한의 고정 입력 검사와 실제 package-source 매핑이 필요하다. 서명된 package라는 사실은 parser 정확도나 선언 source commit과의 일치를 증명하지 않는다.

## 회복, CST 무결성, replay

5개 사전 등록 repair 입력은 native 실행 전 SDK/compiler11개 문맥에서 모두 수용됐다. 이 준비 과정의 추가 SDK 기록72개는 repair11·empty-symbol C#6·그 외 controls51·새 T-SQL restore4개다. 새 입력72개가 아니며 위168개 기본 SDK 관측과 합쳐 독립 표본으로 세지 않는다. Go/C#/C/C++의 oracle restore는 원본 bytes의 기존 관측을 재사용했고 T-SQL4개만 새로 실행했다.

native는82개 원본과10개 edit step을 한 번 수집했다. full CST는 initial82 + edited incremental10 + fresh10 = **102개**다. 별도 Go 검증기로 다음을 확인했다.

- 원본 SHA/길이와 `ApplyEdits`로 재구성한 모든 버전·edit byte 좌표를 대조했다.
- 12개 node 필드, 구조·digest·node 수·depth·root error 상태를 검사하고 모든 span의 LF row·UTF-8 byte column을 실제 원문 prefix에서 독립 재계산했다.
- incremental/fresh10쌍의 전체 12필드 CST와 원문·CST 복원5건이 일치했다. 다섯 repair의 native tree는 오류가 없었다.
- 임시 검증기의 duplicate key·null bool·초과 point 원소·ASCII/Unicode case alias, digest·source·좌표·복원 반례10개가 거부됐다. 좌표 반례는 구조 검증을 통과한 후 byte/point 검사에서, 복원 반례는 두 final tree가 같은 상태에서 원본 대조로 거부됐다.

오류·missing node가 있는 정상적인 recovery tree를 버리지 않았다. SDK 진단 좌표는 기존 독립 변환 receipt와 함께 보존하며 Go의 parser 원래 line/column과 byte-prefix 좌표 차이4개를 같다고 만들지 않았다. 자동 semicolon token 위치 전체가 native token stream과 일치한다고 주장하지 않는다.

`kit.VerifyOracleSet`은5개 set을 유효하다고 판정했다. 원본 set을 바꾸지 않은 복사본과 고정 workload로 `oracle-set-r2`를 replay하여 Go/C#/C/C++ PASS, T-SQL BLOCKED를 재현했다. `case-source` 역할은 logger용 `native-result-r3` 전용이다. 첫 r2 등록에서 이를 넣은 실패(`MEMBER_ROLE_UNKNOWN`)를 보존하고, 지원하는 r2 역할만 사용한 별도 등록으로 검증했다. r2 replay가 원문 기반 edit-point 재계산까지 수행했다고 주장하지 않는다. 그 검사는 위 별도 검증기의 근거다.

T-SQL `unclosed-bracket`의 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1`은 앞뒤 full tree 중 오류 tree가 있고 재사용 node가0개인 route 증명 한계다. incremental equality는 PASS이며 실제 full CST 대조·복원도 통과했다. set 무결성과 구문 판정, route 증명을 서로 승격하지 않는다. #197의 별도 logger를 이 r2 결과에 소급 적용하지 않았다. 더 강한 관측에는 그 producer로 새로 수집한 근거와 해당 관측을 소비하는 명시적 계약이 필요하다.

근거는 task-local `artifacts/198-checks`와 보존된 `artifacts/followup-goal-20261014/198-preflight`에 있다. native collection receipt, `cst-check/result-r4.json`, `sdk-comparison-r3.json`, `offline-replay-r2/*/result.json`이 각각 수집·좌표/복원·SDK 집계·replay를 구분한다. artifacts는 저장소 정책에 따라 untracked이며 이 문서의 표만으로 독립 실행 인증을 주장하지 않는다. right-pane Codex의 리뷰는 제공한 코드·사실에 대한 `STATIC_REVIEW`이며 독립 실행 검증이 아니다.

## 미측정 범위와 release 의미

이 평가는 Windows 고정 소규모 합성 입력이다. 다른21 route의 정확도, 전체 언어 명세, consumer 실제 corpus, Linux/macOS의 같은 holdout, query/API 의미, 장시간 안정성·RSS·처리량·latency를 검증하지 않았다. 수집 시간에는 compile/orchestration이 포함되므로 benchmark로 쓰지 않는다. ScriptDOM source commit 매핑, 새 SQL Server 실행과 소비자 재개도 미수행이다.

#199의 source-only 0.x release 준비는 이 관측과 미해결 #205·C# 문맥 경계를 공개해야 한다. 전체 grammar 정확도·세 플랫폼 동일 정확도·경쟁 제품 대비 속도 우위를 주장할 근거는 없다. tag/Release 발행은 별도 권한과 최종 후보 검증을 요구한다. #198의 평가 완료와 개별 제약의 해결 완료는 구분한다.
