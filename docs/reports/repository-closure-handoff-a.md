# 인계 A부 — `schema check`/`schema diff` 흡수 명세와 C# G1 발견

이 문서는 저장소 종료 결정([#195 코멘트](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/195#issuecomment-6875080857))에 따라 go-treesitter가 흡수할 두 가지를 적는다. A1은 `schema check`/`schema diff`(정적 node-types 비교)의 이식 명세이고, A2는 C# G1(B01–B04)과 #447에서 나온 문법 결함·불일치 지식이다.

## 읽는 법과 기준

- **kit 기준.** `origin/main` `8f2ed6f0c9563364b5e0fdc77bc8b0f8cb1d5a3d`(v0.3.0 이후 main). 파일 링크는 이 commit의 blob이다: `https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/8f2ed6f0c9563364b5e0fdc77bc8b0f8cb1d5a3d/<경로>`. 아래 경로는 모두 저장소 상대 경로다.
- **go-treesitter 기준.** 읽기 전용으로 본 HEAD `f496a18`의 ADR-0020(`docs/design/decisions/ADR-0020-cgo-free-library-on-unmodified-tree-sitter.md`)과 `tools/gen/README.md`·`tools/gen/pins.json`이다. 이 저장소의 내부 설계는 정하지 않는다.
- **크기는 계산한 값이다.** `wc -c`와 `sed -n 'a,bp' | wc -c`로 구했고 추정이 없다. 줄 범위는 위 commit 기준이다.
- **실행하지 않은 것.** Go 빌드·시험, `schema check`의 실제 grammar 실행, upstream grammar의 parse는 하지 않았다(읽기 전용 작업). 실행으로만 알 수 있는 것은 A1.8·A2.6에 "미확인"으로 적었다.

---

## A1. `schema check` / `schema diff` 흡수

### A1.1 한 줄 요약

`node-types.json`(tree-sitter generator 산출물) 두 개를 파서·컴파일러·process·network 없이 읽어, ① 한 파일이 형식 계약을 지키는지 검사하고(`check`) ② 두 파일의 정적 계약 차이를 방향성(baseline → candidate)과 검토 위험 범주로 보고한다(`diff`). 결과는 안전·SemVer·runtime 동작 판정이 아니라 검토 목록이다. 구현은 Go 표준 라이브러리만 쓰며 `schema.go` 한 파일이 핵심이다.

### A1.2 동작 명세

**입력.** 각 입력은 caller가 가진 bytes(`SchemaInput{Name, Data}`)이며 kit는 파일을 열지 않는다(`Name`은 표시용). CLI는 파일을 `MaxDocumentBytes`+1(16,777,217) bytes까지만 읽어 넘긴다.

**형식 `node-types-r1`**(`SchemaFormat`). 문서는 strict JSON decoder를 거친다. 중복 key(escape 동치 포함)·잘못된 UTF-8·trailing 값은 오류이고 중첩 깊이 32, JSON 값 8×records를 넘으면 한도 오류다. 위반은 첫 위반에서 멈추지 않고 모두 finding으로 모은다.

- 최상위는 비어 있지 않은 node 배열이다. `[]`은 `SCHEMA_EMPTY`, `null`은 `JSON_NULL`, 다른 값은 `JSON_TYPE`이다. 형식이 허용하지 않는 `null`은 어디서든 "없는 값"으로 보지 않는다.
- node identity는 `(type, named)`다. `type`은 문자열(빈 문자열 허용: generator의 `alias(rule, "")`), `named`는 boolean이며 둘 다 필수(`JSON_MISSING_FIELD`)다. 같은 identity가 두 번 나오면 모양이 달라도 `NODE_DUPLICATE`다.
- 선택 key는 `root`·`extra`(boolean), `fields`(이름 → set), `children`(set), `subtypes`(참조 배열)뿐이다. 그 밖의 key는 `SCHEMA_KEY_UNSUPPORTED`이고 이것만 있으면 판정이 `BLOCKED`다(fail-closed). `root`/`extra`가 없는 것과 `false`는 다르고, `fields`가 없는 것(leaf)과 `fields: {}`도 다르다.
- set은 `multiple`·`required`·`types`를 모두 가져야 한다. `types`·`subtypes`는 비어 있으면 안 되고(`TYPES_EMPTY`, `SUBTYPES_EMPTY`) 같은 참조를 두 번 적으면 `MEMBER_DUPLICATE`다. 집합 의미는 순서 없음이다.
- supertype(`subtypes`를 가진 node)은 `fields`·`children`을 가질 수 없다(`SUPERTYPE_SHAPE`). `subtypes`가 선언되지 않은 node를 가리키면 `REFERENCE_UNRESOLVED`, supertype 순환은 `SUPERTYPE_CYCLE`(반복 DFS), `root: true`가 둘 이상이면 `ROOT_MULTIPLE`이다.
- field·children의 허용 type이 별도 node로 선언되지 않은 것은 위반이 아니라 `REFERENCE_UNDECLARED` warning이다(generator의 alias가 이렇게 나온다).

**`check` 출력.** 판정: 위반이 하나라도 있으면 `FAIL`, `SCHEMA_KEY_UNSUPPORTED`만 있으면 `BLOCKED`, 그 밖은 `PASS`(warning 허용). 셋 다 `execution_status: COMPLETED`다. `input{role, name, bytes, sha256, counts}`를 내며 `counts`(node·named·anonymous·supertype·field·참조 수와 root 목록)는 PASS일 때만 있다. 항상 `SCHEMA_STATIC_ONLY` info finding이 붙는다. finding은 입력당 1,000건까지만 기록하고 넘치면 `FINDINGS_TRUNCATED` info로 총 건수를 남기며, 판정은 기록하지 않은 finding까지 포함해 계산한다. finding path는 `이름#JSON pointer`이며 1,024 bytes를 넘으면 `...(truncated)`로 자른다.

**`diff` 출력.** 두 입력이 모두 PASS일 때만 비교한다. 아니면 그 입력의 finding(path 앞에 `baseline:`/`candidate:`)을 보존하고 `INVALID_INPUT`/`SCHEMA_INVALID` 또는 `UNSUPPORTED`/`SCHEMA_KEY_UNSUPPORTED` 오류로 끝나며 부분 비교는 없다. 차이가 없으면 `PASS`, 있으면 `FAIL`(결함 판정이 아님)이고 항상 `SCHEMA_DIFF_DESCRIPTIVE` info가 붙는다. 각 차이는 `code`, `risk`, `node{type,named}`, `field`, `member`, 비교 값 `before`/`after`(canonical JSON, 없는 쪽은 `null`), 원본 문서 안의 JSON pointer `baseline_path`/`candidate_path`를 갖는다. 정렬은 node(type bytes 오름차순, anonymous 먼저) → code 순서표 → field → member이며 같은 입력은 byte 동일한 결과를 낸다.

**검토 위험 범주(`risk`) 6개**(`schema.go` 140–148행): `ADDITION`, `REMOVAL`, `IDENTITY`, `CLASSIFICATION`, `CARDINALITY_NARROWED`, `CARDINALITY_WIDENED`. "안전하다"를 뜻하는 값은 없다.

**diff code 20개와 risk**(`diffOrder`, 150–154행):

| code | risk |
|---|---|
| `NODE_REMOVED` / `NODE_ADDED` | `REMOVAL` / `ADDITION` |
| `NODE_NAMED_CHANGED` (같은 spelling의 identity가 baseline에서 정확히 하나 사라지고 candidate에 정확히 하나 생김 — 이때는 제거·추가 대신 이것 하나만 낸다) | `IDENTITY` |
| `ROOT_CHANGED` / `EXTRA_CHANGED` | `IDENTITY` / `CLASSIFICATION` |
| `FIELDS_PRESENCE_CHANGED` (leaf ↔ nonterminal) | `IDENTITY` |
| `FIELD_REMOVED` / `FIELD_ADDED` | `REMOVAL` / `ADDITION` |
| `FIELD_REQUIRED_CHANGED`, `FIELD_MULTIPLE_CHANGED` | 새 값이 `required: true` 또는 `multiple: false`이면 `CARDINALITY_NARROWED`, 아니면 `CARDINALITY_WIDENED` |
| `FIELD_TYPE_REMOVED` / `FIELD_TYPE_ADDED` | `REMOVAL` / `ADDITION` |
| `CHILDREN_PRESENCE_CHANGED` | 생기면 `ADDITION`, 없어지면 `REMOVAL` |
| `CHILDREN_REQUIRED_CHANGED`, `CHILDREN_MULTIPLE_CHANGED`, `CHILDREN_TYPE_REMOVED`, `CHILDREN_TYPE_ADDED` | field와 같은 규칙 |
| `SUBTYPES_PRESENCE_CHANGED` | 생기면 `ADDITION`, 없어지면 `REMOVAL` |
| `SUBTYPE_REMOVED` / `SUBTYPE_ADDED` | `REMOVAL` / `ADDITION` |

규칙: 없는 `fields`·`children`·`subtypes`는 원소 비교에서 빈 집합이고 유무 변화는 `*_PRESENCE_CHANGED`로 따로 낸다. cardinality는 양쪽에 set이 있을 때만 비교한다. 다른 spelling 사이의 rename은 추정하지 않는다(`NODE_REMOVED` + `NODE_ADDED`). field가 supertype을 가리킬 때 subtype 변화는 그 supertype의 `SUBTYPE_*`로만 보고하고 field의 실효 type 집합으로 펼치지 않는다(coverage `unsupported`: `supertype-expansion`, `runtime-tree`).

**한도**(`DefaultSchemaLimits`, `schema.go` 35–37행): 입력당 문서 16,777,216 bytes, record 200,000(node 항목·field·type 참조), JSON 값 8×record, JSON 중첩 32, 출력 16,777,216 bytes, wall 120초. 문서 한도를 넘은 입력은 hash하지 않는다(`bytes` 0, `sha256` ""). diff는 path를 만들기 전에 크기를 보수적으로 추정(차이마다 160 bytes + 문자열·path 길이)해 `OUTPUT_LIMIT`로 끝낸다. 취소 checkpoint는 JSON 값 4,096개·record 1,024개·그래프 단계/비교 node 1,024개마다 있다. API는 deadline이 있는 context를 요구한다(`DEADLINE_REQUIRED`, `NIL_CONTEXT`).

**identity.** 결과는 입력 원본 bytes의 sha256(schema `node-types-r1`)과 `policy`(`tsgk-schema-policy/r1` 텍스트: 연산·format·comparator·한도의 sha256)를 `identities`에 담는다.

### A1.3 소유 계약 문서

모두 한국어이며 `docs/` 아래에 있다. 이식 시 계약 본문의 출처가 된다.

| 문서 | 절 | 소유하는 것 |
|---|---|---|
| `docs/specs/tree-and-adapter-protocol.md` | "정적 node-types 계약 — S03 구현"(31–65행) | 형식 검사, 판정, 차이 모델 표, 한도·취소 — **형식과 의미의 정본** |
| `docs/specs/public-go-api.md` | "S03 함수와 추가 필드 — 설계 r4"(334–397행) | 공개 타입·상수·시그니처, 오류 규칙 |
| `docs/specs/cli-and-profile.md` | "S03 구현 — schema check, schema diff"(131–141행) | CLI 인자·exit code·`--out` |
| `docs/reports/session-03-schema-contract.md` | 전체(60행) | 구현 범위, 로컬 검사, 26 route 실측(A13), 리뷰 처분, 한계 |
| `docs/specs/platform-support.md` | 14행, "Release v0.3.0의 claim" | VERIFIED 범위(A1.9) |

### A1.4 코드 파일과 크기

| 파일 | 크기 | 내용 |
|---|---:|---|
| `src/kit/schema.go` | 35,879 B / 1,128줄 | 전부 이식 대상. 아래 구간표 참조 |
| `src/cmd/tsgk/main.go` | 31,843 B (전체) | 이 중 schema에 쓰이는 부분은 아래 A1.6 |
| `src/kit/schema_test.go` | 33,209 B / 656줄 | 단위·한도·변이 시험(A1.7) |
| `src/cmd/tsgk/schema_test.go` | 7,022 B / 156줄 | CLI 시험 |
| `src/kit/example_test.go` | 1,849 B / 41줄 | `ExampleSchemaDiff` 하나 |

`schema.go` 구간(합 35,879 B):

| 줄 | 바이트 | 내용 |
|---|---:|---|
| 1–155 | 5,694 | import, 상수, `SchemaLimits`, 요청·결과 타입, risk 상수, `diffOrder` |
| 156–273 | 5,005 | `SchemaCheck`, `SchemaDiff`, policy·summary·identities·coverage |
| 274–293 | 558 | `schemaRun`(공통 guard와 실패 기록) |
| 294–403 | 3,019 | `collector`(finding 수집·한도·path 절단), `ptrClip` |
| 404–838 | 12,188 | 내부 모델, `parseSchema`/`parseNode`/`parseSet`/`parseRefs`, supertype 순환, counts |
| 839–1128 | 9,415 | canonical JSON, `diffSchemas`, `diffSet`, `diffMembers` |

### A1.5 `src/kit` 안의 의존(공용 헬퍼)

`schema.go`가 같은 package의 다른 파일에서 쓰는 것이다. 표의 "이식 처리"가 최소 집합이다.

| 의존 | 위치 | 크기 | 이식 처리 |
|---|---|---:|---|
| `Report`, `IdentityRef`, `Finding`, `Coverage` 타입 | `src/kit/kit.go` 98–130행 | 1,122 B | 유지(JSON 모양이 결과 계약) 또는 go-treesitter 자체 타입으로 교체 |
| `newReport`, `failReport`, `messageFor` | `src/kit/kit.go` 171–205행 | 1,587 B | 유지. **`messageFor`와 `schema.go`의 finding message는 한국어다**(아래 A1.10) |
| 보고서·상태 상수(`ReportSchema`, `Status*`, `Mode*`, `AssessBlocked`, `AssessNotAssessed`)와 `Kind*` 오류 종류 상수 | `src/kit/kit.go` 17–44행 | 573 B | 유지 |
| `Error`, `fail` | `src/kit/kit.go` 46–67행 | 466 B | 유지 |
| `AssessPass`, `AssessFail` | `src/kit/verify.go` 17–21행 | 83 B | 상수 두 개만 가져온다(`verify.go` 나머지 18.5 KB는 버린다) |
| `run`, `startRun`, `(*run).check`, `errWall`, `sealOutput` | `src/kit/kit.go` 208–219행(`run` 구조체), 256행(`errWall`), 280–347행(`startRun`·`check`·`sealOutput`) | 280–347행 2,015 B | **축소해서 이식.** `run`은 `caller`·`wall`·`cancel` 세 필드만 필요하다. `runOptions`·`eligibility`·`testHookWall`·`forbiddenFiles` 등 나머지 필드와 `startRunWithOptions`의 옵션 분기는 버린다 |
| strict JSON decoder: `jv`, `ptr`/`ptrLen`/`cptr`, `valueBudget`, `decodeBounded`, `readJSON`, `jsonError`, `jsonMaxDepth`, `pointerEscaper` | `src/kit/strict.go` 1–198행 | 5,428 B | 이식. 표준 `encoding/json/jsontext`에 의존한다(A1.10). `valueBudget.charge`/`reserve` 필드는 schema가 쓰지 않으므로 버려도 된다 |
| `typed`, `jsonUint`, `maxJSONInteger`, `decodeStrict` | `src/kit/strict.go` 76–81행, 199–끝 | 약 2.7 KB | 버린다(profile·expected 문서용) |
| `MaxDocumentBytes` | `src/kit/profile.go` 16행 | 상수 하나 | CLI의 `readDocument`와 시험이 참조. 상수로 복사 |
| `clipPath`, `maxFindingPath`, `ptrClip`(`*jv`) | `src/kit/schema.go` 안 | — | 이미 `schema.go`에 있다(strict.go가 거꾸로 `clipPath`·`maxFindingPath`를 쓴다 — 두 파일을 함께 옮겨야 한다) |

즉 **비테스트 이식량은 `schema.go` 35,879 + `strict.go` 5,428 + `kit.go` 부분(573 + 466 + 1,122 + 1,587 + 2,015 = 5,763) + `verify.go` 83 = 47,153 B(약 47 KB)**이며, `run` 축소와 decoder 필드 정리로 더 줄어든다. 나머지 `src/kit`(약 300개 파일)은 schema와 무관하다.

### A1.6 이식 최소 집합과 버릴 것

**가져올 것(라이브러리 핵심)**
1. `src/kit/schema.go` 전체.
2. A1.5의 공용 헬퍼 위 행들(`kit.go` 일부, `strict.go` 1–198행, `verify.go` 상수 두 개).

**CLI를 함께 옮길 때(`src/cmd/tsgk/main.go`의 일부)**

| 부분 | 줄 | 크기 |
|---|---|---:|
| exit 상수(`exitOK 0`, `exitFail 1`, `exitUsage 2`, `exitBlocked 3`, `exitIO 4`, `exitCanceled 130`) | 26–32 | 118 B |
| `runSchema`(인자 파싱·읽기·호출) | 239–290 | 1,917 B |
| `assessExit` | 514–522 | 168 B |
| `exitFor` | 740–750 | 232 B |
| `readDocument` | 718–724 | 185 B |
| `finish`/`finishWithRoots`(JSON 한 줄 출력, exit 매핑, `--out` 게시) | 390–491 | **qualification·replay 분기와 `--out` 게시가 섞여 있어 그대로 옮기지 않는다.** schema에 필요한 것은 "결과를 JSON 한 줄로 stdout, 오류면 stderr에 `tsgk: KIND: CODE path`, exit는 아래 표" 정도다 |

**버릴 것**
- `--out`(no-clobber 게시): `src/cmd/tsgk/paths.go`, `publication_{windows,linux,other}.go`와 그 시험(glob 합 39,742 B). shell 리다이렉트로 대신한다. 이 코드가 schema 결과 게시의 유일한 OS 분기다.
- `SchemaPolicy`의 `tsgk-schema-policy/r1` identity 계산(`ref()`)은 go-treesitter가 policy identity를 쓰지 않으면 생략 가능하다. 단 결과 byte 동일성 시험을 옮긴다면 유지.
- kit의 나머지 전부: `inspect`·`identity`·`verify`·`corpus`·`reproduce`·`incremental`·`oracle`·`replay`·`evidence`·`qualify`와 `src/internal/*`.

### A1.7 함께 옮길 시험과 fixture

**fixture는 파일이 없다.** 모든 입력이 Go 문자열 literal이다: `schemaBase`·`schemaCandidate`(`schema_test.go` 13–40행)와 기대 목록 `schemaForward`·`schemaReverse`(19행짜리, 비교기를 보지 않고 심은 변경에서 독립 작성), CLI의 `cliBase`·`cliCandidate`(`cmd/tsgk/schema_test.go` 15–18행).

| 시험 | 위치 | 이식 | 메모 |
|---|---|---|---|
| `TestSchemaAcceptsEmptyAliasType` | `src/kit/schema_test.go` 163 | 예 | `alias(rule, "")` → `{"type": ""}` 수락 |
| `TestSchemaEquivalence` | 175 | 예 | key·항목·허용 type 순서만 다른 같은 schema |
| `TestSchemaDiffKinds` | 209 | 예 | 모든 code·risk 19행, before/after/path 대조 |
| `TestSchemaReverse` | 255 | 예 | 방향 반전, 같은 이름 두 입력의 role·hash 분리 |
| `TestSchemaInvalid` | 290 | 예 | 위반 사례 묶음(보고서 기준 39 사례) |
| `TestSchemaLongNames` | 393 | 예(주의) | 1 MiB 이름 아래 할당 256 MiB 미만을 `runtime.ReadMemStats`로 확인 — Go runtime/GC 버전에 민감하다 |
| `TestSchemaFindingBounds` | 455 | 예 | finding 1,000건·path 1,024 bytes |
| `TestSchemaLimits` | 506 | 예 | 한도 at/one-over, 깊이, 20,000단 supertype 사슬, 취소 checkpoint, deadline 없는 context, `Wall = 0`(LIMITS_INVALID). 1 ns wall timer에 의존하지 않는다 |
| `TestSchemaScopeOrderedTree` | 635 | **절반만** | 앞부분은 kit의 ordered-tree 문서와 `decodeStrict`를 시험(버림). 뒷부분 `MEMBER_DUPLICATE` 단언만 유지 |
| `TestSchemaCLI` | `src/cmd/tsgk/schema_test.go` 46 | 예(CLI를 옮길 때) | 7개 exit 사례, CLI 출력 = API JSON, 이름·hash 분리, `--out` 시험은 `--out`을 버리면 함께 뺀다. `cli(t, ctx, args...)` 헬퍼(`main_test.go` 46행)가 필요 |
| `ExampleSchemaDiff` | `src/kit/example_test.go` | 예 | 출력 3줄을 고정하는 실행 예시 |
| `BenchmarkSchema` + `benchmarkSchema` | `src/kit/benchmark_test.go` 126–~170행(약 1.5 KB) | 선택 | |
| `TestExternalConsumerSchema` | `src/cmd/tsgk/schema_test.go` 132 | **버린다** | `buildCLI`/`buildConsumer`/`runBin`, `src/testdata/consumer/main.go`(3,343 B), Windows `SystemRoot` 환경에 묶인 harness |
| `TestFactMappingAgainstSchemas` | `src/kit/factmap_test.go` | 버린다 | `fact-mapping.json`과 로컬 upstream schema 사본 의존, hosted CI에서 skip |
| `TestGrammarUpdateWorkflow` | `src/cmd/tsgk/qualify_test.go` 87행 부근 | 버린다 | `verify` 명령과 결합. `schema diff` 부분만 CLI 시험에 흡수 가능 |

시험이 쓰는 같은 package 헬퍼: `testCtx`(`identity_test.go` 35행), `kindOf`(42행), `hasFinding`(309행), `hasCode`(318행) — 합 약 0.6 KB. 이 넷과 `decodeStrict`·`MaxDocumentBytes` 참조를 함께 옮기거나 시험에서 대체한다.

**실제 grammar에 대한 근거(fixture 아님).** session 03은 26 route의 upstream `node-types.json`(로컬 추적 안 하는 사본)에 `check`를 돌려 25 PASS(python은 `REFERENCE_UNDECLARED` warning 1건)와 1 FAIL(swift: `("?", false)`·`("??", false)`가 각각 두 번 선언돼 `NODE_DUPLICATE`; kit 결함이 아니라 upstream generator 산출물의 형식 위반으로 기록)을 얻었다. go-treesitter의 일곱 grammar(json·go·python·typescript·tsx·c-sharp·mssql)와 **같은 commit**의 사본이 이 25 PASS 안에 있다(commit 대조: go-treesitter `tools/gen/pins.json`의 go `2346a3ab…`, python `26855eab…`, typescript/tsx `75b3874e…`, c-sharp `9150f7d5…`, json `254c42a6…`, mssql `8620fbcf…`는 kit가 S01에 결속한 26 route source 사본(추적하지 않는 `_ref/campaign-01-s01/sources`의 디렉터리 이름에 commit이 있다)과 mssql의 경우 `docs/provenance/upstream-sources.md`의 등록과 같다). 이식 후 go-treesitter 자체 pin의 `node-types.json` 일곱 개에 `check`가 PASS인지를 골든 시험으로 두는 것이 가장 좋은 회귀다(이 문서 작성 중에는 실행하지 않았다).

### A1.8 공개 API와 CLI 표면

**공개 API**(`kit` package, `docs/specs/public-go-api.md` 334–397행):

```go
func SchemaCheck(ctx context.Context, request SchemaCheckRequest) (SchemaCheckResult, error)
func SchemaDiff(ctx context.Context, request SchemaDiffRequest) (SchemaDiffResult, error)
func DefaultSchemaLimits() SchemaLimits // document 16777216, records 200000, output 16777216, wall 120s

const SchemaFormat = "node-types-r1"; SchemaComparator = "node-types-diff-r1"; SchemaPolicyName = "tsgk-schema-policy/r1"
const RiskAddition, RiskRemoval, RiskIdentity, RiskClassification, RiskNarrowed, RiskWidened // 위 6개 문자열

type SchemaInput struct{ Name string; Data []byte }
type SchemaLimits struct{ DocumentBytes, Records, OutputBytes uint64; Wall time.Duration }
type SchemaCheckRequest struct{ Input SchemaInput; Limits SchemaLimits }
type SchemaDiffRequest struct{ Baseline, Candidate SchemaInput; Limits SchemaLimits }
type SchemaCheckResult struct{ Report; Policy SchemaPolicy; Input SchemaSummary }
type SchemaDiffResult struct{ Report; Policy SchemaPolicy; Baseline, Candidate SchemaSummary; Differences []SchemaDifference }
type SchemaPolicy / SchemaSummary / SchemaCounts / NodeRef / SchemaDifference // JSON key는 snake_case
```

오류 규칙: 형식 위반은 `SchemaCheck`의 `error`가 아니라 `error == nil`인 결과의 `FAIL`/`BLOCKED`다. `SchemaDiff`는 잘못된 입력을 `*Error`(`INVALID_INPUT`/`SCHEMA_INVALID`, `UNSUPPORTED`/`SCHEMA_KEY_UNSUPPORTED`)로 돌려주고 그 입력의 finding을 결과에 남긴다. 한도·취소·deadline 없는 context·잘못된 limits(`LIMITS_INVALID`)도 `*Error`이며 오류와 함께 반환된 결과는 `COMPLETED`/`PASS`가 아니고 `Counts`는 nil, `Differences`는 비어 있다. 결과는 호출별 새 할당이고 서로 다른 입력의 동시 호출은 공유 상태가 없다.

**CLI**(`tsgk`, `src/cmd/tsgk/main.go` 239–290행, `docs/specs/cli-and-profile.md` 131–141행):

```text
tsgk schema check --input FILE [--out PATH]
tsgk schema diff  --before FILE --after FILE [--out PATH]
```

- 결과는 stdout에 JSON 한 줄. `name`은 파일의 base name만(디렉터리·절대 경로 없음). 같은 base name의 두 입력은 `role`과 sha256으로 구분된다.
- `--out`은 새 파일에만 쓰고(기존 대상 거부, hard link 게시) 완전한 결과만 받는다. 오류 report는 stdout 전용이다.
- 필수 인자가 없거나 하위 명령이 `check`·`diff`가 아니면 파일을 읽기 전에 exit 2.

| 상황 | exit |
|---|---:|
| check PASS / diff 차이 없음 | 0 |
| check FAIL / diff 차이 있음 | 1 |
| 사용법 오류, 또는 diff 입력이 형식 위반(`SCHEMA_INVALID`) | 2 |
| check BLOCKED, diff 입력에 해석하지 않는 key(`SCHEMA_KEY_UNSUPPORTED`), 한도(`RESOURCE_LIMIT`) | 3 |
| 읽을 수 없는 입력(`SCHEMA_UNREADABLE`), 출력 실패 | 4 |
| 취소 | 130 |

### A1.9 go-treesitter 파이프라인에 두는 제안

ADR-0020 결정 2는 "a grammar change ships with a static node-type diff"를 요구하지만 `tools/gen`에는 `node-types`를 다루는 코드가 없다(확인: `tools/gen/*.go`, `regen.sh`, `docs/design/*.md`에 "node-types" 언급 없음). 아래는 ADR이 정한 범위 안의 제안이며 go-treesitter의 내부 설계를 정하지 않는다.

**재료는 이미 있다.** `tools/gen/pins.json`의 grammar 항목은 `grammarDir`(`src`, `typescript/src`, `tsx/src`)와 `dirs`를 가지며 `treeSHA256`이 그 트리 전체를 묶는다. `node-types.json`은 `<extractDir>/<grammarDir>/node-types.json`에 있고 별도 hash(`files`는 `parser.c`만)는 없지만 `treeSHA256`이 덮는다. `regen.sh`는 이미 pin이 가리키는 archive를 sha256 검증 후 받아 푼다. 따라서 새 download 없이 입력을 얻는다.

**둘 곳.** `tools/` 아래 독립 module(`tools/native-ref`, `tools/gen`처럼 자체 `go.mod`)로 둔다. 예: `tools/node-types`(ported 핵심 + `check`/`diff` 진입점). 입력은 두 파일 경로뿐이라 `tools/gen`의 pin 해석 코드와 결합하지 않는다. 추출 경로를 만드는 일은 `regen.sh`/workflow 쪽 몫이다. 제품 module은 이것을 import하지 않는다(ADR-0020 결정 6).

**언제 돌릴지.**

1. **`check`: 매 regen에서.** archive 검증·추출 뒤, 생성 전에 pin된 모든 grammar의 `node-types.json`을 검사한다. `PASS`(warning 허용)가 아니면 그 pin은 검토 없이 채택하지 않는다. `BLOCKED`(해석하지 않는 key)는 generator가 새 key를 내기 시작했다는 신호로 의도한 fail-closed다.
2. **`diff`: grammar pin이 바뀌는 PR에서.** `tools/gen/pins.json`의 grammar `commit`이 base와 다를 때만, baseline = base ref의 pin이 가리키는 archive의 `node-types.json`(같은 `regen.sh` 경로로 옛 archive를 sha256 검증 후 추출), candidate = 새 pin의 것. 결과 JSON을 CI artifact(또는 PR 요약)로 남기고, 그 존재를 ADR-0020 결정 2의 "static node-type diff를 함께 낸다"의 증거로 삼는다. exit 1(차이 있음)은 **검토 대상일 뿐 실패가 아니다** — CI가 막아야 할 것은 입력 오류(exit 2·3·4)와 보고서 누락이다. 이 게이트 모양은 go-treesitter가 정할 일이다.
3. **(선택) 선행 경고.** `upstream-watch.yml`은 이미 grammar head가 pin보다 앞섰음을 "나열만" 한다. 그 head의 `node-types.json`과 현재 pin의 diff를 같은 방식으로 요약하면 pin 갱신 전에 검토 범위를 알 수 있다. pin은 명시 권한 없이는 움직이지 않는다는 기존 규칙을 바꾸지 않는다.

**baseline 보관.** `node-types.json` 사본을 repo에 커밋하지 않는 것을 권한다(upstream 사본이고 C# 하나가 133,257 B). 옛 archive를 pin에서 다시 받는 방식이면 사본 관리가 없다. 필요하면 grammar별 "마지막으로 검토한 diff 보고서"만 문서로 남긴다.

### A1.10 이식 시 주의할 점(코드에서 확인한 것)

- **메시지가 한국어다.** `schema.go`의 finding `Message`와 `kit.go`의 `messageFor`는 한국어 문자열이다. 영어 문서 정책이면 번역하되 `code`·`severity`·`path`는 기계 값이므로 바꾸지 않는다. 시험은 message를 단언하지 않고 code·path를 단언한다(확인: `hasFinding(fs, code, path)`).
- **`encoding/json/jsontext` 의존.** strict decoder는 이 표준 package를 쓴다. kit `go.mod`는 `go 1.27.4`이고 로컬 도구(1.27.4)에서 표준 library로 존재함을 확인했다. go-treesitter의 `tools/*/go.mod`는 `go 1.26.0`을 선언하고 CI는 `1.27.x`를 쓴다. **1.26.0에서 이 package가 실험 플래그 없이 가능한지는 확인하지 못했다.** 새 module의 `go` 지시어를 1.27 이상으로 두거나 중복 key 거부를 직접 구현해야 하는지 이식 전에 확인한다.
- **fail-closed key 정책.** 해석하지 않는 key는 `BLOCKED`/`SCHEMA_KEY_UNSUPPORTED`다. 알 수 없는 key를 무시하는 것보다 안전하지만 generator 버전이 올라 새 key가 생기면 `diff`가 멈춘다. 이때 `node-types-r2`를 정하는 별도 결정이 필요하다.
- **중복 identity 입력은 diff할 수 없다.** swift는 `NODE_DUPLICATE`라 `diff` 입력이 될 수 없었다(exit 2). go-treesitter의 일곱 grammar에는 해당 사례가 없었다(위 A1.7). 새 grammar를 추가할 때 먼저 `check`를 돌린다.
- **결과 envelope가 kit의 것이다.** `tsgk-report/r1`(`execution_status`, `evidence_mode`, `assessment`, `identities`, `findings`, `coverage`)과 E0 상태 값 규칙을 그대로 쓴다. go-treesitter가 더 단순한 결과를 원하면 `Report`를 교체하되 `FAIL`/`BLOCKED`와 오류의 구분은 유지해야 같은 시험이 통한다.
- **supertype은 펼치지 않는다.** field가 supertype을 가리킬 때 실효 type 집합 변화는 보고되지 않는다. C# 같은 supertype이 많은 grammar에서 검토자가 알아야 하는 한계다.

### A1.11 v0.3.0에서 VERIFIED였던 것과 Windows·Linux 의존

- **VERIFIED 범위**(`docs/specs/platform-support.md` 14행·"Release v0.3.0의 claim", `docs/reports/issue-220-v030-release.md`): offline `inspect`·`identity`·`verify`·`schema check/diff`와 공개 API(`src/kit`). 검증 host는 Windows·Linux CI이고 **macOS는 v0.2.0 시점 근거만 있으며 v0.3.0에서 재검증하지 않았다(NOT_RUN, 제외를 PASS로 세지 않는다)**. 이 VERIFIED는 임의 입력에 대한 언어 정확성이 아니라 "선언된 형식·차이 모델·결정성이 시험으로 확인됨"이다.
- **코어(`schema.go`·`strict.go`·`kit.go` 부분)는 OS 분기가 없다.** import는 표준 library뿐이고 `os`·`syscall`을 쓰지 않는다(확인: `schema.go` import 목록). Windows/Linux 차이는 코어 밖에 있다.
- **OS에 묶인 주변부:**
  - CLI의 `--out` 게시: `publication_windows.go`(4,053 B)·`publication_linux.go`(1,934 B)·`publication_other.go`(1,259 B)와 `paths*.go`. 이식에서 버리는 부분(A1.6).
  - `TestExternalConsumerSchema`: Windows `SystemRoot` 환경변수와 빈 `PATH`로 외부 consumer binary를 실행(A1.7에서 버림).
  - `TestSchemaLongNames`: 할당량 임계(256 MiB)가 OS가 아니라 Go runtime/GC에 민감하다.
- **session 03의 실측은 Windows amd64, Go 1.27.1**이었다(26 route 실행·mutant 37종·외부 consumer). v0.3.0의 Windows·Linux CI 결과는 위 문서 기록이며 이 작업에서 다시 실행하지 않았다.

---

## A2. C# 발견 — 지식으로 이월할 것

### A2.0 범위와 신뢰 수준

**무엇을 비교한 결과인가.** G1은 C# 명세([dotnet/csharpstandard](https://github.com/dotnet/csharpstandard) alpha-v13, commit `79ad4e90ef63…`)와 Roslyn(SDK 10.0.401의 `Microsoft.CodeAnalysis.CSharp.dll` 5.9.0.0, sha256 `12b0428203fa…`; 관측 asset `src/contracts/csharp-g1-roslyn-observed-r3.json`)이 기대하는 구조를, 등록된 C# route의 tree-sitter-c-sharp **native 빌드**(Windows amd64, gcc 16.2.0)의 parse 결과와 대조한 것이다.

**그 native 빌드는 upstream 원본이 아니다.** 같은 upstream commit `9150f7d56bb47f1a809fa23623f1ba1413e93fa9`(go-treesitter의 `tools/gen/pins.json` c-sharp pin과 같다)에 kit의 patch chain `src/dev/c2-patches/csharp.json`(211,964 B; `grammar.js` 66연산 90치환 + `src/scanner.c` 42연산 42치환 = 108연산 132치환; 결함 id 24개)을 적용해 생성한 것이다. 증거: executed asset `src/contracts/csharp-g1-executed-b01b02-r2.json`의 `native.grammar`가 `src/scanner.c` 61,493 bytes(sha256 `8d00d37d…`)와 `parser.c` 53,114,064 bytes(sha256 `1fa40d53…`)를 기록하는데, upstream `src/scanner.c`는 23,064 bytes(sha256 `2ee1241a…`)이고 upstream `parser.c`는 32,021,728 bytes다. 매핑표 r2가 pin하는 `grammar.js`(sha256 `63d357e0…`, 61,851 B)와 `node-types.json`(sha256 `54bf9fa9…`, 133,257 B)은 **upstream 원본**의 것이다(upstream 사본의 sha256을 직접 계산해 일치를 확인했다). 그래서 G1의 "grammar 근거"는 원본 `grammar.js`에서, "native 일치"는 patch된 빌드에서 나왔다. (참고: #447 코멘트는 "patch 8단계, 치환 122개"라 적었으나 저장소의 `csharp.json`은 108연산·132치환이다. 차이의 원인은 확인하지 못했다.)

**이 문서의 표지.**

| 표지 | 뜻 |
|---|---|
| **[관측]** | G1 asset에 기록된 native 관측. **patch된 빌드에서** 나온 값이다 |
| **[정적]** | upstream 원본 `grammar.js`/`scanner.c`/`node-types.json`을 직접 읽어 확인한 것(줄 번호는 upstream commit `9150f7d5` 기준) |
| **[패치무관]** | `csharp.json`의 108연산 중 어느 것도 해당 규칙 이름을 바꾸지 않음을 문자열 검색으로 확인(규칙 텍스트가 같다는 뜻이며 parse table·scanner까지 같다는 증명은 아니다) |
| **[추론]** | 위 근거에서 이끌었으나 upstream을 실제로 parse해 보지 않은 것 |

**upstream 원본을 parse해서 확인한 것은 하나도 없다.** "upstream에도 적용된다"는 문장은 [정적]·[패치무관]까지만 근거가 있다.

**G1 규모(참고).** B01·B02 대안 426개는 r2에서 426/426 `MATCH`(봉인 시 일치 411, 사후 정정 15; 정정분은 독립 일치로 세지 않음), B03·B04 대안 388개는 295개가 EXPECTED(전부 `MATCH`, 그중 4개는 사후 정정), 93개가 BLOCKED다(PR #443 상태). 원장 r2 기준 `EXECUTED` 721 + `BLOCKED` 172 + `NO_EXPECTED` 638(B05·V\* 행은 시작하지 않았다). **G1은 grammar 결함을 따로 확정하지 못했다**: B01·B02는 "`GRAMMAR_DEFECT` 후보는 없다"(native가 오류 없이 파싱했고 네 불일치는 기대값·매핑표 오류), B03·B04의 불일치 5건은 기대값 오류 4건과 설계 차이 1건이다. 아래 종류 1은 G1 asset의 `NOT_IN_PINNED_GRAMMAR` 계열과 원본 grammar 원문에서 나온 **공백(gap)** 이다.

PR #443(G1-3b)은 병합되지 않았다. B03·B04 asset은 브랜치 `origin/feat/233-g1-3b`(commit `440f4dac8cca6e2231b83de096b1db7bd36e1c05`)에만 있다: `https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/440f4dac8cca6e2231b83de096b1db7bd36e1c05/<경로>`. **이 브랜치를 지우면 B03·B04 증거 링크가 사라진다.** PR을 닫을 때 브랜치(또는 tag)를 보존하라. 근거 코멘트: [#233 6869108075](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6869108075)(PR3 native 대조와 결정), [#233 6873534481](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/233#issuecomment-6873534481)(2차 승인 묶음), [#447 교차 검증](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/447#issuecomment-6874888430).

### A2.1 요약표

| # | 발견 | 종류 | upstream 원본에 적용? | 표지 |
|---|---|---|---|---|
| 1-1 | `where T : default`(C# 9 default 제약)에 대응하는 규칙이 없다 | 1 upstream 공백 | **예** | 정적, 패치무관, 추론 |
| 1-2 | `allows ref struct`(C# 13)가 없다 | 1 upstream 공백(patch가 채움) | 예(패치 빌드는 지원) | 정적, 관측 |
| 1-3 | C# 14 file-based 지시문(`#:sdk` 등)이 없다 | 1 upstream 공백(patch가 채움) | 예(패치 빌드는 지원) | 정적, 관측 |
| 2-1 | G1의 native 일치 721개는 patch 빌드에서 쟀다 | 2 patch | **원본으로 이월 안 됨** | 관측 |
| 2-2 | patch가 추가한 node 4개, 바꾼 규칙(`switch_section`, `var_pattern`, 지시문 전부) | 2 patch | 원본에는 없음 | 정적, 관측 |
| 2-3 | patch 체인의 회귀 3건(directive `//` 거부, `#line` filename 인접성, pragma verbatim quote) | 2 patch 회귀 | **원본에는 해당 없음**(patch 코드의 결함) | 보고서 |
| 3-1 | `#define`/`#undef` 심볼이 불투명 `preproc_arg` | 3 설계 차이 | 예(범위 의미는 다름) | 정적, 관측 |
| 3-2 | 보간 문자열 시작이 두 node로 갈라진다 | 3 설계 차이 | 예(규칙 같음, scanner는 patch됨) | 정적, 관측 |
| 3-3 | `interpolation`의 경계와 `,5` 정렬 절 | 3 설계 차이 | 예 | 정적, 관측 |
| 3-4 | 매개변수 `ref`/`out`/`in`이 `modifier` leaf, `ref`는 경합 | 3 설계 차이 | 예 | 정적, 패치무관, 관측 |
| 3-5 | `var`는 `implicit_type`(+ 익명 자식 `"var"`) | 3 설계 차이 | 예 | 정적, 패치무관, 관측 |
| 3-6 | `var ((g, h), i) = …`의 안쪽이 `tuple_pattern` | 3 설계 차이 | 예 | 정적, 관측 |
| 3-7 | 그 밖의 명세↔tree 이름·범위 차이(제약 목록, `join_into_clause`, `with_initializer`, `argument_list`, nullable `?`) | 3 설계 차이 | 예 | 정적, 패치무관, 관측 |
| 3-8 | 명세 구문 중 자기 node가 없는 것들(BLOCKED 172의 대부분) | 3 설계 차이 | 예 | 관측(분류) |

### A2.2 종류 1 — upstream 원본 grammar의 공백

세 건 모두 **"명세·Roslyn에 있는 구문이 upstream `grammar.js` 원문에 없다"** 는 정적 사실이다. 결함인지 의도한 미지원인지는 upstream 소유자가 정한다. upstream 기여 후보로 쓸 수 있다.

#### 1-1. `where T : default` — default 제약

- **명세/Roslyn 기대.** C# 9의 `default` 제약: 명세 production `primary_constraint: 'default'`(원장 대안 `csharp-B02.a079`, 근거 링크: `src/contracts/feature-alternatives.json`의 `ref` `https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/classes#1525-type-parameter-constraints`). Roslyn 종류 `DefaultConstraint`.
- **upstream 산출물.** [정적] upstream `grammar.js` 460–467행의 `type_parameter_constraint`는 `seq('class', optional('?'))`, `'struct'`, `'notnull'`, `'unmanaged'`, `$.constructor_constraint`, `field('type', $.type)`만 고른다. `default`가 들어갈 자리가 없고, `type` 규칙(843–854행)은 `default`를 받지 않으며 `_reserved_identifier`(2125행~)에도 `'default'`가 없다. [패치무관] `csharp.json` 108연산 중 `type_parameter_constraint`를 건드리는 것이 없고, `default` 제약을 추가하는 연산도 없다(`allows ref struct`만 추가한다: 1-2).
- **kit의 처분.** 매핑표 r2는 Roslyn `DefaultConstraint`를 `map: none`, `reason: NOT_IN_PINNED_GRAMMAR`로 두고(`src/contracts/csharp-g1-kind-map-r2.json`), 원장은 `csharp-B02.a079`를 `BLOCKED NOT_IN_PINNED_GRAMMAR`로 둔다(`docs/specs/csharp-g1-expected.md` 54행: "`default` 제약: 고정 grammar에 node type이 없다"). 이것이 G1이 낸 **유일한** `NOT_IN_PINNED_GRAMMAR` 대안이다(B01–B04 BLOCKED 172개 중 1개).
- **증거.** 등록 사례 `alt-csharp-B02-04`(`src/testdata/native/routes/csharp.json`)의 source 끝부분:
  ```csharp
  class E : D
  {
      public override void N<T>() where T : default { }
  }
  ```
  이 사례의 step 0 기대값은 `syntax: NO_ERROR`이고 anchor에 `type_parameter_constraints_clause` 텍스트 `where T : default`가 있다. 이 기대값은 session 05에서 patch 빌드로 통과한 것으로 기록되어 있다(`docs/reports/issue-233-csharp-lexical-recovery.md`의 "S05 C# 361/361 PASS"). **이 문서 작성 중에 다시 실행하지는 않았다.**
- **추론(미확인).** 규칙에 `default` alternative가 없는데도 오류가 없다면, `default`가 키워드가 아니라 식별자 token으로 읽혀 `type_parameter_constraint`의 `type` field로 들어갔을 가능성이 크다(tree-sitter의 keyword extraction은 현재 parse 상태에서 키워드가 유효하지 않으면 word token `identifier`를 그대로 쓴다). 그렇다면 **오류 없는 조용한 오분류**다(`default`가 type 이름 `identifier`가 됨). 추적되는 asset에는 이 위치의 node type이 기록돼 있지 않다. go-treesitter에서 위 source를 parse해 `where T : default` 부분의 node를 덤프하면 한 번에 확정된다.
- **적용 범위.** 원본 grammar에 같은 규칙 텍스트가 있으므로(정적·패치무관) 원본에도 적용될 가능성이 높다. 원본의 실제 tree는 미관측.

#### 1-2. `allows ref struct` — C# 13 반경 제약

- **명세/Roslyn 기대.** `where T : allows ref struct`. Roslyn 종류 `AllowsConstraintClause`, `AllowsKeyword`, `RefStructConstraint`(C# 13).
- **upstream 산출물.** [정적] 위 `type_parameter_constraint`에 `allows` 경로가 없고, `_reserved_identifier`에 `'allows'`가 없으며, `src/node-types.json`(133,257 B)에 해당 node가 없다. [관측] 매핑표 r2의 세 종류가 모두 `map: none, NOT_IN_PINNED_GRAMMAR`이다.
- **patch가 한 일.** `csharp.json`의 grammar 연산이 `'unmanaged'` 뒤에 `seq('allows', 'ref', 'struct')`를 더하고 `_reserved_identifier`에 `'allows'`를 더한다. 등록 사례 `alt-csharp-V13-01`(source에 `allows ref struct`)은 patch 빌드에서 `NO_ERROR`다.
- **원본의 결과는 미확인.** 규칙이 없으므로 `ref struct`가 type 위치에서 오류가 될 가능성이 크나(`struct`는 type이 아님) 관측하지 않았다.

#### 1-3. C# 14 file-based app 지시문(`#:sdk`, `#:package`, `#:property`, `#:project`, `#:include`)

- **명세/Roslyn 기대.** C# 14 file-based 프로그램의 `#:` 지시문. Roslyn은 file-based 모드가 아닌 일반 parse에서 이를 거부한다(CS9298, CS9314).
- **upstream 산출물.** [정적] upstream은 `shebang_directive: token(seq('#!', /.*/))`(2288행)만 있고 `#:` 지시문 규칙이 없으며, `node-types.json`에 `file_based_app_directive`가 없다(`grep` 결과 0건). `preproc_directive`의 정규식 `/#[ \t]*[a-zA-Z0-9]\w*/`(2172행)는 `#:`와 맞지 않는다.
- **patch의 결과.** 매핑표 r2의 `grammar.additions`가 patch가 추가한 node를 기록한다: `file_based_app_directive`, `if_header`, `preproc_skipped_section`, `preproc_try_statement`(네 개 모두 upstream `node-types.json`에 없음을 확인). [관측] executed asset의 source `alt-csharp-V14c-01#0`·`alt-csharp-V14c-02#0`는 `roslyn_accepted: false`인데 native는 오류 없이(`native_error: false`) 파싱한다. 즉 patch 빌드는 Roslyn보다 **더 넓게 수락**한다. source 앞부분:
  ```text
  #!/usr/bin/env dotnet
  #:sdk Microsoft.NET.Sdk.Web
  #:package Humanizer@2.14.1
  ```
- **원본의 결과는 미확인**(규칙이 없으므로 오류일 가능성이 크다).

### A2.3 종류 2 — kit의 patch(C2·remedy 체인)가 바꾼 것

**여기 있는 것은 patch된 빌드에만 해당한다.** go-treesitter가 쓰는 원본에는 적용되지 않거나 의미가 다르다.

#### 2-1. G1 native 일치는 patch 빌드의 것이다

- B01·B02의 426/426과 B03·B04의 295/295 `MATCH`(총 `EXECUTED` 721)는 모두 위 A2.0의 patch 빌드 parse에 대한 것이다. **원본 `parser.c`가 같은 결과를 낸다는 증거가 아니다.** 원본에서 확인하려면 같은 source(`alt-csharp-*` 사례)를 원본으로 parse해 asset의 `found`(type과 범위)와 대조해야 한다. 대조 입력으로는 executed asset의 `alternatives[].witnesses[]`(`source`, `start`, `end`, `found`)와 source 본문(`src/testdata/native/*/csharp.json`)이 그대로 쓸 수 있다. 증거 자원은 아래 "재사용 방법"에 정리했다.
- 근거 asset: B01·B02 `src/contracts/csharp-g1-executed-b01b02-r2.json`(210,330 B), B03·B04 PR #443의 `src/contracts/csharp-g1-executed-b03b04-r2.json`(137,944 B).

#### 2-2. patch가 바꾼 구조(원본과 다른 점)

| 변경 | 증거 | 의미 |
|---|---|---|
| node 4개 추가: `file_based_app_directive`, `if_header`, `preproc_skipped_section`, `preproc_try_statement` | 매핑표 r2 `grammar.additions`; 원본 `node-types.json`에 0건 | 원본 tree에는 이 type이 없다. G1 기대값 중 이 node를 가리키는 것이 있다면 원본에서는 성립하지 않는다 |
| `switch_section`이 여러 case label을 하나의 section으로 묶음 | `csharp.json` grammar 연산 #10: `repeat1(seq(choice(case…, 'default'), ':'))` + `repeat1($.statement)` | 원본(1086–1100행)은 label 하나당 section 하나다. `case 1: case 2: …`의 tree 모양이 다르다 |
| `var_pattern`에 `prec.dynamic(2, …)` | `csharp.json` grammar 연산 #18. 원본 1285행은 `seq('var', $._variable_designation)` | `var` 패턴과 `implicit_type`/식별자의 동적 우선순위가 다르다. 원본 동작은 미측정 |
| 지시문 처리 전체(`preproc_*` external token 다수, scanner 직접 판독, `preproc_if` 계열의 새 문맥) | `csharp.json`의 `issue160`~`issue184`, `issue233` 계열과 scanner 연산 42개. 원본은 지시문이 `extras`의 정규식 규칙(2171행 `preproc_arg` 등)이다 | 지시문·조건부 컴파일 영역의 tree는 원본과 다르다. G1 B01의 지시문 대안(`#define` 등)은 patch 빌드로 쟀다 |
| `argument_list`에 conditional fragment 허용 | grammar 연산 #41(`_argument_prefix`, `preproc_if_in_argument_tail`) | 규칙 모양이 다르나 괄호·인자 범위는 같다(3-7 참조) |
| 보간 문자열 scanner 수정 | scanner 연산 중 19개가 `interpolation_`을 건드림 | 규칙 텍스트는 같지만 시작 token 방출은 scanner가 정한다(3-2) |

원본→patch 두 `node-types.json`을 A1의 `schema diff`로 비교하면 이 구조 차이가 형식적으로 드러난다. 이 비교는 수행하지 않았다.

#### 2-3. patch 체인이 만든 회귀(원본의 결함이 아님)

`docs/reports/issue-233-csharp-lexical-recovery.md`에 기록된, kit가 직접 쓴 scanner 코드의 결함이다. **원본 scanner는 이 지시문 판독 코드를 갖고 있지 않으므로 이 결함은 원본에 존재하지 않는다.**

- **#279 네 사례(교정 전).** patch된 scanner가 지시문 `#`를 문자열·주석 안에서 잘못 노출했다: 지시문 문자열의 escape 오처리(byte 25), Razor `@*…*@` 주석(byte 9), `$$""`(byte 37), 중첩 문자열 closing quote(byte 62). Roslyn 기준 위치로 교정되었다.
- **directive `//` 거부 회귀.** 1차 이관에서 `scan_comment_token`이 `#define`·`#undef`·`#endif` 뒤의 유효한 `//` 주석을 거부해 11개 C# 기대값과 4개 SVC가 깨졌다. guard를 옮겨 해소했고 이어서 `#pragma … // text`의 `preproc_arg`가 `comment`로 바뀌는 회귀도 고쳤다.
- **`#line` filename 인접성 회귀.** `#line 10 /* comment\n*/ "file.cs"`가 기대한 `ERROR`를 잃었다. filename을 내부 `token.immediate`로 복원해 해소했다.
- **pragma verbatim quote 경계.** `scan_pragma_raw_tail`이 `@"a"""""`의 첫 quote에서 닫아 정상 source 30개 중 18개에서 다음 `#undef`와 class를 삼켰다(NO_ERROR로는 검출되지 않음).

**교훈:** 원본 scanner 위에 지시문 판독을 새로 쓰면 "정상 입력의 CST가 조용히 바뀌는" 회귀가 반복된다. 원본에 의존하는 go-treesitter는 이 위험을 지지 않는다. 대신 원본의 지시문 처리(정규식 기반 `preproc_arg`)가 가진 한계는 3-1에 적었다.

### A2.4 종류 3 — 설계 차이(결함이 아님)

원본 `grammar.js`가 명세·Roslyn과 다른 **표현을 택한 것**이다. 둘 다 유효하다. 소비자(예: go-treesitter의 query·snapshot 문서)가 알아야 하는 사실이며 upstream에 "수정"으로 기여할 대상이 아니다. 각 항목은 G1 asset의 `found`로 patch 빌드에서 관측되었고, 규칙 텍스트가 원본과 같음을 [정적]으로 확인했다.

#### 3-1. `#define`/`#undef` 심볼은 불투명 `preproc_arg`

- **명세/Roslyn.** 심볼은 `PP_Conditional_Symbol`(식별자 형태)이고 Roslyn `DefineDirectiveTrivia`는 식별자 token을 가진다.
- **산출물.** [정적] 원본 2276–2286행: `preproc_define: seq(preprocessor('define'), $.preproc_arg, /\n/)`, `preproc_undef` 동일. `preproc_arg`(2171행)는 `token(prec(-1, /\S([^/\n]|\/[^*]|\\\r?\n)*/))`로 **한 줄의 나머지 전부**다.
- **증거.** [관측] `csharp-B01.a335`, source `alt-csharp-B01-10#0`의 `#define DEBUG`에서 witness `[8,13)`(`DEBUG`)에 native node `preproc_arg [8,13)`(`src/contracts/csharp-g1-executed-b01b02-r2.json`). 봉인 r1의 기대는 `identifier`였고(`#if` 조건에서 가져온 오류), 불일치(`EXPECTED_ERROR`) 처분 뒤 r2에서 `preproc_arg`로 정정되었다.
- **원본과 patch의 차이.** 원본의 `preproc_arg`는 줄 끝까지(같은 줄의 `//` 주석과, CRLF 파일이면 `\r`도 `[^/\n]`에 걸려 포함) 이어진다 — 이것은 정규식 원문에서 읽은 것이고 실행해 보지 않았다. patch는 `alias($._preproc_identifier_token, $.preproc_arg)`로 **식별자 token만** 소비한다. 따라서 type은 같지만 범위 의미가 다르다. `#define DEBUG // note` 같은 입력에서 원본의 범위는 이 문서의 어떤 asset으로도 검증되지 않았다.
- **참고.** #447은 승인된 `a335` 정정이 인용한 원본 grammar 문장 세 개가 **최종(patch) grammar에는 없다**고 보고했다. 위 patch 연산이 그 규칙을 바꿨기 때문이다.

#### 3-2. 보간 문자열의 시작: 하나의 어휘소가 두 node로

- **명세/Roslyn.** `$"`, `$@"`, `@$"`는 한 token이다(`InterpolatedStringStartToken` 등, 명세 `Interpolated_Regular_String_Start`).
- **산출물.** [정적] 원본 1738–1756행: 일반·verbatim은 `alias($.interpolation_regular_start|_verbatim_start, $.interpolation_start)` + **익명 `'"'`**(`interpolation_start_quote`의 alias)이고, raw는 `interpolation_raw_start`(역시 `interpolation_start`로 alias) + **이름 있는 `interpolation_quote`** 다. 같은 구성에서 따옴표가 익명(일반·verbatim)과 named(raw)로 불일치한다. scanner `TokenType`(원본 `src/scanner.c` 8–22행)에 `INTERPOLATION_START_QUOTE`가 별도로 있다.
- **증거.** [관측] `csharp-B01.a383`: source `alt-csharp-B01-09#0`(`$"a{x}b{y,5}…`)의 `$"` `[66,68)`에 대해 native가 `interpolation_start [66,67)`와 `"` `[67,68)` 두 node를 낸다. `a396`(`$@"`), `a397`(`@$"`)도 같다. r1 기대(`interpolation_start`가 `[66,68)` 전체)는 `KIND_MAP_ERROR`였고 r2가 매핑표 항목 네 개를 `none NO_SEPARATE_NODE`로 정정하고 witness를 `starts-at`(시작만)로 약화했다.
- **한계.** 시작 token의 **범위**는 외부 scanner가 정하므로 원본 grammar 원문만으로는 도출되지 않는다(G1이 "시작만 단언"한 이유). 이 구성은 patch된 scanner(보간 관련 연산 19개)로 쟀다. 원본 scanner의 방출은 미관측.
- **후속 과제(G1-3d로 넘겨짐).** 시작 token을 "시작 node + 따옴표 node" 복합으로 단언하려면 매핑표 schema 확장이 필요하다. 이월하지 않는다.

#### 3-3. `interpolation` node의 경계와 정렬 절

- **산출물.** [정적] 원본 1775–1781행: `interpolation`은 외부 token `interpolation_open_brace`(alias `interpolation_brace`)로 시작해 `interpolation_close_brace`(같은 alias)로 끝난다. `interpolation_alignment_clause: seq(',', $.expression)`(1783행)는 쉼표를 node 안에 둔다. 명세 심볼은 중괄호 사이의 식(정렬은 `,` 뒤)이다.
- **증거.** `csharp-B01.a381`·`a395`(`regular_interpolation`/`verbatim_interpolation`)는 노드 양 끝이 외부 token이라 범위가 grammar 원문에 없으므로 r2에서 `span` → `starts-at`으로 약화했다. `a382`(`,5`, 정렬 절)는 `node`(앞 `,`)로 판정하는 witness `,5`(`[75,77)`)를 유지했다.

#### 3-4. 매개변수 한정자와 `ref`의 경합

- **명세/Roslyn.** `ref`·`out`·`in`은 키워드 token(`RefKeyword`, `OutKeyword`, `InKeyword`).
- **산출물.** [정적, 패치무관] 원본 720–726행 `_parameter_type_with_modifiers`는 `alias(choice('this','scoped','ref','out','in','readonly'), $.modifier)`를 반복한다. 즉 이 자리의 한정자는 익명 키워드 token이 아니라 **이름 있는 leaf `modifier`**다. `ref`는 같은 자리에서 `type`의 `ref_type`(`ref` + type)과도 경합하고, 이 쌍은 `conflicts`에 적혀 있지 않으며 `prec.left`가 암시하는 우선순위에 해소가 달려 있다(type 감사가 이를 근거로 "원문만으로 확정 불가"로 판정).
- **증거.** [관측] source `alt-csharp-B02-03#0`의 `void M(ref int a, out int b, in int c)`: `a084`·`a086`·`a087`(`ref`)은 `[31,34)`에서 `modifier`, `a088`(`out`)은 `[42,45)`에서 `modifier`, `a089`(`in`)도 `modifier`로 관측. `a088`·`a089`의 봉인 기대 `["\"out\"", modifier]`는 type 감사가 찾아 `[modifier]`로 좁혔다(r2).

#### 3-5. `var`는 `implicit_type`

- **명세/Roslyn.** type 위치의 `var`는 Roslyn `IdentifierToken`(`IdentifierName`)이다.
- **산출물.** [정적, 패치무관] 원본 856행 `implicit_type: _ => prec.dynamic(1, 'var')`, `type`(843행)의 첫 alternative. 또 `[$._reserved_identifier, $.implicit_type]`·`[$._reserved_identifier, $.implicit_type, $.var_pattern]` conflict(95·97행)로 식별자와 동적 우선순위로 가른다.
- **증거.** [관측] `csharp-B03.a184`·`a236`, `csharp-B04.a028`·`a029`: source `alt-csharp-B03-09#0`의 `[238,241)` `var`에 대해 native가 같은 범위에 `implicit_type`과 그 익명 자식 `"var"`를 낸다(PR #443 `src/contracts/csharp-g1-executed-b03b04-r1.json`). 봉인 기대 `["identifier","implicit_parameter"]`는 **기대값 오류**(`EXPECTED_ERROR`)이고 r2에서 `["\"var\"","implicit_type"]`으로 정정되었다. 이것은 Roslyn 매핑표가 틀린 것이 아니라 "한 token 종류에서 type을 가져온" 도출 오류다.
- **교훈.** 문맥에 따라 type이 달라지는 어휘 27개(`var`, `_`, `new`, `ref`, 문맥 키워드 등)는 어휘 단위 합집합으로 검증하면 한 문맥의 오류가 숨는다(r1 type 감사가 놓치고 대안별 비교인 r2 type 감사가 잡았다).

#### 3-6. 중첩 분해 선언의 designation은 `tuple_pattern`

- **명세/Roslyn.** `var ((g, h), i) = …`의 안쪽 `(g, h)`는 Roslyn `ParenthesizedVariableDesignation`.
- **산출물.** [정적] 원본 `variable_declarator`(790행)는 `choice(field('name', $.identifier), $.tuple_pattern)`이고 `tuple_pattern`(750–757행)은 `(`·identifier·`discard`·`tuple_pattern`의 목록이다. `parenthesized_variable_designation`(1408행)은 `_variable_designation`(1402행, **패턴 문맥**)에만 있다.
- **증거.** [관측] `csharp-B03.a239`: source `alt-csharp-B03-09#0`의 `var ((g, h), i) = …`에서 witness `[287,293)`(`(g, h)`)에 native가 `tuple_pattern`을 낸다(자식 `(`, `identifier`, `,`, `identifier`, `)`). 기대 `parenthesized_variable_designation`은 `DESIGN_DIFFERENCE`였고 r2에서 대안을 **BLOCKED `POSITION_DEPENDENT`**로 바꿨다(Roslyn 종류와 grammar node가 이 문맥에서 1:1이 아니고 witness가 하나뿐).
- **참고.** `tuple_pattern`은 patch가 건드린 `argument_list`와 같은 연산에 나란히 있으나(연산 #41) `tuple_pattern` 자체의 규칙은 바뀌지 않았다.

#### 3-7. 그 밖의 명세↔tree 차이(작은 것)

- **제약 목록.** [정적, 패치무관] 원본 453–458행 `type_parameter_constraints_clause`가 `where T : …` 전체를 한 node로 두고 각 제약이 `type_parameter_constraint`다. 명세의 제약 목록(`type_parameter_constraints`)은 자기 node가 없다. 단일 제약 목록이면 목록 범위가 그 제약과 같다(`csharp-B02.a072`, 증거 `where T : class`의 `class` `[76,81)`).
- **`join_into_clause`.** [정적, 패치무관] 원본 1620행 `join_into_clause: seq('into', $.identifier)`. 명세의 같은 이름 production은 join 절 전체이므로 tree의 node가 더 좁다(`csharp-B03.a209`: BLOCKED `NO_OWN_NODE_FOR_CONSTRUCT`).
- **`with_initializer`.** [정적, 패치무관] 원본 1961행은 `with` 식의 본문(`_with_body`)에서만 만든다. 객체 초기화자의 `X = 1`은 `assignment_expression`이다. 매핑표 r3가 Roslyn `SimpleAssignmentExpression`을 부모 종류로 가른다(PR #443 `src/contracts/csharp-g1-kind-map-r3.json`의 `contextual` 항목 1개).
- **`argument_list`의 괄호.** 명세 `argument_list`는 괄호를 포함하지 않지만 tree의 `argument_list` node는 `(`와 `)`를 포함한다(위치 감사가 `node` 항목으로 적었다).
- **nullable `?`.** [정적, 패치무관] 원본 879행 `nullable_type: seq(field('type', …), '?')`의 `?`는 익명 token이다. [관측] `csharp-B02.a030`: source `alt-csharp-B02-01#0`의 `int? a`에서 `"?"` `[89,90)`.
- **`#error`.** [관측] source `alt-csharp-B01-11#0`(`class Before { }\n#error build stopped here\nclass After { }`)을 Roslyn은 CS1029로 "수락하지 않는다"고 기록하지만 이는 `#error` 지시문이 의도적으로 내는 진단이고 syntax 불일치가 아니다. native는 오류 없이 파싱한다.

#### 3-8. 명세 구문 중 자기 node가 없는 것들(BLOCKED 172)

BLOCKED는 "G1이 기대값을 만들 수 없었다"이며 결함 목록이 아니다. #447의 재분석은 "약 150개가 구조상 풀 수 없다"는 1차 판단이 정당화되지 않는다고 결론냈다(아직 새로 EXECUTED가 된 것은 없다). 사유별로 읽으면:

| 사유 | 개수 | 의미(소비자에게) |
|---|---:|---|
| `ABSTRACT_CATEGORY` | 46 | `reference_type`·`pattern` 같은 union production. 자기 node 없이 하위 alternative로 넘어간다 |
| `SEMANTIC_BINDING` | 31 | `class_type`/`interface_type`/`struct_type`/`enum_type`/`delegate_type`은 모두 `type_name`이므로 **구문만으로 구별할 수 없다.** tree는 이들을 같은 `identifier`/`generic_name` 계열로 둔다. 포인터 대 배열 요소 접근, `nameof` 대상도 같은 이유 |
| `NO_OWN_NODE_FOR_CONSTRUCT` | 31 | 명세의 **목록 production에 node가 없다**: `expression_list`, `statement_list`, `member_declarator_list`, `orderings`, `catch_clauses`, `statement_expression_list`, `fixed_pointer_declarators`, `constant_declarators`, `query_body` 등. 원소들이 부모 아래 직접 자식이다 |
| `POSITION_DEPENDENT` | 22 | `namespace_name`·`type_name`처럼 같은 token이 자리(slot)에 따라 다른 node가 된다. G1 관측기가 slot을 기록하지 않았다(도구 한계) |
| `NO_TREE_COUNTERPART` | 19 | 공백·줄바꿈·주석 문자 클래스·지시문 줄의 공백: tree-sitter가 `extras`로 건너뛰어 node가 없다 |
| `NO_WITNESS_IN_LINKED_SOURCES` | 8 | 연결된 source에 증거가 없다(도구 한계). `B02.a075`·`a085`는 아래 A2.5의 finder 오류였다 |
| `LEXICAL_META_PRODUCTION` | 7 | `input`·`input_section`·`input_element`처럼 입력의 나열 구조 |
| `EXTERNAL_SCANNER_REPRESENTATION` | 7 | 보간 문자열의 `{{`·`}}`·`""` 이스케이프: external scanner가 표현을 정하고 `grammar.js`에 없다 |
| `NOT_IN_PINNED_GRAMMAR` | 1 | `default` 제약(1-1) |

### A2.5 G1 자산을 재사용할 때 알아야 할 도구 쪽 결함

이월하는 것은 **지식**이다. 승인 체계·원장·비용 모델·감사 asset의 봉인/정정 절차는 이월하지 않는다(go-treesitter의 native-ref·conformance와 중복되고 patch된 grammar를 기준으로 한 장치이므로). 다만 asset의 기대값을 재사용한다면 다음을 알아야 한다(모두 #447 교차 검증, [코멘트](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/447#issuecomment-6874888430)).

- **finder 오류.** `csharp-B02.a075`·`a085`는 연결 source에 witness가 있는데 finder가 잘못된 문맥만 찾아 "witness 없음"이 되었다. B01·B02 기대값 r3 정정이 결정되었으나 수행되지 않았다.
- **ground가 원본 grammar에 묶여 있다.** 승인된 `B01.a335` 정정의 grammar 근거 인용 세 개가 최종(patch) grammar에는 없다. 기대값의 "grammar 근거"는 원본 `grammar.js`(`63d357e0…`)이고 native는 patch 빌드이므로 둘이 다른 곳이 더 있을 수 있다(전수 감사는 결정만 되었고 수행되지 않았다).
- **비교기는 부모를 확인하지 않는다.** `g1xCompare`(G1 비교기)는 type과 범위만 본다. 같은 범위의 node가 있어도 기대 문맥(부모)이 틀릴 수 있다. 이미 EXECUTED인 721개는 부모 대조를 하지 않았다.
- **형태가 파생된다고 증거가 상속되지 않는다.** `B03.a239`(실제 MISMATCH)와 `B03.a105`(그 문맥의 witness 없음)가 반례다.
- **약한 일치가 많다.** 소리 없는 일치(한 글자 punctuation 앵커, 후보 집합 앵커, 연산자 앵커)를 "변별 일치"와 분리해 센다: B03·B04 EXPECTED 295 중 변별 `word` 62 + B 71, 후보 집합 19, 연산자 27, 약함 112 등. 일치 수를 정확성 비율로 읽지 않는다.
- **독립성 한계.** 기대값은 한 사람이 명세·grammar를 읽은 것이고, native는 한 host의 한 빌드이며 Roslyn oracle도 하나다. 사후 정정 15+4건은 native를 본 뒤 고친 값이라 독립 일치가 아니다.

**재사용 방법(이월할 때 가장 값진 것).** 대안(`<행>.aNNN`)마다 `source`·`start`·`end`·`found`(native가 그 범위에 낸 node type들)가 asset에 있고 source 본문은 `src/testdata/native/{routes,gaps,n461,queries}/csharp.json`의 `source_utf8`에 있다. 원본 grammar를 쓰는 go-treesitter가 같은 source를 parse해 같은 범위의 node type을 `found`와 대조하면, 원본과 patch 빌드의 차이가 대안 단위로 드러난다. B01·B02는 `main`의 asset, B03·B04는 PR #443 브랜치 commit이 필요하다.

### A2.6 확인하지 못한 것

- **원본 grammar로 parse한 결과가 하나도 없다.** 위 모든 "upstream 원본에 적용된다"는 문장은 원문 읽기([정적])와 규칙 이름 검색([패치무관])에 근거한다. 특히 1-1의 식별자 오분류, 1-2·1-3의 원본 오류 여부, 3-1의 `preproc_arg` 줄 끝 범위, 3-2의 원본 scanner 방출은 추론이다.
- **BLOCKED 172개 전체를 개별로 재검토하지 않았다.** 사유별 개수는 `src/contracts/csharp-g1-ledger-r2.json`(PR #443)에서 직접 센 값이며 #447 본문과 일치한다.
- **G1 B05와 V\*(버전별) 행은 시작하지 않았다**(원장 r2 `NO_EXPECTED` 638). C# 전체 선언 범위에 대한 의무→기대값→실행 대조는 이 문서가 닫지 않는다.
- **#233의 lexical producer 교정 결과(A2.3)는 patch 빌드의 것**이며 원본 scanner의 동일 입력 동작은 측정하지 않았다.
- **`schema diff`를 원본→patch `node-types.json`에 적용해 보지 않았다.** 구조 차이는 매핑표의 `grammar.additions`와 patch 연산 목록으로만 확인했다.

