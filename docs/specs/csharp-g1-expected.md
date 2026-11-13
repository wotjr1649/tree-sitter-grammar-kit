# C# B01·B02 대안의 기대값과 Roslyn↔tree-sitter 매핑표 계약 (#233 G1-3a)

이 문서는 별도 추적 계약 두 개의 형식과 검증을 정한다.

- [`csharp-g1-kind-map-r1.json`](../../src/contracts/csharp-g1-kind-map-r1.json)(schema `tsgk-csharp-g1-kind-map/r1`): Roslyn `SyntaxKind`(관측의 node·token 종류 461종)와 tree-sitter-c-sharp node type의 대응표. G1-3b~d·G1-4·G1-5가 공유한다.
- [`csharp-g1-expected-b01b02-r1.json`](../../src/contracts/csharp-g1-expected-b01b02-r1.json)(schema `tsgk-csharp-g1-expected/r1`): 어휘 행 `csharp-B01`(407)과 타입 이름 행 `csharp-B02`(98) 대안의 기대값 또는 BLOCKED 처분, 명세 대조 결과, 표본 감사, metamorphic 검사.

기대값은 native 결과를 읽지 않고 만든다(봉인 commit). 봉인 뒤 단계가 같은 대안을 native로 실행해 대조하고 별도 asset [`csharp-g1-executed-b01b02-r1.json`](../../src/contracts/csharp-g1-executed-b01b02-r1.json)(schema `tsgk-csharp-g1-executed/r1`)에 결과를 남긴다([native 비교](#native-비교-봉인-뒤-단계)). 기대값·매핑표는 native 결과에 맞춰 고치지 않는다. 지원 범위, claim, 한도는 바꾸지 않는다.

## 두 근거 종류

모든 기대값은 근거 종류를 하나 가진다. 이 둘만 있다.

| 근거 | 의미 | 대상 |
|---|---|---|
| `spec-derived` | token의 lexeme과 어휘 형식을 명세(C# 표준 draft v13, learn.microsoft.com, ECMA-334 후속 본문)에서 도출한다. Roslyn 관측은 그 lexeme이 나타난 위치를 알려 줄 뿐이다. | keyword 77, contextual keyword 46, 연산자·구두점 50, literal 형식(정수·실수·문자·문자열), 식별자 형식, 주석, 전처리 지시문 |
| `differential-oracle` | Roslyn이 관측한 구조(node 종류, 보간 문자열)를 매핑표로 tree-sitter type에 옮긴 것이다. 단일 oracle이므로 명세에서 직접 도출한 것과 구별해 적는다. | B02의 56개, B01의 보간 문자열 20개 |

주장은 "고정한 Roslyn DLL의 행동"으로 한정한다. 명세 본문 자체나 Roslyn 소스로 일반화하지 않는다.

**명세 판본.** 대안 inventory의 `ref`가 가리키는 Microsoft Learn의 C# language specification 페이지는 `dotnet/csharpstandard`의 `alpha-v13`(commit `79ad4e90ef63`, 2026-10-15 갱신)이며 ECMA-334 후속 본문이다. 절 번호는 그 페이지의 번호다. 이 draft는 `\e` 단순 이스케이프와 `Raw_String_Literal`을 어휘 문법에 포함하지만, inventory는 그 productions를 B01이 아닌 다른 행(`csharp-V13`, `csharp-V11`)에 둔다(B01의 `Simple_Escape_Sequence`는 12개 중 11개, `String_Literal`은 3개 중 2개 대안).

## 매핑표

- tree-sitter 쪽은 고정 commit `9150f7d5…`의 `grammar.js`·`src/node-types.json`(SHA-256을 asset에 둔다)과 저장소 C2 patch(`src/dev/c2-patches/csharp.json`)가 더하는 named node 4종(`file_based_app_directive`, `if_header`, `preproc_skipped_section`, `preproc_try_statement`; 등록 사례 기대가 이미 쓴다)만으로 만든다. 어휘는 named 232·anonymous 188이다. patch가 더하는 그 밖의 node type은 열거하지 않았다(생성된 parser를 읽지 않았다). anonymous type은 따옴표로 쓴다(`"abstract"`). `type`은 named supertype이자 anonymous 속성 대상어라 따옴표로 구별한다.
- Roslyn 쪽은 관측 asset의 `kinds` 461종(node 268·token 193)이다. 모든 종류가 정확히 한 항목을 갖는다(`G1_KINDMAP_COVERAGE`).
- 분류: **1:1** 365(type 하나), **1:n** 68(grammar가 같은 text 범위에 둘 이상의 node type을 허용: supertype 구성원, alias, 같은 범위를 감싸는 wrapper), **대응 없음** 28. 대응 없음에는 사유를 단다: `ROSLYN_ONLY`(zero-width·오류 복구 종류), `NO_SEPARATE_NODE`(tree-sitter에 그 범위의 node가 없음. 예: `ElseClause`, `EqualsValueClause`), `HIDDEN_NODE`(숨은 규칙. 예: `QueryBody`), `NOT_IN_PINNED_GRAMMAR`(고정 grammar에 어휘가 없음. 예: `allows`·`DefaultConstraint`·`RefStructConstraint`).
- 반대쪽(tree-sitter에만 있는 named 46종)도 사유와 함께 적는다: `SUPERTYPE`, `TS_WRAPPER`, `TRIVIA_OR_DIRECTIVE`(Roslyn 지시문은 node가 아닌 trivia이며 지시문으로 따로 관측한다), `EXTERNAL_SCANNER_NODE`, `PATCH_ONLY_CONSTRUCT`, `ROSLYN_KIND_NOT_OBSERVED`.
- 1:n의 type 집합은 "grammar가 허용하는 후보"다. 집합이 넓을수록 native 대조에서 덜 가려낸다(거짓 불일치는 없다).
- token의 고정 lexeme(`text`)은 관측과 일치해야 한다. 이 일치와 "등록 사례의 기대 type 203종이 모두 어휘에 있음"은 시험이 확인한다.

검증은 구조와 어휘 일관성이다. 판단(어떤 Roslyn 종류가 어떤 type인가)은 사람이 쓴 자료이며 생성 명령 `go run ./src/dev/csharp-g1-kindmap -root . -grammar <고정 checkout>`의 표가 근거다.

## 기대값 asset

- 구조: `inputs`(원장 입력 pin 7개, 관측 asset pin, 매핑표 pin), `counts`, `alternatives`(505개, 원장 순서), `cross_check`, `symbols_context`, `audit`, `metamorphic`.
- 대안마다 `status`가 `EXPECTED` 또는 `BLOCKED`다.
  - `EXPECTED`: `basis`, `form`(기대 형식), `spec`(절), `ts`(허용 tree-sitter type), `witnesses`(최대 3개, 사례당 하나).
  - `BLOCKED`: `block`(사유 code)과 `reason`.
- **witness**: 원장이 대안에 연결한 사례 중 Roslyn이 수락한 source에서, 그 대안을 보이는 첫 위치다. `match`가 `span`이면 `ts` 중 한 type의 node가 `[start,end)`와 정확히 같은 범위여야 하고, `starts-at`이면 그 위치에서 시작하면 된다(외부 scanner가 범위를 정하는 보간 시작 token, 지시문 줄). 수락되지 않은 source는 증거로 쓰지 않는다(`#error`는 Roslyn이 오류로 보므로 그 대안은 witness가 없다).
- B01의 `token: X`·`literal: X`·`identifier: X`·`Comment: X`는 X의 모든 대안의 위치와 type의 합집합이다.

### BLOCKED 사유

| code | 대안 수(B01/B02) | 뜻 |
|---|---:|---|
| `NO_TREE_COUNTERPART` | 19/0 | 공백·줄 끝·문자 클래스·지시문 줄의 공백과 본문: tree-sitter가 extra로 건너뛰어 node가 없고 Roslyn trivia는 관측하지 않았다 |
| `LEXICAL_META_PRODUCTION` | 7/0 | `input`·`input_section`·`input_element`처럼 입력의 나열 구조: 어느 tree에도 node·경계가 없다 |
| `EXTERNAL_SCANNER_REPRESENTATION` | 7/0 | 보간 문자열의 `{{`·`}}`·`""`: tree-sitter 외부 scanner가 표현을 정하고 `grammar.js`에 없다 |
| `NO_WITNESS_IN_LINKED_SOURCES` | 4/2 | 연결된 수락 source에 그 대안이 없다(`default`·`*`·`/` token, `#error`, 생성자 단독 제약, `ref readonly`). 원장의 연결은 사례 수준 선언이라 실제 포함을 보증하지 않는다 |
| `POSITION_DEPENDENT` | 0/7 | `namespace_name`·`type_name`처럼 자리가 node를 정한다. 관측은 자리(slot)를 기록하지 않는다 |
| `ABSTRACT_CATEGORY` | 0/10 | `type: reference_type` 같은 union 생성: 자기 node가 없다 |
| `SEMANTIC_BINDING` | 0/22 | `class_type`과 `interface_type`, `dynamic`, 포인터 대 배열 요소 접근처럼 binding 없이는 구별되지 않는다 |
| `NOT_IN_PINNED_GRAMMAR` | 0/1 | `default` 제약: 고정 grammar에 node type이 없다 |

BLOCKED는 golden으로 채우지 않는다. `POSITION_DEPENDENT`·`SEMANTIC_BINDING`을 풀려면 관측기가 slot을 기록해야 한다(G1-3b 이후 결정).

## 명세 대조(cross_check)

수락된 source 361개의 token 37,077개와 지시문 423개를 같은 명세 도출로 분류해 Roslyn의 종류와 비교한다(예약어·연산자는 lexeme 일치, 숫자·문자·문자열·식별자는 형식 일치, 지시문은 6.5.1의 종류). 불일치는 채택하지 않고 처분을 적는다. 이번 결과: 불일치 22개는 모두 raw string literal 관련 Roslyn token(`SingleLineRawStringLiteralToken` 등 5종)이며 `OTHER_ROW`로 처분했다. 명세(6.4.5.6)가 `Raw_String_Literal`을 정의하되 inventory가 그 productions를 `csharp-V11`에 두므로 Roslyn의 이탈이 아니다. 예약어·연산자·숫자·문자·문자열·식별자·지시문에는 불일치가 없었다.

처분 값은 `SPEC_AMBIGUOUS`, `ROSLYN_DEVIATION`, `GRAMMAR_DEFECT_CANDIDATE`, `OTHER_ROW` 중 하나이고 절 인용과 결정을 갖는다. 처분이 없는 불일치는 `G1_EXPECTED_UNDISPOSED`다.

## 명세 기반 표본 감사 (D2)

- 표본: 행마다 대안의 5%를 올림해 B01 21·B02 5 = 26개. 고정 seed `csharp-g1-3a-audit/r1`로 `SHA-256(seed|id)`가 작은 순서로 고르므로 기대값과 무관하고 결정적이다.
- 각 표본은 근거 문서·절을 갖는 손 도출(`csharpG1AuditHand`)이다. 기대값을 보기 전에 명세 본문과 `grammar.js`에서 썼다. 도출은 기대 상태, 허용 type, witness text가 따라야 할 정규식이다.
- 결과: 일치 25, **COVERAGE_GAP 1**(`csharp-B01.a356`, `#error`: 명세는 기대를 도출하지만 연결된 수락 source에 없다), 불일치 0.
- **상한은 층별로 적는다.** 한 숫자(3/26 ≈ 11.5%)로 합치지 않는다. 상한은 정확한 단측 95% 이항(Clopper–Pearson) 상한이며 `G1AuditUpper95`가 계산하고 `G1AuditStrata`가 층별로 낸다(asset을 바꾸지 않는 보고 함수).

  | 층 | 표본 | 불일치 | 95% 상한(정확) |
  |---|---:|---:|---:|
  | `spec-derived`(lexeme은 명세에서 도출하고 Roslyn은 위치만 관측) | 16 | 0 | 17.1% |
  | `differential-oracle`(구조가 Roslyn 관측이며 매핑표로 옮김) | 4 | 0 | 52.7% |
  | BLOCKED 처분 확인 | 5 | 0 | 45.1% |
  | COVERAGE_GAP(비교 없음) | 1 | — | — |
  | 기대값을 비교한 표본(위 두 층) | 20 | 0 | 13.9% |
  | 전체 | 26 | 0 | 10.9% |

- **전수 감사를 반영한 층별 보고**(`G1AuditStrata(x, rows)`, rows는 전수 감사 asset의 행): 차분 oracle 층은 표본 4개 대신 76개 전체이고 불일치는 r2 witness 대비 현재의 불일치 수다. `found_and_corrected`는 처음 비교에서 통과하지 못해 명세·grammar 원문으로 정정된 대안 수이고, `upper_95_percent_with_found`는 그 수를 오류로 센 정확한 상한이다. 76개는 층 전체이므로 이 상한은 "비교한 성질"에 대한 것이며 비교하지 않은 대안의 추정이 아니다. 비교한 성질은 witness의 위치와 lexeme 형식이고 type(`ts`)이 아니다(type은 표본 4개만 비교됐다).

  | 층 | 표본 | 불일치 | 95% 상한(정확) | 정정된 대안 | 정정을 오류로 센 상한 |
  |---|---:|---:|---:|---:|---:|
  | `spec-derived` | 16 | 0 | 17.1% | (later_wrong 1) | 26.4% |
  | `differential-oracle`(전수 감사 76개) | 76 | 0 | 3.9% | 11 | 22.8% |
  | BLOCKED 처분 확인 | 5 | 0 | 45.1% | — | — |
  | COVERAGE_GAP | 1 | — | — | — | — |
  | 기대값을 비교한 표본(위 두 층) | 92 | 0 | 3.2% | 11 | 20.3% (later_wrong 1 포함) |
  | 전체 | 98 | 0 | 3.0% | 11 | 19.1% (later_wrong 1 포함) |

  두 층은 비교한 성질이 달라 합산 상한(3.2%·3.0%)은 "spec-derived의 type·text 일치"와 "차분 oracle의 위치 일치"를 섞은 값이다. 층별 값을 우선한다.
- 봉인된 r1 asset의 `estimate.upper_95_percent`는 rule of three(`300/n`)의 `11.5`다. 정확한 값(10.9)보다 크므로 보수적이며 r1은 바꾸지 않는다. 불일치가 있을 때 asset을 유도하는 `g1Audit`는 불일치율 자체가 아니라 정확한 상한을 쓴다(이전에는 불일치율을 상한으로 적었다. `TestCSharpG1AuditEstimateWithMismatches`).
- 표본 감사가 확인한 것은 같은 명세·grammar를 읽는 손 도출과 도출한 기대값의 일치다. Roslyn의 오류율도, 기대값 asset 전체의 오류율도 아니다. 층마다 표본이 작아 차분 oracle 층(4개)은 상한 52.7%밖에 보이지 못한다. 이 층의 대안 76개(B02 56 + B01 보간 20)는 [전수 감사](#차분-oracle-대안-전수-감사-g1audit76)가 모두 명세에서 손으로 도출해 대조했고, 층별 보고(`G1AuditStrata(x, rows)`)의 차분 oracle 층은 그 76개를 센다(표본 4개는 76개에 포함되어 두 번 세지 않는다).
- **사후 확인:** 표본에 든 `csharp-B01.a335`는 감사에서 MATCH였다(손 도출도 `identifier`). native 대조가 이 값이 틀림을 드러냈고 r2가 정정했다. 이를 오류로 세면 `spec-derived` 층은 1/16(상한 26.4%), 기대값을 비교한 표본은 1/20(21.6%), 전체는 1/26(17.0%)이다(`later_wrong`, `upper_95_percent_with_later_wrong`). 표본 감사 하나가 이 오류를 잡지 못했으므로 위 상한을 "기대값이 틀릴 확률의 상한"으로 읽으면 안 된다.

## metamorphic 검사 (D2)

- 대표 source 20개(수락, 두 행의 대안에 연결된 사례, 8,000 B 이하, 고정 seed `csharp-g1-3a-metamorphic/r1`)에 변형 넷을 적용한다: `whitespace`(token 사이 공백만 있는 틈의 첫 byte를 두 번), `comment`(그 틈 끝에 `/*c*/` 삽입), `rename`(예약어·contextual keyword가 아닌 일반 식별자를 `Zq<n>`으로 일관되게 변경), `combined`(셋 모두). 80개 변형이 모두 원본과 다르다.
- 각 변형을 Roslyn(관측기 `g1` 모드, SDK 10.0.401, HARD Job)으로 다시 관측하고 구조 digest(수락 여부, 진단 ID, node 종류·깊이, token 종류·누락 표지. span 제외)를 원본과 비교한다. 결과: **80/80 동일**. 같은 입력의 두 실행은 byte 단위로 같다.
- digest는 asset에 봉인한 Roslyn 출력이다. offline verifier는 변형 source(원본에서 결정적으로 다시 만든 SHA-256)와 digest의 일관성을 확인하며 Roslyn을 다시 실행하지 못한다. 재실행은 `go run ./src/dev/csharp-g1-expected -root . -metamorphic-input <파일>`로 입력을 만들고 관측기로 돌린 뒤 `-metamorphic-results <출력>`으로 봉인한다.

## symbols 문맥 (Q3)

원장 syntax가 ERROR인데 Roslyn이 기본 모드(symbols 없음)에서 수락한 13 step은 모두 이 두 행의 의무에 연결되어 있다(`csharp-B01` P·N·R·E 등). 각각 `DEFAULT_MODE_ONLY_SYMBOLS_CONTEXT_UNJUDGED`로 적었다. 이 단계는 그 13건의 symbols 문맥을 판정하지 않는다(G1-4가 별도 asset으로 다룬다).

## 원장 상태 전환 규칙

원장은 두 asset의 내용에서 상태를 유도한다. 전환은 다음 하나뿐이다.

| 전환 | 조건 |
|---|---|
| `NO_EXPECTED` → `SEALED` | 기대값 asset이 그 대안을 `EXPECTED`로 가진다 |
| `NO_EXPECTED` → `BLOCKED` | 기대값 asset이 그 대안을 `BLOCKED`로 가진다. 원장의 `block` member가 그 사유 code를 같은 값으로 옮긴다 |
| `SEALED` → `EXECUTED` | executed asset이 그 대안을 `executed: true`이고 `result: MATCH`로 가진다(native와 대조해 **일치했다**) |

- `EXECUTED`는 "native와 대조해 일치했다"를 뜻한다. 대조가 돌았어도 `MISMATCH`인 대안과 대조가 돌지 못한 `NOT_APPLICABLE` 대안은 `SEALED`에 남는다(상태 어휘와 schema는 그대로다). 그래서 행의 P 의무가 `EXECUTED`이면 그 행의 모든 대안이 native와 일치한 것이다. executed asset의 `executed` member는 "비교가 돌았다"는 뜻이고 `result`가 판정이다. 두 member가 모순이거나(`executed: true`인데 `result`가 `NOT_APPLICABLE`, `executed: false`인데 `MATCH`·`MISMATCH`), `result`가 없거나 `MATCH`·`MISMATCH`·`NOT_APPLICABLE` 밖의 값이면 원장 유도가 `G1_LEDGER_EXECUTED`로 거부한다.
- `NO_EXPECTED`는 "아직 기대값을 만들지 않음"에만 쓴다. 두 asset이 다루지 않는 대안(B03 이후 행의 모든 대안)과 다른 행의 의무는 `NO_EXPECTED`다.
- `BLOCKED`는 기대값을 만들 수 없다고 처분한 대안이며 `EXECUTED`가 될 수 없다. 사유를 풀려면 기대값 asset의 새 revision이 필요하다. executed asset이 `BLOCKED`·알 수 없는 대안·다른 행·중복을 가리키면 원장 유도가 `G1_LEDGER_EXECUTED`로 거부하고, 기대값 asset이 `BLOCKED`에 사유를 달지 않았거나 알 수 없는 상태를 쓰면 `G1_LEDGER_SEALED`다. executed asset이 기대값 asset 없이 주어져도 `G1_LEDGER_EXECUTED`다.
- 행의 P 의무는 그 행의 모든 대안이 `SEALED` 또는 `EXECUTED`일 때만 `SEALED`, 모두 `EXECUTED`일 때만 `EXECUTED`다. `csharp-B01`·`csharp-B02`는 모두 `BLOCKED` 대안이 있어 P 의무가 `NO_EXPECTED`다. N·R·E·Q·W 의무는 다른 단계의 기대값이 필요하므로 바뀌지 않는다.
- 이 revision의 상태: B01·B02 대안 505개 중 `EXECUTED` 426(native와 대조해 일치한 EXPECTED 대안 전부: r2 executed asset 기준이며 426개가 모두 `MATCH`다), `BLOCKED` 79(사유 code는 위 표), `SEALED` 0. 나머지 1,026개는 `NO_EXPECTED`다. 원장은 기대값·executed의 r2 쌍에서 유도한다. 같은 원장 코드에 r1 쌍을 주면 r1 executed의 `MISMATCH` 4건이 `SEALED`로 남아 `EXECUTED` 422·`SEALED` 4가 된다(`TestCSharpG1LedgerStateRules`가 이 실제 자료를 반례로 쓴다).
- 원장 verifier는 유도 결과와의 정확한 일치를 요구한다. 손으로 바꾼 상태(`BLOCKED`→`EXECUTED`, `EXECUTED`→`SEALED`, 사유 삭제·변경, 실행되지 않은 대안과 `MISMATCH` 대안의 `EXECUTED`)는 `G1_LEDGER_ALTERNATIVE`·`G1_LEDGER_ROWS`로, 어휘 밖의 상태는 `G1_LEDGER_STATUS`로 거부된다(`TestCSharpG1LedgerForgeries`, `TestCSharpG1LedgerStateRules`).
- asset 간 결속은 한 방향이다: executed asset이 기대값·매핑표·관측 asset을, 기대값 asset이 관측 asset을, 관측 asset이 원장을 pin한다. 원장은 기대값·executed asset의 bytes를 pin하지 않고 상태만 받는다(되돌아 pin하면 순환이다). 원장 bytes가 바뀌면 관측 asset의 `ledger` pin(관측 내용은 그대로)과 그 SHA-256을 pin한 기대값·executed asset의 pin 한 줄이 따라 바뀌므로, 재생성은 원장 → 관측 pin → 기대값 → executed → 원장 → … 순서로 고정점까지 돌린다.

## native 비교 (봉인 뒤 단계)

별도 asset `csharp-g1-executed-b01b02-r1.json`이 봉인된 기대값 426개를 등록된 C# route의 tree-sitter-c-sharp native 빌드로 대조한 결과다. 기대값·매핑표는 이 단계에서 바뀌지 않는다. 관측 asset은 `ledger` pin 한 줄만, 그 결과로 기대값 asset은 `inputs.observation.sha256` 한 줄만 따라 바뀐다. 시험 `TestCSharpG1ExpectedContentIsTheSealedOne`은 그 한 줄을 비운 내용의 digest가 봉인 commit의 값과 같음을 고정한다. 관측 r2 asset은 동결된 입력이다: 기대값·executed의 r1·r2가 그 파일을 pin하며 다시 만든 관측은 새 파일 `csharp-g1-roslyn-observed-r3.json`(r2와 `provenance.tool_sha256` 값 하나만 다르다)이므로 r1 파일은 바뀌지 않는다([Roslyn 관측 계약](csharp-g1-ledger.md#roslyn-관측-계약-g1-2)).

### 실행

- 대상: EXPECTED 대안의 witness source와 원장이 그 대안에 연결한 P step의 source version을 합친 72개(`CSharpG1NativeSourceIDs`). BLOCKED 대안은 대상이 아니다.
- 개발 도구 `src/dev/csharp-g1-native`가 `native-routes.json`의 `csharp` closure(준비는 기존 `src/dev/s05-native/prepare-routes.ps1`)를 build하고 source마다 전체 입력 parse 하나를 실행해 원시 관측(`tsgk-csharp-g1-native-raw/r1`)을 쓴다. 원시 관측은 추적하지 않는 로컬 artifact이며 source별 node 목록 `[type, 플래그, 시작, 끝, 부모]`(anonymous type은 매핑표처럼 따옴표)와 digest를 담는다.
- 한도: native process 1 GiB·30 s(등록된 `native-parse-edit` 정책의 test-only 사본이며 제품 정책은 그대로), build 단계 4 GiB·120 s. 전체를 HARD Job 안에서 실행한다. 같은 입력의 두 실행은 원시 관측이 byte 단위로 같다.
- `src/dev/csharp-g1-executed -native <원시 관측>`이 asset을 유도한다(native를 실행하지 않는다). `-native` 없이 실행하면 추적 asset을 offline으로 검증한다.

### 분류

witness는 봉인된 값을 그대로 옮긴 것이다. 각 witness를 그 source의 native tree와 대조한다.

| 결과 | 뜻 |
|---|---|
| `MATCH` | 대안의 봉인된 type 집합(`ts`) 중 하나인 native node가 witness의 범위와 정확히 같거나(`span`) 시작 위치가 같다(`starts-at`) |
| `MISMATCH` | native가 완료됐는데 그런 node가 없다. 같은 범위를 겹치는 더 작은 node(`near`)를 함께 적는다 |
| `NOT_APPLICABLE` | native parse가 완료되지 않아(자원 한도·실패) 비교할 tree가 없다 |

대안은 witness 하나라도 `MISMATCH`이면 `MISMATCH`, 아니고 `MATCH`가 있으면 `MATCH`, 아니면 `NOT_APPLICABLE`이다. 비교가 실제로 돈 대안(완료된 native witness가 있음)은 asset에서 `executed: true`이고, 그중 `MATCH`인 대안만 원장에서 `EXECUTED`가 된다(`MISMATCH`는 `SEALED`). 증거(`found`: 같은 범위 또는 같은 시작의 native node, `ts`에 속한 type을 앞에 두고 8개까지)가 witness마다 asset에 있고, verifier가 그 증거에서 결과를 다시 계산한다.

매핑표 점검(`map`): witness가 Roslyn token·node 하나의 범위와 정확히 같으면 native type이 매핑표가 그 Roslyn 종류에 적은 type 중 하나인지(`IN_MAP`) 아닌지(`OUTSIDE_MAP`)를 적고, 그 종류의 일부(예: 문자 literal 안의 escape)이면 `FRAGMENT`다. 판정은 봉인된 `ts`로 하며 `map`은 매핑표가 봉인된 `ts`를 거쳐 가려질 수 있는 불일치를 드러내는 보조 점검이다.

### 결과

- source 72개 모두 native 완료이며 오류 tree가 없다(Roslyn이 수락한 69개에서 native `ERROR`는 0). Roslyn이 수락하지 않은 3개(`#error`의 CS1029, C# 14 file-based app 지시문 두 개의 CS9298·CS9314)도 native는 오류 없이 파싱했다. 이 셋은 대안의 비교에 쓰이지 않으며(witness는 수락된 source만 쓴다) 수락 여부의 차이만 `sources`에 남긴다.
- 대안 426개: `EXECUTED` 426, `MATCH` 422, `MISMATCH` 4, `NOT_APPLICABLE` 0. witness 626개: `MATCH` 622, `MISMATCH` 4. `map`: `IN_MAP` 555, `OUTSIDE_MAP` 3(아래 불일치의 세 witness), `FRAGMENT` 64, Roslyn 종류가 없는 witness 4(`map` 생략).
- 불일치 4개는 처분 목록(`dispositions`)에 원인 분류와 함께 적었다. 원인은 `GRAMMAR_DEFECT`(봉인된 기대·매핑표·고정 grammar 원문이 모두 같은 것을 가리키는데 native가 다름), `KIND_MAP_ERROR`(native가 고정 grammar 원문과 같은데 매핑표 항목이 원문과 어긋남), `EXPECTED_ERROR`(매핑표는 맞는데 기대값 asset의 값이 원문과 어긋남), `DESIGN_DIFFERENCE`(둘 다 유효하나 표현이 다름) 중 하나다.

| 대안 | witness | native 결과 | 원인 |
|---|---|---|---|
| `csharp-B01.a335` (#define·#undef의 조건부 심볼) | `alt-csharp-B01-10#0` [8,13) | `preproc_arg`. 봉인된 값은 `identifier` | `EXPECTED_ERROR`: 고정 grammar는 `preproc_define`·`preproc_undef`의 심볼을 `preproc_arg` 하나로 둔다. 봉인된 type은 같은 production을 쓰는 #if·#elif 조건에서 가져왔다 |
| `csharp-B01.a383` (`$"`) | `alt-csharp-B01-09#0` [66,68) | `interpolation_start` [66,67)와 `"` [67,68) | `KIND_MAP_ERROR`: 매핑표는 Roslyn `InterpolatedStringStartToken`을 같은 범위의 `interpolation_start` 하나로 둔다. grammar 원문은 시작을 두 node로 만든다 |
| `csharp-B01.a396` (`$@"`) | `alt-csharp-B01-09#0` [141,144) | 두 node | `KIND_MAP_ERROR`(같은 항목, `InterpolatedVerbatimStringStartToken`) |
| `csharp-B01.a397` (`@$"`) | `alt-csharp-B01-09#0` [171,174) | 두 node | `KIND_MAP_ERROR`(같은 항목) |

`GRAMMAR_DEFECT` 후보는 없다: native는 오류 없이 파싱했고 네 불일치는 모두 고정 grammar 원문과 같은 표현이다. 기대값 오류와 매핑표 오류로 본 네 건은 r1에서 **고치지 않았다**. 사용자 결정(2026-11-13)에 따라 r1을 그대로 두고 아래 "봉인 뒤 정정 (r2)"의 변경 기록으로 정정했다. 이 처분은 사람이 쓴 판단이며 `kit`의 `g1ExecDispositions` 표에 있다. 처분이 없는 불일치가 있으면 verifier가 `G1_EXECUTED_UNDISPOSED`로 거부한다.

### 기대값을 거꾸로 만들 수 없음

- executed asset은 기대값·매핑표·관측을 pin으로만 가리킨다. witness·`ts`·근거는 봉인된 것과 같아야 하고(`G1_EXECUTED_EXPECTATION`), 봉인된 type을 native 결과에 맞춰 넓히면 거부된다.
- 도출 경로의 코드(명세 도출기, 기대값, 규칙·검사·자료, 매핑표, 관측, r2 정정)는 비교 쪽 파일의 어떤 선언도 호출·참조하지 않고, 파일·process·network·환경 변수에 닿는 package를 쓰지 않는다. 이것은 식별자 이름이 아니라 도출 진입점에서 닿는 선언의 **호출 폐쇄**로 증명한다(`TestCSharpG1SeparationClosure`: 도달한 선언이 비교 파일에 있거나 허용 목록 밖 package·`//go:embed`·`//go:linkname`을 쓰면 실패하고, 분류 표에 없는 새 파일도 실패한다. 음성 대조 `TestCSharpG1SeparationClosureCatchesBypasses`가 분석기가 비어서 통과하는 일을 막는다). 도출은 입력 bytes가 바뀌면 결과가 바뀌고 native 결과에 반응하지 않는다(`TestCSharpG1ExpectedFollowsItsInputsOnly`). 식별자·import 정규식 검사(`TestCSharpG1NativeNeverFeedsExpectations`·`TestCSharpG1ExpectedSeparation`)는 보조 검사로 남긴다. native tree가 전부 어긋나는 합성 입력에서도 기대값 유도는 봉인된 bytes를 그대로 낸다.
- 호출 폐쇄의 분류에서 `csharp_g1_correct.go`(r2 정정)는 도출 쪽이다. 정정 값은 native 산출물을 읽어 만들지 않고 grammar.js·명세 원문에서 저작한 상수이며, 그 코드는 봉인된 r1 bytes와 저작된 상수만 입력으로 받는다. 다만 상수가 쓰인 시점은 native 비교 뒤이며 그 사실은 코드로 증명할 수 없어 `after_native` 표지로 기록한다(아래 "봉인 뒤 정정"). 폐쇄가 보이는 것은 호출 경로에 native 입력이 없다는 사실까지다.

### 한계

- 판정은 witness(대안당 사례 하나·최대 3개)에 한정되며 grammar가 대안의 모든 구문을 파싱한다는 뜻이 아니다. 대안의 `ts`가 넓으면(1:n) 일치가 약한 근거일 수 있다.
- offline verifier는 native를 다시 실행하지 못한다. 증거가 판정과 모순 없음, 봉인된 값·pin·source 신원·처분·bytes까지 확인하며, 증거 자체의 진위는 원시 관측과의 재유도(`TSGK_G1_NATIVE_RAW`로 `TestCSharpG1ExecutedMatchesRawNative`)가 확인한다. 원시 관측은 추적하지 않는다.
- 한 host(Windows, gcc 16.2.0)의 한 빌드, 기본 모드(preprocessor symbols 없음)다. symbols 문맥 13건과 오류 복구는 비교하지 않았다.

## 봉인 뒤 정정 (r2)

봉인된 asset의 값이 native 결과를 본 뒤에 명세·grammar 원문과 어긋난 것으로 드러나면 그 asset을 고치지 않고 **새 revision(r2)**으로 올려 정정한다. 규칙은 다음과 같다.

- **r1 보존.** r1 asset(`csharp-g1-expected-b01b02-r1.json`, `csharp-g1-kind-map-r1.json`, `csharp-g1-executed-b01b02-r1.json`)은 봉인된 그대로 둔다. 시험 `TestCSharpG1CorrectR1StaysAsSealed`가 크기와 SHA-256을 고정한다. r2 asset은 r1의 SHA-256과 크기를 `previous` pin으로 가리키고, r2 verifier는 그 r1이 봉인된 asset으로 검증되는지부터 확인한다(r1 변조 거부).
- **변경 기록(`corrections`).** r2 asset은 r1과 변경 기록만큼만 다르다. 항목마다 대안 id(매핑표는 Roslyn 종류, 목록 정정은 type)·`field`·이전 값(`old`)·새 값(`new`)·`reason`·`ground`·`after_native`·`basis_independent`를 둔다. `after_native`는 시간이다: native 결과를 본 뒤의 정정이면 `true`이며, 사용자 결정(2026-11-13)에 따라 **r2의 모든 정정이 `true`**다(raw 시작 token 두 항목도 native 불일치를 본 뒤 같은 구조 오류를 고친 것이다). verifier는 `false`를 형식 위반으로 거부한다. `basis_independent`는 근거다: 정정의 근거가 명세·grammar 원문뿐이면 `true`다(r2의 모든 정정이 그렇다). 사후 정정의 통계(표본 감사의 `later_wrong` 등)는 `after_native`로 센다. 두 표지는 저작된 표와 같아야 하고 시험이 항목 이름이 아니라 저작된 표를 직접 고정한다(`TestCSharpG1CorrectedAssetsVerify`). `ground`는 고정 grammar 원문(`grammar.js`의 규칙)과 명세(csharpstandard alpha-v13 commit `79ad4e90…`의 파일·절)의 인용문이며 둘 다 있어야 한다(tree-sitter 쪽은 grammar 원문만이 근거다). 각 ground는 문서 없이 CI에서 확인된다(`doc_sha256`·`heading`·`gap`과 인용문이 정정 기록 자체에 있다): 인용문은 공백을 하나로 접어 40자 이상이어야 하고, `doc_sha256`은 고정 문서의 SHA-256이어야 하며(grammar.js는 매핑표의 pin `63d357e0…`, 명세는 `lexical-structure.md` `332d1129…`·`expressions.md` `bcc4c11c…`·`types.md` `294308e7…`·`classes.md` `0d6ac924…`·`unsafe-code.md` `ba020202…`), `heading`은 `at`이 가리키는 절(명세는 절 번호를 가진 markdown 제목, grammar는 `<규칙>:`)이어야 하고 인용문까지의 줄 수 `gap`은 200 이하여야 한다. `src/dev/csharp-g1-correct -grammar … -spec …`(시험의 `TSGK_G1_GRAMMAR_JS`·`TSGK_G1_SPEC_DIR`)은 문서가 있을 때만 도는 **실제 문서 점검**이다: 문서의 SHA-256이 ground와 같고, 인용문이 있고, heading 줄이 인용문 시작 줄보다 정확히 `gap`줄 앞에 있다. 한계: CI는 인용문이 문서에서 왔음을 증명하지 못하고(문서가 추적되지 않는다), 문서가 고정 SHA-256과 같고 인용문이 거기 있음은 문서가 있는 환경의 점검만이 보인다. `old`는 r1이 가진 값과 같아야 하고, 기록에 없는 차이·근거 없는 항목·저작되지 않은(표지 값이 다른 것 포함) 항목은 거부된다.
- **근거는 명세·grammar 원문만.** 정정 값은 native 산출물이 아니라 명세와 grammar 원문에서 다시 도출한다. native와 같아지는 것은 근거가 아니다. 원문으로 정할 수 없으면 정정하지 않고 BLOCKED와 사유로 둔다. 도출 코드(`csharp_g1_correct.go`)는 native 입력을 받지 않는다.
- **통계를 둘로 나눈다.** "봉인 시 일치"(r1 executed asset: 426 중 422)와 "사후 정정 뒤 일치"(r2 executed asset의 `corrected`·`corrected_match`: 정정 15건 중 n건)를 합치지 않는다. 정정한 대안은 독립 기대값 통계(표본 감사, 봉인 시 일치율)에서 "사후 정정"으로 따로 센다. r2의 표본 감사·metamorphic·cross_check 절은 r1의 것 그대로다(봉인 당시의 기록이며 정정 값으로 다시 계산하지 않는다).
- **r2 executed asset**은 r2 기대값·매핑표를 같은 native 원시 관측(`raw_native`)과 다시 대조한 기록이며, `sealed_match`(정정하지 않은 대안 중 일치)·`corrected`·`corrected_match`를 가진다.

### r2 정정 23건 (기대값 18 + 매핑표 5)

| 항목 | field | 이전 → 새 | 근거 |
|---|---|---|---|
| `csharp-B01.a335` (#define·#undef의 조건부 심볼) | `ts` | `["identifier"]` → `["preproc_arg"]` | grammar.js `preproc_define`·`preproc_undef`는 심볼을 `preproc_arg` 하나로 둔다(identifier는 #if·#elif 조건에만 있다). 명세 §6.5.4는 심볼을 `PP_Conditional_Symbol`(어휘 조각)로만 정의하고 tree-sitter type을 정하지 않는다 |
| `csharp-B01.a383` (`$"`) | `witnesses[0].match` | `span` → `starts-at` | 명세 `Interpolated_Regular_String_Start`는 `$"` 한 lexeme이다. grammar.js `interpolated_string_expression`은 시작을 `interpolation_start`(외부 token의 alias)와 그 뒤 따옴표 node 둘로 만든다. 외부 token의 범위는 grammar 원문에 없으므로 도출할 수 있는 것은 시작뿐이다 |
| `csharp-B01.a396` (`$@"`), `a397` (`@$"`) | `witnesses[0].match` | `span` → `starts-at` | 위와 같은 규칙(`interpolation_verbatim_start`). 명세 `Interpolated_Verbatim_String_Start` |
| `csharp-B01.a383`·`a396`·`a397` | `ts_basis` | `kind-map` → `grammar-text` | (F4) 세 대안은 `differential-oracle`이고 r1의 `ts`(`interpolation_start`)는 r1 매핑표의 항목을 옮긴 것이었다. 매핑표 r2는 시작 token에 node type을 주지 않으므로 r2의 `ts`는 더 이상 r2 매핑표의 도출이 아니다(r1 도출에 r2 매핑표를 넣으면 세 대안은 `BLOCKED`가 된다). 값은 그대로 `interpolation_start`이고 근거는 grammar 원문이다: `interpolated_string_expression`이 시작을 `interpolation_start` node(외부 token의 alias)와 따옴표 node로 만든다. 대안의 `basis`는 `differential-oracle`로 두고(witness의 lexeme·위치는 Roslyn 관측이다) 이 기록이 `ts`의 근거가 매핑표에서 grammar 원문으로 바뀌었음을 밝힌다. 시험 `TestCSharpG1CorrectedExpectedTSFollowsTheR2KindMap`이 r2 EXPECTED 대안의 `ts`가 r2 매핑표 도출과 같거나 변경 기록(`ts`·`ts_basis`)에 열거됨을 고정하고, 기록 누락·무기록 변경·불필요한 기록을 변이로 거부한다 |
| 매핑표 `InterpolatedStringStartToken` | `entry` | `1:1 [interpolation_start]` → `none NO_SEPARATE_NODE` | 매핑표는 같은 범위의 node type을 적는데 grammar 원문에는 그 범위의 node가 없다 |
| 매핑표 `InterpolatedVerbatimStringStartToken` | `entry` | 위와 같은 변경 | 위와 같다 |
| 매핑표 `InterpolatedSingleLineRawStringStartToken`·`InterpolatedMultiLineRawStringStartToken` | `entry` | 위와 같은 변경 | 명세 `Interpolated_Raw_String_Start`는 lexeme 하나이고 grammar.js의 raw 선택지는 `interpolation_start`(`interpolation_raw_start`의 alias)와 `interpolation_quote` 둘이다. native 불일치로 드러난 적은 없으나 같은 구조 오류라 사용자 결정으로 함께 정정했고(`after_native`, 근거는 명세·grammar 원문뿐이다), 이 두 종류를 쓰는 B01·B02 대안은 없다(관측 token은 cross_check의 `TOKEN_OF_OTHER_ROW`) |
| `csharp-B02.a021` (`non_array_type: pointer_type`) | `ts+witnesses[0]` | witness `int* p`의 `int*`(24–28) → `int*[] arr`의 원소 `int*`(62–66), ts 그대로 | 틀린 규칙 문맥: 필드 `int* p`는 `type: pointer_type`(a008)의 증거다. 명세 §8.2.1 `non_array_type`·§24.3.2 `int*[]`, grammar.js `_array_base_type`이 `pointer_type`을 허용 |
| `csharp-B02.a062` (`unmanaged_type: pointer_type`) | `ts+witnesses[0]` | `int* p`(24–28) → `sizeof(int*)`의 피연산자(211–215), ts 그대로 | 명세 §8.8·§12.8.19, grammar.js `sizeof_expression`의 `type` 필드 |
| `csharp-B02.a030` (`nullable_type_annotation`) | `ts+witnesses[0]` | `NullableType int?`(86–90) `["nullable_type"]` → `QuestionToken ?`(89–90) `["\"?\""]` | 포함 node를 증거로 쓴 것이다. grammar.js `nullable_type`에서 `?`는 anonymous token이다. 새 ts는 매핑표 r2가 `QuestionToken`에 준 값 |
| `csharp-B02.a072` (`type_parameter_constraints`) | `ts+witnesses[0]` | 절 `where T : class`(66–81) → `ClassConstraint class`(76–81), ts `type_parameter_constraint` | grammar.js에 제약 목록 node는 없고 각 제약만 node다. 단일 제약 목록에서 목록의 범위는 그 제약과 같다. 다중 제약 목록은 증명하지 않는다 |
| `csharp-B02.a084`·`a086`·`a087` (`ref_kind 'ref'`, `parameter_modifier`, `parameter_mode_modifier: ref_kind`) | `ts+witnesses[0]` | `Parameter ref int a`(31–40) → `RefKeyword ref`(31–34), ts `["\"ref\"", "modifier"]` | grammar.js `_parameter_type_with_modifiers`가 한정자 키워드마다 독립 `modifier` node를 만든다. 세 대안의 증거 위치가 같아진다(규칙 문맥만 다르다) |
| `csharp-B02.a088` (`'out'`) · `a089` (`'in'`) | `ts+witnesses[0]` | `Parameter out int b`(42–51) → `OutKeyword`(42–45), `Parameter in int c`(53–61) → `InKeyword`(53–55) | 위와 같다 |
| `csharp-B01.a381` (`regular_interpolation`) · `a395` (`verbatim_interpolation`) | `witnesses[0].match` | `span` → `starts-at` | 명세 심볼은 중괄호 사이의 식이고 grammar.js `interpolation` node는 외부 scanner token의 alias `interpolation_brace`로 시작해 끝난다. 외부 token의 범위는 grammar 원문에 없으므로 시작만 도출할 수 있다(a383과 같은 원칙) |
| (`csharp-B01.a382` 판정) | 변경 없음 | witness `,5`(75–77) 유지 | grammar.js `interpolation_alignment_clause: seq(',', $.expression)`가 쉼표를 node 안에 두고 쉼표는 일반 anonymous token(§6.4.6)이다. 감사 asset의 `node`(앞 `,`, 외부 아님)로 표현한다 |
| 매핑표 `ts_only` `interpolation_start` | `ts_only` | 없음 → `EXTERNAL_SCANNER_NODE` | 위 네 항목의 결과: 이 type을 가리키는 Roslyn 종류가 더는 없어 목록에 든다. grammar.js `externals`가 세 시작 token을 외부 scanner token으로 선언하며 r1이 `interpolation_brace`에 준 사유와 같다 |

**사후 정정의 통계(F6).** 사후 정정은 `after_native`로 센다(r2의 23건 모두). 표본 감사의 `later_wrong`은 `after_native`이고 `ts`를 바꾼 정정이 감사에서 일치로 판정된 표본에 닿은 수(`csharp-B01.a335` 1건)이며, 항목 이름으로 판정하지 않는다. `basis_independent`는 통계가 아니라 근거의 성격이다. 사후 정정한 기대값 대안은 15개(native 비교에서 4개, 전수 감사에서 11개)이고 모두 일치로 센다: r2 executed의 `corrected` 15·`corrected_match` 15·`sealed_match` 411이다. 전수 감사가 고친 11건의 `after_native`는 시간 표지다: 정정 시점이 native 대조 이후이고 근거에는 native 결과가 없다(`basis_independent`). native는 정정된 값이 모순되지 않음을 확인할 뿐이다.

한계: 보간 시작 정정 대안의 새 기대값은 `interpolation_start` node의 **시작**만 요구하고 뒤따르는 따옴표 node는 확인하지 않는다(범위·순서를 도출할 근거가 grammar 원문에 없다). **남은 과제(G1-3d):** 시작 token을 "시작 node + 따옴표 node"의 복합으로 매핑표에 적고 그 순서·범위를 대조하는 것은 매핑표 schema 확장이 필요해 G1-3d(보간 문자열)로 넘겼다.

재생성(순서): `go run ./src/dev/csharp-g1-correct -root . [-write] [-grammar … -spec …]`(r2 매핑표·기대값, native 없음) → `csharp-g1-native`(HARD Job, 원시 관측) → `go run ./src/dev/csharp-g1-executed -root . -native <원시 관측> [-write]`(r2 executed; `-r1`은 봉인된 r1) → `csharp-g1-ledger`·`csharp-g1-roslyn-asset -verify`(원장·관측은 내용이 그대로여서 pin이 바뀌지 않는다).

## 차분 oracle 대안 전수 감사 (G1AUDIT76)

차분 oracle 대안(`basis: differential-oracle`) 76개(B02 56 + B01 보간 20)는 witness를 Roslyn 관측에서 받아 명세는 매핑표를 거쳐서만 뒤에 있었다. 이 전수 감사가 76개 모두를 명세 원문에서 손으로 도출해 r2 witness와 대조한다.

- **asset:** [`csharp-g1-audit76-r1.json`](../../src/contracts/csharp-g1-audit76-r1.json)(schema `tsgk-csharp-g1-audit76/r1`). 도구 `src/dev/csharp-g1-audit76`(`-in`으로 저작한 행을 비교 결과와 함께 완성해 쓰고, `-in` 없이 추적 asset을 검증하고, `-grammar -spec`으로 모든 ground를 고정 문서와 대조한다), 검증 `ParseCSharpG1Audit76`.
- **행(76개, id순):** `id`, witness `source`, 손 도출한 수락 여부(`accepted`), lexeme 형식(`form`), source 안의 모든 인스턴스 범위(`instances`), 명세 ground(`ground`: 절·인용문·`doc_sha256`·`heading`·`gap`, 정정 기록의 ground와 같은 형식과 같은 점검), 선택적 `node`, 비교 결과(`result`·`relation`)와 정정 전 witness의 결과(`initial`).
- **비교 규칙:** `match: span`이면 witness가 인스턴스와 범위가 같아야 하고 `starts-at`이면 시작이 같아야 한다. `node`는 grammar.js가 node 안에 넣는 구분 기호(앞 `leading`·뒤 `trailing`)를 적고 grammar ground를 가진다. 이때 인스턴스(명세 심볼) 대신 구분 기호를 더한 범위를 비교한다(`NODE_EXTENT_*`). 구분 기호가 외부 scanner token(`external`)이면 범위를 grammar 원문이 정하지 않으므로 `span` 주장은 맞지 않고(`EXTENT_NOT_DERIVABLE`) `starts-at`만 맞을 수 있다. 3행(a381·a382·a395)만 `node`를 가진다.
- **verifier가 보이는 것:** asset이 파싱되고, 행이 r2의 차분 oracle 대안 76개와 정확히 같고(중복·누락 없음), 모든 행에 명세 ground가 있고(offline 점검), 각 행이 말하는 `result`·`relation`·`initial`이 **지금의 r2 witness와 r2의 정정 기록의 이전 값으로 다시 계산한 것과 같다**(r2를 정정하면 감사도 같이 갱신하지 않으면 시험이 실패한다). r2는 pin하지 않는다: 비교는 주어진 r2에 대해 이뤄진다.
- **결과:** 처음 손 도출(r2 정정 전)은 불일치 12건이었다(witness가 어휘소 대신 포함 node를 쓴 7건, 규칙 문맥이 틀린 2건, 구분 기호를 포함한 node 3건). 판정은 사용자 결정에 따라 7건을 정확 token witness로(`contains` match 종류는 도입하지 않는다), 2건을 올바른 위치로 교체하고, a381·a395는 `starts-at`로 약화하고 a382는 witness를 유지했다(위 표). 정정 뒤 **76/76 MATCH**이고 `initial`이 MISMATCH였던 행은 11(`corrected`)이다. a382는 `node`로 판정하므로 asset에서는 `initial`도 MATCH다(손 도출의 심볼 범위와는 다르고 구분 기호는 grammar ground가 근거다).
- **독립성과 한계:**
  - 감사 비교는 type(`ts`)을 읽지 않는다. 비교 함수는 witness와 source 본문만 받고, 정정 기록의 이전 값은 witness로만 읽는다. `TestCSharpG1Audit76NeverNamesTheExpectedTypes`가 감사 파일에 필드 `TS`의 선택자·복합 리터럴이 없음을 AST로 고정하고(변이로 확인), 행에 `ts` 멤버를 넣으면 decoder가 거부한다. 감사 파일은 분리 시험에서 **도출 쪽**으로 분류되어 native 입력에 닿지 않음은 호출 폐쇄가 증명한다. 호출 폐쇄는 함수가 어떤 필드를 읽는지는 표현하지 못하므로 `ts`를 읽지 않음은 위 구문 검사에 기댄다. 같은 파일 밖의 도우미가 `ts`를 읽는 경로는 이 검사가 막지 못한다.
  - 손 도출은 명세 원문(csharpstandard `79ad4e90…`)만으로 했다. 도출 전에 작업자는 `a378`·`a379`의 `ts`와 `a378`의 witness를 보았고 Roslyn·tree-sitter-c-sharp 설계에 대한 일반 지식이 있었다. 어긋남의 판정(정정)에는 grammar.js 원문을 읽었다. native 산출물은 읽지 않았다.
  - 인스턴스 목록은 한 사람이 명세 문장에서 도출한 것이고 구문 분석 결과가 아니다. 비교하는 성질은 **위치와 lexeme 형식**이며 type이 아니다. 판정이 명세로 정해지지 않는 인스턴스(`T?`의 값 형식 여부 등)는 감사 초안에서 따로 적었고 비교 결과에 영향이 없다.
  - 인용문은 명세의 production 한 문단이다(어휘소가 명세에서 정의되는 곳). 문장 근거는 JSON의 `form`과 이 절이 요약한다.

## 분리 시험

`TestCSharpG1ExpectedSeparation`은 다음을 보인다.

- (보조) 도출 경로의 코드(`csharp_g1_spec.go`, `_expected*.go`, `csharp_g1_kindmap.go`)는 `os`·`exec`·네트워크·native·reproduce·runner 패키지를 import하지 않고 native 입력을 가리키는 식별자·문자열을 갖지 않는다. 이 검사는 이름과 고정 파일 목록에 기대므로 우회할 수 있어, 우회를 막는 증명은 위 호출 폐쇄 시험이 한다. 도출은 바이트 입력만 받는 함수다.
- 명세 도출기(`csharp_g1_spec.go`)는 `regexp`·`strings`·`unicode/utf8`만 쓰며 Roslyn 관측·매핑표·기대값·원장을 가리키는 식별자가 없다. 관측 verifier와 매핑표 코드는 기대값 쪽을 가리키지 않는다.
- 전수 감사 파일(`csharp_g1_audit76.go`)은 도출 쪽으로 분류하고 호출 폐쇄의 진입점(`ParseCSharpG1Audit76` 등)에 넣었다. 폐쇄가 표현하지 못하는 "`ts`를 읽지 않음"은 [전수 감사](#차분-oracle-대안-전수-감사-g1audit76)의 AST 시험이 따로 고정한다.
- asset이 pin하는 입력은 선언 3개, 사례 원문 fixture 4개, 관측 asset, 매핑표뿐이다. fixture에는 native 결과를 담는 member가 없다.

## 검증 code

| code | 거부하는 것 |
|---|---|
| `G1_EXPECTED_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES` | 형식·알 수 없는 member, schema, 4 MiB 상한, 정규 bytes와 다름 |
| `G1_EXPECTED_PIN` | 입력·관측·매핑표 pin 불일치 |
| `G1_EXPECTED_ALTERNATIVES` | 대안의 누락·추가·중복·교환, 상태·근거·type·witness·사유 변조 |
| `G1_EXPECTED_CROSS`, `_UNDISPOSED` | 명세 대조 결과 변조, 처분 없는 불일치 |
| `G1_EXPECTED_SYMBOLS`, `_AUDIT`, `_METAMORPHIC`, `_METAMORPHIC_UNSEALED`, `_COUNTS` | symbols 목록, 감사, 변형, 봉인되지 않은 변형, 집계 변조 |
| `G1_EXECUTED_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES` | executed asset의 형식·알 수 없는 member, schema, 4 MiB 상한, 정규 bytes·basis와 다름 |
| `G1_EXECUTED_PIN`, `_SOURCES`, `_NATIVE` | 기대값·매핑표·관측 pin, source 목록·신원·수락 여부, native provenance·한도·원시 관측 pin |
| `G1_EXECUTED_ALTERNATIVES`, `_EXPECTATION` | 대안의 누락·추가, 봉인된 witness·type·근거·행의 변조 |
| `G1_EXECUTED_RESULT`, `_EVIDENCE`, `_COUNTS`, `_UNDISPOSED`, `_RAW` | 증거와 모순되는 결과·`executed`·`map`, 증거 형식, 집계, 처분 없거나 바뀐 불일치, 원시 관측과 다른 asset |
| `G1_CORRECTION_FORM`, `_GROUND`, `_ITEM`, `_OLD` | r2 변경 기록의 형식(`after_native`·사유·중복·필드), 근거(grammar·명세 인용 둘 다, 인용 길이·문서 SHA-256·heading·gap), 알 수 없는 항목, r1이 가진 값과 다른 이전 값·새 값 형식 |
| `G1_CORRECTION_UNLISTED`, `_UNAUTHORED`, `_PIN`, `_BYTES` | 변경 기록에 없는 차이, 저작되지 않은(바뀐) 기록, r1·매핑표 pin 불일치, 정규 bytes와 다름. r1 변조는 r1 verifier의 code(`G1_EXPECTED_*`·`G1_KINDMAP_*`)로 거부된다 |
| `G1_NATIVE_JSON`, `_SCHEMA`, `_SIZE`, `_SOURCE`, `_TREE` | 원시 관측의 형식, source 중복·상태, node 범위·부모·digest |
| `G1_LEDGER_SEALED`, `_EXECUTED` | 원장이 읽는 기대값·executed asset의 상태·사유·대안 참조 위반 |
| `G1_KINDMAP_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_GRAMMAR`, `_VOCABULARY`, `_ADDITION`, `_KINDS`, `_CLASS`, `_ENTRY`, `_COVERAGE`, `_TS_ONLY`, `_COUNTS` | 매핑표의 형식·pin·어휘·항목·관측 종류 덮임·집계 |

```
go run ./src/dev/csharp-g1-kindmap -root . -grammar <tree-sitter-c-sharp checkout>
go run ./src/dev/csharp-g1-expected -root .
go run ./src/dev/csharp-g1-executed -root . [-native <원시 관측> [-write]]
go run ./src/dev/csharp-g1-correct -root . [-grammar <checkout> -spec <dir>]
go run ./src/dev/csharp-g1-ledger -root .
```

## 이 단계가 주장하지 않는 것

- tree-sitter 결과가 맞다는 판정. 기대값 asset은 native를 처음 실행하기 전의 기대값이며, native 비교는 별도 asset이 witness에 한정해 기록한다.
- Roslyn이 명세와 같다는 일반 주장. 표본 감사와 대조는 이 관측 범위 안의 근거다.
- BLOCKED 대안과 B03~B05·V 행의 기대값, 오류 복구(D3)와 symbols 문맥. native 비교의 기록(`EXECUTED`)은 BLOCKED 대안에 없다.
- 승인 pin: 관측·기대값·매핑표·executed asset은 사용자가 함께 승인한다(Q1). 승인 tuple은 관측 **r3**를 묶는다(사용자 결정 2026-11-13); 관측 r2는 기대값·executed가 pin하는 동결 입력이다. 승인 object는 만들지 않았다. 승인 대상 SHA-256은 위 재생성 순서의 고정점 값이다.
