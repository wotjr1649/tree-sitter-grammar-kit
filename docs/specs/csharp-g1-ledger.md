# C# 선언 지원 범위의 의무 원장 계약 (#233 G1-1)

이 문서는 별도 추적 계약 [`csharp-g1-ledger-r1.json`](../../src/contracts/csharp-g1-ledger-r1.json)(schema `tsgk-csharp-g1-ledger/r1`)의 형식과 검증을 정한다. 원장은 grammar-kit가 선언한 C# 지원 범위 전체를 의무로 펼친 **정적 목록**이다. 기대값이 없고, native·Roslyn·`dotnet`을 실행하지 않으며, 어떤 지원 범위나 release claim도 바꾸지 않는다. 계약 자체는 완료된 기능이 아니라 G1(C# 전체 선언 범위의 의무 → 독립 기대값 → 실행 근거 대조)의 첫 단계다. v0.3.0의 C# claim은 그대로 C1–C4 교정과 기존 qualification PASS로 한정된다.

## 범위와 상태

- 대상은 inventory(`qualification-c1.json`)의 `csharp` route 15행(B01–B05, V73, V08–V13, V14a/b/c)의 필수 kind 의무 65개(P15·N15·R9·E12·Q12·W2), `feature-alternatives.json`의 production 1,425·fact 106 = 대안 1,531개, 등록 사례 389개(514 step), 선언 node 20종, 그리고 C#을 실행하는 추가 역할(`n461-svc`, `n461-large`, `csharp-symbols`)이다.
- 의무의 상태는 `NO_EXPECTED`, `SEALED`, `EXECUTED` 중 하나다. 이 revision은 모든 의무가 `NO_EXPECTED`이며 verifier는 다른 상태를 `G1_LEDGER_STATUS`로 거부한다. `SEALED`(독립 기대값 봉인)와 `EXECUTED`(실행 근거 대조)는 기대값 asset을 갖춘 뒤의 revision이 허용한다.
- 원장은 기대값을 만들지도 베끼지도 않는다. 사례가 이미 등록한 기대 node type(`contains`와 anchor의 type)을 사례 step별로 옮겨 적을 뿐이며, 이는 대안별 기대값이 아니다.

## 입력과 결속

원장은 다음 파일의 경로·bytes·SHA-256에 결속된다(`inputs`). 하나라도 바뀌면 `G1_LEDGER_PIN`이다.

- `src/contracts/qualification-c1.json`, `src/contracts/feature-alternatives.json`, `src/contracts/fact-mapping.json`
- `src/testdata/native/{routes,gaps,n461,queries}/csharp.json`(사례 원문)

사례 원문은 fixture에서 읽고 inventory의 `input.sha256`·`bytes`와 같아야 하며(`G1_LEDGER_SOURCE`), edit을 `ApplyEdits`로 적용한 step별 원문을 대안 존재 확인에 쓴다. native 실행 결과는 읽지 않는다.

## 구조

| 절 | 내용 |
|---|---|
| `rows` | 행마다 대안 수와 kind 의무(`P`/`N`/`R`/`E`/`Q`/`W`). 의무마다 연결 사례와 그 사례의 해당 step. step은 coverage rule(`stepCovers`)이 그 kind로 세는 step이다(E는 edit 뒤 step, Q는 capture 기대가 있는 query step) |
| `cases` | 등록된 requirement 사례 389개: fixture, 입력 SHA-256, step별 syntax와 기대 node type |
| `alternatives` | 1,531개를 행 순서·대안 순서대로. 대안 id·행·kind·상태·`presence`·사례 연결(`links`) |
| `declarations` | `fact-mapping.json`의 선언 node 20종(type 8·member 12). production head가 그 node인 대안과 그 node를 기대하는 사례 수 |
| `outside_mapping` | 매핑 밖 선언 구문 11종의 목록과 선언한 지원 범위 안의 대안 |
| `extra_roles` | C# route를 실행하는 추가 역할의 workload별 사례·step 수 |
| `counts` | 위 집계 |

대안의 사례 연결은 inventory 사례의 `alternatives` 선언에서 온다. `links[].steps`는 P step(후보)이고 `links[].matched`는 그중 정적 패턴이 일치한 step이다.

## 대안의 존재 확인(`presence`)

대안의 존재는 선언에서 기계적으로 도출한 **정적 패턴**으로만 확인한다. 패턴은 production 우변에서 괄호 밖에 있고 `?`·`*`가 붙지 않은 따옴표 terminal(`literals`)이다. `\uXXXX`는 그 code point로 해석한다. 패턴이 연결 사례의 P step 원문 중 어디선가 순서대로 나타나면 `LITERAL_MATCH`이고 첫 일치의 사례·step·byte offset을 `evidence`에 남긴다. 단어 literal은 식별자 경계에서만 일치한다(`fragment` production은 경계 없이 부분 문자열로 일치한다).

| 상태 | 뜻 |
|---|---|
| `LITERAL_MATCH` | 패턴이 일치했다. `strength`가 `KEYWORD`(단어가 구문을 지명한다)·`PUNCTUATION`·`FRAGMENT`(어휘 fragment의 부분 문자열)이며 뒤의 둘은 약한 근거다 |
| `LITERAL_ABSENT` | 패턴이 있으나 연결 사례의 P step 어디에도 없다. 선언한 연결이 원문과 어긋난다는 뜻이다 |
| `UNVERIFIED_PRESENCE` | 정적 패턴을 도출할 수 없다. `reason`: `NO_REQUIRED_LITERAL`(비-terminal만), `CHARACTER_CLASS_OR_RANGE`, `TOP_LEVEL_ALTERNATION`, `FACT_DESCRIPTION`(fact 대안은 서술문) |

일치는 **텍스트 일치**다. 주석·문자열 안의 일치를 제외하지 않으며 어떤 대안이 실제로 그 구문으로 파싱되는지 증명하지 않는다. 그 확인은 이후 단계의 독립 기대값이 맡는다. 대안이 `LITERAL_ABSENT`이거나 `UNVERIFIED_PRESENCE`라고 해서 의무가 줄지 않는다.

## 매핑 밖 선언 구문(D1)

`declarations`는 fact 매핑이 가진 선언 node 20종이다. 매핑에 없는 선언 성격 구문은 손으로 적은 구문 표(`csharpG1Constructs`: 구문 이름, tree-sitter node type, production head 패턴)로 열거한다. 선언한 대안의 production head가 패턴에 일치하면 `IN_DECLARED_SCOPE`이고 `NO_EXPECTED` 의무가 된다. 일치하는 대안이 없으면 `NOT_IN_DECLARED_SCOPE`로 목록에만 남는다. 구문은 지역 함수, 지역 변수 선언, 접근자, 매개변수, 형식 매개변수, using alias, global using, extern alias, global attribute, top-level statement, record·primary constructor positional 매개변수다. 표를 넓히려면 원장 revision을 올린다.

## 검증과 재생성

`kit.ParseCSharpG1Ledger`는 원장을 엄격히 decode하고(알 수 없는 member 거부, 4 MiB 상한), 저장소 선언에서 다시 유도한 결과와 정확히 같음을 요구한다.

| code | 거부하는 변조 |
|---|---|
| `G1_LEDGER_JSON`, `_SCHEMA`, `_SIZE` | 형식, schema, 크기 |
| `G1_LEDGER_PIN` | 입력 파일의 경로·bytes·SHA-256, fixture pin 목록 |
| `G1_LEDGER_STATUS` | `NO_EXPECTED` 아닌 상태(사실상의 기대값 주장) |
| `G1_LEDGER_ROWS` | 행·kind 의무·분모·사례 연결의 변조 |
| `G1_LEDGER_ALTERNATIVE_MISSING`, `_DUPLICATE`, `_EXTRA`, `_ORDER` | 대안의 누락·중복·추가·순서 |
| `G1_LEDGER_ALTERNATIVE`, `_CASES`, `_DECLARATIONS`, `_OUTSIDE_MAPPING`, `_ROLES`, `_COUNTS` | 각 절의 내용 |
| `G1_LEDGER_BYTES` | 정규 형식(한 줄 한 항목)이 아닌 bytes |

원본이 달라 유도 자체가 안 되는 경우의 code는 `G1_LEDGER_ALTERNATIVES`(inventory 행과 대안 목록 불일치), `_MAPPING`, `_FIXTURE`, `_SOURCE`, `_CASE`, `_ROUTE`다.

```
go run ./src/dev/csharp-g1-ledger -root . [-write]
```

`-write` 없이 실행하면 추적 asset과 유도 결과를 비교한다. 이 명령은 native·Roslyn·`dotnet`을 실행하지 않는다.

## 아직 주장하지 않는 것

- 대안별 기대 node type·범위(독립 기대값), 오류 복구의 속성 기대값, 선언 항목의 이름·범위 기대값, version 경계 수락 행렬, Roslyn 관측은 이후 단계(G1-2부터)의 일이다.
- 원장은 어떤 의무가 충족됐다고 판정하지 않는다. `NO_EXPECTED`는 "기대값이 아직 없다"는 뜻이다.
- 추가 역할은 사례·step 수만 연결한다. SVC inline·large 오류 fixture·ordinary query 좌표의 소유는 #362·#216이다.
