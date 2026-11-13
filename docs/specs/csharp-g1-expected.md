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
- 결과: 일치 25, **COVERAGE_GAP 1**(`csharp-B01.a356`, `#error`: 명세는 기대를 도출하지만 연결된 수락 source에 없다), 불일치 0. 불일치가 0이므로 Roslyn 오류율의 95% 상한은 3/26 ≈ **11.5%**(rule of three)다. 표본이 작아 이 상한은 느슨하다.
- 표본 감사가 확인한 것은 도출한 기대값과 명세의 일치다. 표본 밖의 오류율은 이 결과로 줄지 않는다.

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
| `SEALED` → `EXECUTED` | executed asset이 그 대안을 `executed: true`로 가진다(native로 실행해 대조했다) |

- `NO_EXPECTED`는 "아직 기대값을 만들지 않음"에만 쓴다. 두 asset이 다루지 않는 대안(B03 이후 행의 모든 대안)과 다른 행의 의무는 `NO_EXPECTED`다.
- `BLOCKED`는 기대값을 만들 수 없다고 처분한 대안이며 `EXECUTED`가 될 수 없다. 사유를 풀려면 기대값 asset의 새 revision이 필요하다. executed asset이 `BLOCKED`·알 수 없는 대안·다른 행·중복을 가리키면 원장 유도가 `G1_LEDGER_EXECUTED`로 거부하고, 기대값 asset이 `BLOCKED`에 사유를 달지 않았거나 알 수 없는 상태를 쓰면 `G1_LEDGER_SEALED`다. executed asset이 기대값 asset 없이 주어져도 `G1_LEDGER_EXECUTED`다.
- 행의 P 의무는 그 행의 모든 대안이 `SEALED` 또는 `EXECUTED`일 때만 `SEALED`, 모두 `EXECUTED`일 때만 `EXECUTED`다. `csharp-B01`·`csharp-B02`는 모두 `BLOCKED` 대안이 있어 P 의무가 `NO_EXPECTED`다. N·R·E·Q·W 의무는 다른 단계의 기대값이 필요하므로 바뀌지 않는다.
- 이 revision의 상태: B01·B02 대안 505개 중 `EXECUTED` 426(native 비교가 끝난 EXPECTED 대안 전부), `BLOCKED` 79(사유 code는 위 표), `SEALED` 0. 나머지 1,026개는 `NO_EXPECTED`다.
- 원장 verifier는 유도 결과와의 정확한 일치를 요구한다. 손으로 바꾼 상태(`BLOCKED`→`EXECUTED`, `EXECUTED`→`SEALED`, 사유 삭제·변경, 실행되지 않은 대안의 `EXECUTED`)는 `G1_LEDGER_ALTERNATIVE`·`G1_LEDGER_ROWS`로, 어휘 밖의 상태는 `G1_LEDGER_STATUS`로 거부된다(`TestCSharpG1LedgerForgeries`, `TestCSharpG1LedgerStateRules`).
- asset 간 결속은 한 방향이다: executed asset이 기대값·매핑표·관측 asset을, 기대값 asset이 관측 asset을, 관측 asset이 원장을 pin한다. 원장은 기대값·executed asset의 bytes를 pin하지 않고 상태만 받는다(되돌아 pin하면 순환이다). 원장 bytes가 바뀌면 관측 asset의 `ledger` pin(관측 내용은 그대로)과 그 SHA-256을 pin한 기대값·executed asset의 pin 한 줄이 따라 바뀌므로, 재생성은 원장 → 관측 pin → 기대값 → executed → 원장 → … 순서로 고정점까지 돌린다.

## native 비교 (봉인 뒤 단계)

별도 asset `csharp-g1-executed-b01b02-r1.json`이 봉인된 기대값 426개를 등록된 C# route의 tree-sitter-c-sharp native 빌드로 대조한 결과다. 기대값·매핑표는 이 단계에서 바뀌지 않는다. 관측 asset은 `ledger` pin 한 줄만, 그 결과로 기대값 asset은 `inputs.observation.sha256` 한 줄만 따라 바뀐다. 시험 `TestCSharpG1ExpectedContentIsTheSealedOne`은 그 한 줄을 비운 내용의 digest가 봉인 commit의 값과 같음을 고정한다.

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

대안은 witness 하나라도 `MISMATCH`이면 `MISMATCH`, 아니고 `MATCH`가 있으면 `MATCH`, 아니면 `NOT_APPLICABLE`이다. 비교가 실제로 돈 대안(완료된 native witness가 있음)만 원장에서 `EXECUTED`가 된다. 증거(`found`: 같은 범위 또는 같은 시작의 native node, `ts`에 속한 type을 앞에 두고 8개까지)가 witness마다 asset에 있고, verifier가 그 증거에서 결과를 다시 계산한다.

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

`GRAMMAR_DEFECT` 후보는 없다: native는 오류 없이 파싱했고 네 불일치는 모두 고정 grammar 원문과 같은 표현이다. 기대값 오류와 매핑표 오류로 본 네 건은 **고치지 않았다**. 봉인 뒤 수정은 별도 결정이 필요하다. 이 처분은 사람이 쓴 판단이며 `kit`의 `g1ExecDispositions` 표에 있다. 처분이 없는 불일치가 있으면 verifier가 `G1_EXECUTED_UNDISPOSED`로 거부한다.

### 기대값을 거꾸로 만들 수 없음

- executed asset은 기대값·매핑표·관측을 pin으로만 가리킨다. witness·`ts`·근거는 봉인된 것과 같아야 하고(`G1_EXECUTED_EXPECTATION`), 봉인된 type을 native 결과에 맞춰 넓히면 거부된다.
- 도출 경로의 코드(명세 도출기, 기대값, 규칙·검사·자료, 매핑표, 관측)는 비교 쪽 식별자(`G1Exec…`, `g1x…`, 원시 관측 타입 등)와 asset 이름을 가리키지 않는다. 비교 쪽 파일은 기대값·매핑표를 쓰거나 유도하거나 반환하는 함수를 쓰지 않는다. native tree가 전부 어긋나는 합성 입력에서도 기대값 유도는 봉인된 bytes를 그대로 낸다(`TestCSharpG1NativeNeverFeedsExpectations`).

### 한계

- 판정은 witness(대안당 사례 하나·최대 3개)에 한정되며 grammar가 대안의 모든 구문을 파싱한다는 뜻이 아니다. 대안의 `ts`가 넓으면(1:n) 일치가 약한 근거일 수 있다.
- offline verifier는 native를 다시 실행하지 못한다. 증거가 판정과 모순 없음, 봉인된 값·pin·source 신원·처분·bytes까지 확인하며, 증거 자체의 진위는 원시 관측과의 재유도(`TSGK_G1_NATIVE_RAW`로 `TestCSharpG1ExecutedMatchesRawNative`)가 확인한다. 원시 관측은 추적하지 않는다.
- 한 host(Windows, gcc 16.2.0)의 한 빌드, 기본 모드(preprocessor symbols 없음)다. symbols 문맥 13건과 오류 복구는 비교하지 않았다.

## 분리 시험

`TestCSharpG1ExpectedSeparation`은 다음을 보인다.

- 도출 경로의 코드(`csharp_g1_spec.go`, `_expected*.go`, `csharp_g1_kindmap.go`)는 `os`·`exec`·네트워크·native·reproduce·runner 패키지를 import하지 않고 native 입력을 가리키는 식별자·문자열을 갖지 않는다. 도출은 바이트 입력만 받는 함수다.
- 명세 도출기(`csharp_g1_spec.go`)는 `regexp`·`strings`·`unicode/utf8`만 쓰며 Roslyn 관측·매핑표·기대값·원장을 가리키는 식별자가 없다. 관측 verifier와 매핑표 코드는 기대값 쪽을 가리키지 않는다.
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
| `G1_NATIVE_JSON`, `_SCHEMA`, `_SIZE`, `_SOURCE`, `_TREE` | 원시 관측의 형식, source 중복·상태, node 범위·부모·digest |
| `G1_LEDGER_SEALED`, `_EXECUTED` | 원장이 읽는 기대값·executed asset의 상태·사유·대안 참조 위반 |
| `G1_KINDMAP_JSON`, `_SCHEMA`, `_SIZE`, `_BYTES`, `_GRAMMAR`, `_VOCABULARY`, `_ADDITION`, `_KINDS`, `_CLASS`, `_ENTRY`, `_COVERAGE`, `_TS_ONLY`, `_COUNTS` | 매핑표의 형식·pin·어휘·항목·관측 종류 덮임·집계 |

```
go run ./src/dev/csharp-g1-kindmap -root . -grammar <tree-sitter-c-sharp checkout>
go run ./src/dev/csharp-g1-expected -root .
go run ./src/dev/csharp-g1-executed -root . [-native <원시 관측> [-write]]
go run ./src/dev/csharp-g1-ledger -root .
```

## 이 단계가 주장하지 않는 것

- tree-sitter 결과가 맞다는 판정. 기대값 asset은 native를 처음 실행하기 전의 기대값이며, native 비교는 별도 asset이 witness에 한정해 기록한다.
- Roslyn이 명세와 같다는 일반 주장. 표본 감사와 대조는 이 관측 범위 안의 근거다.
- BLOCKED 대안과 B03~B05·V 행의 기대값, 오류 복구(D3)와 symbols 문맥. native 비교의 기록(`EXECUTED`)은 BLOCKED 대안에 없다.
- 승인 pin: 관측 r2·기대값·매핑표·executed asset은 사용자가 함께 승인한다(Q1). 승인 object는 만들지 않았다. 승인 대상 SHA-256은 위 재생성 순서의 고정점 값이다.
