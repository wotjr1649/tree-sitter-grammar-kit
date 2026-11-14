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

- **전수 감사를 반영한 층별 보고**(`G1AuditStrata(x, rows)`, rows는 전수 감사 asset의 행): 차분 oracle 층은 표본 4개 대신 76개 전체이고 불일치는 r2 witness 대비 현재의 불일치 수다. `found_and_corrected`는 처음 비교에서 틀려 있다가 명세·grammar 원문으로 정정된 대안 수이며 두 갈래를 합친다: 전수 감사가 찾은 것(`found_by_audit`: 처음 비교가 MISMATCH였던 11건)과 감사 전에 native 비교가 드러낸 것(`found_by_native`: 사후 witness 정정이 있으나 감사가 명세만 보고 처음부터 MATCH로 계산한 `a383`·`a396`·`a397` 3건, 매핑표 항목의 오류였다). `upper_95_percent_with_found`는 둘을 모두 오류로 센 정확한 상한이고 `upper_95_percent_with_found_by_audit`는 감사가 찾은 것만 센 상한이다(감사는 native가 찾은 3건을 못 찾았으므로 두 값을 나란히 적는다). **표의 `upper_95_percent`(3.9%)와 불일치 0은 정정을 마친 뒤의 값이다**: 찾을 때까지 고쳐서 76/76이 된 것이므로 정정을 오류로 센 상한과 함께 읽어야 한다. 76개는 층 전체이므로 이 상한은 "비교한 성질"에 대한 것이며 비교하지 않은 대안의 추정이 아니다. 비교한 성질은 witness의 위치와 lexeme 형식이고 type(`ts`)이 아니다(type은 표본 4개만 비교됐다).

  | 층 | 표본 | 불일치(정정 뒤) | 95% 상한(정정 뒤) | 정정된 대안 | 정정을 오류로 센 상한(감사가 찾은 것만 센 상한) |
  |---|---:|---:|---:|---:|---:|
  | `spec-derived` | 16 | 0 | 17.1% | (later_wrong 1) | 26.4% |
  | `differential-oracle`(전수 감사 76개) | 76 | 0 | 3.9% | 14 (감사 11 + native 3) | 27.3% (22.8%) |
  | BLOCKED 처분 확인 | 5 | 0 | 45.1% | — | — |
  | COVERAGE_GAP | 1 | — | — | — | — |
  | 기대값을 비교한 표본(위 두 층) | 92 | 0 | 3.2% | 14 | 24.0% (20.3%; later_wrong 1 포함) |
  | 전체 | 98 | 0 | 3.0% | 14 | 22.6% (19.1%; later_wrong 1 포함) |

  `TestCSharpG1AuditStrataCountTheNativeFoundCorrectionsToo`가 이 표의 모든 행(그리고 type 상한)을 그대로 고정한다.

  두 층은 비교한 성질이 달라 합산 상한(3.2%·3.0%)은 "spec-derived의 type·text 일치"와 "차분 oracle의 위치 일치"를 섞은 값이다. 층별 값을 우선한다.
- **차분 oracle 층의 type 상한(`G1AuditStrataWithTypes(x, rows, typeRows)`).** 위 표의 차분 oracle 행은 **위치** 상한이다. type 상한은 같은 76개를 type 감사([G1TYPEAUDIT76](#차분-oracle-대안-type-감사-g1typeaudit76))로 따로 대조한 값이고 같은 층 행의 별도 member(`type_samples`·`type_mismatches`·`type_found_and_corrected`·`type_not_derivable`·`type_upper_95_percent`·`type_upper_95_percent_with_found`)로 적는다. 두 상한을 하나로 합치지 않고, 합계 행도 위치 상한만 센다.

  | 차분 oracle 층(76개) | 비교 | 불일치 | 95% 상한(정확) | 정정된 대안 | 정정을 오류로 센 상한 | 문법 원문으로 정해지지 않음 |
  |---|---|---:|---:|---:|---:|---:|
  | 위치(witness) | 76 | 0 | 3.9% | 14 | 27.3% (감사만 22.8%) | — |
  | type(`ts`) | 76 | 0 | 3.9% | 2 | 8.1% | 3 (후보 집합 유지) |

  type 행의 "비교 76"에는 grammar 원문이 type을 정하지 못해 후보 집합(`["ref", modifier]`)을 `ts`와 대조한 3개가 들어 있다. 이들은 후보 집합과 같음만 보이고 type을 도출한 것이 아니므로 "도출한 73 + 후보 일치 3"으로 읽는다. 위치 상한도 type 상한도 grammar.js와 명세를 읽는 손 도출과 기대값의 일치를 말하며 Roslyn이나 tree-sitter-c-sharp의 오류율이 아니다.
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
- **원장 r2(G1-3b):** 위 규칙은 r2에서 두 쌍에 적용된다: B01·B02(r2 기대값·executed)와 B03·B04([native 비교](#원장-r2)). r2는 새 schema `tsgk-csharp-g1-ledger/r2`·새 파일이고 r1은 동결이다. B03·B04의 상태는 `EXECUTED` 291(MATCH)·`SEALED` 5(MISMATCH)·`BLOCKED` 92이고 P 의무는 `NO_EXPECTED`로 남는다.
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
- **변경 기록(`corrections`).** r2 asset은 r1과 변경 기록만큼만 다르다. 항목마다 대안 id(매핑표는 Roslyn 종류, 목록 정정은 type)·`field`·이전 값(`old`)·새 값(`new`)·`reason`·`ground`·`after_native`·`basis_independent`를 둔다. `after_native`는 시간이다: native 결과를 본 뒤의 정정이면 `true`이며, 사용자 결정(2026-11-13)에 따라 **r2의 모든 정정이 `true`**다(raw 시작 token 두 항목도 native 불일치를 본 뒤 같은 구조 오류를 고친 것이다). verifier는 `false`를 형식 위반으로 거부한다. `basis_independent`는 근거다: 정정의 **ground**(정정을 정당화하는 인용문)가 명세와 grammar 원문뿐이고 native 산출물이나 Roslyn 관측이 ground가 아니면 `true`다(r2의 모든 정정이 그렇다. ground는 `grammar.js`와 `spec` 두 문서 종류만 가질 수 있다). **이 표지는 새 값(`new`)의 출처가 독립이라는 뜻이 아니다.** 증거 정정(`ts+witnesses[0]`) 중 a030·a084·a086·a087·a088·a089의 새 `ts`는 이유문이 밝히듯 매핑표 r2가 새 witness의 Roslyn 종류에 주는 값이며(verifier가 새 `ts`가 그 값에 속함을 확인한다) grammar 원문에서 따로 도출한 값이 아니다. 이 정정에서 ground가 정당화하는 것은 witness의 이동(포함 node → 정확한 token)이다. 그 `ts`를 grammar 원문만으로 다시 대조한 것이 type 감사다: a030은 도출과 같고(`EQUAL`), a088·a089는 도출과 달라 `[modifier]`로 좁혔고(좁힌 값의 근거는 grammar 원문이다), a084·a086·a087은 grammar 원문이 정하지 못해 후보 집합(`["ref", modifier]`, 매핑표 값)을 유지하며 '도출 불가'로 표시한다. 따라서 `basis_independent: true`인 행의 `ts`가 grammar 도출이라는 뜻은 아니며 도출 여부는 type 감사의 분류가 말한다. 사용자 결정(#233, 2026-11-13)이 정한 정의는 'true이려면 ground가 명세·grammar 원문뿐'이고 위 여섯 행은 이를 만족하므로 `true`로 둔다(`TestCSharpG1EvidenceTSProvenanceIsStated`가 여섯 행과 이 구분을 고정한다). 사후 정정의 통계(표본 감사의 `later_wrong` 등)는 `after_native`로 센다. 두 표지는 저작된 표와 같아야 하고 시험이 항목 이름이 아니라 저작된 표를 직접 고정한다(`TestCSharpG1CorrectedAssetsVerify`). `ground`는 고정 grammar 원문(`grammar.js`의 규칙)과 명세(csharpstandard alpha-v13 commit `79ad4e90…`의 파일·절)의 인용문이며 둘 다 있어야 한다(tree-sitter 쪽은 grammar 원문만이 근거다). 각 ground는 문서 없이 CI에서 확인된다(`doc_sha256`·`heading`·`gap`과 인용문이 정정 기록 자체에 있다): 인용문은 공백을 하나로 접어 40자 이상이어야 하고, `doc_sha256`은 고정 문서의 SHA-256이어야 하며(grammar.js는 매핑표의 pin `63d357e0…`, 명세는 `lexical-structure.md` `332d1129…`·`expressions.md` `bcc4c11c…`·`types.md` `294308e7…`·`classes.md` `0d6ac924…`·`unsafe-code.md` `ba020202…`), `heading`은 `at`이 가리키는 절(명세는 절 번호를 가진 markdown 제목, grammar는 `<규칙>:`)이어야 하고 인용문까지의 줄 수 `gap`은 200 이하여야 한다. `src/dev/csharp-g1-correct -grammar … -spec …`(시험의 `TSGK_G1_GRAMMAR_JS`·`TSGK_G1_SPEC_DIR`)은 문서가 있을 때만 도는 **실제 문서 점검**이다: 문서의 SHA-256이 ground와 같고, 인용문이 있고, heading 줄이 인용문 시작 줄보다 정확히 `gap`줄 앞에 있다. 한계: CI는 인용문이 문서에서 왔음을 증명하지 못하고(문서가 추적되지 않는다), 문서가 고정 SHA-256과 같고 인용문이 거기 있음은 문서가 있는 환경의 점검만이 보인다. `old`는 r1이 가진 값과 같아야 하고, 기록에 없는 차이·근거 없는 항목·저작되지 않은(표지 값이 다른 것 포함) 항목은 거부된다.
- **근거는 명세·grammar 원문만.** 정정 값은 native 산출물이 아니라 명세와 grammar 원문에서 다시 도출한다. native와 같아지는 것은 근거가 아니다. 원문으로 정할 수 없으면 정정하지 않고 BLOCKED와 사유로 둔다. 도출 코드(`csharp_g1_correct.go`)는 native 입력을 받지 않는다.
- **통계를 둘로 나눈다.** "봉인 시 일치"(r1 executed asset: 426 중 422)와 "사후 정정 뒤 일치"(r2 executed asset의 `corrected`·`corrected_match`: 정정 15건 중 n건)를 합치지 않는다. 정정한 대안은 독립 기대값 통계(표본 감사, 봉인 시 일치율)에서 "사후 정정"으로 따로 센다. r2의 표본 감사·metamorphic·cross_check 절은 r1의 것 그대로다(봉인 당시의 기록이며 정정 값으로 다시 계산하지 않는다).
- **r2 executed asset**은 r2 기대값·매핑표를 같은 native 원시 관측(`raw_native`)과 다시 대조한 기록이며, `sealed_match`(정정하지 않은 대안 중 일치)·`corrected`·`corrected_match`를 가진다.

### r2 정정 25건 (기대값 20 + 매핑표 5)

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
| `csharp-B02.a088` · `a089` | `ts` | `["\"out\"", modifier]` → `[modifier]`, `["\"in\"", modifier]` → `[modifier]` | type 감사(G1TYPEAUDIT76)가 찾았다. 파라미터 위치의 `out`·`in`은 grammar.js `_parameter_type_with_modifiers`의 `alias(choice('this', 'scoped', 'ref', 'out', 'in', 'readonly'), $.modifier)`를 거쳐 `modifier` 리프 하나가 되고, `type`의 `ref_type`은 `ref`만 가져 같은 자리에서 익명 `"out"`·`"in"`을 만드는 규칙이 없다. 앞 정정이 증거와 함께 가져온 `ts`가 매핑표 r2의 일대다 대응(익명 token과 `modifier`)이었던 것을 좁힌다. 매핑표는 바꾸지 않는다. 저장된 native 관측과 다시 대조해 두 대안은 MATCH다(executed r2 426/426) |
| `csharp-B01.a381` (`regular_interpolation`) · `a395` (`verbatim_interpolation`) | `witnesses[0].match` | `span` → `starts-at` | 명세 심볼은 중괄호 사이의 식이고 grammar.js `interpolation` node는 외부 scanner token의 alias `interpolation_brace`로 시작해 끝난다. 외부 token의 범위는 grammar 원문에 없으므로 시작만 도출할 수 있다(a383과 같은 원칙) |
| (`csharp-B01.a382` 판정) | 변경 없음 | witness `,5`(75–77) 유지 | grammar.js `interpolation_alignment_clause: seq(',', $.expression)`가 쉼표를 node 안에 두고 쉼표는 일반 anonymous token(§6.4.6)이다. 감사 asset의 `node`(앞 `,`, 외부 아님)로 표현한다 |
| 매핑표 `ts_only` `interpolation_start` | `ts_only` | 없음 → `EXTERNAL_SCANNER_NODE` | 위 네 항목의 결과: 이 type을 가리키는 Roslyn 종류가 더는 없어 목록에 든다. grammar.js `externals`가 세 시작 token을 외부 scanner token으로 선언하며 r1이 `interpolation_brace`에 준 사유와 같다 |

**사후 정정의 통계(F6).** 사후 정정은 `after_native`로 센다(r2의 25건 모두). 표본 감사의 `later_wrong`은 `after_native`이고 `ts`를 바꾼 정정이 감사에서 일치로 판정된 표본에 닿은 수(`csharp-B01.a335` 1건)이며, 항목 이름으로 판정하지 않는다. `basis_independent`는 통계가 아니라 근거의 성격이다. 사후 정정한 기대값 대안은 15개(native 비교에서 4개, 전수 감사에서 11개)이고 모두 일치로 센다: r2 executed의 `corrected` 15·`corrected_match` 15·`sealed_match` 411이다. type 감사가 고친 a088·a089의 `ts` 좁힘 2건은 이미 센 대안(전수 감사의 11건 안)에 대한 두 번째 정정이므로 정정한 대안 수 15를 늘리지 않는다. 전수 감사가 고친 11건의 `after_native`는 시간 표지다: 정정 시점이 native 대조 이후이고 근거에는 native 결과가 없다(`basis_independent`). native는 정정된 값이 모순되지 않음을 확인할 뿐이다.

한계: 보간 시작 정정 대안의 새 기대값은 `interpolation_start` node의 **시작**만 요구하고 뒤따르는 따옴표 node는 확인하지 않는다(범위·순서를 도출할 근거가 grammar 원문에 없다). **남은 과제(G1-3d):** 시작 token을 "시작 node + 따옴표 node"의 복합으로 매핑표에 적고 그 순서·범위를 대조하는 것은 매핑표 schema 확장이 필요해 G1-3d(보간 문자열)로 넘겼다.

재생성(순서): `go run ./src/dev/csharp-g1-correct -root . [-write] [-grammar … -spec …]`(r2 매핑표·기대값, native 없음) → `csharp-g1-native`(HARD Job, 원시 관측) → `go run ./src/dev/csharp-g1-executed -root . -native <원시 관측> [-write]`(r2 executed; `-r1`은 봉인된 r1) → `csharp-g1-ledger`·`csharp-g1-roslyn-asset -verify`(원장·관측은 내용이 그대로여서 pin이 바뀌지 않는다).

## 차분 oracle 대안 전수 감사 (G1AUDIT76)

차분 oracle 대안(`basis: differential-oracle`) 76개(B02 56 + B01 보간 20)는 witness를 Roslyn 관측에서 받아 명세는 매핑표를 거쳐서만 뒤에 있었다. 이 전수 감사가 76개 모두를 명세 원문에서 손으로 도출해 r2 witness와 대조한다.

- **asset:** [`csharp-g1-audit76-r1.json`](../../src/contracts/csharp-g1-audit76-r1.json)(schema `tsgk-csharp-g1-audit76/r1`). 도구 `src/dev/csharp-g1-audit76`(`-in`으로 저작한 행을 비교 결과와 함께 완성해 쓰고, `-in` 없이 추적 asset을 검증하고, `-grammar -spec`으로 모든 ground를 고정 문서와 대조한다), 검증 `ParseCSharpG1Audit76`.
- **행(76개, id순):** `id`, witness `source`, 손 도출한 수락 여부(`accepted`), lexeme 형식(`form`), source 안의 모든 인스턴스 범위(`instances`), 명세 ground(`ground`: 절·인용문·`doc_sha256`·`heading`·`gap`, 정정 기록의 ground와 같은 형식과 같은 점검), 선택적 `node`, 비교 결과(`result`·`relation`)와 정정 전 witness의 결과(`initial`).
- **비교 규칙:** `match: span`이면 witness가 인스턴스와 범위가 같아야 하고 `starts-at`이면 시작이 같아야 한다. `node`는 grammar.js가 node 안에 넣는 구분 기호(앞 `leading`·뒤 `trailing`)를 적고 grammar ground를 가진다. 이때 인스턴스(명세 심볼) 대신 구분 기호를 더한 범위를 비교한다(`NODE_EXTENT_*`). 구분 기호가 외부 scanner token(`external`)이면 범위를 grammar 원문이 정하지 않으므로 `span` 주장은 맞지 않고(`EXTENT_NOT_DERIVABLE`) `starts-at`만 맞을 수 있다. 3행(a381·a382·a395)만 `node`를 가진다.
- **인스턴스 규칙(`csharp_g1_audit76_rules.go`):** 행마다 `form`(자유 문장)과 별개로 인스턴스 한 개의 본문 `text[lo:hi]`에 대한 패턴(전체 일치, 선택적으로 일치하면 안 되는 `Not`)을 코드 표(`g1a76Rules`)로 두고, verifier가 목록의 모든 인스턴스 본문이 그 행의 규칙에 맞는지 확인한다(`G1_AUDIT76_RULE`). 규칙은 asset이 아니라 코드에 있으므로 위치 감사 asset의 bytes(SHA-256 `d2d0a400…`)는 바뀌지 않는다. 규칙은 목록에 적힌 인스턴스가 그 형식의 것인지를 판정할 뿐 빠뜨린 인스턴스를 찾지 않는다(그러려면 lexer가 필요하다). 그래서 witness를 다른 token으로 옮기고 같은 자리를 가리키게 인스턴스 목록을 짝지어 바꾸는 경우(검토 프로브: a030의 witness와 인스턴스를 `[3,4)`의 `n`으로)도 거부된다.
- **ground 핀(`csharp_g1_ground_pins.go`):** 행의 ground(문서 종류·절·공백을 접은 인용문·문서 SHA-256·heading·gap)의 SHA-256을 행마다(node의 ground는 `<id>#node`) 코드 표로 고정한다. 날조한 인용문, 다른 행에서 복사하거나 서로 바꾼 ground, 나중에 고친 인용문·heading·gap·절은 문서 없이 CI에서 `G1_AUDIT76_GROUND`로 거부된다. 핀은 인용문이 문서에서 왔다는 증명이 아니다: 그것은 문서가 있는 환경의 점검(`TSGK_G1_GRAMMAR_JS`·`TSGK_G1_SPEC_DIR`, 도구의 `-grammar -spec`)이 보이며 CI는 이 변수를 설정하지 않는다. 핀 표는 도구의 `-pins`가 추적 asset에서 출력한다(`gofmt -w` 뒤 붙인다). ground를 바꾸면 표의 변경이 함께 검토된다.
- **verifier가 보이는 것:** asset이 파싱되고, 행이 r2의 차분 oracle 대안 76개와 정확히 같고(중복·누락 없음), 모든 행에 명세 ground가 있고(offline 점검과 핀), 모든 인스턴스가 행의 규칙에 맞고, 각 행이 말하는 `result`·`relation`·`initial`이 **지금의 r2 witness와 r2의 정정 기록의 이전 값으로 다시 계산한 것과 같다**(r2를 정정하면 감사도 같이 갱신하지 않으면 시험이 실패한다). r2는 pin하지 않는다: 비교는 주어진 r2에 대해 이뤄진다.
- **결과:** 처음 손 도출(r2 정정 전)은 불일치 12건이었다(witness가 어휘소 대신 포함 node를 쓴 7건, 규칙 문맥이 틀린 2건, 구분 기호를 포함한 node 3건). 판정은 사용자 결정에 따라 7건을 정확 token witness로(`contains` match 종류는 도입하지 않는다), 2건을 올바른 위치로 교체하고, a381·a395는 `starts-at`로 약화하고 a382는 witness를 유지했다(위 표). 정정 뒤 **76/76 MATCH**이고 `initial`이 MISMATCH였던 행은 11(`corrected`)이다. a382는 `node`로 판정하므로 asset에서는 `initial`도 MATCH다(손 도출의 심볼 범위와는 다르고 구분 기호는 grammar ground가 근거다).
- **독립성과 한계:**
  - **위치 감사는 type(`ts`)을 읽지 않는다. 이 독립성은 어휘 검사가 아니라 구조로 만든다.** 감사는 r2 asset도 정정 기록도 받지 않는다. 입력은 witness 위치만 담은 좁은 값(`G1Audit76Input`: 대안마다 id·r2 witness·정정 전 witness, 그리고 source 본문)이고, r2에서 그 값을 만드는 곳은 `csharp_g1_audit_view.go`(`G1Audit76View`) 한 군데이며 감사 함수는 그것을 부르지 않는다(정정 기록의 이전 값도 그 뷰가 witness로만 읽어 넘긴다). 세 겹으로 확인한다. (1) `TestCSharpG1AuditInputsAreNarrow`가 입력 타입의 모든 member를 고정한다: `ts`·매핑표·r2 bytes를 실을 member가 없고(JSON map 접근은 bytes가 있어야 하는데 bytes member가 없다) 인터페이스·함수 member도 없다. (2) 감사 파일에 r2 bytes를 읽는 문장을 쓰면 컴파일되지 않는다(검토자의 S2: `json.Unmarshal(expected, &map)` 뒤 `am["ts"]`. `TestCSharpG1AuditsRejectTheBypassesOfTheReview`가 타입 검사 오류로 확인한다). (3) 감사 함수의 호출 폐쇄는 자기 파일·규칙·ground·핀 표 밖의 G1 파일에 닿지 않는다(`TestCSharpG1AuditsReachOnlyTheirOwnInputs`. 검토자의 S: 다른 파일의 도우미가 `G1ExpAlt.TS`를 읽는 우회를 같은 시험의 음성 사례로 둔다). 행에 `ts` 멤버를 넣으면 decoder도 거부한다. 구문 검사(`TestCSharpG1Audit76NeverNamesTheExpectedTypes`: 필드 `TS`의 선택자·키, 문자열 `"ts"`·`"kind"`, r2·정정 기록 타입 이름)는 보조다. 감사 파일은 분리 시험에서 **도출 쪽**으로 분류되어 native 입력에 닿지 않음은 호출 폐쇄가 증명한다.
  - 손 도출은 명세 원문(csharpstandard `79ad4e90…`)만으로 했다. 도출 전에 작업자는 `a378`·`a379`의 `ts`와 `a378`의 witness를 보았고 Roslyn·tree-sitter-c-sharp 설계에 대한 일반 지식이 있었다. 어긋남의 판정(정정)에는 grammar.js 원문을 읽었다. native 산출물은 읽지 않았다.
  - 인스턴스 목록은 한 사람이 명세 문장에서 도출한 것이고 구문 분석 결과가 아니다. verifier는 목록의 각 항목이 원문에서 행의 규칙에 맞는 본문임을 확인할 뿐(위) 목록이 완전한지는 확인하지 못한다. 비교하는 성질은 **위치와 lexeme 형식**이며 type이 아니다. 판정이 명세로 정해지지 않는 인스턴스(`T?`의 값 형식 여부 등)는 감사 초안에서 따로 적었고 비교 결과에 영향이 없다.
  - 인용문은 명세의 production 한 문단이다(어휘소가 명세에서 정의되는 곳). 문장 근거는 JSON의 `form`과 이 절이 요약한다.

## 차분 oracle 대안 type 감사 (G1TYPEAUDIT76)

[전수 감사](#차분-oracle-대안-전수-감사-g1audit76)는 위치만 본다(결정 R-4 B: 위치 감사에 grammar 지식이 섞이지 않게 type 대조는 따로 둔다). 같은 76개의 기대 type(`ts`)은 Roslyn 종류를 매핑표로 옮긴 값이어서 표본 4개만 대조됐다. 이 감사가 나머지를 대조한다.

- **asset:** [`csharp-g1-typeaudit76-r1.json`](../../src/contracts/csharp-g1-typeaudit76-r1.json)(schema `tsgk-csharp-g1-typeaudit76/r1`). 위치 감사 asset(`csharp-g1-audit76-r1.json`)은 바꾸지 않고(SHA-256 고정: `TestCSharpG1PositionAuditIsFrozenAndFreeOfTypes`) 별도 계약으로 둔 것은 그 asset의 header가 "type을 읽지 않고 도출했다"고 약속하고 verifier·시험이 그것을 고정하기 때문이다. 같은 파일에 type을 넣으면 약속의 뜻이 바뀐다. 도구 `src/dev/csharp-g1-typeaudit76`(`-in`으로 저작한 행을 비교 결과와 함께 완성해 쓰고, `-in` 없이 추적 asset을 검증하고, `-grammar`로 모든 ground를 고정 grammar.js와 대조한다), 검증 `ParseCSharpG1TypeAudit76`.
- **도출:** 고정 `grammar.js`(SHA-256 `63d357e0…`, 매핑표의 pin)의 규칙 이름·alias·`field()`·숨김 규칙(`_x`)·`inline`·`supertypes`·`externals`만으로 witness 위치의 node type을 손으로 도출했다. 매핑표·기대 type·Roslyn 종류·native 출력은 도출 중에 보지 않았다. 기대 type은 도출표를 확정한 뒤 처음 읽었다.
- **행(76개, id순):** `id`, 도출 분류(`class`), 도출한 type(`types`), 같은 범위의 다른 node type(`coincident`, 선택), 외부 token 표지(`extent`), 사유(`reason`), grammar ground(`ground`: 규칙·줄이 아니라 인용문·`doc_sha256`·`heading`·`gap`, 정정 기록의 ground와 같은 형식과 같은 offline 점검), 비교 결과(`result`·`relation`)와 `ts`를 고치기 전의 결과(`initial`). ground는 grammar.js만이다(명세 ground는 없다: type은 grammar가 정한다).
- **분류(76):** `NODE` 55(규칙 이름이 곧 node type), `NO_OWN_NODE` 5(명세 production이 자기 node가 없고 같은 범위의 node가 type: 어휘 조각 4, 제약 목록 1), `ALIAS` 6(`prefix_unary_expression`으로 alias된 `*`·`&` 4, 파라미터 `out`·`in`의 `modifier` 2), `EXTERNAL_TOKEN` 7, `NOT_DERIVABLE` 3. 익명 token은 `ts`와 같은 따옴표 표기(`"?"`)로 적는다.
- **외부 scanner token 7행**(보간 문자열의 시작·본문·닫는 따옴표: a383·a384·a386·a396·a397·a398·a400)은 `extent`가 "type only, extent in the position audit"이다. grammar.js는 외부 token의 type 이름(`externals`와 alias)만 정하고 범위는 scanner가 정하므로 type만 감사하고 범위는 위치 감사가 맡는다. 이 표지는 외부 token 행에만 있어야 하고 verifier가 강제한다.
- **`NOT_DERIVABLE` 3행**(a084·a086·a087, 파라미터 `ref`): 같은 자리에서 `modifier` alias와 `ref_type`의 첫 token이 경합한다. grammar.js는 이 쌍을 `conflicts`에 적지 않았고 해소는 `prec.left`(`_parameter_type_with_modifiers`)가 암시하는 우선순위에 달려 있어 원문만으로 확정할 수 없다. 그래서 후보 집합 `["\"ref\"", modifier]`를 그대로 두고 "grammar 원문으로 도출 불가"로 표시한다(사용자 결정). native 출력으로 닫지 않는다: 닫으면 기대값이 native에서 나오게 된다. verifier는 후보 집합과 `ts`가 정확히 같을 것을 요구한다(좁히거나 넓히면 거부).
- **비교 규칙:** 도출 type 하나가 `ts`와 같으면 `EQUAL`, `ts`가 도출 type에 같은 범위의 node(`coincident`)를 더한 것이면 `WITH_COINCIDENT_NODES`(예: `new()`는 `constructor_constraint`와 그것을 감싼 `type_parameter_constraint`), 후보 집합이면 `CANDIDATE_SET_EQUAL`이고 모두 `MATCH`다. 그 밖은 `MISMATCH`이며 relation은 `EXPECTED_SUPERSET`·`EXPECTED_SUBSET`·`OVERLAP`·`DISJOINT`다.
- **결과:** 처음 비교(r2 정정 전)는 일치 70, 같은 범위의 동시 node 1(a083), 후보 집합 일치 3, **불일치 2**(a088·a089: `ts`가 파라미터 위치에서 만들어지지 않는 익명 `"out"`·`"in"`을 더 가졌다)였다. 사용자 결정으로 r2에 `ts` 좁힘 정정 2건(`[modifier]`)을 더했고(`after_native`·`basis_independent`, 위 표) 저장된 native 관측과 다시 대조해 둘 다 MATCH였다. 정정 뒤 **76/76 MATCH**이고 `initial`이 MISMATCH였던 행은 2(`corrected`)다.
- **verifier가 보이는 것:** asset이 파싱되고, 행이 r2의 차분 oracle 대안 76개와 정확히 같고, 모든 행이 형식(분류별 type 개수, 정렬·중복 없는 type 이름, 외부 token 표지, 사유, grammar ground)을 지키고, 각 행의 `result`·`relation`·`initial`이 **지금의 r2 `ts`와 정정 기록(`ts` 정정의 이전 값)으로 다시 계산한 것과 같고**, 행의 ground가 행별 SHA-256 핀([ground 핀](#차분-oracle-대안-전수-감사-g1audit76), `G1_TYPEAUDIT76_GROUND`)과 같고, 합계와 정규 bytes가 맞다. r2는 pin하지 않는다.
- **독립성과 한계:**
  - **type 감사는 매핑표를 읽지 않는다. 위치 감사와 같은 구조로 만든다.** 입력은 저작한 행과 r2의 `ts`만 담은 좁은 값(`G1TypeAudit76Input`: 대안마다 id·r2 `ts`·`ts` 정정 전 값)이고 `csharp_g1_audit_view.go`(`G1TypeAudit76View`)가 r2에서 만든다. 입력 타입의 member는 `TestCSharpG1AuditInputsAreNarrow`가 고정하고(매핑표·witness·bytes member 없음), 감사 파일에 입력에 없는 member를 읽는 문장은 컴파일되지 않으며, 감사 함수의 호출 폐쇄는 자기 파일·위치 감사의 공용 부분·ground·핀 표 밖의 G1 파일(매핑표 파일 포함)에 닿지 않는다(`TestCSharpG1AuditsReachOnlyTheirOwnInputs`와 `…RejectTheBypassesOfTheReview`의 매핑표 도우미·매핑표 parser 호출 사례). `TestCSharpG1TypeAudit76NeverReadsTheKindMap`은 보조다: 감사 파일에 매핑표를 가리키는 식별자(`KindMap`·`G1KM` 계열)와 문자열이 없고 관측·executed asset 이름도 없음을 AST로 고정한다. 감사 파일은 분리 시험에서 **도출 쪽**으로 분류되어 native 입력에 닿지 않음은 호출 폐쇄가 증명한다.
  - 도출은 한 사람이 grammar.js 원문에서 한 것이고 파서 실행이 아니다. tree-sitter의 alias·우선순위 의미에 대한 일반 지식이 쓰였다(예: alias된 문자열 token은 이름 있는 리프 하나가 된다). 이 점이 a088·a089의 판정을 받친다.
  - 도출 전에 작업자는 이 문서의 r2 정정 표를 훑다가 몇 줄(`a335`, 보간 시작의 `interpolation_start`)을 보았다. 보간 시작 type은 grammar.js 원문에서 독립적으로 확인되고 `a335`는 76개에 없다.
  - 후보 집합과 같음(3행)은 type 도출이 아니다. 외부 token 7행은 type만 도출했다.
  - 위치는 [전수 감사](#차분-oracle-대안-전수-감사-g1audit76)가 보고 type은 이 감사가 본다. 둘 다 기대값 asset 전체의 오류율이 아니다.

## G1-3b 분류 (B03·B04, G1CLASSIFY)

행 `csharp-B03`(257)·`csharp-B04`(131)의 production 대안 388개는 기대값을 만들기 전에 아래 셋 중 하나로 나뉜다. 이 단계(PR1)는 분류기와 dry-run만이며 asset을 만들지 않는다. 코드는 `csharp_g1_classify.go`(분류)·`csharp_g1_classify_rules.go`(규칙 표)·`csharp_g1_classify_view.go`(입력 뷰), 도구는 `go run ./src/dev/csharp-g1-classify -root . [-list] [-kinds]`다.

| 부류 | 근거 종류 | 대상 | 증거 |
|---|---|---|---|
| A | `spec-derived` | 필수 literal이 있는 대안, 또는 고정 lexeme인 비단말(`!`·`>>`·`>>=`·`_`)을 가진 대안 | 연결 사례의 수락된 source에서 anchor가 token 문자열로 **순서대로** 나타난다 |
| B | `differential-oracle` | 자기 Roslyn node 종류가 있는 대안(규칙 표가 종류를, 필요하면 직접 부모 종류를 적는다) | 연결 사례의 수락된 source에 그 종류의 node가 있다(부모 종류가 있으면 그 아래) |
| C | BLOCKED | 나머지 | 사유 code(아래) |

- **D-1(구조 대조의 깊이).** literal 앵커는 그 keyword·punctuation token이 나타남만 증명하고 구성 node의 type은 증명하지 않는다. 구성 node는 literal이 없는 대안(B)에서만 본다. 약한 일치는 결과의 `discriminating`이 `false`다. A는 anchor에 낱말이나 두 글자 이상의 token이 하나라도 있으면 `true`다(`(`·`,`·`;`·`.`·`:`·`=`처럼 한 글자 punctuation만이면 `false`). B는 규칙의 모든 종류가 매핑표에서 1:1이고 같은 node type을 구성 밖의 다른 Roslyn 종류와 나누지 않으면 `true`다. 이 정의는 R-5의 정의를 대신하지 않는다: PR2의 `discriminating` asset 필드는 기대 type 집합으로 따로 정한다.
- **D-1과 선택적 literal(PR2 독립 검토 지적 6).** 앵커는 production이 **모든 사용에서** 요구하는 literal이다(`g1RequiredLiterals`): 괄호 밖이고 `?`·`*`가 뒤따르지 않는 따옴표 terminal. 인스턴스가 갖지 않을 수 있는 literal(`'scoped'?`, `(',' x)*`, `('=' x)?`)은 앵커가 아니므로 선택적 literal만 가진 production은 앵커가 없어 B(자기 node가 있는 대안)다. `B03.a013`(`argument (',' argument)*`)·`B03.a182`(`'scoped'? local_variable_type identifier`)·`B04.a031`(`'scoped'? type …`)·`B04.a033`(`identifier ('=' …)?`) 네 대안은 이 한 규칙으로 B이며 일관된다(`TestCSharpG1B34OptionalLiteralsAreNotAnchors`가 규칙의 반례 표와 네 대안의 부류를 고정한다). 반대 방향의 네 대안은 따옴표 literal이 없어도 A다: 고정 lexeme인 비단말(`!`·`>>`·`_`·`>>=`)은 그 lexeme이 앵커다(위 표). 따라서 "literal이 있으면 앵커만"은 필수 literal에만 쓰는 말이며 선택적 literal이 있는 B 대안은 구성 node를 본다(그 node가 선택적 literal을 품는지는 witness의 첫 node가 정하고 위치 감사가 인스턴스와 대조한다).
- **사유 code 5개.** `ABSTRACT_CATEGORY`(사다리·범주 pass-through: 다음 단계나 union을 가리키며 자기 node가 없다), `POSITION_DEPENDENT`(부모 안의 역할: slot을 관측이 기록하지 않는다), `SEMANTIC_BINDING`(이름 binding 없이는 구별되지 않는다), `NO_OWN_NODE_FOR_CONSTRUCT`(Roslyn 또는 grammar에 자기 node가 없다: 분리된 목록, 숨은 node, 매핑표가 `none`인 종류), `NO_WITNESS_IN_LINKED_SOURCES`(연결 source에 증거가 없다). 앞의 셋과 마지막은 G1-3a와 같은 이름이고 `NO_OWN_NODE_FOR_CONSTRUCT`만 새 코드다. 매핑표가 `none`을 주는 종류만 있는 규칙은 규칙에 적힌 사유가 아니라 매핑표의 사유(`HIDDEN_NODE`·`NO_SEPARATE_NODE`)를 문장에 담아 이 코드로 막는다.
- **입력은 좁은 값이다(`G1ClassifyInput`).** 대안(id·행·production·원장의 필수 literal·연결 사례), source(수락 여부·token 문자열·node 종류와 부모 종류), Roslyn 종류(class·map·reason과 node type을 공유하는 다른 종류의 이름)뿐이다. **node type과 기대값 asset은 member가 없다.** `TestCSharpG1ClassifyInputIsNarrow`가 member를 고정하고, 호출 폐쇄 시험의 항목 "classification"이 분류 함수가 자기 두 파일 밖의 G1 파일(기대값·정정·뷰 포함)에 닿지 않음을, 우회 사례가 입력에 없는 `TS` member를 읽는 문장이 컴파일되지 않음을 고정한다. 분류 파일은 분리 시험에서 도출 쪽이며 native 입력에 닿지 않음은 같은 호출 폐쇄가 증명한다.
- **규칙 표의 한계.** 표는 production 텍스트와 Roslyn 종류 이름에서 사람이 쓴 것이다. 키는 `head#i`·`head>target`·`head`·`>target` 순으로 찾고, 규칙이 없는 대안은 오류다(기본 부류가 없다). 쓰이지 않는 규칙과 관측에 없는 종류를 가리키는 규칙은 시험이 거부한다. anchor는 순서만 보며 인접성이나 같은 node 안인지는 보지 않는다. B의 증거는 "그 종류의 node가 있다"이며 명세 production의 범위와 같다는 뜻이 아니다(예: Roslyn `ArgumentList`는 괄호를 포함하고 명세 `argument_list`는 그렇지 않다): 범위는 PR3의 위치 감사가 본다.
- **dry-run 수치(관측 r3·매핑표 r2).** 388 = A 198 + B 100 + C 90. A 198 = 변별 131 + 약 67(원장의 `LITERAL_MATCH` 194 + 고정 lexeme 비단말 4). B 100 = 변별 74 + 약 26. C 90 = `ABSTRACT_CATEGORY` 36 + `NO_OWN_NODE_FOR_CONSTRUCT` 29 + `POSITION_DEPENDENT` 14 + `SEMANTIC_BINDING` 9 + `NO_WITNESS_IN_LINKED_SOURCES` 2. 행별은 B03이 A 137·B 61·C 59, B04가 A 61·B 39·C 31이다(`TestCSharpG1ClassifyCountsOfThePinnedInputs`).

## G1-3b 기대값 (B03·B04, G1B34)

분류(위)를 기대값으로 바꾼 PR2다. asset 넷을 더한다. native를 읽지 않고, 봉인 전에 두 감사를 끝내 발견을 고친 뒤 봉인한다(사용자 결정, #233). 승인된 묶음 11개는 한 byte도 바뀌지 않았고 새 asset은 새 승인 tuple을 기다린다([대기 asset](#g1-번들-승인-tuple-g1approval)).

| asset | schema | 내용 |
|---|---|---|
| [`csharp-g1-expected-b03b04-r1.json`](../../src/contracts/csharp-g1-expected-b03b04-r1.json) | `tsgk-csharp-g1-expected-b03b04/r1` | 대안 388개의 기대값 또는 BLOCKED. 도출 `kit.DeriveCSharpG1B34`, 검증 `ParseCSharpG1B34`(도출과 member별 정확 일치) |
| [`csharp-g1-kind-map-r3.json`](../../src/contracts/csharp-g1-kind-map-r3.json) | `tsgk-csharp-g1-kind-map/r3` | 매핑표 r2 + class B가 쓰는 1:n 종류의 문맥 항목. r2는 pin하고 바꾸지 않는다 |
| [`csharp-g1-audit-b03b04-r1.json`](../../src/contracts/csharp-g1-audit-b03b04-r1.json) | `tsgk-csharp-g1-audit-b03b04/r1` | 위치 감사: B 98행 전수와 A 198개의 앵커 |
| [`csharp-g1-typeaudit-b03b04-r1.json`](../../src/contracts/csharp-g1-typeaudit-b03b04-r1.json) | `tsgk-csharp-g1-typeaudit-b03b04/r1` | type 감사: B 98행 전수와 A 앵커 어휘 119개 |

모두 `go run ./src/dev/csharp-g1-b34 -root . [-list] [-write]`, `-kindmap [-write]`, `-audit pos|type [-in <저작 행> -write] [-pins] [-grammar <dir> -spec <dir>]`가 쓰고 검증한다. 명세 문서는 고정 commit `79ad4e90…`의 `statements.md`(SHA-256 `3b718298…`)·`patterns.md`(`2952f64a…`)를 `G1SpecFiles`에 더했다(기존 다섯 문서의 SHA-256은 그 commit의 파일과 같음을 다시 확인했다).

### 기대값

- 입력 pin: 원장 원천, 관측 r3, 매핑표 **r3**. 대안은 원장 순서이고 `group`(A·B·C)·`status`·`basis`·`discriminating`을 가진다.
- **A**(198, `spec-derived`, D-1): `anchors`는 production이 요구하는 keyword·punctuation token과 그 Roslyn token 종류가 매핑표에서 갖는 type이다. witness는 연결 사례의 수락된 source에서 앵커를 순서대로 가진 **가장 좁은 run**이고(`tokens`: 앵커마다 span 하나) 기대는 "그 type의 node가 그 token의 범위와 정확히 같다"이다. 앵커를 품은 구성 node는 단언하지 않는다.
- **B**(98, `differential-oracle`): witness는 분류기의 Roslyn 종류(와 부모 종류)를 가진 첫 node의 범위(`span`)이고 `ts`는 **그 witness node의 종류**가 갖는 type이다(분류의 종류 합집합이 아니다. 아래 type 발견). 부모 종류에 따라 갈리는 종류는 매핑표 r3의 문맥 항목이 정한다.
- **C**(92): BLOCKED와 분류의 사유 code. 감사가 B에서 C로 옮긴 둘(`a028`·`a209`)을 포함한다.
- `discriminating: false`는 약한 일치다(D-1, R-5): 한 글자 punctuation만의 앵커, 구성 밖의 다른 Roslyn 종류와 type을 나누는 종류, `ts`가 `identifier`를 담거나 type이 5개 이상인 집합. A는 앵커 하나라도 그 type에 `identifier`가 있으면 약하다(문맥 keyword의 `identifier` 쪽).
- 수치(관측 r3·매핑표 r3): 388 = A 198 + B 98 + C 92. EXPECTED 296(변별 180·약 116), BLOCKED 92(`ABSTRACT_CATEGORY` 36·`NO_OWN_NODE_FOR_CONSTRUCT` 31·`POSITION_DEPENDENT` 14·`SEMANTIC_BINDING` 9·`NO_WITNESS_IN_LINKED_SOURCES` 2). 행별은 B03이 A 137·B 59·C 61, B04가 A 61·B 39·C 31이다. B 98개 중 변별 72·약 26, A 198개 중 변별 108·약 90.
- 한계: 증거는 대안마다 witness 하나다(연결 사례가 대안당 하나). A는 앵커가 나타남만 증명하고 같은 구성에 속함은 증명하지 않는다(위치 감사의 앵커 점검은 그 token이 렉서가 읽는 어휘소임만 본다). B는 단일 Roslyn oracle의 구조다.

### 매핑표 r3 (class B 항목)

- r3는 r2에서 도출한다: 같은 461 종류 항목, 같은 어휘, `previous`가 r2의 길이·SHA-256을 pin하고 `contextual` 항목이 더해진다. 항목은 Roslyn 종류와 **직접 부모 종류**로 type을 가른다(`parent`면 `ts`, 아니면 `otherwise`; 둘은 r2 항목의 type을 분할한다). 근거는 `grammar.js`의 인용문(ground)이며 항목은 코드의 저작 목록(`g1KindMapContextual`)이 정하고 verifier는 r2에서 다시 도출해 bytes까지 대조한다.
- 항목은 하나다: `SimpleAssignmentExpression`의 r2 항목 `[assignment_expression, with_initializer]`(1:n)을 부모 `WithInitializerExpression`이면 `with_initializer`, 그 밖에는 `assignment_expression`으로 가른다. `grammar.js`는 `with_initializer`를 `with` 식의 본문(`_with_body`)에서만 만들고 object initializer의 `X = 1`은 `assignment_expression`이다. class B가 쓰는 종류 95개 중 1:n은 이것 하나이고 새 종류 항목은 필요 없었다(PR1의 결론대로). 나머지 94개의 type 존재와 의미는 type 감사가 대조한다.

### 위치 감사

- 행(98)은 B 대안마다 witness source 안의 **모든 인스턴스**를 명세 production에서 손으로 적는다(`instances`), 규칙(`g1b34PosRuleTexts`: 인스턴스 하나의 본문 전체 일치, 선택적 `Not`)을 코드 표로 두고 verifier가 모든 인스턴스를 확인한다. ground는 명세 production의 인용문이고(문서 SHA-256·heading·gap, 행별 핀 `g1b34PosPins`) 문법이 production 밖의 terminal을 node에 넣는 행은 `node`(앞 `(`·뒤 `)`)와 grammar ground를 가진다(`argument_list`: 명세는 괄호를 포함하지 않고 `grammar.js`의 node는 포함한다). 비교는 witness가 인스턴스와 범위가 같은지다.
- A 198개는 앵커만 본다: 렉서(`g1b34Lexemes`: 주석·문자열·문자·숫자·식별자·punctuator, Roslyn 출력 없음)가 witness의 각 앵커 span에서 같은 어휘소를 읽어야 한다.
- **발견 8건(첫 비교의 불일치, 봉인 전에 도출을 고침)**: `B03.a014`(tuple 요소는 `tuple_element`이지 인수가 아님 → 부모 `ArgumentList`·`BracketedArgumentList`), `a025`(`base[0]`은 `base_access` → `base[` 제외), `a026`(`a?.b`는 멤버 접근 → `?[` 꼴만), `a028`(`base_access`의 범위는 `base.g`이고 Roslyn `BaseExpression`은 `base`뿐 → BLOCKED `NO_OWN_NODE_FOR_CONSTRUCT`), `a209`(`join_into_clause`는 join 절 전체이고 Roslyn·grammar의 node는 `into g`뿐 → BLOCKED 같은 사유), `B04.a004`(witness가 method 본문 → 부모 `Block`), `a031`·`a033`(`var a = 1`은 암시적 형식 선언 → `var`·`ref` 제외). 보정은 `csharp_g1_b34_rules.go`(`g1b34Picks`·`g1b34Overrides`)에 있고 시험이 보정을 빼면 달라짐을 보인다(`TestCSharpG1B34CorrectionsOfTheAuditsMatter`). 발견은 asset의 `findings`에 남는다. 보정 뒤 98/98 MATCH, 앵커 198/198 MATCH.

### type 감사

- 입력은 기대 type만이고 위치·매핑표가 없다. 행(98)은 `grammar.js`만으로 도출한 witness 위치의 type(`class`: `NODE` 93·`NO_OWN_NODE` 4(지정자가 `identifier`, `this`가 익명 token)·`ALIAS` 1(`using`의 선언))와 grammar ground를 가진다. 앵커 어휘 119개는 `grammar.js`가 그 어휘에 주는 역할의 합집합이다: 한 역할이면 `NODE`(익명 token 또는 `predefined_type`), 둘 이상이면 `NOT_DERIVABLE`(후보 집합: 문맥 keyword의 `identifier`, `modifier` alias, `implicit_type`·`implicit_parameter` 등) 27개. 후보 집합과 같음은 type 도출이 아니다(G1-3a와 같은 읽기).
- **발견 3건(첫 비교의 불일치, 봉인 전에 고침)**: `B03.a031`·`B04.a002`(기대 type이 분류의 종류 **합집합**이었다 → witness node 종류의 type), `B03.a226`(`with_initializer`가 object initializer에도 있는 것처럼 둔 r2 항목 → 매핑표 r3의 문맥 항목).
- 감사 도출기의 첫 실행에서 앵커 4개가 불일치였으나 모두 감사 저작 스크립트의 역할 판정 오류(여러 줄에 걸친 `alias(choice(…), $.modifier)`와 이웃 줄의 `delegate`)였고 기대값 오류가 아니다. 저작을 고쳐 재생성했고 기대값은 그대로다.

### 독립성

- 두 감사의 입력은 좁은 값이다(`G1B34AuditInput`: witness·앵커·source 본문, `G1B34TypeInput`: type만). 뷰 `csharp_g1_b34_view.go`가 기대값 asset을 읽는 유일한 곳이다. `TestCSharpG1AuditInputsAreNarrow`가 member를 고정하고 `TestCSharpG1AuditsReachOnlyTheirOwnInputs`·`…RejectTheBypassesOfTheReview`의 항목 "B03/B04 position audit"·"B03/B04 type audit"이 호출 폐쇄가 자기 파일(과 G1-3a 감사의 공용 비교 함수·ground 표)밖의 G1 파일에 닿지 않음을, 우회 사례(기대값 파일의 도우미, 기대값 decoder, 뷰, 매핑표 parser, 기대값 도출)가 잡힘을 고정한다. 새 여섯 파일은 분리 시험에서 도출 쪽이다.
- 한계: 인스턴스 목록은 한 사람이 명세에서 도출한 것이고(규칙은 적힌 인스턴스만 확인한다), 위치 감사는 슬롯(대안이 놓인 문법 자리)을 단언하지 않는다 — 보정의 `Parent`는 Roslyn 부모 종류이며 명세 슬롯의 증명이 아니다. type 감사는 grammar 도출이 한 사람이 읽은 것이고 파서 실행이 아니다. 작성자는 도출 전에 PR1의 분류 결과와 **Roslyn witness 목록을 보았다**(PR2 독립 검토 지적 5): 개발 도구 `csharp-g1-b34 -list`는 witness의 텍스트 옆에 기대 type(`TS`)과 Roslyn 종류를 같은 출력에 인쇄하므로 도구가 이 분리를 강제하지 않는다. 분리는 저작 절차가 지켰다(위치 감사의 인스턴스는 별도 source 목록에서, type 감사는 기대값 asset의 앵커 어휘 목록과 `grammar.js`에서 저작했다). 봉인된 감사 r1의 `basis`는 이 노출을 말하지 않고 바꾸지 않으며, 이 줄이 그 한계를 적는다.

### 봉인과 다음 단계

- 봉인: 이 단계의 asset 넷의 SHA-256을 `TestCSharpG1B34AssetsAreSealed`가 고정한다. native 결과를 읽기 전이다. 정정은 새 revision이다.
- native 비교(executed asset), 원장 r2, 두 번째 승인 tuple은 PR3다([아래](#g1-3b-native-비교-b03b04-pr3)). 이 asset의 EXPECTED(296)는 원장 r2에서 `SEALED` 또는 `EXECUTED`, BLOCKED(92)는 `BLOCKED`와 사유로 옮겨지고, B03·B04의 P 의무는 BLOCKED 대안이 있어 `NO_EXPECTED`로 남는다.

## G1-3b native 비교 (B03·B04, PR3)

봉인된 B03·B04 기대값([위](#g1-3b-기대값-b03b04-g1b34))을 등록된 C# route의 tree-sitter-c-sharp native 빌드로 대조한 결과와, 그것을 받는 원장 r2·두 번째 승인 tuple이다. 봉인된 네 asset(`csharp-g1-expected-b03b04-r1.json`, `csharp-g1-kind-map-r3.json`, 두 감사)은 **한 byte도 바뀌지 않았다**. native 결과가 봉인된 값과 어긋나는 대안은 고치지 않고 executed asset에 처분과 함께 기록한다(사용자 결정: 봉인 뒤 정정은 새 revision의 몫이고 결정이 필요하다).

### 실행

- 대상: EXPECTED 296개 대안의 witness source 23개(`CSharpG1B34NativeSourceIDs`). 원장의 P step 연결은 쓰지 않는다: 이 행의 대안은 witness 하나가 전부다. BLOCKED 대안은 대상이 아니다.
- `go run ./src/dev/csharp-g1-native ... -set b34`(기본값 `-set b01b02`는 G1-3a와 같다)가 원시 관측을 쓴다. 한도·build·HARD Job은 [G1-3a와 같다](#실행)(native process 1 GiB·30 s, build 4 GiB·120 s, 전체를 `run-job.ps1`의 HARD 4 GiB Job 안에서).
- 원시 관측 `tsgk-csharp-g1-native-raw/r1`은 저장소 밖에 두고 **SHA-256만 pin**한다(결정 B2): 100,032 B, `5cc0f8bd6a954b8b51b1f05466bab9bc125816f166623458970875a5b0ddd03e`. source 23개 모두 native 완료이고 같은 입력의 두 실행이 byte 단위로 같다(wall 19.5 s·21.1 s, peak 약 640 MB, cleanup verified).
- `go run ./src/dev/csharp-g1-executed -set b34 -native <원시 관측> [-write]`가 asset을 유도한다(native를 실행하지 않는다). `-native` 없이 실행하면 추적 asset을 offline으로 검증한다.

### 비교

asset `csharp-g1-executed-b03b04-r1.json`(schema `tsgk-csharp-g1-executed-b03b04/r1`)은 EXPECTED 대안의 witness(봉인된 값 그대로)를 **부분(part)**으로 나누고 부분마다 native tree를 본다.

| witness `match` | 부분 | 부분의 판정 |
|---|---|---|
| `tokens`(A) | 앵커 token마다 하나: token의 span과 **그 앵커의** 봉인된 type | span이 정확히 같고 type이 봉인된 집합에 드는 native node가 있으면 `MATCH`, 없으면 `MISMATCH`(같은 범위를 겹치는 더 작은 node를 `near`에 적는다) |
| `span`(B) | 하나: witness의 span과 대안의 봉인된 `ts` | 같음 |

- witness는 모든 부분이 `MATCH`일 때 `MATCH`, 하나라도 `MISMATCH`면 `MISMATCH`, native parse가 완료되지 않으면 `NOT_APPLICABLE`이다. 대안은 자기 witness 하나의 결과를 가지며 비교가 돌았으면(`executed: true`) 원장에서 `MATCH`만 `EXECUTED`가 된다(R-2): `MISMATCH`는 `SEALED`로 남는다.
- **`match` 값 `tokens`는 이 단계가 새로 쓰는 값이 아니라 기대값 asset이 이미 정한 값**이다(`g1b34MatchTokens`). executed asset은 그 값을 그대로 옮기고 의미를 위처럼 정하며, 값 집합은 `{tokens, span}`이다(`TestCSharpG1B34ExecutedSealedAndBound`가 asset의 모든 witness의 `match`가 둘 중 하나이고 `tokens`가 A, `span`이 B임을 고정한다). 앵커를 품은 구성 node는 단언하지 않는다(D-1): A의 `MATCH`는 "그 span에 그 type의 node가 있다"이며 구성의 모양을 말하지 않는다.
- 약한 일치(`discriminating: false`)와 변별 일치는 `counts`에서 **갈라 센다**(`match_discriminating_a`·`match_weak_a`·`match_discriminating_b`·`match_weak_b`).
- **A의 변별은 세 단으로 나눈다(PR2 독립 검토 지적 3, #233 사용자 결정).** 봉인된 기대값 asset과 그 `discriminating` 표지는 바꾸지 않는다. executed asset의 대안 행이 `tier`를 더한다(변별 A에만, 약한 A와 B는 없음). 값은 봉인된 앵커만으로 정해진다(`g1bxTier`).

  | `tier` | 뜻 | 세는 곳 |
  |---|---|---|
  | `word` | 단어 앵커(`[A-Za-z_][A-Za-z0-9_]*`) 중 봉인된 type이 node type 하나인 것이 있다(예: `while`, `typeof`, `int`→`predefined_type`) | `match_discriminating_a` — **변별 MATCH는 이 단뿐이다** |
  | `candidate` | 단어 앵커가 있으나 그 type이 모두 후보 집합이다(예: `new`·`ref`·`async`의 `["\"x\"", modifier]`; type 감사가 `NOT_DERIVABLE`로 인정한 어휘). 식 안의 `new`가 `modifier`로 파싱되어도 기대값을 만족한다 | `match_candidate_a` |
  | `operator` | 단어 앵커가 없다. 두 글자 이상 연산자(`++ == && => ::`)만으로 변별이다. 이 token은 생성 방식과 무관하게 같은 node다 | `match_operator_a` |

  tier는 executed 검사가 봉인된 대안에서 다시 계산해 대조하고(`G1_B34X_EXPECTATION`), 집계는 이것을 따른다(`G1_B34X_COUNTS`). 새 `match` 값은 없다. 이 단계의 수치는 리뷰의 근사치(단어 59·후보 18·연산자 31)와 다르다: 후보 19는 리뷰의 18개와 `csharp-B04.a055`(`=>` `ref` `;`: 후보 집합 단어 앵커가 있어 연산자 단이 아니다)이고, 단어 62·연산자 27의 나머지 차이는 리뷰의 집계 규칙을 몰라 원인을 확인하지 못했다. 이 asset의 값은 위 정의에서 나오며 `…ExecutedTiers`가 정의와 대조한다.
- 단어 정규식은 분류기의 것과 같은 식이다(비교 파일은 도출 쪽 변수를 쓰지 못하므로 사본을 두고 `TestCSharpG1B34ExecutedTiers`가 두 식이 같음을 확인한다).

### 결과

| | 대안 | MATCH | MISMATCH | 변별 MATCH | 후보 집합 MATCH | 연산자 MATCH | 약한 MATCH |
|---|---:|---:|---:|---:|---:|---:|---:|
| A (`spec-derived`, 앵커) | 198 | 194 | 4 | 62 (`word`) | 19 (`candidate`) | 27 (`operator`) | 86 |
| B (`differential-oracle`, 범위) | 98 | 97 | 1 | 71 | — | — | 26 |
| 합 | 296 | **291** | **5** | **133** | 19 | 27 | **112** |

- 변별 MATCH는 133개다(A의 `word` 62 + B 71). 이전 보고의 179는 A의 `candidate`·`operator` 46개를 변별로 센 값이다. 두 단은 변별 표지를 가졌으나 증명하는 바가 적어(후보 집합 type은 type 도출이 아니고, 연산자 token은 구성 인식을 가르지 못한다) 따로 센다.

- 부분 425개 중 420 `MATCH`, 5 `MISMATCH`. `NOT_APPLICABLE`은 없다. source 23개 모두 Roslyn이 수락했고 native tree에 `ERROR`가 없다.
- 원장 r2에서 MATCH 291개는 `EXECUTED`, MISMATCH 5개는 `SEALED`, BLOCKED 92개는 `BLOCKED`다.

### 불일치 5건과 처분

처분은 사람이 쓴 판단이며 `kit`의 `g1B34ExecDispositions` 표에 있다(처분이 없는 불일치는 `G1_B34X_UNDISPOSED`). 봉인된 값은 고치지 않았다.

| 대안 | witness | native 결과 | 원인 |
|---|---|---|---|
| `csharp-B03.a184`·`a236`, `csharp-B04.a028`·`a029`(A, 약한 일치) | 앵커 `var` | 같은 범위에 `implicit_type`과 그 익명 자식 `"var"` | `EXPECTED_ERROR` |
| `csharp-B03.a239`(B, 변별) | `ParenthesizedVariableDesignation` `(a, b)` ([287,293) of `alt-csharp-B03-09#0`) | `tuple_pattern` | `DESIGN_DIFFERENCE` |

- **`var` 네 건**(`EXPECTED_ERROR`): 봉인된 앵커 type은 그 위치의 Roslyn `IdentifierToken`이 매핑표에서 갖는 type(`identifier`, `implicit_parameter`)이다. 고정 grammar 원문은 `implicit_type: 'var'`이고 type 위치의 `var`는 `implicit_type` 노드와 그 익명 자식 `"var"`가 token의 범위와 정확히 같다. 매핑표 항목은 식별자에 맞고 틀리지 않았다. 틀린 것은 앵커 type을 그 위치의 token 종류 하나에서만 가져온 도출이다. **type 감사 r1은 이를 잡지 못했다**: 감사의 앵커 행 `var`는 grammar만으로 후보 집합 `"var"`·`identifier`·`implicit_parameter`·`implicit_type`을 적었으나(`NOT_DERIVABLE`) 비교는 기대 type의 **어휘 단위 합집합**(모든 `var` 앵커가 쓰는 type의 합)과 한 것이라, 다른 대안(`var` 패턴)이 `"var"`와 `implicit_type`을 가져와 합집합이 후보 집합과 같아졌다(`CANDIDATE_SET_EQUAL`). 대안별 부분집합은 보이지 않는다. 이 한계는 r1 감사 asset을 바꾸지 않고 적으며, 대안별 비교를 하는 [type 감사 r2](#type-감사-r2-csharp-g1-typeaudit-b03b04-r2json)가 이를 잡는다.
- **`a239`**(`DESIGN_DIFFERENCE`): 봉인된 witness는 암시적 형식 `var`의 분해 선언 안쪽 designation이고 Roslyn은 `ParenthesizedVariableDesignation`으로 읽는다. 고정 grammar 원문은 그 자리를 `variable_declarator`의 `tuple_pattern`(중첩 가능)으로 두고, `parenthesized_variable_designation`은 `_variable_designation`(패턴 문맥)에만 있다. 두 표현 모두 유효하고 봉인된 type은 대안의 패턴 문맥에는 맞지만 대안이 witness 하나라 그 문맥은 비교된 적이 없다. 정정은 새 revision의 몫이었고 [r2](#g1-3b-봉인-뒤-정정-r2)가 BLOCKED로 둔다.

### 검증

- offline(`ParseCSharpG1B34Executed`): pin(기대값·관측·원시 관측), 대안·witness·부분이 봉인된 asset과 같음, 증거(`found`는 부분의 span과 정확히 같고 봉인된 type의 node가 앞에 오며 8개까지, `near`는 불일치에만)에서 모든 결과를 다시 계산, source 신원, 처분, 집계, 정규 bytes. 원시 관측이 주어지면 asset이 그것에서 다시 유도한 것과 byte 단위로 같아야 한다(`G1_B34X_RAW`). 자기 모순 없는 위조 증거는 offline으로는 못 잡고 원시 관측이 잡는다(`…ForgedEvidenceNeedsTheRaw`).
- 시험(`csharp_g1_b34_executed_test.go`): 추적 asset의 수치와 pin, 합성 원시 관측에서의 비교 논리(앵커 하나가 빠진 A는 `MISMATCH`, 한 칸 어긋난 span, 봉인되지 않은 type, 완료되지 않은 parse), 변조 mutant 30여 개, tier(정의·수치·위조·집계, `…ExecutedTiers`), 처분 표와 불일치의 일치, 그리고 `TSGK_G1_NATIVE_RAW_B34`가 있을 때 원시 관측에서의 재유도(`…MatchesRawNative`).
- native 비교 코드는 `csharp_g1_b34_executed.go`(분리 시험에서 **비교 쪽**)에 있고 기대값 유도 쪽은 그것에 닿지 않는다(`TestCSharpG1SeparationClosure`).

### 원장 r2

`csharp-g1-ledger-r2.json`(schema `tsgk-csharp-g1-ledger/r2`, `kit.DeriveCSharpG1LedgerR2`·`ParseCSharpG1LedgerR2`, 도구 `go run ./src/dev/csharp-g1-ledger -root . -r2 [-write]`)은 r1의 입력(선언·fixture)과 기대값·executed의 두 쌍을 받는다: B01·B02의 r2 쌍(`SEALED`/`EXECUTED`/`BLOCKED`는 r1과 같다)과 B03·B04의 쌍. **원장 r1은 동결**이다: r1 파생은 B03·B04 asset을 받지 않고(`G1_LEDGER_SEALED`) 추적 파일은 한 byte도 바뀌지 않았으며(승인 object의 pin) 각 revision의 verifier는 서로의 schema를 거부한다(`G1_LEDGER_SCHEMA`).

- 상태(PR3 시점, 정정 전; 정정 뒤의 값은 [봉인 뒤 정정 (r2)](#g1-3b-봉인-뒤-정정-r2)): B03은 `EXECUTED` 193·`SEALED` 3·`BLOCKED` 61, B04는 `EXECUTED` 98·`SEALED` 2·`BLOCKED` 31이다. `SEALED`는 `MISMATCH` 5건(`a184`·`a236`·`a239`·`B04.a028`·`a029`)이었다. B03·B04의 P 의무는 BLOCKED 대안이 있어 `NO_EXPECTED`로 남는다. 그 밖의 행과 B01·B02는 r1과 같다(r2는 r1과 스키마·basis·B03·B04 대안의 상태만 다르다: `TestCSharpG1LedgerR2IsTheDerivation`).
- 다중 asset의 점검(R-8): 한 쌍의 기대값 asset은 **한 대안을 두 번** 적거나 자기 행 밖·원장에 없는 대안을 적을 수 없고(`G1_LEDGER_SEALED`), B03·B04의 기대값 asset은 자기 행의 **모든 대안**을 한 번씩 적어야 한다. executed asset은 자기 행의 `SEALED` 대안을 한 번씩만 가리킬 수 있고 `BLOCKED`·알 수 없는 대안·다른 행·중복은 `G1_LEDGER_EXECUTED`다. `EXECUTED`는 `MATCH`만이다(R-2; `TestCSharpG1LedgerR2StateRules`, `…Forgeries`).

### 두 번째 승인 tuple

승인은 PR #436의 첫 object와 **별개의 두 번째 object**로 준비한다(만들지 않았다: [아래](#두-번째-묶음-g1-3b)).

## G1-3b 봉인 뒤 정정 (r2)

PR3의 native 비교가 드러낸 불일치 5건을 사용자 결정(#233, 2026-11-14)에 따라 새 revision으로 정정한다: 기대값 r2, 대안별 type 감사 r2, 정정된 기대값의 native 재대조(executed r2)와 원장 r2·두 번째 승인 tuple. **봉인된 r1 asset**(`csharp-g1-expected-b03b04-r1.json`·`csharp-g1-kind-map-r3.json`·두 감사 r1·`csharp-g1-executed-b03b04-r1.json`)**은 한 byte도 바뀌지 않았다**(SHA-256 고정 시험과 승인 tuple의 pin). 방식은 G1-3a의 [봉인 뒤 정정 (r2)](#봉인-뒤-정정-r2)와 같다.

### 기대값 r2 (`csharp-g1-expected-b03b04-r2.json`)

- schema `tsgk-csharp-g1-expected-b03b04/r2`. 도출 `kit.DeriveCSharpG1B34R2`, 검증 `ParseCSharpG1B34R2`(코드 `csharp_g1_b34_correct.go`), 도구 `go run ./src/dev/csharp-g1-b34 -root . -r2 [-write] [-grammar <dir> -spec <dir>]`. r2는 r1 asset(`inputs.previous`가 경로·길이·SHA-256을 pin)과 `corrections`만큼만 다르다. 정정 값은 `grammar.js`와 csharpstandard 원문에서 저작했고 native 산출물을 읽지 않는다(호출 폐쇄가 호출 경로를 증명하고 저작 시점은 `after_native`가 기록한다).
- 정정 5건(모두 `after_native: true`, `basis_independent: true`, 근거는 grammar 원문과 명세 인용문 둘 다):

| 대안 | field | 이전 → 이후 | 근거 |
|---|---|---|---|
| `csharp-B03.a184`·`a236`, `csharp-B04.a028`·`a029` (A) | `anchors[0].ts` | `["identifier","implicit_parameter"]` → `["\"var\"","implicit_type"]` | grammar의 `implicit_type: 'var'`와 `type`이 `implicit_type`을 가진다: type 위치의 `var`는 `implicit_type` 노드와 그 익명 자식 `"var"`이다. 이전 값은 그 위치 Roslyn `IdentifierToken`의 매핑표 type이며 식별자에는 맞다(매핑표는 틀리지 않았다) |
| `csharp-B03.a239` (B) | `alternative` | EXPECTED(`parenthesized_variable_designation`) → **BLOCKED `POSITION_DEPENDENT`** | 암시적 `var` 분해 선언의 중첩 designation을 grammar는 `variable_declarator`의 `tuple_pattern`으로 두고 `parenthesized_variable_designation`은 패턴 문맥(`_variable_designation`)에만 있다. Roslyn 종류와 grammar node가 이 문맥에서 1:1이 아니고 대안은 witness 하나뿐이라 기대값을 새로 고르지 않고 막는다 |

- **사유 code는 등록된 것을 쓴다.** `G1ClassBlockCodes`의 `POSITION_DEPENDENT`(부모 안의 역할: node가 자리에 따라 정해진다)이며 새 code는 없다. `a239`의 막힘은 정정 기록에 남는다(검증기는 `alternative` 정정의 새 값이 BLOCKED이고 code가 등록된 것이며 근거·사유가 있음을 요구한다).
- **`discriminating` 표지는 바꾸지 않는다.** 네 `var` 앵커는 약한 일치 그대로다. 사후 정정이 더 강한 주장을 만들면 native 불일치를 승리로 세게 된다. 앵커의 새 type은 `identifier`를 담지 않아도 표지는 봉인 값이다.
- **사후 정정은 따로 센다**(`post_seal`): `corrections` 5·`after_native` 5·`basis_independent` 5·정정한 대안 5(앵커 type 4·BLOCKED 1). 독립 기대값의 수와 섞지 않는다. 수치(r2): 388 = A 198 + B 97 + C 93. EXPECTED 295(변별 179·약 116), BLOCKED 93(`POSITION_DEPENDENT` 15).
- 검증: 핀한 r1 asset이 r1 verifier를 통과, 정정의 형식(`after_native`·사유·중복·필드·grammar와 명세 근거), `old`가 r1이 가진 값과 같음, r2가 r1에 정정만 적용한 것과 같음(기록에 없는 차이 거부), 정정이 저작된 표와 같음, 정규 bytes. r1 verifier는 r2를, r2 verifier는 r1을 `G1_B34_SCHEMA`로 거부한다. 코드는 `G1_CORRECTION_*`(위 정정 코드 표)다.

### type 감사 r2 (`csharp-g1-typeaudit-b03b04-r2.json`)

- **r1의 구멍.** r1의 앵커 행은 어휘 단위로 기대 type의 **합집합**과 비교한다. 문맥에 따라 type이 달라지는 어휘(`NOT_DERIVABLE` 27개: `var`·`_`·`new`·`ref`·문맥 keyword 등)는 후보 집합만 적고 합집합이 후보 집합과 같으면 `CANDIDATE_SET_EQUAL`이므로, 한 대안의 type이 틀려도 다른 대안이 합집합을 채우면 보이지 않는다. 네 `var` 앵커가 그 경우다(`var` 패턴 `a011`이 `"var"`·`implicit_type`을 가져 합집합이 후보 집합과 같았다).
- **r2의 비교.** 어휘마다 grammar가 그 어휘를 **어느 자리에서 어느 type으로** 만드는지를 역할(`roles` 31개: 이름·type·사유·`grammar.js` ground, 행별 핀)로 적고, 그 어휘 앵커 55개(대안·앵커 위치 단위, `contexts`)마다 **대안의 production이 가리키는 자리**의 역할을 배정해(witness의 자리가 아니다: `B04.a126`의 witness `unsafe`는 메서드 수식어지만 production `unsafe_statement`의 역할로 판정한다) 그 앵커의 기대 type과 역할의 type을 비교한다. 관계: `EQUAL`·`EXPECTED_SUPERSET`(기대 type이 역할의 type과 grammar가 그 자리에 주지 않는 type을 가진다)·`EXPECTED_SUBSET`·`OVERLAP`은 MATCH, `DISJOINT`(기대 type에 grammar가 그 자리에 주는 type이 하나도 없다)는 MISMATCH다. 역할의 type은 r1이 그 어휘에 적은 후보 집합의 부분집합이어야 하고 모든 역할이 쓰여야 한다.
- 결과: 55개 모두 MATCH(`EQUAL` 7·`EXPECTED_SUPERSET` 48), 불일치 0. **정정 전 r1 기대값에 같은 감사를 적용하면 정확히 `var` 네 대안이 `DISJOINT`로 나오고 r1 감사는 같은 r1 기대값을 통과한다**(`TestCSharpG1TypeAuditB34R2FindsWhatR1Missed`: r1이 놓쳤고 r2는 잡는다). 정정 뒤에는 불일치가 없다. `EXPECTED_SUPERSET` 48개는 후보 집합 앵커의 기대 type이 grammar가 그 자리에 주지 않는 type(예: 식 안의 `new`의 `modifier`)을 더 가진 것으로, 오류가 아니라 executed의 `candidate` 단으로 이미 따로 센 약한 증거의 다른 표현이다. 이 정정은 그것을 좁히지 않는다(봉인된 값의 변경은 새 revision의 몫이다).
- r2는 r1 감사 asset(`inputs.previous` 핀)이 `NOT_DERIVABLE`로 분류한 어휘만 다룬다. B 98행과 derivable 어휘 92개는 r1 감사가 맡고 r2는 반복하지 않는다(정정이 건드리지 않은 값이며 r1이 같은 값을 감사했다. `a239`의 행은 BLOCKED라 r2의 기대값에 없다).
- **독립성과 한계.** 이 감사는 native 비교 **뒤에** 저작했고 작성자는 그 결과(불일치 5건)를 알고 있었다: asset의 `basis`가 그렇게 적는다. 입력은 앵커만이다(`G1B34AnchorPair`: 대안·앵커 위치·어휘·type, witness·Roslyn 종류·매핑표는 member가 없다. `TestCSharpG1AuditInputsAreNarrow`, 호출 폐쇄 항목 "B03/B04 type audit r2"). 역할 배정은 한 사람이 production과 `grammar.js`를 읽은 것이고(역할의 `modifier` alias 읽기는 G1-3a type 감사와 같다) 맥락 행의 핀(`g1b34ContextPins`)이 배정의 사후 변경을 막는다. 파서 실행이 아니다.
- 도구: `go run ./src/dev/csharp-g1-b34 -root . -audit type2 [-in <저작 JSON> -write] [-pins] [-grammar <dir>]`.

### executed r2와 원장 r2

- **executed r2**(`csharp-g1-executed-b03b04-r2.json`, schema `tsgk-csharp-g1-executed-b03b04/r2`, `go run ./src/dev/csharp-g1-executed -root . -set b34 [-native <원시 관측> [-write]] [-r1]`): 정정된 기대값 r2를 PR3와 **같은 원시 관측**(`5cc0f8bd…`, source 23개 그대로)에서 다시 대조한다. native는 다시 실행하지 않았다(재유도에 필요하지 않다). 정정이 건드리지 않은 봉인 값은 봉인된 그대로, 정정한 대안은 정정된 값으로 비교한다. 결과: EXPECTED 295개 **모두 MATCH**(불일치 0, 처분 없음), 부분 424개 모두 MATCH. 대안 행의 `corrected`가 정정한 4개를 표지하고 집계가 따로 센다: `corrected` 4·`corrected_match` 4·`sealed_match` 291. 변별은 `word` 62·`candidate` 19·`operator` 27, 약한 A 90(봉인 시 86 + 정정 4), B 변별 71·약 26이다. **정정한 4개의 MATCH는 native를 본 뒤의 값이므로 독립 일치가 아니다**(`corrected_match`). r1(MISMATCH 5와 처분)은 그대로이고 `-r1`이 그것을 검증한다.
- **원장 r2**(`csharp-g1-ledger-r2.json`)는 기대값 r2·executed r2에서 다시 유도한다(도구 `-r2`가 두 r2를 읽는다): B03은 `EXECUTED` 195·`SEALED` 0·`BLOCKED` 62, B04는 `EXECUTED` 100·`SEALED` 0·`BLOCKED` 31이다(`a239`는 `BLOCKED` `POSITION_DEPENDENT`). `SEALED`에 남는 대안은 없다. P 의무는 BLOCKED 대안이 있어 `NO_EXPECTED`다. 코드는 바뀌지 않았다(원장은 두 asset의 ID·상태·결과만 읽는다).

### PR2 독립 검토 지적 4·5·6의 반영

- **지적 4(살아남은 변이).** 위치 감사 인스턴스의 순서·중복·범위, 노드 행의 grounds(핀·문서 종류·삭제), type 감사 앵커 행의 순서, type 목록의 순서·중복, 행의 `reason` 필수, 렉서의 verbatim `""`, 매핑표 r3 항목의 순서·중복, 약한 일치의 "type 5개 이상" 규칙을 각각의 편집만 거부할 수 있는 시험으로 고정했다(`csharp_g1_b34_verifier_test.go`). 변이 실행 결과는 이 단계의 보고에 적는다.
- **지적 5.** 독립성 한계 문장([독립성](#독립성))에 적었다.
- **지적 6.** D-1의 선택적 literal 규칙을 [분류](#g1-3b-분류-b03b04-g1classify)에 적고 네 대안을 한 규칙으로 B로 둔다.

## G1 번들 승인 tuple (G1APPROVAL)

사용자 결정([#233, 2026-11-13 "승인 묶음"](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6851518154))은 아래 11개를 한 묶음으로 승인한다. 이 절은 그 승인을 **content-addressed tuple object**로 만들고 검사하는 계약이며, object 자체는 아직 없다.

| 역할 | revision | 경로 |
|---|---|---|
| `expected`·`kind-map`·`executed` (봉인) | r1 | `src/contracts/csharp-g1-expected-b01b02-r1.json`, `csharp-g1-kind-map-r1.json`, `csharp-g1-executed-b01b02-r1.json` |
| `expected`·`kind-map`·`executed` | r2 | 같은 이름의 `-r2.json` |
| `roslyn-observation` | r2 (유도 입력), r3 (재현 가능한 도구 hash) | `csharp-g1-roslyn-observed-r2.json`, `-r3.json` |
| `ledger` | r1 | `csharp-g1-ledger-r1.json` |
| `position-audit` | r1 | `csharp-g1-audit76-r1.json` |
| `type-audit` | r1 | `csharp-g1-typeaudit76-r1.json` |

- **tuple:** schema `tsgk-csharp-g1-approval/r1`, scope `csharp-g1-bundle`. 멤버는 이 순서의 `schema`, `scope`, `decision`(위 결정 코멘트 URL), `members`(경로·`role`·`revision`·`bytes`·`sha256`, 위 표의 순서)이다. 정규 bytes는 compact JSON 한 줄과 줄바꿈 하나이며, object의 이름은 그 bytes의 SHA-256이다. 위치는 `src/contracts/evidence-pins/objects/<sha256>`다.
- **scope:** 기존 값을 넓히지 않고 새 schema에 새 scope를 둔다. `legacy-observation`(roslyn pin tuple)과 `historical-materials`(evidence pin 승인)의 뜻·object·set은 그대로다. 이 object는 어떤 set·selection·`repository-defaults.json`에도 들어가지 않고 reader가 읽지 않는다: 저장소 경로 묶음의 승인이며 evidence subject의 승인이 아니다.
- **검증(`kit.VerifyCSharpG1Approval`):**
  - object가 없으면 상태는 **`PENDING_APPROVAL`**이다. 오류도 PASS도 아니며 아무것도 승인되지 않았다는 보고다. 도구는 exit 3, 시험은 통과하되 로그에 상태를 적는다.
  - object가 있으면(approval schema의 object가 정확히 하나) 이름이 bytes의 SHA-256이어야 하고(`G1_APPROVAL_OBJECT`), 엄격히 decode되어 정규 bytes와 같아야 하고, scope(`G1_APPROVAL_SCOPE`)·decision(`G1_APPROVAL_DECISION`)·멤버의 경로·역할·revision과 순서(`G1_APPROVAL_BUNDLE`)가 코드의 묶음과 같아야 하고, 모든 멤버 파일의 길이와 SHA-256이 같아야 한다(`G1_APPROVAL_MEMBER`: 한 byte 변경·추가·삭제·누락). 마지막으로 `src/contracts/csharp-g1-*.json`에 묶음 밖의 asset이 없어야 한다(`G1_APPROVAL_UNAPPROVED`). 예외는 **대기 asset**뿐이다: 다음 단계(G1-3b)의 asset 아홉(아래 [두 번째 묶음](#두-번째-묶음-g1-3b)의 멤버)은 묶음 옆에 있어도 묶음의 `APPROVED`를 깨지 않는다. 대기 asset은 승인된 것이 아니며(`CSharpG1PendingAssets`가 두 번째 묶음이 `APPROVED`가 아닌 동안 존재하는 것을 나열하고 도구가 `-verify`에 적는다) 두 번째 묶음의 object가 생겨 `APPROVED`가 되면 목록은 저절로 비는 것이다(코드의 목록을 고치지 않는다). 두 묶음에 없는 G1 asset은 여전히 거부된다(`TestCSharpG1ApprovalToleratesOnlyThePendingAssets`). 같은 scope의 approval object가 둘 이상이면 `G1_APPROVAL_AMBIGUOUS`, 어느 묶음의 scope도 아니면 `G1_APPROVAL_SCOPE`다. 모두 맞을 때만 `APPROVED`다.
  - 새 G1 asset이나 asset의 변경은 묶음 밖이므로 새 승인(새 revision의 tuple)이 필요하다. 기존 object는 바꾸지 않는다.
- **CI의 pending:** object가 생기기 전의 CI는 `TestCSharpG1ApprovalOfTheTrackedBundle`이 `PENDING_APPROVAL`을 로그에 적고 통과한다(승인 전에 CI가 통과해야 Root가 object를 만들 수 있다). 이는 승인의 주장이 아니다. 게이트가 필요한 곳은 도구의 exit 3을 실패로 취급한다. object가 생기면 같은 시험이 `APPROVED`를 요구하며 이후 asset 변경은 모두 실패한다.
- **도구:** `go run ./src/dev/csharp-g1-approval -root . -print > tuple.json`은 현재 추적 asset의 정규 tuple bytes를 stdout에, 길이와 SHA-256(object 이름)을 stderr에 적는다. `-verify`는 위 검증이다. 도구는 `src/contracts/evidence-pins/`에 쓰지 않는다. Root가 사용자가 SHA 목록을 확인한 뒤 `<sha256>` 이름으로 손으로 만든다. 첫 object는 PR #436의 `9a5dbd01…`이고 이 단계에서도 한 byte도 바뀌지 않았다(`TestCSharpG1ApprovalFirstObjectIsUnchanged`: 추적 asset에서 만든 첫 tuple이 그 object와 byte 단위로 같고 `base` member가 나타나지 않는다).

### 두 번째 묶음 (G1-3b)

PR3는 같은 schema의 **두 번째 object**를 준비하고(멤버 여섯), 봉인 뒤 정정(r2)이 멤버를 아홉으로 늘렸다. 사용자 결정(#233)과 승인 object는 아직 없다: Root가 사용자 확인 뒤에 만든다.

| 역할 | revision | 경로 |
|---|---|---|
| `expected` | r1 (봉인) | `csharp-g1-expected-b03b04-r1.json` |
| `kind-map` | r3 | `csharp-g1-kind-map-r3.json` |
| `position-audit`·`type-audit` | r1 | `csharp-g1-audit-b03b04-r1.json`, `csharp-g1-typeaudit-b03b04-r1.json` |
| `executed` | r1 (봉인 값의 비교) | `csharp-g1-executed-b03b04-r1.json` |
| `expected` | r2 (봉인 뒤 정정) | `csharp-g1-expected-b03b04-r2.json` |
| `type-audit` | r2 (대안별) | `csharp-g1-typeaudit-b03b04-r2.json` |
| `executed` | r2 (정정된 값의 비교) | `csharp-g1-executed-b03b04-r2.json` |
| `ledger` | r2 | `csharp-g1-ledger-r2.json` |

- **tuple:** schema `tsgk-csharp-g1-approval/r1`, **scope `csharp-g1-b34-bundle`**. 첫 tuple과 멤버 순서가 같고 `decision`과 `members` 사이에 `base`(첫 object의 이름)가 더해진다. 첫 tuple에는 `base`가 나타나지 않으므로(`omitzero`) 첫 object의 bytes는 그대로다.
- **`decision`:** 사용자가 결정하기 전에는 이슈 URL(`CSharpG1ApprovalDecisionB34`)이다. 사용자 결정 코멘트가 생기면 Root가 그 URL로 상수를 바꾼 뒤 `-print`를 다시 하고 object를 만든다(그러면 tuple의 SHA-256이 바뀐다).
- **첫 묶음을 건드리지 못한다:** 두 번째 tuple은 첫 묶음의 어떤 asset도 멤버로 가지지 못하고(`G1_APPROVAL_BUNDLE`) `base`가 첫 object의 이름과 같아야 하며(`G1_APPROVAL_BASE`), 첫 묶음이 `APPROVED`가 아니면 두 번째 묶음은 만들 수도 승인될 수도 없다(첫 묶음 asset의 한 byte 변경은 첫 묶음과 두 번째 묶음을 모두 실패시킨다). 두 번째 묶음 asset의 변경은 첫 묶음의 `APPROVED`를 깨지 않는다. 두 묶음 사이에 같은 scope의 object를 두 번 두거나 모르는 scope의 object를 두면 거부된다(`TestCSharpG1ApprovalSecondBundleRejects`).
- **대기:** 두 번째 object가 없는 동안 `VerifyCSharpG1ApprovalB34`는 `PENDING_APPROVAL`이고 asset 아홉은 대기 asset이다. 대기 asset이 바뀌면 `-print`의 tuple과 SHA-256이 바뀌므로 승인은 보인 bytes에 대한 것이다(`TestCSharpG1ApprovalPendingAssetsAreBoundByTheTuple`).
- **대기 목록은 시험에 literal로 고정된다(PR2 독립 검토 지적 1).** `g1ApprovalPendingPinned`가 아홉 asset의 경로·role·revision·SHA-256을 literal로 갖고, `TestCSharpG1ApprovalPendingAssetsArePinned`가 두 번째 묶음의 멤버 목록·추적 파일의 bytes·추적 G1 asset의 총수(첫 묶음 11 + 9)와 대조한다. 열째 asset을 더하거나 하나를 바꾸면 코드와 시험을 함께 고쳐야 한다. 두 번째 tuple이 승인되면 asset은 대기 목록에서 빠지고(`CSharpG1PendingAssets`가 비어 있다) object가 그 bytes를 고정한다. 목록에 없는 G1 asset은 대기 asset도 승인된 asset도 아니어서 첫 묶음 검증이 `G1_APPROVAL_UNAPPROVED`로 거부한다.
- **도구:** `go run ./src/dev/csharp-g1-approval -root . -b34 -print > tuple.json`(첫 묶음이 `APPROVED`여야 한다). `-verify`는 첫 묶음 다음에 두 번째 묶음 상태를 적고 asset 아홉을 `pending, not approved:`로 나열한다.
- **종료 코드(PR2 독립 검토 지적 2):** `-verify`의 exit 0은 **추적된 모든 G1 asset이 승인되었다**는 뜻이다. 첫 묶음이 `APPROVED`이고 두 번째 묶음의 asset이 대기 중이면 **exit 4**다(`APPROVED by object …` 줄과 `pending, not approved:` 줄이 함께 나온다). 첫 object가 없으면 exit 3, 불일치·읽기 오류는 exit 1, 사용법 오류는 exit 2다. 두 번째 object가 쓰이면 대기 줄이 사라지고 exit 0이 된다(`TestVerifyPendingAssetsHaveTheirOwnExitCode`가 임시 root에서 두 경우와 승인된 asset 변조의 exit 1을 모두 시험한다). `TestCSharpG1ApprovalOfTheTrackedBundle`은 두 번째 묶음 상태와 대기 asset 목록을 로그에 적고 목록이 고정표와 다르면 실패한다.

## 분리 시험

`TestCSharpG1ExpectedSeparation`은 다음을 보인다.

- (보조) 도출 경로의 코드(`csharp_g1_spec.go`, `_expected*.go`, `csharp_g1_kindmap.go`)는 `os`·`exec`·네트워크·native·reproduce·runner 패키지를 import하지 않고 native 입력을 가리키는 식별자·문자열을 갖지 않는다. 이 검사는 이름과 고정 파일 목록에 기대므로 우회할 수 있어, 우회를 막는 증명은 위 호출 폐쇄 시험이 한다. 도출은 바이트 입력만 받는 함수다.
- 명세 도출기(`csharp_g1_spec.go`)는 `regexp`·`strings`·`unicode/utf8`만 쓰며 Roslyn 관측·매핑표·기대값·원장을 가리키는 식별자가 없다. 관측 verifier와 매핑표 코드는 기대값 쪽을 가리키지 않는다.
- 전수 감사 파일(`csharp_g1_audit76.go`)과 그 규칙·ground·핀 표·뷰 파일은 도출 쪽으로 분류하고 호출 폐쇄의 진입점(`ParseCSharpG1Audit76`, `G1Audit76View` 등)에 넣었다. 폐쇄가 표현하지 못하는 "`ts`를 읽지 않음"은 호출 폐쇄 시험이 아니라 [전수 감사](#차분-oracle-대안-전수-감사-g1audit76)가 적은 구조(좁은 입력 타입, 감사별 폐쇄의 파일 목록, 타입 검사)가 고정하고 AST 시험은 보조다.
- type 감사 파일(`csharp_g1_typeaudit76.go`)도 도출 쪽으로 분류하고 진입점(`ParseCSharpG1TypeAudit76` 등)에 넣었다. "매핑표를 읽지 않음"도 [type 감사](#차분-oracle-대안-type-감사-g1typeaudit76)가 적은 같은 구조가 고정한다.
- G1-3b 기대값 파일 여섯(`csharp_g1_b34.go`·`_rules.go`·`_audit.go`·`_audit_rules.go`·`_view.go`와 `csharp_g1_kindmap_r3.go`)은 도출 쪽이다. 감사 둘의 호출 폐쇄는 [G1-3b 기대값](#g1-3b-기대값-b03b04-g1b34)의 독립성이 적은 대로 자기 파일(과 G1-3a 감사의 공용 비교·ground 표) 밖의 G1 파일에 닿지 않는다.
- G1-3b 분류 파일 셋(`csharp_g1_classify.go`·`_rules.go`·`_view.go`)도 도출 쪽으로 분류하고 진입점(`ClassifyCSharpG1`·`G1ClassifyView`)에 넣었다. 분류 함수만의 폐쇄는 [G1-3b 분류](#g1-3b-분류-b03b04-g1classify)가 적은 대로 기대값·정정·뷰 파일에 닿지 않는다.
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
| `G1_B34_SCHEMA`, `G1_CORRECTION_*`(B03·B04 r2) | r1 verifier가 r2 asset을, r2 verifier가 r1 asset을 받음. r2 기대값의 정정: 형식·근거·알 수 없는 항목(EXPECTED가 아닌 대안, 없는 앵커)·r1과 다른 `old`·BLOCKED 새 값의 형식(등록되지 않은 code 포함)·기록에 없는 차이·저작되지 않은 기록·pin·정규 bytes |
| `G1_TYPEAUDITB34R2_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_COUNTS`, `_PIN`, `_ROWS`, `_ROLE`, `_ROW`, `_GROUND`, `_COMPARISON`, `_FINDINGS` | type 감사 r2의 형식·schema·합계·정규 bytes, r1 감사 asset의 pin, 앵커와 맞지 않는 맥락 집합, 역할의 형식(r1 후보 집합 밖 type·정렬·중복·사유·grammar ground·미사용 역할·순서), 맥락의 역할·핀, 행별 핀과 다른 ground·역할 배정, 앵커와 어긋난 결과·relation, 발견 |
| `G1_B34X_SCHEMA`(r2), `G1_B34X_PIN`(r2) | executed r2가 다른 revision의 기대값에 묶임, r1 asset이 r2로 검증됨(그 반대도) |
| `G1_NATIVE_JSON`, `_SCHEMA`, `_SIZE`, `_SOURCE`, `_TREE` | 원시 관측의 형식, source 중복·상태, node 범위·부모·digest |
| `G1_LEDGER_SEALED`, `_EXECUTED` | 원장이 읽는 기대값·executed asset의 상태·사유·대안 참조 위반. 원장 r2는 같은 대안의 중복, 행 밖·원장에 없는 대안, B03·B04의 기대값 asset이 행의 대안을 빠뜨림, r1 파생에 B03·B04 asset을 줌도 `G1_LEDGER_SEALED`다 |
| `G1_B34X_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES` | B03·B04 executed asset의 형식·알 수 없는 member, schema, 4 MiB 상한, 정규 bytes·basis와 다름 |
| `G1_B34X_PIN`, `_SOURCES`, `_NATIVE`, `_OBSERVATION` | 기대값·관측 pin(원장 원천 pin 포함), source 목록·신원·수락 여부, native provenance·한도·원시 관측 pin, 읽을 수 없는 관측 |
| `G1_B34X_ALTERNATIVES`, `_EXPECTATION` | 대안의 누락·추가·중복, 봉인된 witness·부분·type·앵커·근거·행·`discriminating`·`tier`의 변조 |
| `G1_B34X_RESULT`, `_EVIDENCE`, `_COUNTS`, `_UNDISPOSED`, `_RAW` | 증거와 모순되는 결과·`executed`, 증거 형식, 집계, 처분 없거나 바뀐 불일치, 원시 관측과 다른 asset |
| `G1_KINDMAP_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_GRAMMAR`, `_VOCABULARY`, `_ADDITION`, `_KINDS`, `_CLASS`, `_ENTRY`, `_COVERAGE`, `_TS_ONLY`, `_COUNTS` | 매핑표의 형식·pin·어휘·항목·관측 종류 덮임·집계 |
| `G1_AUDIT76_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_COUNTS`, `_EXPECTED`, `_ROWS`, `_ROW`, `_WITNESS`, `_CORRECTION`, `_INSTANCE`, `_RULE`, `_GROUND`, `_COMPARISON` | 위치 감사의 형식·schema·합계·정규 bytes, r2가 아닌 입력, 행 집합·형식(ground의 offline 점검), 행과 witness의 source 불일치, 읽을 수 없는 정정 기록, 범위·정렬이 틀린 인스턴스, 규칙에 맞지 않는 인스턴스, 행별 핀과 다른 ground, r2 witness와 어긋난 결과·relation·initial |
| `G1_TYPEAUDIT76_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_COUNTS`, `_ROWS`, `_ROW`, `_CORRECTION`, `_GROUND`, `_COMPARISON` | type 감사의 같은 항목(행 형식은 분류별 type 개수·정렬·외부 token 표지·grammar ground) |

```
go run ./src/dev/csharp-g1-kindmap -root . -grammar <tree-sitter-c-sharp checkout>
go run ./src/dev/csharp-g1-expected -root .
go run ./src/dev/csharp-g1-executed -root . [-native <원시 관측> [-write]]
go run ./src/dev/csharp-g1-executed -root . -set b34 [-native <원시 관측> [-write]] [-r1]
go run ./src/dev/csharp-g1-b34 -root . -r2 [-write] [-grammar <dir> -spec <dir>]
go run ./src/dev/csharp-g1-b34 -root . -audit type2 [-in <저작 JSON> -write] [-pins] [-grammar <dir>]
go run ./src/dev/csharp-g1-native ... -set b34
go run ./src/dev/csharp-g1-correct -root . [-grammar <checkout> -spec <dir>]
go run ./src/dev/csharp-g1-ledger -root .
go run ./src/dev/csharp-g1-ledger -root . -r2 [-write]
go run ./src/dev/csharp-g1-approval -root . -verify
go run ./src/dev/csharp-g1-approval -root . -b34 -print
```

## 이 단계가 주장하지 않는 것

- tree-sitter 결과가 맞다는 판정. 기대값 asset은 native를 처음 실행하기 전의 기대값이며, native 비교는 별도 asset이 witness에 한정해 기록한다.
- Roslyn이 명세와 같다는 일반 주장. 표본 감사와 대조는 이 관측 범위 안의 근거다.
- BLOCKED 대안과 B03~B05·V 행의 기대값, 오류 복구(D3)와 symbols 문맥. native 비교의 기록(`EXECUTED`)은 BLOCKED 대안에 없다.
- 승인 pin: 사용자 결정(2026-11-13, 승인 묶음)은 G1 asset 11개를 한 묶음으로 승인한다. 묶음의 구성과 tuple은 [G1 번들 승인 tuple](#g1-번들-승인-tuple-g1approval)이 정하며, 관측은 r2(유도 입력)와 r3(재현 가능한 도구 hash) 둘 다 묶는다. 승인 object는 만들지 않았다: Root가 CI 통과와 사용자의 SHA 확인 뒤에 만든다.
