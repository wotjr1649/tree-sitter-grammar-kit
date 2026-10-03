# Session 03 — Schema Contract 결과

Campaign `TSGK-C1-20260929-R1`, 추적 `TSGK-C1-S03`, Issue #5, Milestone 4, branch `session/03-schema-contract`(base main `a3d3eca`). 이 보고서는 아래에 적은 revision과 로컬 관측만 설명한다. PR·세 OS CI·merge·post-merge 결과는 이 문서 작성 시점에 없으며 그 단계의 receipt가 소유한다.

## 구현 범위

* 공개 offline API `kit.SchemaCheck`, `kit.SchemaDiff`, `kit.DefaultSchemaLimits`와 CLI `tsgk schema check --input FILE`, `tsgk schema diff --before FILE --after FILE`. 두 경로는 같은 core와 공유 strict JSON decoder를 쓰며 parser·`parser.c`·compiler·process·network가 필요 없다. 계약은 [tree/protocol](../specs/tree-and-adapter-protocol.md)의 `정적 node-types 계약`, [CLI/profile](../specs/cli-and-profile.md)의 `S03 구현`, [공개 API](../specs/public-go-api.md)의 `S03 함수와 추가 필드`다.
* 형식 `node-types-r1`: `(type, named)` identity, field·children set의 `required`/`multiple`/허용 type 집합, supertype/subtype, 없는 것과 빈 것의 구분(`fields` 없음 ↔ `{}`, `root`/`extra` 선언 없음 ↔ `false`), 중복 node·field·member, 해석하지 않는 key(BLOCKED), subtype 참조 해석, 반복 DFS의 supertype 순환 검사, root 중복. 위반은 모두 finding으로 모으며 부분 PASS는 없다.
* 비교 `node-types-diff-r1`: node 추가·제거·named 변화, root·extra, fields 유무, field 추가·제거·required·multiple·허용 type, children 유무·cardinality·type, supertype 유무·subtype을 방향성 있게 보고한다. risk는 `ADDITION`/`REMOVAL`/`IDENTITY`/`CLASSIFICATION`/`CARDINALITY_NARROWED`/`CARDINALITY_WIDENED`의 검토 범주이며 안전·SemVer 판정이 아니다. rename은 추정하지 않는다. 결과는 canonical before/after와 원본 JSON pointer를 갖고 결정적으로 정렬된다.
* 한도: 문서 16777216 bytes, record 200000, JSON 값 8×record, 출력 16777216 bytes, wall 120초. 취소 checkpoint는 decode·record·그래프 walk·비교에 있다. finding은 입력당 1000건과 path 1024 bytes로 제한하고, 공유 decoder와 schema model은 JSON pointer를 필요할 때만 만든다.
* 계약 자료: [선언·사실 mapping](../../src/contracts/fact-mapping.json)(`declaration-facts-r1`, `xml-structure-r1`, `dynamic-sql-r1`, `csharp-command-api-r1`), `tsgk-tree/r1` 입력 encoding field, `tsgk-tree-summary/r1`([예시](../../src/contracts/examples/tree-summary-r1.json)), `.svc` composite record `tsgk-svc-composite/r1`, `known_gaps` S03 항목의 구조 계약·처분 표.

제외: 언어 사양 판정, runtime tree·native 실행, supertype을 field 허용 type으로 펼치는 실효 비교, 자동 SemVer, query 호환성. 채택 후보 6 route의 재생성 schema는 S04가 만든다.

## 관측 revision과 로컬 검사

구현 commit `bce21ce`, `9424e87`, `3d23947`과 리뷰 수정 commit `25a8df4`, `c2fb57b`, `308338c`, 문서 수정 `d742ba9`(base `a3d3eca`)에서 Windows amd64, Go 1.27.1, `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`로 실행했다. 세 OS CI는 PR 단계에서 따로 기록한다.

| 검사 | 결과 |
|---|---|
| `gofmt -l src`, `go vet ./src/...`, `go build ./src/...`, `git diff --check` | 통과 |
| `go test ./src/... -count=1` | `src/kit`, `src/cmd/tsgk`, `src/internal/foundation` 통과 |
| `TestFactMappingAgainstSchemas`(로컬 upstream schema 사본 4개) | 통과. hosted CI에는 schema 사본이 없어 skip된다 |
| 외부 consumer module(checkout 밖, `PATH` 비움) | fixture와 26 route 모두 schema check·diff의 CLI와 API JSON bytes가 같고 오류 code가 같다 |
| targeted mutant 37종(S03 17종 + S01·S02 20종, 최종 코드 `308338c`) | 모두 컴파일되고 해당 시험이 의도한 진단으로 실패. `3d23947`, `25a8df4`, `c2fb57b`에서도 37/37 |
| `src/kit` 15회 반복(`25a8df4`) | 1회 S01 `TestCorpusLimits/WALL_LIMIT`(1 ns wall timer) 실패. S03이 건드리지 않은 시험이며 #61과 같은 원인으로 기록한다 |

S03 mutant는 named 무시, required·multiple 비교 제거, subtype 비교 제거, 제거된 허용 type 무시, 미해결 subtype 허용, 중복 member 병합, fields 없음과 `{}` 동일시, 빈 schema 허용, 순환 검사 제거, record 한도 제거, 공유 decoder의 배열 중복 제거·정렬(ordered tree 범위 누출), CLI diff exit 무시, mapping locator 오류, mapping 사실 종류 누락, summary 예시의 텍스트 추출이다.

acceptance 연결: A01 `TestSchemaEquivalence`(key·항목·허용 type 순서를 바꾼 같은 schema, 같은 입력의 byte 동일 결과, 재배열 입력의 같은 finding); A02 같은 시험의 `null` named/anonymous와 `TestSchemaInvalid`의 named 불일치 subtype; A03·A04·A05 `TestSchemaDiffKinds`(독립 작성 기대 목록 19행, before/after/path 대조, supertype 추가·전환); A06·A07 `TestSchemaInvalid` 39 사례와 정당한 관례 사례, fields 유무 diff, `TestSchemaFindingBounds`, `TestSchemaLongNames`; A08 `TestSchemaLimits`(record·문서·출력 at/one-over, JSON 값·깊이 한도, 20000단 supertype 사슬과 닫힌 사슬, check의 시작 전·첫·중간·마지막 checkpoint 취소와 diff 비교 단계 취소, deadline 없는 context)와 `TestSchemaLongNames`(1 MiB 이름 아래 할당 256 MiB 미만, 출력 한도 안 실패 report); A09 `TestSchemaReverse`(역방향 기대 목록, 같은 이름 두 입력의 role·hash 분리); A10 `TestSchemaCLI`, `TestExternalConsumerSchema`, `ExampleSchemaDiff`, `TestOfflineClosure`; A11·A12 위 mutant와 `TestSchemaScopeOrderedTree`; A13 아래 26 route 표; A14 세 OS CI(PR 단계); A15·A17 `TestFactMapping`, `TestFactMappingAgainstSchemas`; A16 `TestTreeSummaryExample`; A18 [구조 계약·처분 표](../specs/tree-and-adapter-protocol.md).

## 26 route schema 검사 (A13)

S01이 결속한 26 route source 사본(`_ref/campaign-01-s01/sources`, 새 download 0)의 `node-types.json`에 최종 코드 `308338c` build를 실행했다(`3d23947`, `25a8df4`, `c2fb57b`도 같은 판정). 26 route 모두 외부 consumer의 API 결과와 CLI 결과가 bytes까지 같다.

* PASS 25: csharp, go, python, javascript, jsx(javascript와 같은 schema), typescript, tsx, java, kotlin, c, cpp, rust, dart, php, ruby, r, bash, powershell, html, css, json, yaml, xml, tsql, postgresql-sql. python은 field 허용 type `as_pattern_target`이 node로 선언되지 않아 `REFERENCE_UNDECLARED` warning 1건이다.
* FAIL 1: swift. `("?", false)`와 `("??", false)`가 각각 두 번(하나는 `fields: {}`, 하나는 field 없음) 선언되어 `NODE_DUPLICATE`다. 공식 문서는 `(type, named)`의 유일성을 요구한다. PREPARE에서 tree-sitter CLI 0.27.0으로 재생성한 후보 사본에도 같은 중복이 있으므로 grammar·generator 쪽 형식 위반으로 기록한다. kit 결함이 아니며 S08 판정으로 넘긴다. swift는 `schema diff` 입력이 될 수 없다(`SCHEMA_INVALID`, exit 2).
* 채택 6 route의 PREPARE 후보 tree 안 `node-types.json`은 upstream과 byte가 같다(재생성본이 아님). 후보 native tree에는 upstream schema에 없는 node(C# `file_based_app_directive`, T-SQL `generated_always_clause`·`graph_table_type`·keyword 4개)가 있다. 재생성 schema의 검사와 upstream 대비 diff는 S04 의무다.
* 실제 upstream 변형 diff: typescript→tsx 16건(JSX node 추가 등), php_only→php 4건, javascript→typescript 219건. 역방향은 건수가 같고 추가·제거가 정확히 뒤집혔다. 이것은 의도된 dialect 차이의 관측이며 결함 판정이 아니다.

## 분리 context 리뷰와 처분

`a3d3eca..3d23947`에 대한 분리 context 정적 리뷰(STATIC_REVIEW, devflow reviewer subagent, 명령 미실행; GitHub 승인 아님)는 BLOCKER 1, MATERIAL 2, MINOR 5, NOTE 2를 보고했다.

* BLOCKER: field 값이 object가 아니면(`"fields":{"f":null}`) nil set 역참조로 panic했다. 재현을 확인했고 위반 finding만 남기도록 고쳤다(`25a8df4`, 사례 5개).
* MATERIAL: finding 수와 크기에 상한이 없어 출력이 한도를 넘었다 → 입력당 1000건과 `FINDINGS_TRUNCATED`, 판정은 전체 기준, 출력 한도를 넘는 실패 report는 실패 finding 하나(`25a8df4`, `c2fb57b`). node 전체 canonical 값에서 `fields: {}`가 사라졌다 → 보존(`25a8df4`).
* MINOR: escape 중복 시험에 escape가 없던 것, `8×record` overflow, 한도 초과 입력의 CLI/API identity 불일치(한도 초과 입력은 hash하지 않음), 동적 SQL mapping의 schema 정합성 미검사(`requires` 목록을 schema로 검사), `children:` locator의 여러 이름 처리 규칙을 고쳤다(`25a8df4`). 동적 SQL·mapping 검사가 hosted CI에서 skip되는 점은 upstream schema를 vendor하지 않는 계약 때문이며 로컬 증거와 S04 재실행으로 처분했다.
* NOTE: `sp_executesql`은 `static_exec_target`이 아니라 `dynamic_sql_site`만으로 정했다. swift의 `NODE_DUPLICATE` FAIL은 공식 유일성 규칙과 S03-A06에 따라 유지하고 grammar·generator gap으로 S08에 넘긴다.
* 자체 발견: `SchemaCheckResult`의 `schema` JSON key가 E0의 `schema: tsgk-report/r1`을 가렸다. 결과 필드를 `input`으로 바꿨다(`25a8df4`).

재리뷰(`3d23947..25a8df4`)는 이전 처분을 확인하고 BLOCKER 1(공유 decoder가 값마다 전체 pointer 문자열을 만들어 긴 key 아래 메모리가 key 길이×값 수로 커짐, S02 경로에도 있던 결함), MATERIAL 1(finding 크기와 diff 입력 오류 report의 출력 상한), NOTE 1을 보고했다. decoder와 model이 부모 연결만 두고 pointer를 필요할 때 만들도록 바꾸고, finding path를 1024 bytes로 자르며, diff는 path를 포함한 크기를 path를 만들기 전에 보수적으로 추정해 `OUTPUT_LIMIT`으로 끝낸다(`c2fb57b`). 다음 재리뷰의 MINOR 2(decoder 오류 path 절단, diff 추정의 보수성 문서화)는 `308338c`, NOTE(S02 계약의 오류 path 규칙)는 `d742ba9`로 처리했다. 마지막 재리뷰(`d742ba9`)는 NO_FINDINGS다.

## 남은 일과 한계

* 세 OS CI(A14)는 PR 단계에서 실행한다. `go test -race`는 CGO를 쓰지 않는 조건이라 실행하지 않았다.
* mapping은 schema에서 도출한 계약이다. tree 위 동작(선언 순회, 사실 query, 동적 SQL 위치 추출)은 S05·S06이 native로 검증한다. 등록 T-SQL 사례에는 `execute_statement`가 없어 EXEC 관련 mapping은 grammar source와 schema로만 도출했다.
* supertype 변화는 supertype node의 `SUBTYPE_*`로만 보고하고 field의 실효 type 집합으로 펼치지 않는다.
* 비공개 corpus는 다시 실행하지 않았다. S03은 corpus inventory 경로를 바꾸지 않았고, 공유 decoder 변경(지연 pointer, 오류 path 절단)은 corpus 경로에서는 profile 문서 decode에만 닿고(verify의 profile·expected 문서에도 같은 규칙이 적용된다) S01·S02 시험 전체와 S01·S02 mutant 20종이 같은 결과다.
* 같은 identity가 모양만 다르게 두 번 나오는 swift schema(generator 0.27.0 출력 포함)는 `schema diff` 입력이 될 수 없다. S04가 swift 재생성 schema를 upstream과 비교하려면 이 처분을 바꾸는 별도 결정이 필요하다.
