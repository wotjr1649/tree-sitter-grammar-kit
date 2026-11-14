# C# 선언 지원 범위의 의무 원장 계약 (#233 G1-1)

이 문서는 별도 추적 계약 [`csharp-g1-ledger-r1.json`](../../src/contracts/csharp-g1-ledger-r1.json)(schema `tsgk-csharp-g1-ledger/r1`)의 형식과 검증을 정한다. 원장은 grammar-kit가 선언한 C# 지원 범위 전체를 의무로 펼친 **정적 목록**이다. 기대값이 없고, native·Roslyn·`dotnet`을 실행하지 않으며, 어떤 지원 범위나 release claim도 바꾸지 않는다. 계약 자체는 완료된 기능이 아니라 G1(C# 전체 선언 범위의 의무 → 독립 기대값 → 실행 근거 대조)의 첫 단계다. 이 문서는 뒤에서 G1-2의 Roslyn 관측 계약도 정한다. v0.3.0의 C# claim은 그대로 C1–C4 교정과 기존 qualification PASS로 한정된다.

## 범위와 상태

- 대상은 inventory(`qualification-c1.json`)의 `csharp` route 15행(B01–B05, V73, V08–V13, V14a/b/c)의 필수 kind 의무 65개(P15·N15·R9·E12·Q12·W2), `feature-alternatives.json`의 production 1,425·fact 106 = 대안 1,531개, 등록 사례 389개(514 step), 선언 node 20종, 그리고 C#을 실행하는 추가 역할(`n461-svc`, `n461-large`, `csharp-symbols`)이다.
- 의무의 상태는 `NO_EXPECTED`, `SEALED`, `BLOCKED`, `EXECUTED` 중 하나다. `NO_EXPECTED`는 아직 기대값을 만들지 않았다는 뜻, `SEALED`는 독립 기대값이 봉인됐다는 뜻, `BLOCKED`는 기대값을 만들 수 없다고 처분했다는 뜻(대안의 `block` member가 사유 code를 적는다), `EXECUTED`는 봉인된 기대값을 실행 근거(native)와 대조해 **일치했다**는 뜻이다(대조가 `MISMATCH`이거나 돌지 못한 대안은 `SEALED`에 남는다). 전환은 `NO_EXPECTED`→`SEALED`/`BLOCKED`(기대값 asset)와 `SEALED`→`EXECUTED`(executed asset의 `result`가 `MATCH`)뿐이며 `BLOCKED`는 `EXECUTED`가 될 수 없다. `csharp-B01`·`csharp-B02`의 대안은 기대값 asset([기대값 계약](csharp-g1-expected.md))이 `EXPECTED`로 가진 것이 `SEALED`(native와 대조해 일치하면 `EXECUTED`), `BLOCKED`로 가진 것이 `BLOCKED`이고 다른 행의 모든 대안은 `NO_EXPECTED`다. 한 행의 P 의무는 그 행의 모든 대안이 `SEALED` 또는 `EXECUTED`일 때만 `SEALED`, 모두 `EXECUTED`일 때만 `EXECUTED`다. verifier는 유도 결과와 정확히 같을 때만 받아들이며(`G1_LEDGER_ALTERNATIVE`·`G1_LEDGER_ROWS`) 어휘 밖의 값은 `G1_LEDGER_STATUS`로 거부한다. 규칙과 반례는 [상태 전환 규칙](csharp-g1-expected.md#원장-상태-전환-규칙)에 있다. 원장 유도 코드(`csharp_g1_ledger.go`)는 도출 쪽이라 executed asset의 `result`만 읽을 뿐 비교 쪽 파일의 선언에 닿지 않는다(`TestCSharpG1SeparationClosure`가 호출 폐쇄로 증명하고, 원장 안의 `result` 낱말이 executed asset의 것과 같음은 `TestCSharpG1LedgerResultWordsAreTheExecutedAssets`가 확인한다). 원장은 기대값 asset의 bytes를 pin하지 않는다(관측 asset이 원장을, 기대값 asset이 관측 asset을 pin하므로 되돌아 pin하면 순환이다). 기대값 asset은 `SEALED`가 없는 원장 유도 결과(상태 미설정)에서 나온다.
- 이 문서의 asset은 원장 r1이고 **동결**이다. 새 revision [`csharp-g1-ledger-r2.json`](../../src/contracts/csharp-g1-ledger-r2.json)(schema `tsgk-csharp-g1-ledger/r2`, G1-3b)은 같은 입력에 B03·B04의 기대값·executed asset 쌍을 더해 그 두 행의 대안 상태를 옮긴다(`EXECUTED` 291·`SEALED` 5·`BLOCKED` 92). 규칙과 점검은 [기대값·매핑표 계약의 원장 r2](csharp-g1-expected.md#원장-r2)가 정한다. r1의 입력·도출·verifier는 바뀌지 않았다.
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
| `alternatives` | 1,531개를 행 순서·대안 순서대로. 대안 id·행·kind·상태·`block`(`BLOCKED`만: 기대값 asset의 사유 code)·`presence`·사례 연결(`links`) |
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
| `G1_LEDGER_STATUS` | `NO_EXPECTED`·`SEALED`·`BLOCKED`·`EXECUTED` 밖의 상태 |
| `G1_LEDGER_SEALED`, `G1_LEDGER_EXECUTED` | 원장이 읽는 기대값 asset(상태·`BLOCKED`의 사유 누락·알 수 없는 대안)과 executed asset(`BLOCKED`·알 수 없는 대안·다른 행·중복·기대값 asset 없음·`result` 누락·알 수 없는 `result`·`executed`와 모순되는 `result`)의 위반 |
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

- 대안별 기대 node type·범위(독립 기대값), 오류 복구의 속성 기대값, 선언 항목의 이름·범위 기대값, version 경계 수락 행렬은 이후 단계(G1-3부터)의 일이다. Roslyn 관측은 G1-2가 별도 asset으로 더했다(아래 절).
- 원장은 어떤 의무가 충족됐다고 판정하지 않는다. `NO_EXPECTED`는 "기대값이 아직 없다"는 뜻이다.
- 추가 역할은 사례·step 수만 연결한다. SVC inline·large 오류 fixture·ordinary query 좌표의 소유는 #362·#216이다.

## Roslyn 관측 계약 (G1-2)

별도 추적 계약 [`csharp-g1-roslyn-observed-r2.json`](../../src/contracts/csharp-g1-roslyn-observed-r2.json)(schema `tsgk-csharp-g1-roslyn-observed/r2`)은 원장의 사례 source를 **로컬 .NET SDK 10.0.401에 포함된 Roslyn DLL이 관측한 결과**다. 이 asset의 주장은 "그 DLL이 그 source에서 보인 행동"으로 한정한다. Roslyn 소스나 C# 사양 자체의 일반화가 아니며, 기대값이 아니다(원장의 모든 의무는 계속 `NO_EXPECTED`). 이것은 G1-3 이후 단계가 "차분 oracle 기대값"(사용자 결정 D2)을 만들 때 읽는 입력이다. native 출력은 읽지 않는다.

### 도구

- `src/dev/csharp-g1-roslyn`(`Observe.cs`, `Observe.csproj`, `global.json`)이 **하나뿐인 Roslyn 관측기**다. 모드가 셋이다: `g1`(이 계약의 관측), `pp-r2`·`pp-r3`(#218의 조건부 컴파일 관측 `tsgk-csharp-pp-roslyn/r2`·`r3`, 입력은 `src/testdata/csharp-pp/cases.json`). 세 모드는 `features`, `directives`, `disabled`, `errors`, `diagnostics`를 한 번의 같은 source별 pass로 낸다. 기존 `src/dev/csharp-pp-oracle`은 삭제했다: `pp-r2`가 다시 만든 출력의 SHA-256(`261213a1…`)이 보존한 `src/testdata/csharp-pp/roslyn-observed.json`과 승인 pin(`legacy-observation` tuple)의 `observation_sha256`과 byte 단위로 같음을 확인했기 때문이다. `pp-r3`는 옛 oracle의 r3 출력(SDK와 DLL 3개의 provenance)과 `oracle_sha256`(생산 어셈블리 hash)을 뺀 나머지가 byte 단위로 같다. `g1` 모드는 거기에 선언·구문 node·token·수락 여부·`LanguageVersion` 관측을 더한다.
- `global.json`이 SDK를 10.0.401로 고정한다(`rollForward: disable`). `Observe.csproj`는 패키지 없이 SDK의 `Roslyn/bincore`에 있는 `Microsoft.CodeAnalysis.dll`·`Microsoft.CodeAnalysis.CSharp.dll`만 참조하므로 NuGet 복원은 패키지를 받지 않는다. 어셈블리 hash가 checkout이나 PDB 경로에 달라지지 않도록 source revision과 PDB를 끈다. 설치 경로도 어셈블리에 넣지 않는다: `AssemblyMetadata`는 `TSGK.SDK`(버전 문자열) 하나뿐이고(시험 `TestCSharpG1ObserverCarriesNoInstallPath`는 csproj 전체와 `Observe.cs`에서 `$(RoslynDir)`·`$(NetCoreRoot)`가 `Reference`의 `HintPath`와 `RoslynDir` 정의 밖의 속성으로 흘러듦, `GetFullPath`, 절대 경로 리터럴, 경로를 받는 assembly 속성을 금지하고 변이 7종으로 확인한다), 관측기는 Roslyn 디렉터리를 실행 시점에 `RuntimeEnvironment.GetRuntimeDirectory()`의 세 단계 위(dotnet root)에서 `sdk/<TSGK.SDK>/Roslyn/bincore`로 찾으며 없으면 실패한다. 그래서 같은 SDK를 가리키는 서로 다른 dotnet 경로(디렉터리 junction 포함)에서 build한 `Observe.dll`의 SHA-256, 곧 `tool_sha256`이 같다. 이 동일성은 Go 시험으로 확인할 수 없어 로컬(HARD Job)에서 두 junction 경로 build와 두 build 디렉터리의 SHA-256을 비교해 확인한다. 이 관측기로 다시 만든 관측은 **새 asset** [`csharp-g1-roslyn-observed-r3.json`](../../src/contracts/csharp-g1-roslyn-observed-r3.json)이다(schema는 r2와 같다). r2 asset(`csharp-g1-roslyn-observed-r2.json`, SHA-256 `8930422a…`)은 동결된 역사 입력이다: 기대값·executed의 r1·r2가 그 파일의 SHA-256을 pin하므로 r2를 고쳐 쓰면 봉인된 파일이 연쇄로 바뀐다. r3는 r2와 `/provenance/tool_sha256` 값 하나만 다르다(`e18e3305…` → `1cf824bd…`, 파일 SHA-256 `01b220ba…`). verifier `ParseCSharpG1RoslynObservedR3`(도구 `csharp-g1-roslyn-asset -verify-r3`)와 시험 `TestCSharpG1RoslynR3IsR2WithTheBuildHash`가 이를 고정한다: r3가 같은 schema로 parse되고, r2와 그 한 값 외에 같고, 그 값이 상수 `CSharpG1RoslynR3ToolSHA256`이며, 추적된 관측기 소스(`Observe.cs`·`Observe.csproj`·`global.json`)의 SHA-256이 상수로 고정된다(소스가 바뀌면 시험이 실패해 다시 build와 새 revision을 요구한다). **`tool_sha256`을 정하는 build 방식:** `Release`, `--no-incremental`, `-p:UseSharedCompilation=false`(SDK 10.0.401). 다른 방식(Debug, 증분 build, 공유 컴파일)은 다른 hash를 낸다. `1cf824bd…`는 commit `a12bd3c`의 관측기 소스(그 뒤 변경 없음)를 이 방식으로 clean build한 `Observe.dll`의 SHA-256이다. 이후 관측기는 r3를 쓰고 검증하며, G1-3 승인 tuple은 r3를 묶는다. 알려진 한계: `Observe.cs`가 기록 경로를 `IndexOf("/sdk/<버전>/")`의 첫 일치로 자르므로 dotnet 설치 경로 자체에 그 조각이 있으면 기록 경로가 길어진다. 고치면 `tool_sha256`이 바뀌어 r3를 다시 만들어야 하므로 고치지 않고 둔다. 분석기 한계: 호출 폐쇄 시험은 현재 GOOS·build tag의 파일만 읽으며 CI는 windows·linux 두 환경에서 돈다.
- 입력은 `go run ./src/dev/csharp-g1-roslyn-asset -root . -input <파일>`이 원장과 fixture에서 만든다(사례마다 원본과 edit 뒤의 source 각각이 한 항목, id `<사례>#<step>`, preprocessor symbols 없음). `dotnet Observe.dll g1 <입력> <출력> <원장>`이 관측하고 원장의 pin을 출력에 적는다. 출력은 결정적이어서 같은 입력의 두 실행이 byte 단위로 같다. 도구는 개발용이며 CI에서 실행하지 않는다. CI는 추적 asset만 offline으로 읽는다.
- 실행은 HARD Job(메모리 4 GiB)과 단계별 시간 한도 안에서만 하고, 끝난 뒤 `dotnet`·`VBCSCompiler`·MSBuild node가 남지 않았는지 확인한다.

### 관측 항목

모든 offset은 source의 UTF-8 byte offset이다. source마다 한 줄이다.

| 항목 | 내용 |
|---|---|
| `id`, `sha256`, `bytes` | `<사례>#<step>`, source의 SHA-256과 크기. 원장에서 다시 유도한 source와 같아야 한다 |
| `accepted` | `LanguageVersion.Latest`로 파싱한 결과에 error 진단이 하나도 없는가 |
| `diagnostics`, `errors` | error 진단의 ID·span과 Roslyn이 붙인 문맥(directive 종류, `EndOfFile`, `Other`), 그 ID 집합 |
| `features`, `directives`, `disabled` | 기존 oracle의 관측: parser feature, directive의 종류·`#` 위치·active·taken·물리 줄, 비활성 구간 |
| `declarations` | 선언 node의 SyntaxKind·span과 이름(이름 token의 text·span. 이름이 없거나 누락 token이면 빈 목록) |
| `nodes` | 모든 구문 node(token·trivia 제외)의 `[kind 번호, start, end]`를 preorder로. `kinds`가 번호의 SyntaxKind 이름표(node와 token 공용) |
| `tokens`, `missing` | 모든 token(trivia 제외, skipped-token trivia 안의 token 포함 안 함)의 `[kind 번호, start, end]`를 source 순서로(`start`·`end`는 token 본문의 span이며 앞뒤 trivia는 넣지 않는다). `missing`은 Roslyn이 `IsMissing`으로 표시한 token의 `tokens` 안 번호 목록. 누락 token은 tree-sitter의 MISSING node와 같은 개념이고 D3의 오류 위치 속성 P2와 어휘 행 B01의 근거다 |
| `versions` | `LanguageVersion` C# 7.3, 8, …, 14 각각의 수락 여부와 error 진단(ID·span) |
| `oracle_error` | Roslyn이 진단을 열거하지 못한 source(관측 없음). 이 항목은 다른 필드를 갖지 않는다 |

선언 node는 `MemberDeclarationSyntax`(top-level statement·`extension` block 포함), type·delegate·namespace 선언, 접근자, 지역 함수, 지역 변수 선언문, 매개변수, 형식 매개변수, using 지시문, extern alias, 최상위 attribute 목록이다. 이는 원장의 선언 node 20종과 매핑 밖 11종을 모두 덮는 Roslyn 쪽 분류이며 tree-sitter node와의 대응표(G1-3)는 아니다.

### provenance와 결속

- `ledger`: 원장 asset의 경로·bytes·SHA-256. `input`: 도구 입력 문서의 bytes·SHA-256(verifier가 원장과 fixture에서 다시 유도한 문서와 같아야 한다).
- `provenance`: `sdk`(10.0.401), `runtime`(`.NET 10.0.12`), 도구 어셈블리 SHA-256, 로드한 Roslyn 어셈블리 2개(`Microsoft.CodeAnalysis`, `Microsoft.CodeAnalysis.CSharp`)의 SDK 상대 경로·bytes·SHA-256·assembly 버전·product 버전, `LanguageVersion` 열거 전체와 관측한 버전 목록과 `Latest`가 가리키는 유효 버전(CSharp14), 파싱 옵션(`DocumentationMode.Parse`, `SourceCodeKind.Regular`). 도구는 SDK 파일과 로드한 사본의 bytes가 같음을 확인한 뒤에만 기록한다.
- verifier는 provenance의 **형식과 일관성**만 확인한다(`sdk`가 `주.부.수` 형식, 어셈블리 경로가 그 SDK를 가리킴, 두 DLL의 product 버전이 같음, SHA-256이 hex 64자, bytes가 양수 등). 저장소의 버전 가드(`TestNoHardcodedToolVersions`)가 코드에 도구 버전·DLL pin을 두는 것을 금지하므로 SDK 10.0.401과 DLL SHA-256은 코드 상수가 아니라 asset의 provenance에 있다. 그것을 고정하는 것은 asset의 SHA-256이다: 시험이 asset 전체의 크기·SHA-256을 봉인하고, 승인된 evidence pin(#370 V2-D)에 결속하는 일은 G1-3 봉인 단계의 몫이다(이 단계는 승인을 만들지 않는다).

### 검증

`kit.ParseCSharpG1RoslynObserved(data, ledger, sources)`는 먼저 원장이 저장소 선언의 유도 결과와 같음을 확인하고, asset을 엄격히 decode한 뒤(알 수 없는 member 거부, 4 MiB 상한) 다음을 요구한다. 이 verifier는 Roslyn을 다시 실행하지 않는다.

| code | 거부하는 것 |
|---|---|
| `G1_ROSLYN_JSON`, `_SCHEMA`, `_SIZE` | 형식, schema, 크기 |
| `G1_ROSLYN_LEDGER_PIN`, `_INPUT_PIN` | 원장·입력 문서와 다른 pin |
| `G1_ROSLYN_PROVENANCE` | SDK 형식·어셈블리 이름·경로·크기·SHA-256 형식·product 버전 일관성, 열거·관측 버전 목록(C# 7.3 이상, 열거의 값과 일치, `Latest`의 유효 버전 포함)·파싱 옵션이 맞지 않음 |
| `G1_ROSLYN_KINDS` | SyntaxKind 이름표가 정렬·유일·형식을 지키지 않음 |
| `G1_ROSLYN_SOURCE_SET` | 원장 사례의 source 목록과 id·순서·SHA-256·bytes가 다름(누락·추가·교환 포함) |
| `G1_ROSLYN_OBSERVATION`, `_ORACLE_ERROR` | 관측 필드 누락, `oracle_error`와 다른 필드의 공존 |
| `G1_ROSLYN_DIAGNOSTIC`, `_DIRECTIVE` | 진단 span·ID 형식·집합·수락 여부 불일치, `#`가 아닌 directive 위치·줄 범위·비활성 구간 순서 |
| `G1_ROSLYN_NODES` | 첫 node가 compilation unit이 아님, kind 번호·span 범위, 부분 겹침(중첩 위반) |
| `G1_ROSLYN_TOKENS` | token 목록이 비었거나 kind 번호·span이 범위를 벗어남, 시작 순서가 아니거나 서로 겹침, 마지막 token이 end-of-file token이 아님, 누락 token의 폭이 0이 아니거나 번호가 범위 밖·비오름차순, 수락한 source에 누락 token이 있음(Roslyn은 누락 token마다 오류를 붙인다), node의 시작이나 끝이 어떤 token의 경계와도 맞지 않음 |
| `G1_ROSLYN_DECLARATION` | 선언이 node가 아님(종류·span), 이름 text가 source와 다르거나 선언 밖 |
| `G1_ROSLYN_VERSIONS` | 버전 목록·순서, 수락 여부와 진단의 모순, `Latest`의 유효 버전과 주 관측의 진단 불일치 |

```
go run ./src/dev/csharp-g1-roslyn-asset -root . -verify-r3 src/contracts/csharp-g1-roslyn-observed-r3.json
go run ./src/dev/csharp-g1-roslyn-asset -root . -verify src/contracts/csharp-g1-roslyn-observed-r2.json
go run ./src/dev/csharp-g1-roslyn-asset -root . -pp-check <pp-r2 모드의 출력>
```

`-pp-check`는 `dotnet Observe.dll pp-r2 src/testdata/csharp-pp/cases.json <출력>`(로컬, SDK 10.0.401)의 출력이 `roslyn-observed.json`과 byte 단위로 같고 그 SHA-256을 승인된 `legacy-observation` tuple이 `observation_sha256`으로 적고 있는지 확인한다. 다르면 첫 차이의 byte 위치와 줄을 보고한다. dotnet은 실행하지 않는다. 시험 `TestCSharpPPObservationSealed`가 추적 파일(124,328 B, SHA-256 `261213a1…`)과 그 tuple object를 고정한다. 승인 pin·tuple object·`roslyn-observed.json`은 이 통합에서 바뀌지 않았다.

### 이 단계가 주장하지 않는 것

- 기대값, native 대조, 수락 여부가 "정답"이라는 판정. Roslyn과 tree-sitter 결과의 차이는 G1-3 이후에 사양을 인용해 처분한다.
- trivia(skipped token 포함), 의미 분석(binding), preprocessor symbols가 있는 문맥(기본 모드만), `LanguageVersion.Preview`.
- 승인 pin(evidence-pin approval)과의 결속, CI에서의 재현.
