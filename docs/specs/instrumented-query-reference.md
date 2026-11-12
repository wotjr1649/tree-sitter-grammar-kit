# 계측 query reference 계약 (#216)

## #362 ordinary batch capture 준비 (P2-B 일부)

P2-B의 첫 구현 단위는 [actual batch wire와 실패 보존](tree-and-adapter-protocol.md#362-batch-actual-capture-기반-p2-b-일부) 및 [process owner](identity-and-evidence.md#362-batch-capture의-process-소유권-p2-b-일부)다. 내부 captured 실행 primitive는 `native-query-batch`, native/r4, full tree, API=true, UTF-8, SVC·preprocess·reuse-accepted 없는 입력에 한정한다. 일반/single producer 출력과 기존 reader/reducer의 schema 허용은 그대로다.

P2-B 2단계는 내부 실행 API `OracleRequest.Ordinary`로 명시적으로 선택한다. `Profile`에는 기존 실행 profile/r1 또는 r2를, `Ordinary.Inputs`에는 P2-A의 원문 registry·qualification·slots·P1 witness·계획 profile/r4·assets·registrations와 해당 본문을, `ShardPlan`과 `Registration`에는 선택한 저장 계획과 등록 원문을 제공한다. `ShardIDs`는 실행 case 각각에 대응하는 계획의 case shard를 순서대로 명시한다. 계획은 `ValidateOrdinaryShardPlan`으로 전체 재구축·대조하고, 선택 shard의 route/platform/case 및 실행 profile의 route·host·grammar·runtime manifest·query 순서와 bytes·case 순서·전체 edits·각 source version/points를 다시 결속한다. manifest/binding의 `selected_shard_ids`와 `full_scope_complete=false`로 부분 수집을 표시한다. 이 명시 경로에만 batch source witness를 열며 legacy `BATCH_SCOPE_UNSUPPORTED`는 유지한다. CLI의 새 입력 flag나 r4 단독 실행 parser는 추가하지 않는다.

수집 결과는 record/r5·manifest/r8과 caller에게 반환하는 run-binding/r2다. 실행은 source와 history를 축약하지 않고 기존 batch coordinator를 사용한다. 계획의 `CAPACITY_UNPROVEN`/BLOCKED는 수집 허가나 구조 검사로 admission PASS가 되지 않는다. case projection에는 기존 base PASS/FAIL을 보존하고 record의 `query_semantics`에는 `PRODUCER_LOCAL`, `BLOCKED`, `EXTERNAL_APPROVAL_PENDING`을 남긴다. `VerifyOracleSet`의 r8 처리는 저장 구조·물리 pin 검사이며 guarded 의미 reader·eligibility·새 replay나 qualification 지원이 아니다.

일반 query 뒤 fact pack의 route query 전체를 원래 순서의 suffix로 요구한다. `FactPack`과 `FactMapping` 원문을 보존하고 `step_facts`의 `tsgk-ordinary-step-facts/r1`에 pack SHA·mapping schema/revision/SHA·동적 SQL/XML revision과 ordered query ID/kind/SHA의 길이 접두 digest를 기록한다. 각 case/step/tree_role/query의 source 길이·SHA와 실제 capture로 선언·dynamic SQL·XML을 도출한다. 편집 뒤 dynamic SQL은 해당 version bytes를, fresh는 fresh captures를 독립적으로 사용한다. 내부 재현 일치는 독립 expected나 외부 승인을 대신하지 않는다.

독립 expected 자산의 compact row 형식(B3)과 reference stdout의 별도 pinned member 및 `tsgk-reference-process/r2`(B5)는 이 단위에서 만들지 않는다. 이번 형식은 native actual transcript이며 expected/reference 형식을 먼저 고정하면 아직 연결되지 않은 reader·receipt 계약을 확장하게 된다. 기존 자산/receipt 비용 추정과 SQL `CAPACITY_UNPROVEN`/BLOCKED를 유지하고 B3/B5 절감액을 선반영하지 않는다. reference 실행·한도 조정·public ordinary API·reader/replay 완료를 뜻하지 않는다.

## DartPHP12 단일 frame 정의 분류

Dart7·PHP5는 기존 Web16·Script17·Small17·CJR 이름 집합과 겹치지 않는 닫힌 수동 profile이며 공통 empty-profile 검증기를 사용한다. 다음 값은 pinned grammar와 generated parser에서 도출한 독립 사전 기대값이다. 실제 parse·compile·query 결과는 별도 관측으로 확인한다.

| Profile / anchor | 공개 CST | 정의 | ABI symbol/field |
|---|---|---:|---:|
| `dart7-empty` / [Dart7](../../src/testdata/query-semantics/dart7-classification-anchors.json) | `program` root 1개 | 7 | 533/22 |
| `php5-empty` / [PHP5](../../src/testdata/query-semantics/php5-classification-anchors.json) | `program` root 1개 | 5 | 437/31 |

두 profile은 빈 UTF-8·편집 없음·API=false·`native-query`·단일 frame·step0 `NO_ERROR`를 요구한다. Root는 parent=-1, field=null, named=true, 나머지 flag=false, byte span0..0과 양쪽 point(0,0)이며 공개 자식이 없다. ABI language/runtime/min-compatible은15/15/13이고 Dart symbol533은 generated532와 alias1의 합이다. 두 pinned parser의 state1 EOF action은 zero-child `program` reduction이며 원본 grammar의 nullable root와 일치한다. Scanner·실제 CST와 모든 공개 필드는 관측에서 다시 확인한다. C2 전체 regeneration closure를 증명하지 않는다.

원문 query12개의 ID·순서·UTF-8 bytes·SHA를 고정한다. 사전 metadata는42 patterns·43개의 query별 capture-name 항목·전역 고유 이름38개다. Dart의 마지막 query는 두 pattern에서 같은 `default` capture를 사용하므로 capture-name 목록은 하나다. 각 query의 첫 등장 이름 순서와 명시적 `predicates:[]`를 유지한다. 빈 root에 대상 construct가 없다는 근거로 structural/expected tuple은 빈 배열 후보이며 원래 비어 있지 않은 workload의 의미론을 증명하지 않는다.

Dart는 parser·scanner·header3개, PHP는 `common/scanner.h`와 `php/src/parser.c`·`php/src/scanner.c`·`php/src/tree_sitter/` header3개로 정확한 role/path/bytes/SHA를 고정한다. PHP include는 `php/src`와 `common`이며 경로를 평탄화하거나 공통 header를 생략하지 않는다. Parser 크기7,872,952/8,115,395 bytes는 각16MiB plain-input 한도 안이다. 개별 physical file 한도와 aggregate JSON/context 한도는 별개다. 기존 small 한도·정렬된9-name compiler environment·`full-input-utf8/r2`와 다른 profile 경계를 유지한다.

원래 ordinary qualification의 API=true 및 Dart53/24/77/101/707, PHP48/46/94/140/700은 각각 cases/edits/case steps/tree roles/potential query-tree-role 좌표다. 합계101/70/171/241/1,407은 이번 정의12개의 실행 분모가 아니다. PowerShell의 patched root는 nonnullable이므로 이 빈 소스 집합에 포함하지 않고, [PowerShell3 절](#powershell3-단일-byte-frame-정의-분류)의 별도 1-byte profile로 다룬다.

각 profile의 새 current graph·서로 다른 query별 reference invocation·provenance·외부 caller approval과 같은 guarded snapshot의 정의 projection·공개 API parity 후에만 조건부 `STRUCTURAL_ONLY`를 기록한다. 원장 `STRUCTURAL_CANDIDATE/UNVERIFIED`, semantic `NOT_ASSESSED`, `semantic_required:null`과 전체 #216/#195의 미완료는 유지한다. 이후 보고서/merge revision을 관측 source/reader로 표시하지 않는다.

## Script17 단일 frame 정의 분류

Python8·Ruby4·R3·Bash2는 Web16과 이름 집합이 겹치지 않는 닫힌 수동 profile이다. 기존 empty-profile descriptor와 compiler·원장·guarded projection 검증을 공유하며, 기존 Web16·Small17·CJR 진입에 새 이름을 허용하지 않는다. 다음 값은 관측 전에 고정한 독립 사전 기대값이며 실제 parse·compile·query match 또는 caller 승인 실적이 아니다.

| Profile / anchor | 공개 CST | 정의 | ABI symbol/field |
|---|---|---:|---:|
| `python8-empty` / [Python8](../../src/testdata/query-semantics/python8-classification-anchors.json) | `module` root 1개 | 8 | 284/32 |
| `ruby4-empty` / [Ruby4](../../src/testdata/query-semantics/ruby4-classification-anchors.json) | `program` root 1개 | 4 | 354/32 |
| `r3-empty` / [R3](../../src/testdata/query-semantics/r3-classification-anchors.json) | `program` root 1개 | 3 | 144/20 |
| `bash2-empty` / [Bash2](../../src/testdata/query-semantics/bash2-classification-anchors.json) | `program` root 1개 | 2 | 293/19 |

네 profile은 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`·step0 `NO_ERROR`를 요구한다. Root는 parent=-1, field=null, named=true, 나머지 flag=false, byte span0..0과 양쪽 point(0,0)이며 공개 자식은 없다. ABI language/runtime/min-compatible은15/15/13이고 symbol 수는 generated symbol과 alias의 합이다. 고정 runtime은 초기 stack state1에서 시작한다. Python·Ruby·Bash의 초기 EOF reduction과 R의 zero-width invisible `_start` 뒤 EOF reduction을 소스에서 검토해 root 후보를 도출했으며, 실제 scanner/parse 결과에서 full CST와 모든 공개 필드를 다시 대조해야 한다. Raw grammar root 규칙 확인은 C2 전체 regeneration closure 증명이 아니다.

고정 원문17개의 ID·정확한 UTF-8 bytes·SHA·순서를 결속한다. 사전 metadata는52 patterns·59개의 query별 capture-name 항목·전역 고유 이름47개다. 각 query의 첫 등장 이름 순서와 명시적 `predicates:[]`를 유지하고 nil/null·순서·중복/누락·nonempty predicate를 거부한다. 하위 wildcard와 R의 quoted backslash·`"|>"`를 predicate나 최상위 wildcard로 세지 않는다. 빈 root 아래 필요한 construct가 없다는 근거로 structural/expected tuple은 빈 배열 후보이며, 원래 비어 있지 않은 workload의 의미론을 증명하지 않는다.

각 grammar는 parser1·scanner1·header3개의 정확한 role/path/bytes/SHA와 compiler source/include·출력·한도·정렬된9-name environment를 고정한다. Ruby parser16,268,990 bytes는16MiB보다508,226 bytes 작아 기존 plain-input 경계를 사용한다. 개별 physical file 한도와 JSON 문서 한도는 별개이며 parser bytes/base64를 JSON에 합치거나 네 closure의 합계를16MiB 안이라고 주장하지 않는다. 기존 `full-input-utf8/r2`, small 한도, Go/C#/CPP paging 및 다른 닫힌 profile 경계는 유지한다.

원래 ordinary qualification의 API=true와 이번 anchor의 API=false를 구분한다. Python49/60/109/169/1,352, Ruby30/34/64/98/392, R17/28/45/73/219, Bash31/18/49/67/134는 각각 cases/edits/case steps/tree roles/potential query-tree-role 좌표다. 합계127/140/267/407/2,097은 이번 빈 입력의 실행 횟수가 아니다.

새 profile마다 case·current graph·query별 서로 다른 reference invocation·provenance와 새 외부 caller approval을 별도로 보존한다. 같은 guarded snapshot에서 eligibility·full CST/metadata·request/frame·compiler/library·stderr owner와 별도 공개 API parity를 통과해야 조건부 `STRUCTURAL_ONLY`를 기록한다. 관측 source/reader와 후속 report/merge revision은 구분한다. 원장 `STRUCTURAL_CANDIDATE/UNVERIFIED`, semantic `NOT_ASSESSED`, `semantic_required:null`, 전체 #216/#195와 per-case·Windows/Linux·SVC69·large4·replay/inventory/fan-in 미완료는 이 단위로 바꾸지 않는다.

## Web16 단일 frame 정의 분류

Windows의 source/reader `fc0a7433fb2b5fd27cf6e41f1d166b1a23c66fa6`에서 네 profile의 실제 관측·외부 caller approval·guarded 정의 projection과 공개 API parity를 확인한 범위는 [Web16 보고서](../reports/issue-216-web16-classification.md)에 기록한다. 이번16개는 조건부 구조 분류이며 원장의 미검증 metadata·semantic 미평가와 전체 #216/#195 미완료를 유지한다. 후속 보고서/PR/merge revision을 당시 실행 source로 표시하지 않는다.

Web16은 기존 Small17과 별도로 닫힌 네 profile이다. 모두 빈 UTF-8 source, 편집 없음, API=false, `native-query`, `Args=["single"]`, step0 `NO_ERROR`를 사전등록한다. 기대값은 관측 전에 고정한 후보이며 실제 compile·native/reference 실행을 대신하지 않는다.

좌표를 가져오는 기존 ordinary qualification workload의 `API=true`는 유지한다. 이는 이번 빈 anchor의 `API=false`와 다른 입력 경계이며, 좌표 연결이 기존 API workload의 실행이나 검증을 뜻하지 않는다.

| Profile / anchor | 공개 CST | 정의 | ABI symbol/field |
|---|---|---:|---:|
| `javascript4-empty` / [JavaScript4](../../src/testdata/query-semantics/javascript4-classification-anchors.json) | `program` root 1개 | 4 | 291/36 |
| `jsx3-empty` / [JSX3](../../src/testdata/query-semantics/jsx3-classification-anchors.json) | `program` root 1개 | 3 | 291/36 |
| `typescript7-empty` / [TypeScript7](../../src/testdata/query-semantics/typescript7-classification-anchors.json) | `program` root 1개 | 7 | 388/40 |
| `tsx2-empty` / [TSX2](../../src/testdata/query-semantics/tsx2-classification-anchors.json) | `program` root 1개 | 2 | 408/43 |

고정 parser의 초기 EOF reduction·goto·accept와 해당 external scanner state를 검토해 빈 `program`을 도출했다. 공개 root는 parent=-1, field=null, named=true, extra/error/missing=false, byte span0..0, 양쪽 point(0,0)이다. 공개 API에서 보이는 자식 수0과 runtime 내부의 숨겨진 EOF subtree를 혼동하지 않는다. ABI language/runtime/min-compatible은15/15/13이며 symbol 수는 generated symbol과 alias의 합이다. 이 값과 full CST의 모든 공개 필드는 현재 candidate의 실제 관측에서 다시 확인해야 한다.

네 query 원본의 ID·UTF-8 bytes·SHA·순서를 각각 결속한다. 16개 정의의 사전 metadata는51 patterns·54개의 query별 unique capture-name 항목이며 전역 고유 이름 수는48이다. 각 query의 첫 등장 이름 순서와 명시적 `predicates:[]`를 고정하고 nil/null·다른 predicate·중복/누락·순서 변경을 거부한다. 원문의 `!parameter`, 하위 `(_)`, 문자열 `"**"`·`"??"`는 predicate나 최상위 wildcard로 세지 않는다. 모든 pattern이 `program` 이외의 시작 node를 요구하므로 위 빈 CST의 structural/expected tuple은 빈 배열 후보다. 이 무매치 경로는 비어 있지 않은 원래 workload의 query 의미론을 증명하지 않는다.

기존 `full-input-utf8/r2`와 small 한도를 유지한다. JavaScript/JSX의 grammar pin과 symbol이 같아도 route·query·anchor·graph·승인 identity는 별개다. TypeScript/TSX의 중첩 parser/scanner 경로와 `common/scanner.h`를 평탄화하지 않는다. 각 route의 정확한 role/path/bytes/SHA closure와 compiler source/include·출력·한도·정렬된9-name environment를 사전 request 및 관측 request에 결속한다. Small17 closed5/17, CJR closed3/13, Go와 C#/CPP paging 경계를 넓히지 않는다.

새 cohort의 provenance 검토·외부 caller approval 뒤 같은 guarded snapshot에서 eligibility·독립 full CST·완전한 metadata·query별 서로 다른 reference invocation·request/frame·compiler/library·stderr owner와 공개 API parity를 확인해야 조건부 `STRUCTURAL_ONLY`를 기록한다. 원장의 `STRUCTURAL_CANDIDATE`, `compiled_metadata:UNVERIFIED`, `semantic_evaluated:false`와 `semantic_required:null`은 이 부분 분류로 승격하지 않는다. 네 ordinary workload의268 cases·112 edits·380 case steps·492 tree roles·2,039 potential query/tree-role 좌표는16개 정의의 실행 횟수가 아니다. 실제 source/reader revision과 후속 report/merge revision을 구분하며 전체 #216/#195·per-case/semantic·Windows/Linux·SVC/large·replay/fan-in 미완료를 보존한다.

## Small17 단일 frame 정의 분류

별도 닫힌 profile 다섯 개는 다음 사전 기대값을 선택한다. 모두 UTF-8, 편집 없음, API=false, `native-query`, `Args=["single"]`, step0 `NO_ERROR`다. 표의 CST·ABI·query metadata는 정적 사전등록이며 실제 parser·compiler·query 관측을 대신하지 않는다.

| Profile / anchor | Source | 전체 CST nodes | 정의 | ABI symbol/field |
|---|---|---:|---:|---:|
| `json1-tiny` / [JSON1](../../src/testdata/query-semantics/json1-classification-anchors.json) | `null` | 2 | 1 | 23/2 |
| `yaml1-empty` / [YAML1](../../src/testdata/query-semantics/yaml1-classification-anchors.json) | 빈 source | 1 | 1 | 301/2 |
| `xml2-tiny` / [XML2](../../src/testdata/query-semantics/xml2-classification-anchors.json) | `<r/>` | 6 | 2 | 142/2 |
| `html2-empty` / [HTML2](../../src/testdata/query-semantics/html2-classification-anchors.json) | 빈 source | 1 | 2 | 48/0 |
| `css11-empty` / [CSS11](../../src/testdata/query-semantics/css11-classification-anchors.json) | 빈 source | 1 | 11 | 204/0 |

JSON의 현재 문법은 `document`에 value 하나를 요구한다. JSON CST는 `document`와 named `null` 두 노드다. XML은 `document`의 `root` field에 `element`가 있고 그 아래 `EmptyElemTag`, anonymous `<`, named `Name`, anonymous `/>`를 포함한다. 모든 node의 parent·field·flags·byte/point span을 고정한다. XML fact query의 pattern1은 바깥 `element` capture(ID1, node2, bytes0..4) 뒤 `element.name`(ID0, node4, bytes1..2)을 기대한다. 다른 query의 structural/expected capture는 빈 배열이다. YAML·HTML·CSS root는 각각 `stream`, `document`, `stylesheet`다. ABI language/runtime/min-compatible은15/15/13이며 CSS symbol204는 generated `SYMBOL_COUNT=199`와 `ALIAS_COUNT=5`를 합하는 고정 runtime API의 기대값이다.

17개 query의 사전 metadata 합계는44 patterns·46개의 query별 unique capture-name 항목이고 predicate 목록은 모두 명시적 `[]`다. 원문 occurrence의 중복 capture 이름을 compiled name ID의 중복으로 세지 않는다. 각 ordinary query 원본 파일의 전체 bytes·SHA와 ID·원문·순서를 따로 결속하고, XML의 `facts.xml_structure`는 `fact-query-pack.json`의 정확한 별도 원본과 연결한다. Null metadata·source alias 교환·누락·중복·다른 predicate는 거부한다.

다섯 route는 기존 `full-input-utf8/r2`만 사용하며 parser paging이나16MiB 물리 파일 한도를 넓히지 않는다. Grammar closure는 JSON4·YAML8·XML6·HTML6·CSS5, 총29개의 고정 role/path/bytes/SHA다. JSON은 scanner가 없다. YAML의 schema C 파일3개와 HTML의 `src/tag.h`는 header closure로 보존하며 확장자만 보고 별도 compile source에 넣지 않는다. XML의 `xml/src/`와 `common/scanner.h` 경로를 평탄화하지 않는다. Compiler argv는 parser/scanner role과 정확한 route 경로·include·출력·한도에 결속한다. Compiler environment는 `COMSPEC`, `PATH`, `PATHEXT`, `SystemDrive`, `SystemRoot`, `TEMP`, `TMP`, `TMPDIR`, `WINDIR`의 아홉 이름만 이 정렬 순서로 허용하며 각 값은 비어 있지 않아야 한다. Controller가 실제 값을 사전등록 compiler request에 고정하고 observed request를 그 request와 대조한다. 임의 inherited compiler variables는 허용하지 않는다. 기존 CJR 폐쇄 목록과 C#/CPP parser-only paging 경로는 유지한다.

`definitions`는 새 graph의 외부 caller approval과 같은 guarded snapshot에서 eligibility·full CST·query별 서로 다른 reference invocation의 순서·metadata·request/frame·compiler/library·stderr owner 및 별도 공개 API parity를 모두 확인한 뒤에만 조건부 `STRUCTURAL_ONLY`를 기록한다. 원래 inventory/qualification의 case·edit·tree-role 좌표는 정적 잠재 연결이며 이번 단일 frame 실행 횟수가 아니다. 기존 r1/r10 상태와 semantic/per-case·Windows/Linux·SVC/large·replay/fan-in 미완료를 유지하고 관측 source와 reader revision을 구분한다. 출력 보호·만료·원 caller return 결속은 기존 정의 분류와 같다.

[Windows Small17 실제 관측 보고서](../reports/issue-216-small17-classification.md)는 source/reader `59deb3c`에서 정의17개의 새 수집·외부 승인·guarded projection과 공개 API parity를 기록한다. 기존 역사적35개와 합친52/130은 서로 다른 source/profile 실행의 조건부 분류 누계이며, 원래 사례·semantic·전체 원장 완료를 뜻하지 않는다. PR CI·공식 artifact 및 실제 main 검증은 별도 통합 gate다.

## 승인된 Go compiled metadata의 정의 수준 연결

수동 helper의 `TSGK_QUERY_CLASSIFICATION_STAGE=definitions`는 기존 `positive-batch` 검증 본체를 재사용한다. 외부 caller approval과 같은 guarded run/snapshot에서 eligibility, 독립 full CST/API, native/reference metadata, request/frame·library/compiler 결속 및 별도 public API parity를 확인한 뒤에만 test-only typed 정의 레코드를 만든다. 저장된 `classification-result.json`의 PASS를 신뢰 입력으로 읽는 경로는 없다. 새 public API·CLI 또는 별도 provenance 승인 gate는 추가하지 않는다.

정의 세 개의 ID·정확한 UTF-8 원문·재계산 hash와 완전한 `patterns`, 순서 있는 `capture_names`, 명시적 빈 `predicates:[]`가 anchor·witness·native/reference·query fixture·원장의 유일한 `go/s06-go` 그룹에서 일치해야 `STRUCTURAL_ONLY`로 분류한다. 누락·null metadata를 빈 배열로 바꾸지 않으며 nonempty predicate도 이 분류로 받아들이지 않는다. 고정 원장·qualification 등록부의 원문 hash와 grammar 다섯 pin을 대조하고, 원래 case ID 및 편집별 incremental/fresh 역할로 잠재 좌표를 재구성한다. Go 57 cases·37 edits·94 case steps·131 tree roles는 정의당 131개, 합계 393개 잠재 연결을 만든다. 동일 source를 공유하는 query 동반 case도 제거하지 않는다.

이 결과의 분모는 query 정의 3개다. 잠재 좌표 연결은 해당 정의가 원장에 속함을 검증하며 393회 실행, 사례별 CST/API·capture 일치, runtime eligibility 또는 semantic PASS를 뜻하지 않는다. `query-slot-inventory-r1`의 `STRUCTURAL_CANDIDATE`·`compiled_metadata:UNVERIFIED`, `semantic_required:null`·`NOT_ASSESSED`는 그대로다. r10 predicate-probe 등록과 `full_scope_complete=false`·전체 BLOCKED completion gate에도 합치지 않는다. 관측한 source revision/run/platform과 새 reader revision을 별도로 보존하고 grammar/runtime/compiler·reference/library·승인/graph/witness/registration의 결속을 기록한다. 과거 graph의 재검증은 새 native/reference 관측이 아니다.

출력은 projection을 메모리에서 완성하고 원래 guarded run의 만료 전후 검사를 통과한 정상 반환 뒤에만, retained case 및 assembly/snapshot 밖의 새 물리적 caller-owned 빈 디렉터리에 `O_EXCL`로 쓴다. 만료된 callback은 결과 파일을 게시하지 않는다. 이전 approval·V2 실패·V3 snapshot과 결과를 덮어쓰지 않는다. `TSGK_QUERY_CLASSIFICATION_DEFINITION_OUTPUT`과 `TSGK_QUERY_CLASSIFICATION_READER_REVISION`은 caller가 제공하며 helper의 reader 검사는 40자리 hex 형식만 확인한다. 이 필드 자체는 실행 코드의 인증이 아니다. 결과 채택 시에는 별도의 동결된 reader controller가 실제 commit·source manifest·실행 파일·요청·출력·원 caller return을 외부 pin에 결속해야 한다. 조건부 저장 일관성이라는 한계와 기존 실행 진위·caller 신뢰 전제는 유지한다.

## C# 15개 정의의 빈 source 단일 입력 분류

`TSGK_QUERY_CLASSIFICATION_PROFILE=csharp15-empty`는 [독립 사전 기대값](../../src/testdata/query-semantics/csharp15-classification-anchors.json)의 빈 UTF-8 source, 편집 없음, API 관측 없음, 단일 `compilation_unit` root와 15개 query를 선택한다. `native-query`와 `Args=["single"]`을 사용한다. ABI 기대값은15/15/13/654/26이며, query metadata는 합계91 patterns·125 capture-name 항목과 명시적 `predicates:[]`다. 각 초기 incremental query의 structural/expected capture는 빈 배열이다. 이는 이 입력의 사전 기대값이며 실제 compiler/native/reference 관측을 대신하지 않는다.

C# parser 원문51,472,926바이트는 기존 `full-input-utf8-paged-parser/r1` 경로로 보존한다. 동일 guarded snapshot의 `oracle/` namespace에서 descriptor·분할 파일·원래 logical parser hash와 나머지 grammar pin을 대조한다. 이 예외는 parser 입력 한 개에만 적용하며 batch·문서 크기 제한이나 product core를 확장하지 않는다. Compiler 출력은 `csharp.dll`이며 기존 graph의 `go-library-build`와 `current/go.dll` 문자열은 opaque evidence ID로 재사용한다. C# 정체성은 route·symbol·ABI·원문 grammar pin과 preregistered context로 따로 검증한다.

`definitions` stage는 새 graph에 대한 외부 caller approval과 같은 guarded eligibility·CST·request/frame·compiler/library·native/reference metadata·public parity 검사를 통과해야 한다. 일반 query13개와 fact query2개의 원본 bytes를 각각 `csharp.json` 및 `fact-query-pack.json` pin으로 연결한다. 원장의 `csharp/s06-csharp`에 등록된390 cases·125 edits·515 case steps·640 tree roles에서 정의당640개, 합계9,600개 잠재 연결을 재구성한다. 이 분모는 qualification의 API 관측을 포함한 원래 배치 사례에 속하며, 이번 API=false 단일 입력 관측으로 그 사례들을 검증하지 않는다. 결과는 정의15개의 조건부 `STRUCTURAL_ONLY` 분류이며 사례별·semantic 완료와 r1/r10 상태는 유지한다. 출력 보호와 reader/관측 revision 구분은 Go 정의 분류와 같은 조건을 따른다.

Go와 C#의 공통 수동 guard는 reference가 사용한 library가 compiler 증거의 `current/go.dll`과 node·path·role·bytes·SHA-256 모두에서 같은 member인지 직접 확인한다. 각각 올바른 별개 library pin을 가진 graph도 이 결속을 대신하지 못한다. Core cache가 선택한 registration invocation과 capture를 읽는 `native-<case>`는 같은 invocation ID·node ID 및 `NATIVE_DRIVER` kind여야 한다. Native stderr는 이 실행의 같은 native owner가 보존한 실제 member를 process의 stderr byte count와 대조한다. C#은 명시적인0바이트 stderr member도 요구하며, 기존 Go 자료는 count가0일 때만 member 생략을 허용한다. Stream pin·owner·byte count의 모순은 분류 결과로 채택하지 않는다.

## C·Java·Rust 13개 정의의 빈 source 단일 입력 분류

닫힌 수동 profile `c4-empty`, `java5-empty`, `rust4-empty`는 각각 [C4](../../src/testdata/query-semantics/c4-classification-anchors.json), [Java5](../../src/testdata/query-semantics/java5-classification-anchors.json), [Rust4](../../src/testdata/query-semantics/rust4-classification-anchors.json)의 사전 기대값을 선택한다. 세 profile 모두 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`이며, step0의 `NO_ERROR`와 named root 하나, 각 query의 빈 structural/expected capture를 요구한다. Root는 각각 `translation_unit`, `program`, `source_file`이다. ABI의 language/runtime/min-compatible 값은15/15/13이고 symbol/field 값은 각각403/40,323/40,360/31이다. Query metadata 합계는37 patterns·36 capture-name 항목이며 모든 predicate 목록은 명시적 빈 배열이다. 이 값은 정적 기대값이며 실제 실행 관측을 대신하지 않는다.

세 parser는 각각5,611,460·2,600,326·7,448,615바이트로 모두16MiB 이하이며 기존 `full-input-utf8/r2`만 사용한다. C와 Java의 native source closure는 parser와 header3개이며 scanner를 compiler argv에 넣지 않는다. Rust는 scanner를 포함한5개 source pin을 요구한다. Compiler 출력은 각각 `c.dll`, `java.dll`, `rust.dll`이고 native capture namespace는 `current/native/`다. 기존 opaque library evidence ID와 공통 guard를 재사용하며 C# paged parser 경로나 public API/schema를 확장하지 않는다. 새 세 profile은 실제 stderr가0바이트일 때 process JSON에 `stderr_bytes: 0`을 기록하고 capture에는 명시적0바이트 stderr member를 보존한다. 비어 있지 않은 stderr도 보존하며, 어느 경우든 같은 native owner와 실제 process byte count에 결속한다. 이 guard는 stderr가 항상 비어 있음을 요구하지 않는다.

`definitions` stage는 원래 query source 파일의 정확한 bytes/hash, 원장의 유일한 ordinary 그룹과 qualification grammar·query 순서를 검사한다. C4의58 cases·18 edits·76 case steps·94 tree roles는376 potential links, Java5의40·32·72·104는520, Rust4의33·14·47·61은244를 만든다. 합계13개 정의와131 cases·64 edits·195 case steps·259 tree roles·1,140 potential links이며 실행 횟수가 아니다. 같은 live guarded snapshot에서 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata·public API parity와 외부 caller approval을 통과해야 조건부 `STRUCTURAL_ONLY` 레코드를 쓸 수 있다. 저장된 PASS와 synthetic metadata만으로 projection하지 않는다.

수동 환경이 없는 helper는 SKIP이며 실제 Windows native/reference 관측은 별도로 동결·검토한 bounded 실행에서만 채택한다. 이 구현과 정적 회귀 자체는 새 실행을 관측하지 않았으며 Linux/macOS 관측이나 원래 batch 사례별·semantic 완료를 만들지 않는다. r1의 `STRUCTURAL_CANDIDATE`·`UNVERIFIED`·`NOT_ASSESSED`·`semantic_required:null`, r10의 `full_scope_complete=false`와 전체 BLOCKED는 유지한다. C++4는 이 닫힌 profile에 포함하지 않는다.

## C++ 4개 정의의 별도 parser paging 분류

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=cpp4-empty`는 [CPP4 사전 기대값](../../src/testdata/query-semantics/cpp4-classification-anchors.json)의 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`을 선택한다. Step0은 `NO_ERROR`, named `translation_unit` root 하나와 query별 빈 structural/expected capture를 요구한다. ABI 기대값은15/15/13/621/53이고, 원래 query4개의 metadata는 합계11 patterns·11 capture-name 항목·명시적 `predicates:[]`다. 이 기대값과 helper의 기본 SKIP은 실제 compile/native/reference 관측을 대신하지 않는다.

CPP parser 원문49,598,509바이트는 기존 `full-input-utf8-paged-parser/r1`으로 보존한다. Parser에만16,777,216·16,777,216·16,044,077바이트의3 parts를 사용하며 logical 원문 hash, descriptor와 모든 part·owner를 같은 guarded eligibility snapshot에서 대조한다. Scanner와 header3개의 원문 pin·순서, compiler argv 및 출력 `cpp.dll`도 고정한다. `oracle/` namespace와 기존 opaque `go-library-build`·`current/go.dll` evidence ID를 재사용하되 CPP 정체성은 route·symbol·ABI·grammar 원문과 사전등록으로 검증한다. CPP는 CJR의 닫힌 C/Java/Rust map과 별도이며, 그 map의 CPP 거부와 `full-input-utf8/r2` parser16MiB 한도를 완화하지 않는다. 다른 입력·library·문서·출력·시간·memory 한도도 늘리지 않는다.

CPP4·Kotlin5·Swift8이 profile별 pin으로 공유하는 paged ordinary loader는 CPP에서 `cpp.json`의 정확한 원문·hash·query 순서와 원장의 유일한 `cpp/s06-cpp` 그룹을 대조한다. C# fact-query-pack loader를 복제하지 않는다. 원래77 cases·13 edits·90 case steps·103 tree roles는4개 정의와412 potential links를 만든다. 이는 빈 입력 관측412회나 원래 사례별 CST/API·capture·runtime/semantic 완료를 뜻하지 않는다. `definitions`는 새 graph 밖 caller 승인, 같은 guarded snapshot의 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata 및 별도 public API parity를 통과한 뒤에만 조건부 `STRUCTURAL_ONLY`를 기록한다. 네 query는 각각 서로 다른 reference invocation에 연결되며, 한 invocation의 batch 응답 여러 개를4회 실행으로 세지 않는다. Invocation ID 자체는 opaque identifier다. 실제 native stderr가0바이트이면 명시적 빈 member를 보존하며, 비어 있지 않은 stderr도 같은 native owner·process byte count와 결속한다. 출력 소유권·관측/reader revision 구분·완료/정리 검사는 기존 분류 guard를 유지한다. r1의 미검증 상태와 r10 `full_scope_complete=false`·전체 BLOCKED, 전체 #216/#195 OPEN은 이 부분 분류로 바뀌지 않는다.

## Kotlin 5개 정의의 parser paging 분류

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=kotlin5-empty`는 [Kotlin5 사전 기대값](../../src/testdata/query-semantics/kotlin5-classification-anchors.json)의 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`을 선택한다. Step0은 `NO_ERROR`, named `source_file` root 하나와 query별 빈 structural/expected capture를 요구한다. 빈 입력 root는 고정 `parser.c` state1의 EOF가 자식 0개 `source_file`로 reduce되는 정적 판정에 근거해 관측 전에 고정한 사전 기대값이다. 실제 parse·compile·query 결과는 별도 관측 기록으로 확인한다. ABI 기대값은15/15/13/304/9이고, 원래 query5개의 metadata는 합계14 patterns·14 capture-name 항목·명시적 `predicates:[]`다. 기대값과 helper의 기본 SKIP은 실제 compile/native/reference 관측을 대신하지 않는다.

Kotlin parser 원문33,870,017바이트는 CPP4와 같은 `full-input-utf8-paged-parser/r1`으로 보존한다. Parser에만16,777,216·16,777,216·315,585바이트의3 parts를 사용하고, scanner와 header3개의 원문 pin·순서, compiler argv 및 출력 `kotlin.dll`을 고정한다. CPP4와 같이 `oracle/` namespace와 기존 opaque `go-library-build`·`current/go.dll` evidence ID를 재사용하되 Kotlin 정체성은 route·symbol·ABI·grammar 원문과 사전등록으로 검증한다. CPP4와 Kotlin5는 profile별 pin을 가진 하나의 닫힌 paged 검증기를 공유하며 서로의 anchor·plan·grammar closure를 거부한다. Kotlin은 CJR의 닫힌 map·empty-profile 계열과 별도이고 `full-input-utf8/r2` parser16MiB 한도를 완화하지 않는다. 다른 입력·library·문서·출력·시간·memory 한도도 늘리지 않는다.

Kotlin ordinary loader는 `kotlin.json`의 정확한 원문·hash·query 순서와 원장의 유일한 `kotlin/s06-kotlin` 그룹을 대조한다. 원래38 cases·32 edits·70 case steps·102 tree roles는5개 정의와510 potential links를 만든다. 이는 빈 입력 관측510회나 사례별 CST/API·capture·runtime/semantic 완료를 뜻하지 않는다. `definitions`는 graph 밖 caller 승인, 같은 guarded snapshot의 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata 및 별도 public API parity를 통과한 뒤에만 조건부 `STRUCTURAL_ONLY`를 기록하며, 다섯 query는 각각 서로 다른 reference invocation에 연결된다. 실제 native stderr가0바이트이면 명시적 빈 member를 보존하며, 비어 있지 않은 stderr도 같은 native owner·process byte count와 결속한다. r1의 미검증 상태와 r10 `full_scope_complete=false`·전체 BLOCKED, 전체 #216/#195 OPEN은 이 부분 분류로 바뀌지 않는다.

## Swift 8개 정의의 parser paging 분류

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=swift8-empty`는 [Swift8 사전 기대값](../../src/testdata/query-semantics/swift8-classification-anchors.json)의 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`을 선택한다. Step0은 `NO_ERROR`, named `source_file` root 하나와 query별 빈 structural/expected capture를 요구한다. 빈 입력 root는 관측 전에 고정한 사전 기대값이며 근거는 고정 `parser.c`와 `scanner.c`의 정적 판정이다. 시작 state1의 EOF lookahead는 자식 0개 `source_file` reduce 뒤 해당 goto state의 EOF `ACCEPT_INPUT`으로 이어진다. 시작 external state에서 `IMPLICIT_SEMI`·`EXPLICIT_SEMI`는 유효하지 않고 `RAW_STR_PART`·`CUSTOM_OPERATOR`는 유효하다. 입력 끝(lookahead0)에서 scanner의 공백·주석·연산자·raw string 분기는 모두 false를 반환한다. `is_legal_custom_operator(0,0,0)`는 범위표 하한이 모두 U+00A1 이상이라 false이고, raw string 분기는 parse마다 `calloc`으로 새로 만드는 scanner 상태의 `hash_count==0`과 `#` 부재 때문에 false이므로 lexer는 EOF로 진행한다. 실제 parse·compile·query 결과는 별도 관측 기록으로 확인한다. ABI 기대값은15/15/13/596/48이다. symbol 수는 driver가 보고하는 `ts_language_symbol_count`, 곧 `SYMBOL_COUNT 591`과 `ALIAS_COUNT 5`의 합이다. 원래 query8개의 metadata는 합계21 patterns·21 capture-name 항목·명시적 `predicates:[]`다. 기대값과 helper의 기본 SKIP은 실제 compile/native/reference 관측을 대신하지 않는다.

Swift parser 원문30,636,167바이트는 CPP4·Kotlin5와 같은 `full-input-utf8-paged-parser/r1`으로 보존한다. Parser에만16,777,216·13,858,951바이트의2 parts를 사용하고, scanner와 header3개의 원문 pin·순서, compiler argv 및 출력 `swift.dll`을 고정한다. `oracle/` namespace와 기존 opaque `go-library-build`·`current/go.dll` evidence ID를 재사용하되 Swift 정체성은 route·symbol·ABI·grammar 원문과 사전등록으로 검증한다. CPP4·Kotlin5·Swift8·PostgreSQL9는 profile별 pin을 가진 하나의 닫힌 paged 검증기를 공유하며 서로의 anchor·plan·grammar closure를 거부한다. Swift는 CJR의 닫힌 map·empty-profile 계열과 별도이고 `full-input-utf8/r2` parser16MiB 한도를 완화하지 않는다. 다른 입력·library·문서·출력·시간·memory 한도도 늘리지 않는다.

Swift ordinary loader는 `swift.json`의 정확한 원문·hash·query 순서와 원장의 유일한 `swift/s06-swift` 그룹을 대조한다. 원래76 cases·62 edits·138 case steps·200 tree roles는8개 정의와1,600 potential links를 만든다. 이는 빈 입력 관측1,600회나 사례별 CST/API·capture·runtime/semantic 완료를 뜻하지 않는다. `definitions`는 graph 밖 caller 승인, 같은 guarded snapshot의 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata 및 별도 public API parity를 통과한 뒤에만 조건부 `STRUCTURAL_ONLY`를 기록하며, 여덟 query는 각각 서로 다른 reference invocation에 연결된다. 실제 native stderr가0바이트이면 명시적 빈 member를 보존하며, 비어 있지 않은 stderr도 같은 native owner·process byte count와 결속한다. r1의 미검증 상태와 r10 `full_scope_complete=false`·전체 BLOCKED, 전체 #216/#195 OPEN은 이 부분 분류로 바뀌지 않는다.

## PostgreSQL 9개 정의의 parser paging 분류

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=postgresql9-empty`는 [PostgreSQL9 사전 기대값](../../src/testdata/query-semantics/postgresql9-classification-anchors.json)의 빈 UTF-8 source·편집 없음·API=false·`native-query`·`Args=["single"]`을 선택한다. 대상 route는 `postgresql-sql`, native symbol은 `tree_sitter_postgres`다. Step0은 `NO_ERROR`, named `source_file` root 하나와 query별 빈 structural/expected capture를 요구한다. 빈 입력 root는 관측 전에 고정한 사전 기대값이며 근거는 고정 `parser.c`와 `scanner.c`의 정적 판정이다. 시작 state1의 EOF lookahead는 자식 0개 `source_file` reduce 뒤 해당 goto state의 EOF `ACCEPT_INPUT`으로 이어진다. 시작 external lex 상태에서 유효한 external token은 `comment` 하나이고, 입력 끝(lookahead0)에서 scanner의 공백·주석·dollar-quoted string 분기는 모두 false를 반환하며 scanner는 상태를 갖지 않는다. 실제 parse·compile·query 결과는 별도 관측 기록으로 확인한다. ABI 기대값은15/15/13/1280/0이다. symbol 수는 driver가 보고하는 `ts_language_symbol_count`, 곧 `SYMBOL_COUNT 1280`과 `ALIAS_COUNT 0`의 합이고 `FIELD_COUNT`는 0이다. Query 9개의 metadata는 합계33 patterns·41 capture-name 항목·명시적 `predicates:[]`다. 기대값과 helper의 기본 SKIP은 실제 compile/native/reference 관측을 대신하지 않는다. 관측한 root·ABI·capture가 사전 기대값과 다르면 anchor를 사후 수정하지 않고 관측을 기록한 뒤 별도 profile로 다시 사전등록한다.

PostgreSQL parser 원문104,285,731바이트는 Swift8과 같은 `full-input-utf8-paged-parser/r1`으로 보존한다. Parser에만16,777,216바이트 6개와3,622,435바이트 꼬리의7 parts를 사용한다. 이는 BuildFileBytes(104,857,600) 이하 parser가 가질 수 있는 최대 part 수이며, 여유는571,869바이트다. Grammar 원천 디렉터리가 `postgres/`이므로 폐쇄 5개 file의 논리 경로는 `postgres/src/parser.c`·`postgres/src/scanner.c`(4,855바이트)·`postgres/src/tree_sitter/` header3개이고, include와 compiler argv의 parser·scanner 경로도 같은 `postgres/src` prefix를 쓴다. 출력은 `postgresql-sql.dll`이다. `oracle/` namespace와 `go-library-build`·`current/go.dll` evidence ID를 재사용한다. 이 ID 문자열은 route나 grammar 이름을 담지 않는 opaque 식별자이므로 PostgreSQL에서도 그대로 쓰며, PostgreSQL 정체성은 route·symbol·ABI·grammar 원문과 사전등록으로 검증한다. CPP4·Kotlin5·Swift8·PostgreSQL9는 profile별 pin을 가진 하나의 닫힌 paged 검증기를 공유하며 서로의 anchor·plan·grammar closure를 거부한다. PostgreSQL은 CJR의 닫힌 map·empty-profile 계열과 별도이고 `full-input-utf8/r2` parser16MiB 한도를 완화하지 않는다. 다른 입력·library·문서·출력·시간·memory 한도도 늘리지 않는다.

PostgreSQL ordinary loader는 원장의 유일한 `postgresql-sql/s06-postgresql-sql` 그룹을 두 원문 file과 대조한다. 일반 query8개는 `postgresql-sql.json`(8,110바이트)의 정확한 원문·hash·순서를 따르고, 마지막 `facts.declarations`는 `fact-query-pack.json`(9,702바이트)의 `postgresql-sql` route에 있는 유일한 fact query다. 두 file은 호출자가 하나의 envelope에 raw byte로 함께 전달하며 각각의 길이·hash·원장 `query_sources` pin이 정확히 일치해야 한다. envelope 누락·변조·query file 단독 전달, fact route의 누락·중복·순서 변경은 거부하고, fact가 없는 profile에는 이 envelope를 허용하지 않는다. 원래537 cases·64 edits·601 case steps·665 tree roles는9개 정의와5,985 potential links를 만든다. 이는 빈 입력 관측5,985회나 사례별 CST/API·capture·runtime/semantic 완료를 뜻하지 않는다. `definitions`는 graph 밖 caller 승인, 같은 guarded snapshot의 eligibility·독립 CST·request/frame·compiler/library·native/reference metadata 및 별도 public API parity를 통과한 뒤에만 조건부 `STRUCTURAL_ONLY`를 기록하며, 아홉 query는 각각 서로 다른 reference invocation에 연결된다. 각 정의의 `source_file`은 일반 query8개가 `postgresql-sql.json`, fact query가 `fact-query-pack.json`이다. 실제 native stderr가0바이트이면 명시적 빈 member를 보존하며, 비어 있지 않은 stderr도 같은 native owner·process byte count와 결속한다. r1의 미검증 상태와 r10 `full_scope_complete=false`·전체 BLOCKED, 전체 #216/#195 OPEN은 이 부분 분류로 바뀌지 않는다.

이 profile의 첫 Windows guarded 실행(2026-11-06)은 당시 eligibility 논리 처리량 한도(1 GiB)에서 끝나지 않았다. 그 실행에서 core eligibility의 logical 원장이 1,056,664,398바이트에 이른 뒤 parser part hash charge(part마다 `2*(bytes+1)`, 16 MiB part는 33,554,434바이트)가 남은 여유 17,077,426바이트를 넘어 `RESOURCE_LIMIT`(`ELIGIBILITY_WORK_LIMIT`, `parser-part-hash`)로 실패했다. 실패 전에 charge된 part 수는 기록되지 않았다. 7 parts 전체 charge는 208,571,476바이트이고, 남은 part charge만 더한 필요 logical은 charge된 part 수에 따라 약 1.10~1.27 GB(part hash 뒤 charge 약 25 MB까지 포함하면 약 1.12~1.29 GB)로 한도 1 GiB(1,073,741,824)를 넘었다. 위 값은 그 실행의 역사적 기록이다. 이후 approval에 결속된 paged-parser context의 eligibility 논리 처리량 한도는 1.5 GiB(1,610,612,736)로 바뀌었다(#216, 원장을 직접 시작한 eligibility 검증에 한함). 9개 정의는 그 뒤 source/reader `96a4792`(Windows/amd64)에서 capture·assembly·guarded를 처음부터 다시 실행해 분류했고, 첫 실행의 산출물은 입력으로 쓰지 않았다. 재실행 kit stdout의 두 원장 줄(`eligibility work=`, `eligibility ledger at exit`)은 logical 분모가 모두 1,610,612,736이었다. core eligibility 원장은 work 7,823,848/32,000,000, logical 1,121,744,989였다. 시험 쪽 charge를 더한 종료 원장은 work 27,062,128, logical 1,122,555,608이었다. 종료 work 가운데 19,223,528은 시험 helper가 compiler·DLL 바이트를 work로 센 값이다. 이 core 원장으로 역산하면 첫 실행은 parser part 5개를 charge한 뒤 6번째 part에서 실패했다. 이 역산은 두 실행의 charge 순서가 같고 part 전 원장 차이가 7.5 MB보다 작다는 가정에 따른다. 9개 정의는 조건부 `STRUCTURAL_ONLY`(`semantic_assessment=NOT_ASSESSED`)다.

## TSQL 8개 정의의 binary paging 분류 준비

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=tsql8-empty`는 [TSQL8 사전 기대값](../../src/testdata/query-semantics/tsql8-classification-anchors.json)의 빈 UTF-8·편집 없음·API=false·`native-query`·`Args=["single"]`·step0 `NO_ERROR`를 선택한다. source 준비 당시 관측 상태는 **NOT_RUN**이었다. 이후 실제 관측과 조건부 분류는 [TSQL8 보고서](../reports/issue-216-tsql8-classification.md)에 기록한다. 고정 parser의 시작 state1은 EOF action5에서 `program`을 자식0개·production0으로 reduce하고, goto29728의 EOF action42546에서 accept한다. root 하나는 named·parent−1·field null·모든 error/extra/missing flag false·byte/point0이다. scanner의 초기 상태는 quoted identifier ON·pending GO 없음이며 EOF에서 token을 만들지 않는다. 공개 ABI 기대값은 language15, runtime15/min13, symbols **1386**(`SYMBOL_COUNT1385+ALIAS_COUNT1`), fields88이다. 실제 parse·compiled metadata·reference 실행 결과를 기대값 근거로 사용하지 않았다.

원장의 유일한 `tsql/s06-tsql` 그룹은 일반 query6개(`q.structure`, `q.r6-structure`, `q.r6-begin`, `q.probe-v16`, `q.probe-v22`, `q.issue128-ownership`) 뒤에 `facts.declarations`, `facts.dynamic_sql` 순서다. query file11,934바이트와 fact pack9,702바이트를 각각 원문 envelope·길이·SHA·원장 `query_sources`로 결속한다. 사전 pattern 수는 **4/9/2/2/1/3/20/14**, capture-name 수는 **4/10/2/2/1/3/30/24**이며 최초 등장 순서와 명시적 `predicates:[]`를 고정한다. dynamic SQL query의 독립 `";"`도 한 pattern이고 `(program _)`는 자식을 요구하므로 빈 root에서 capture하지 않는다. 원래 분모1000 cases·991 edits·1991 case steps·2982 tree roles·23856 potential links와 ordinary batch/API=true 등록을 보존한다. 이 분모는 빈 입력 관측 횟수가 아니다.

route는 `tsql`, symbol은 `tree_sitter_mssql`, grammar는 고정 parser68,647,192바이트(16MiB part4개+1,538,328바이트), scanner14,415바이트와 header3개다. TSQL만 `full-input-utf8-paged-binaries/r1`을 요구하고 기존 CPP4·Kotlin5·Swift8·PostgreSQL9는 parser-only context를 유지한다. 수동 native capture는 기존 `semanticBuildFilesIn`으로 driver/parser를 수집한다. 이 분류 경로는 driver와 library 모두 paged descriptor를 요구하며 manifest/r7 semantic claim을 활성화하지 않는다.

조립 입력의 library descriptor는 `<root>/library/tsql.dll.pages.json`이다. logical 경로는 opaque evidence ID `current/go.dll`, owner는 `go-library-build`, part 경로는 `current/go.dll.part-%05d.bin`이며 controller 보관 파일은 `<root>/library/go.dll.part-%05d.bin`이다. graph에는 `current/go.dll.pages.json`(`library-paged-descriptor`)과 각 part(`library-paged-part`)를 넣는다. 실행 원본 `tsql.dll`은 graph의 plain member로 복사하지 않는다. driver는 `oracle/capture/driver.pages.json`과 같은 node의 part를 사용한다. registration/invocation은 descriptor의 물리 pin을, native node·witness·build·reference argv/response·projection guard는 검증된 logical SHA를 사용한다.

guarded compiler 재검사는 같은 live `s.member` snapshot의 library descriptor를 decode해 `Logical.Bytes`·`SHA256`를 얻고, compiler 출력과 reference library의 물리 pin 일치 및 8개 response의 hex64 `Grammar.LibrarySHA256` 동일성을 확인한다. descriptor의 schema·owner·logical kind/path·크기·SHA도 교차 확인한다. library를 재조립하지 않고 크기만 compiler 검증기로 넘기며 D3(a)의 compiler byte work·library part work·logical `2×(bytes+parts)`는 유지한다. projection은 같은 완료된 live cache/result에서 binary context·native invocation ID/kind·registration invocation 일치와 driver descriptor path/role/NodeID를 확인한 뒤 witness의 `Native.ExecutableSHA256`를 재사용한다. library SHA는 위 response·descriptor 결속으로 재사용하며 driver/library part를 다시 읽거나 hash하지 않는다. part-swap·missing·extra 및 전체 hash 반례는 core paging 시험이 담당하고 합성 동등성 시험이 재사용 SHA와 `nativePagedDriver`/`nativePagedLibrary` 결과를 대조한다. descriptor 읽기/decode charge는 남는다. 물리16MiB, logical binary64MiB, shared256MiB, work32,000,000, owner/bound eligibility1.5GiB를 변경하지 않는다. Γ1에서 남은 charge로 두 원장의 예상치를 갱신하며 실제 rehearsal 전 90% gate 통과를 주장하지 않는다.

새 외부 approval, 같은 guarded snapshot의 CST/ABI/metadata·compiler/library·8개 서로 다른 reference invocation·공개 API parity를 실제로 통과한 뒤에만 `definitions` 결과를 채택한다. source 시험은 실행 관측이나 8/8 분류 PASS가 아니다. 원장 `STRUCTURAL_CANDIDATE`·`compiled_metadata=UNVERIFIED`·`semantic_required:null`, r10 `full_scope_complete=false`, source 준비 당시 누계122/130을 승격하지 않았다. 이후 A6의 조건부 구조 분류 누계는130/130 local candidate이며([TSQL8 보고서](../reports/issue-216-tsql8-classification.md)), 의미 미평가와 #216/#195 미완료는 유지한다.

Windows의 source/reader `29ecdef`에서 실제 관측·외부 caller approval·guarded 정의 검증을 확인한 범위는 [TSQL8 보고서](../reports/issue-216-tsql8-classification.md)에 기록한다. 8개는 조건부 구조 분류이며 130/130은 서로 다른 source/profile 실행의 local candidate 누계로, 원장의 미검증 metadata·semantic 미평가와 #216/#195 미완료를 유지한다.

## PowerShell3 단일 byte frame 정의 분류

닫힌 수동 profile `TSGK_QUERY_CLASSIFICATION_PROFILE=powershell3-tiny`는 [PowerShell3 사전 기대값](../../src/testdata/query-semantics/powershell3-classification-anchors.json)의 UTF-8 source `;`(1 byte, SHA-256 `41b805ea7ac014e23556e98bb374702a08344268f92489a02f0880849394a1e4`)·편집 없음·API=false·`native-query`·`Args=["single"]`·step0 `NO_ERROR`를 선택한다. 이름의 `-tiny`는 빈 source가 아님을 뜻한다. PowerShell root `program`은 nonnullable이라 빈 입력은 error 경로로 가므로 빈 입력 profile이 존재할 수 없고, 가장 작은 유효 입력 중 query 대상 node를 만들지 않는 `;`를 고정한다.

아래 CST는 **관측 전 사전 기대값**이다. 근거는 고정 `parser.c`의 lex·parse table·action을 읽은 정적 도출이며(`;` shift 뒤 EOF에서 `empty_statement`→`statement_list`→`program` reduce와 accept), 실제 runtime parse는 아직 관측하지 않았다. 세 reduce는 production 0이라 모든 field는 null이고 scanner는 이 경로에서 호출되지 않는 것으로 추정한다(시작 상태에는 external lex 상태가 없지만 `;` shift 뒤 상태는 정적으로 확인하지 않았다).

| # | parent | type | named | byte span |
|---:|---:|---|---|---|
| 0 | -1 | `program` | true | 0..1 |
| 1 | 0 | `statement_list` | true | 0..1 |
| 2 | 1 | `empty_statement` | true | 0..1 |
| 3 | 2 | `;` | false | 0..1 |

모든 node의 field는 null이고 extra·is_error·has_error·is_missing은 false이며 point는 (0,0)..(0,1)이다. ABI 기대값은15/15/13/445/16이고 symbol 445는 `SYMBOL_COUNT 445`와 `ALIAS_COUNT 0`의 합이다. 세 query(`q.structure`·`q.probe-v7-chain`·`q.probe-v7-blocks`)의 metadata는 합계6 patterns·6 capture-name 항목·전역 고유 이름6개와 명시적 `predicates:[]`이며, CST에 `enum_statement`·`class_statement`·`function_statement`·`command`·`pipeline_chain_tail`·`named_block`이 없으므로 structural/expected tuple은 빈 배열 후보다. 실제 parse·compile·query 결과, native ABI, reference의 같은 source frame과 CST, stderr 0바이트 member는 별도 관측으로 확인한다. **관측한 CST·ABI·capture가 이 기대값과 다르면 anchor를 사후 수정하지 않고 관측을 기록한 뒤 별도 profile로 다시 사전등록한다.**

Parser `src/parser.c` 6,290,093바이트는 16MiB plain-input 한도 안이라 paging 없이 `full-input-utf8/r2`를 사용한다. Grammar 폐쇄는 `src/parser.c`·`src/scanner.c`(3,607바이트, registry origin `patched`)·`src/tree_sitter/` header3개의 5개 file이며 include는 `src`, 출력은 `powershell.dll`이다. 원래 ordinary qualification의 API=true와 cases/edits/case steps/tree roles/potential query-tree-role 39/24/63/87/261은 이번 정의3개의 실행 분모가 아니다.

이 profile은 unpaged empty-profile 검증기를 공유한다. 검증기는 profile이 고정한 정확한 source bytes(기본값은 빈 source)와 전체 CST(기본값은 root 1개)만 허용하도록 일반화됐다. 기존 empty profile은 새 필드를 비워 두므로 빈 source·root 1개 요구가 그대로 유지되고, 임의의 유효 source나 다른 profile의 CST는 어느 profile에서도 거부된다.

이후 절차는 다른 닫힌 profile과 같다. 실제 parse가 사전 기대값과 일치하는 새 current graph·서로 다른 query별 reference invocation·provenance·외부 caller approval과 같은 guarded snapshot의 정의 projection·공개 API parity 후에만 조건부 `STRUCTURAL_ONLY`를 기록한다. 원장 `STRUCTURAL_CANDIDATE/UNVERIFIED`, semantic `NOT_ASSESSED`, `semantic_required:null`과 전체 #216/#195의 미완료는 유지한다.

## Ordinary Go의 첫 single-frame batch probe

수동 분류 helper의 `TSGK_QUERY_CLASSIFICATION_PROFILE=positive-batch`는 기존 등록 `go/s06-go/probe-go-B04-P-6`의 50-byte 정상 source, 편집 없음, API 관측, 세 query를 선택한다. 명시적 profile이 없으면 기존 빈 source Go3 정의 검사를 유지한다. 별도 C# profile은 위 계약으로 선택하며 미등록 profile은 거부한다. [고정 사전 기대값](../../src/testdata/query-semantics/go-positive-batch-anchors.json)은 수작업 full CST 25개 node, ABI·metadata, `q.structure`의 function capture 하나와 나머지 두 query의 빈 capture를 소유한다. 원 등록의 `NO_ERROR`, `for_statement`·`block`·`break_statement`와 `for_statement` 범위 `30..47`도 검사한다.

이 probe는 `native-query-batch` 한도로 driver의 `Args=["batch"]`를 실행하며 noninteractive `Spec.Stdin`에 정확히 한 r4 frame을 전달한 뒤 정상 종료·검증된 cleanup·잔여0을 요구한다. 같은 guarded eligibility snapshot에서 실제 source·API·query·run·producer·OS 한도·mode, native node identity와 capture descriptor 소유권, stdin 한 frame, stdout 한 frame과 raw의 동일성, process receipt의 동일성을 대조한다. 요청과 witness의 API·node limit·points도 결속하며 응답 API 전체를 [정적 사전 기대값](../../src/testdata/query-semantics/go-positive-batch-api.json)과 대조한다. 기존 interactive `OracleRunBatch` 전체를 재현하거나 producer의 `Batch+SourceWitness` 거부를 완화하지 않는다. r4를 다른 revision으로 재표기하지 않는다.

입력 종료는 고정 runner가 stdin bytes를 쓴 뒤 pipe를 닫고 고정 batch driver가 다음 header 없이 정상 종료한 동작에서 추론한다. Receipt는 Write·Close 성공이나 C stdio의 EOF/읽기 오류를 별도로 증언하지 않으므로 host/pipe 오류까지 배제하는 직접 EOF 인증으로 부르지 않는다. 실제 실행 진위와 정상 고정 도구·pipe 전제는 graph 밖 provenance 검토와 caller 신뢰 범위다.

실제 수집 전 source·controller·도구·시험 binary·preregistration과 독립 사전 검토를 고정한다. 수집 뒤에는 기존 build9/unit4/control53/assertion36 closure, current native/reference full CST와 capture, graph 밖 새 caller 승인 및 provenance 검토가 필요하다. 수동 검사의 기본 SKIP이나 정적 기대값은 실제 실행 근거가 아니다. 성공도 Windows의 이 사례 세 좌표에 대한 조건부 `STRUCTURAL_ONLY`이며 semantic evaluation, Go 전체 57개 사례, 오류 복구 15개 좌표, 전체 #216의 완료를 뜻하지 않는다.

이 도구는 고정 Tree-sitter 0.27.1의 별도 계측 reference다. Stock CLI full-field parity는 계속 **BLOCKED**이며, 이 도구의 빌드·owned 검사 성공은 전체 `query_semantics` qualification 완료가 아니다. 기존 oracle/replay/qualification revision과 보장 범위는 유지한다.

## 독립성 및 identity

Upstream은 `tree-sitter/tree-sitter`의 `10eb5ac119eec9cb4267a8e035066473983f5f3a`다. [instrumentation.patch](../../src/dev/query-reference/instrumentation.patch)는 Rust binding에 읽기 전용 query pointer와 별도 raw capture iterator만 추가한다. 기존 iterator의 기본값은 predicate 평가를 유지한다. 별도 iterator는 predicate 탈락 match를 제거하지 않으며, emitter가 변경하지 않은 upstream `QueryMatch::satisfies_text_predicates`에 iterator가 반환한 원래 match와 원 source를 advance 전에 넘긴다. Upstream은 predicate에 사용되는 capture 단계를 indefinite로 표시하고 선행 단계에 전파한다. Emitter도 각 반환 snapshot에서 지원하는 모든 `ONE` operand의 실제 occurrence가 정확히 하나인지 확인한 뒤 평가한다. Predicate와 무관한 뒤쪽 capture까지 처음 반환 시 이미 모두 발견됐다고 가정하지 않는다. Kit 평가기를 호출하거나 kit 결과를 읽지 않는다.

원본 workspace·library manifest를 유지하고 emitter member 하나를 더한다. 원본 lock의 기존 package·version·checksum·dependency를 바꾸지 않는다. `build.py`는 새 package 하나만 추가됐는지 확인한 뒤 `--offline --frozen`으로 선택 package만 빌드한다. 전체 CLI·WASM·bindgen 빌드는 이 경로에 포함하지 않는다. 실제 selected dependency/features의 전체 `selected-tree.json` receipt hash와 종료 전 불변성, metadata, 실행 요청·결과, 원본 archive·patch·emitter·manifest·lock·도구·binary hash를 별도로 보존한다. Build 도구 목록은 실제 Python interpreter의 절대 경로·파일 hash·`sys.version`도 포함하며 build 전후 hash가 같아야 한다. Cargo offline은 의존성 취득 방식이며 network sandbox를 뜻하지 않는다.

`tsgk-instrumented-reference-build/r1`의 `complete`는 빌드 절차 완료다. Caller가 독립적으로 선택한 upstream·도구 취득 근거 및 등록 hash와 연결해야 한다. Evidence가 자체 제출한 identity/hash만으로 reference 자격을 만들지 않는다. 공개 source, compiler/standard-library provenance, 실제 feature closure와 실행 문맥의 신뢰 검토는 자격 검증에 계속 필요하다.

Build와 owned controls는 기존 wire encoder로 요청 bytes를 한 번 만들고 supervisor stdin에 전달하는 동일 bytes를 실행 전에 `*-request.json`으로 저장한다. `batch-truncated-request.json`은 `StdoutBytes=1024`로 바꾼 요청을 보존하며 원래 `batch-sixteen-request.json`은 변경하지 않는다. 요청 저장이 실패하면 그 실행을 시작하지 않는다. 이 보존은 producer의 송신 bytes와 저장 bytes의 일치이며 실행 진위·후속 변조 방지나 reference 자격을 인증하지 않는다. 과거 누락 요청을 재생성해 원자료로 승격하지 않으며 `build.py` 변경 뒤에는 새 입력 hash에 맞는 build identity가 필요하다.

Supervisor receipt는 명시적 `tsgk-reference-process/r1`이며 정확히 `schema`, `execution`, `stdout_base64`, `stderr_base64`를 갖는다. 두 stream은 UTF-8 문자열 변환 없이 원 bytes를 표준 padding 있는 canonical base64로 보존하고 nil·빈 stream은 빈 문자열이다. 소비자는 receipt 원문을 strict UTF-8로 읽고 모든 깊이의 중복 JSON key를 거부한 뒤 schema·필드·base64 type/형식·canonical 재인코딩과 byte count를 검사한다. 비잘림 stream은 원 bytes 길이가 execution count와 같아야 하며 잘린 stream의 보존 prefix는 관측 count 이하이고 실행 완료로 채택하지 않는다. Rust JSON/NDJSON·unit/tool text를 해석할 때만 strict UTF-8로 decode하며 replacement decode나 UTF-16/32 자동 인식을 사용하지 않는다. stderr 진단은 원 bytes와 구분한다. 과거 익명 `stdout`/`stderr` string receipt는 이 raw closure의 근거로 수용하지 않으며 새 supervisor·Python 입력 identity로 build와 controls를 다시 실행해야 한다. Process·stream·small reference/replay 한도는 유지하고 base64 envelope가 offline 파일 한도를 넘으면 부적격으로 남긴다. 이 receipt 보존은 전체 closure·실행 진위·query_semantics 자격 완료를 뜻하지 않는다.

## 결과와 완료 조건

Emitter 인자는 절대 library path, symbol, source path, query path, query ID, 사전 등록 library SHA-256이다. Source는 UTF-8 32 MiB 이하, query는 UTF-8 64 KiB 이하, library는 64 MiB 이하의 regular file이다. Source·query는 읽은 bytes에서 hash와 결과를 계산한다. Library는 실행 전 예상 hash와 비교하고 실행 후 다시 비교한다. 이 검사는 악의적인 동시 파일 교체나 native code 자체를 격리한다는 보장이 아니다. Build archive는 128 MiB 상한의 단일 buffer로 읽어 pinned SHA-256을 확인하고 동일 bytes에서만 tar를 해석한다. 완료 identity 직전 원 archive 경로도 다시 hash하여 남아 있는 변경을 거부한다. 검토·고정한 생성 grammar만 별도 process에서 실행한다. 출력 소유권은 완료된 [#215](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/215)의 조건부 범위와 [trust/execution](trust-and-execution.md#215-출력-handle-결속과-미완료-경계)을 따른다. 이 경로의 hash 검사는 그 소유권 보장을 대체하지 않는다.

`tsgk-instrumented-query-reference/r1`은 다음을 제공한다.

- Fresh parse의 전체 preorder CST: parent·field·type, named·extra·is_error·has_error·is_missing, byte·point 전체 필드.
- 실제 compiled query의 pattern 수·capture 이름·predicate 단계와 quantifier.
- 원 capture 순서와 pattern/capture 번호를 보존한 structural·evaluated 전체 stream. Capture의 필드는 실제 captured node에서 읽고 독립 CST의 node identity 및 모든 필드와 대조한다. Span만으로 node를 찾지 않는다.
- 원 runtime match ID와 최초 structural 등장 순서의 match 번호 mapping. Predicate 탈락 뒤 번호를 다시 매기지 않는다.
- 실제 사용 한도, cursor 종료·match-limit 초과 여부 및 `complete`.

Raw capture iterator는 아직 완료되지 않은 definite match의 capture를 먼저 반환할 수 있다. 따라서 같은 runtime match ID의 capture signature는 기존 prefix를 그대로 보존하며 뒤에 새 occurrence를 추가할 수 있다. Prefix의 변경·축소와 pattern·upstream predicate 결정 변경은 거부한다. 새 tail만 group capture 한도에 더하고 occurrence 상태를 확장한다. 각 capture **occurrence index**는 정확히 한 번 반환돼야 하며, cursor 종료 시 누락이 있으면 실패한다. 서로 다른 occurrence의 동일한 capture 값은 보존한다. 원 stream의 순서와 최초 등장에 따른 match 번호를 유지하며 단순 개수 합계나 정렬·중복 제거로 이 검사를 대체하지 않는다.

성공은 모든 검사가 끝났고 cursor가 소진됐으며 한도를 넘지 않았을 때만 `COMPLETED`, `complete: true`다. 100,000 node·50,000 stream capture·200,000 group capture·100,000 match·내부 wall 3초를 유지한다. Predicate는 `eq?`, `not-eq?`, `any-of?`, `not-any-of?`의 지원 operand 형태와 `ONE` capture만 허용한다. 그 밖의 연산·directive·quantifier·encoding은 이 기준의 부적격이며 범위를 넓혀 통과시키지 않는다. 원본 C query cursor의 기본 range가 byte0에서 끝나는 capture를 제외하는 동작은 빈 source 대조에 반영한다.

## 공통 parse를 재사용하는 query batch

기존 여섯 인자와 단일 r1 결과를 유지한다. 별도 `--batch LIBRARY SYMBOL SOURCE LIBRARY_SHA256 QUERY_ID QUERY_FILE ...`는 사전 등록한 서로 다른 ID의 query 1–16개를 받는다. Source·library·fresh CST를 한 번 준비하고 query마다 별도 compiled query·cursor·match/occurrence 상태를 사용한다. 같은 source version의 공통 계산만 재사용하며 query 검사·독립 기대값·전체 CST 비교를 생략하지 않는다. Source/query/library 크기 및 query별 node/capture/match 한도는 그대로다.

출력은 query 순서의 기존 `tsgk-instrumented-query-reference/r1` NDJSON과 마지막 `tsgk-instrumented-query-reference-batch/r1` footer다. Footer는 정확한 `schema`, `status`, boolean `complete`, 순서 있는 `queries`, 정수 `responses_completed`만 갖는다. 전체 stdout은 newline 포함64 MiB 이하이며 Query file의 읽기·크기·UTF-8 검사는 그 query 차례에 수행하므로, query 하나가 이 검사나 compile·실행에서 실패하면 앞선 완료 r1을 유지한 채 그 실패 r1과 실패 footer를 출력하고 이후 query를 실행하지 않는다. 앞서 완료한 prefix를 전체 batch 완료로 세지 않는다.

`checks.batch`는 사전 등록 ID·개수·순서, 모든 완료 response, 정확한 완료 footer·마지막 newline·중복 JSON key 부재를 확인한다. 호출자는 이 검사 전에 실제 supervisor의 exit0·EXITED·잘림 부재·cleanup과 요청한 자원 한도를 별도로 확인해야 한다. 이 helper는 전체 reference 자격·CST 의미·독립 anchor 검사를 대신하지 않는다. 각 r1 문서에는 기존 offline 연결 검사를 적용한다.

Library load·공통 parse/CST export는3초에 포함하고 그 경과 시간을 각 query의3초에서 공제한다. Query deadline은 batch 시작부터 `3초 × query 수`도 넘지 않는다. 별도 watchdog thread는 공통 준비와 활성 query의 deadline을 감시하며 callback이 없는 query compile·predicate·library 재검증·직렬화·출력 중에도 만료하면 process를 exit2로 끝낸다. Watchdog 시작 뒤에는 성공·실패 r1과 footer·flush까지 이미 시작한 마지막 deadline을 새 예산 없이 유지한다. Watchdog timeout이나 출력 실패는 정상 JSON 또는 footer를 보장하지 않으며 완료 결과가 아니다. 운영체제 scheduling과 native process 격리는 기존 supervisor의 별도 wall·memory·cleanup 검사를 계속 요구한다. Owned batch16 검사는1 GiB·50초 supervisor에서 실행하며 단일 query의 기존10초 supervisor 문맥을 변경하지 않는다.

Owned 검사는 batch1/16의 전체 r1 결과가 각 단일 실행과 일치하는지 확인하고 공통7-node CST 및 기존 수작업 predicate·중복 anchor도 유지한다. 중간 compile 실패와 후속 query file 누락·크기 초과·UTF-8 실패에서 앞선 완료 prefix 보존과 이후 미실행, 빈 batch·17개·짝 없는 인자·중복 ID·query 크기 초과를 거부하고 실제 supervisor stdout 제한 뒤 cleanup과 전체 완료 거부를 확인한다. 이는 batch 정합성 검사이며 전체 지원 workload·3-host qualification 또는 성능 개선 수치가 아니다.

## 실행 및 검사

Python 3.11 이상과 이미 준비한 절대 경로의 Cargo/Rust 1.97.1(`-vV`의 유일한 `release` 필드와 정확히 일치), C compiler·archiver·Git, 고정 source archive와 checksummed vendor directory가 필요하다. Global 설치나 설정을 바꾸지 않는다. 새 task directory에서만 source를 추출·계측하며 기존 reference checkout을 수정하지 않는다. Source archive는 해당 upstream commit의 `git archive --format=tar` bytes이며 SHA-256은 `6025ffd838c63bb011eeec9b259891ae0e051a3db9aa24c8812679f7462a50a9`다.

먼저 `CGO_ENABLED=0 go build -o <supervisor> ./src/dev/query-reference-run`으로 기존 runner를 사용하는 supervisor를 만든다. Shell 대신 정확한 argv·최소 환경으로 실행하고 stdout/stderr·wall·메모리·process tree 종료를 기록한다. Windows Job Object와 delegated Linux cgroup은 hard memory cap을 사용한다. macOS는 기존 runner의 sampled memory·process group 문맥을 그대로 표시하며 hard cap으로 보고하지 않는다. Linux에서는 두 Python 명령 모두 검증한 delegated `--cgroup-parent`가 필요하다.

```text
python src/dev/query-reference/build.py --archive <absolute-tar> --vendor <absolute-vendor> --cargo <absolute-cargo> --rustc <absolute-rustc> --cc <absolute-compiler> --ar <absolute-archiver> --git <absolute-git> --supervisor <absolute-supervisor> --work <new-absolute-build-directory> --target <target>
python src/dev/query-reference/check.py --reference <absolute-built-binary> --identity <build-directory/identity.json> --supervisor <absolute-supervisor> --cc <absolute-compiler> --work <new-absolute-control-directory>
```

대상은 `x86_64-pc-windows-gnu`, `x86_64-unknown-linux-gnu`, `aarch64-apple-darwin`이다. Build는 2 compiler job·4 GiB·15분 안에서, owned reference 실행은 1 GiB·10초 안에서 감독한다. 실패·출력 잘림·정리 미확인은 성공 receipt를 만들지 않는다. 입력이나 도구가 빌드 중 변경됐으면 identity 완료 전에 거부한다. 의도한 patch·manifest·lock 변경 뒤 source와 vendor 전체 파일 snapshot을 selected-tree 실행 전에 고정하고, 실행 직후와 build·unit 이후 다시 비교한다. 원본 binding hash가 정확할 때만 zero-context patch를 적용한다. 세 Python 진입점(`acquire.py`·`build.py`·`check.py`)은 `-O`·`PYTHONOPTIMIZE` 실행을 인자 처리 전에 거부한다.

Rust unit은 유지·탈락 match 모두에서 capture 누락·중복·범위 오류를 검사한다. `check.py`는 owned grammar의 전체 7-node CST와 전체 capture·metadata를 손으로 고정해 네 연산의 유지·탈락, capture 간 비교, 복수 predicate, structural, 두 capture의 유지·탈락, 정상 중복 보존, 빈 source, 미지원 연산 거부와 잘못된 library hash 거부를 확인한다. Capture 간 비교는 서로 다른 두 node의 값이 같을 때와 다를 때 `eq?`·`not-eq?`의 유지·탈락도 확인한다. 수정한 source는 별도 fresh parse다. Kit의 incremental/fresh 실행을 했다는 근거로 세지 않는다. 각 실행의 stdout·stderr·runner 결과와 최종 요약은 새 directory에 남는다.

선택한 Rust unit은 기존 occurrence 검사와 batch 인자/예산·watchdog 자식 process(시작 뒤 실패 경로 포함) 검사의 정확한4개 실행·4개 성공·미실행0개를 요구한다. Owned parser·header는 읽은 bytes와 hash를 고정한 task copy에서 컴파일하며 최종 copy·binary·identity·도구·검사 source를 재확인한다. 거부 대조도 정상 process 종료·정확한 exit1·잘림 없는 출력·정리 완료를 요구하므로, 오류 JSON을 먼저 출력한 timeout은 성공한 거부가 아니다. `python src/dev/query-reference/test_pipeline.py -v`는 최적화 실행·0개 unit·입력 변경을 검사한다. 실제 supervisor의 절대 경로를 `TSGK_REFERENCE_SUPERVISOR`로 지정하면 정상 exit1, 오류 출력 뒤 wall 초과, stderr 잘림을 별도 process로 검사하며, 미지정 시 그 시험은 SKIP으로 남는다. Linux에서는 `TSGK_REFERENCE_CGROUP_PARENT`도 필요하다.

## 취득과 Linux hosted 실행 (#216)

`python src/dev/query-reference/acquire.py --upstream <pinned-checkout> --git <git> --out <new-directory>`는 build·실행 없이 입력만 준비한다. 고정 commit과 pristine checkout을 확인하고 `git archive --format=tar` bytes가 위 archive SHA-256과 같은지 먼저 검사한다. upstream `Cargo.lock`의 registry package만 `static.crates.io`에서 받아 lock checksum과 대조하고, 크기·개수·시간·경로를 제한해 `vendor/`와 `.cargo-checksum.json`, `acquisition.json`을 쓴다. 같은 입력에서 이 결과의 vendor snapshot은 Windows build9–build12 입력과 같다(13,738 file, `8fec9dcc2dc84590867a7add1b33ad36be98c0a1b58634f228a5ce3a2632171c`).

수동 workflow `Reference Linux`(`.github/workflows/reference-linux.yml`, `workflow_dispatch`, 공유 heavy lane)는 hosted `ubuntu-24.04`에서 이 취득, Rust 1.97.1 toolchain, supervisor build, delegated cgroup v2를 준비한 뒤 `build.py --target x86_64-unknown-linux-gnu`의 build·Rust unit, `test_pipeline.py`(SKIP 없음), owned `check.py` controls를 실행한다. build identity의 vendor snapshot이 위 값과 다르면 실패하며, 요청·결과 receipt만 `reference-linux-<run>-<attempt>` artifact로 30일 보관한다. 이 실행은 필수 PR check가 아니고 전체 reference 자격·3-host qualification·26 route 대조를 대신하지 않는다. 2026-10-25 사용자 결정으로 macOS(`aarch64-apple-darwin`) 신규 검증은 수행하지 않으며 #243은 NOT_PLANNED로 종료했다. 당시에는 기존 macOS CI와 과거 근거를 유지했다. 이후 #332의 2026-11-07 결정으로 macOS CI도 일시 제외했다([platform](platform-support.md)). 제외 전 근거는 역사로 보존하며 이후 macOS는 NOT_RUN이다. 신규 claim을 macOS SUPPORTED로 승격하지 않는다.

## 전체 지원 대상 연결의 자격 기준

새 qualification은 등록된 필수 `(route, platform, role, case, query, step, tree_role)`를 결과 전에 고정해야 한다. 필수 tuple 0개·claim 누락·미실행·partial·reference 부적격·source witness 누락은 BLOCKED다. Raw/projection/reference·identity 모순이나 변조는 FAIL이다. Predicate 의미 PASS는 완료된 EVALUATED 결과, 독립 full CST·structural stream·metadata 일치, 수작업 anchor와 전체 evaluated capture 기대값, 저장 witness의 재계산을 모두 요구한다.

Native의 field patch 때문에 이 reference와 CST 또는 capture가 달라지면 **INELIGIBLE/BLOCKED**로 별도 집계한다. Kit 값으로 reference의 필드를 채우거나 stock CLI와 같다는 이유로 kit 계약을 완화하지 않는다. 신규 semantic 완료 대상인 전체 26 route의 Windows/Linux와 등록된 추가 role, producer의 opt-in source 보관, 새 claim/reducer/inventory/fan-in 연결 및 실제 main 검증은 후속 완료 조건이다. 이 도구나 owned 대조 하나로 해당 조건을 충족한 것으로 처리하지 않는다.

연결 검증은 기존 `VerifyEvidence`가 해석한 graph와 동일 guard 읽기에서 hash가 확인된 요청 member bytes만 사용한다. 보관 대상을 골라도 다른 member·graph·policy·한도·취소 검사는 그대로 실행하며, 일반 member에 replay-result용 record 한도를 추가하지 않는다. 빈 member는 누락과 구분하고 실패한 member의 bytes나 이전 호출의 snapshot을 재사용하지 않는다.

## 저장 eligibility API

`VerifyQueryReferenceEligibility`의 독립 승인·build/control closure·native/r4·shared budget 정본은 [offline eligibility 계약](query-reference-eligibility.md)이다. 기존 connection r1을 재해석하지 않는 별도 experimental API이며 producer/claim/reducer/inventory/fan-in은 후속이다.

## 저장 데이터 연결 API

`VerifyQueryReferenceConnection`은 [공개 Go API](public-go-api.md)의 experimental offline 연산이다. Caller는 evidence 밖에서 `tsgk-query-reference-connection-registration/r1`을 승인한다. 이 등록은 정확한 evidence policy SHA-256, native/reference node와 빈 값 없는 전체 11-role identity(빈 값은 `REFERENCE_REGISTRATION_INVALID`), witness 등록·source·raw·projection·build manifest·각 reference response의 member path/role/bytes/hash, 승인 binary/library hash와 symbol을 고정한다. Build manifest 자체가 제시한 hash를 승인 값으로 채택하지 않는다. `full-input-utf8/r1` 문맥은 full input·included range 없음·composite format 없음과 build/tool/grammar/platform/run 문맥을 caller가 외부에서 승인했다는 전제다. SVC·UTF-16·CP949·large paging의 자격을 추가하지 않는다.

Caller는 승인 member에 정확히 결속하는 witness 등록 원문도 필수 입력으로 제공한다. 이 원문을 먼저 strict 해석·크기/hash 대조한 뒤 공유 native witness 검증기를 항상 적용하며, graph의 witness member가 없어도 등록 분모와 확보한 source·raw·projection·reference의 검사는 유지한다. Caller 원문이 누락·malformed·pin 불일치이면 `KindInvalidInput`이고, 한도·취소 분류는 기존 계약을 유지한다. Graph member의 undeclared 누락은 BLOCKED이며 declared 파일 부재·owner·bytes 불일치는 FAIL이다. Caller 입력으로 graph 누락을 채우거나 완료 tuple을 늘리지 않는다.

각 observation은 `(query, step)`과 독립 source bytes/hash를 고정한 fresh reference다. Witness 등록의 `required`가 판정 분모이며 observation 수로 줄이지 않는다. 같은 source version의 response를 incremental/fresh 양쪽에 사용해도 각 tuple을 별도로 센다. 공유 native witness 검증기의 tuple별 완료 판단을 재사용하므로 다른 tuple의 누락·모순은 정상 tuple의 완료 수를 지우지 않는다. 전체 witness의 BLOCKED·FAIL 판정과 graph witness member 결속은 유지한다. 승인된 policy/graph anchor·platform·선택 node의 유일성/전체 identity/완료 축은 공유 집계 전제다. 선택 node의 policy/graph 제약은 공유 `VerifyEvidence` pass에서 재사용하며 capped finding 목록에 의존하지 않는다. Observation의 source identity 불일치는 해당 tuple을 완료로 세지 않는다. 중복 member는 해당 tuple을 제외하되 무관한 node 실패나 다른 response 누락은 정상 tuple의 완료 수를 지우지 않는다. 다른 version 재사용은 source identity 불일치다. 기존 native witness 재계산, reference의 독립 full CST 구조·source point·capture/node 연결·match 번호와 runtime mapping, 완료/cursor/한도, 전체 compiled metadata·ordered structural/evaluated stream·독립 manual anchors를 대조한다. Evaluated stream은 structural의 전체 match만 순서를 보존해 선택할 수 있고 중복 occurrence를 삭제하거나 더할 수 없다. Capture가 없는 node도 전체 CST 비교에 포함한다.

기존 graph/policy가 선언한 member가 없거나 hash·identity·context·완료 상태가 모순이면 FAIL이다. 유효한 graph에 아직 관측 node/member가 선언되지 않았거나 일관된 producer 실패·native full CST 부재는 BLOCKED다. 실패 response의 code는 고정된 emitter가 실제 직렬화하는 오류 집합에 속해야 하며, 알 수 없는 code는 `REFERENCE_FAILURE_CODE_INVALID`/FAIL이다. 알려진 code라는 사실은 그 실패 원인의 실제 발생을 입증하지 않는다. 누락이 있어도 확보한 다른 response의 모순 검사는 계속한다. 유효한 두 full CST의 차이는 `REFERENCE_CST_INELIGIBLE`/BLOCKED이며 field를 보충하지 않는다. Native와 reference metadata의 확인 가능한 모순은 이 부적격 판정으로 가리지 않는다.

PASS는 외부 승인 기준에 연결한 저장 데이터의 일관성만 뜻한다. 복사한 승인 header를 붙인 fabricated response의 실제 실행 진위는 증명하지 못한다. Manifest의 전체 selected-tree/tool/source/vendor receipt closure 재검증, 독립 실행 진위, qualification eligibility·claim/reducer/inventory/fan-in은 별도 미완료 영역이다. Stock CLI full-field parity의 BLOCKED는 유지한다. 출력 소유권은 [#215의 조건부 완료 범위](trust-and-execution.md#215-출력-handle-결속과-미완료-경계)를 따르며, macOS 생성 순간의 소유권 공백은 남는다.

입력 identity JSON은 1 MiB, reference binary는 64 MiB 한도를 파일 열기 전 stat과 열린 regular file의 fstat에서 확인한다. JSON은 상한+1까지만 읽고 binary는 스트리밍 hash를 사용한다. 실행 중 파일 크기가 늘어나도 상한을 넘으면 거부한다. `complete`는 JSON boolean `true`만 허용하며 truthy 문자열·숫자로 완료를 대신하지 않는다. Owned control은 identity가 결속한 selected-tree receipt도 직접 확인하고 종료 직전 보존 bytes를 재확인한다. `test_pipeline.py`는 1 GiB sparse 입력의 사전 거부와 완료 값 타입 반례를 포함한다.

Owned control의 compiler 출력 library도 hash 계산 전에 64 MiB 한도와 regular-file 검사를 적용한다. 실제 build/check 진입점 fault 회귀는 `TSGK_REFERENCE_SUPERVISOR`·`TSGK_REFERENCE_CC`·`TSGK_REFERENCE_AR`·`TSGK_REFERENCE_GIT`·`TSGK_REFERENCE_ARCHIVE`가 모두 지정됐을 때 실행하며, 없으면 해당 회귀는 SKIP으로 기록한다. 고정 버전의 접미사·다른 patch 버전, selected-tree 중 source/vendor 변경, compiler의 상한 초과 library를 실제 supervisor로 실행해 후속 단계와 완료 identity 생성 전에 거부하는지 확인한다.

### Full-input source·CST 경계

`full-input-utf8/r1`의 완료 reference는 실제 초기·편집 source bytes가 있으면 `utf8.Valid`도 통과해야 한다. 일반 native UTF-8 byte 허용 정책은 바꾸지 않으며, 알려진 `FAILED/SOURCE_UTF8` 응답은 미완료 BLOCKED다. Source가 없으면 encoding을 추측하지 않는다.

제공된 모든 완료 full native raw·projection 및 완료 reference CST의 root 끝은 해당 version의 input bytes와 같아야 한다. Reference CST도 native와 같은 CST 형태 규칙(MISSING의 has_error, 부모 안 자식 범위; 빈 alias type은 허용)을 만족해야 하며 위반은 `REFERENCE_TREE_INVALID`, reference metadata의 `querywire.ValidMetadata` 위반은 `REFERENCE_METADATA_INVALID`다. 필수 tuple 밖의 tree도 검사한다. Root 시작은 leading padding 이후일 수 있으며 공백 전용 입력의 `[input.bytes,input.bytes]`와 빈 입력을 허용한다. 이 끝점 검사는 실행 진위나 임의 prefix의 문법적 적합성을 증명하지 않는다.

CST 비교 전에 독립 anchor의 capture 개수·순서별 match/pattern/capture/name을 대조한다. 확인된 모순은 CST 부적격과 함께 있어도 FAIL이다. Node preorder index·type·flags·range 등 전체 필드는 CST 동등성 확보 후 기존 comparator로 대조한다. Uncaptured node 차이로 index만 이동한 유효한 reference는 CST_INELIGIBLE/BLOCKED로 유지한다. Native full CST가 없을 때에는 기존 전체 anchor 대조를 유지하며, anchor 하나의 누락이 존재하는 다른 anchor의 대조를 생략하지 않는다.

### #216 small 실제 driver 캡처 경계

`tsgk-native-process-capture/r1`은 실행당 실제 요청·반환값을 보존하는 내부 캡처 형식이다. 이 형식 자체는 `query_semantics` claim이나 실행 진위를 승인하지 않는다. 기존 eligibility/r1의 승인 Request wrapper, plain Process 및 unframed Response 의미는 그대로다. 공개 `Build.Exec`·`RunCase`의 기존 경로는 캡처를 만들지 않으며, private 캡처 경로는 runner에 전달할 Args·Env·Stdin의 사본과 반환 직후 Result·stdout·stderr를 보존한다. 새 case의 준비 단계가 실패하면 이전 case의 캡처를 물려받지 않는다.

필수 최상위 필드는 `schema,stage,spec,process,stdout,stderr,error`다. `VERIFY_REFUSED`는 실행 전 build 검증 거부이며 spec·process·stdout·stderr는 null이고 native error가 필수다. `RUN_RETURNED`는 spec과 process pin이 필수이며 runner의 실패 반환도 보존한다. 이 stage에 native error를 함께 기록하지 않는다. 실패 반환만으로 child 미시작이나 cleanup 성공을 추정하지 않는다. 아직 exec에 도달하지 않은 준비 실패에는 캡처가 없다.

spec은 실제 `runner.Spec`의 `Path,Args,Dir,Env,Stdin,Interactive,StdoutBytes,StderrBytes,Wall,Grace,Memory,CgroupParent`를 모두 담는다. Wall·Grace는 signed 정수 나노초이고 stdout/stderr 한도도 signed 원값이다. Args·Env의 null과 []를 구별하며 Env:null은 빈 환경이다. Stdin·stdout·stderr는 Oracle 로컬 `ReplayMember{path,role,bytes,sha256}` pin 또는 null이다. nil stream은 null이고 non-nil 빈 stream은 0바이트 파일 pin이다. Binary stream을 UTF-8 문자열로 바꾸지 않는다. process pin은 plain Result 파일을 참조한다. 캡처 파일은 의존 member 뒤에 직렬화하며 미래 manifest·graph·approval의 hash를 참조하지 않는다.

error는 null 또는 필수 `type,code,kind,blocked` 객체다. native는 kind와 code 및 blocked:null, runner는 code와 blocked 및 kind:null, other는 빈 code와 두 null을 보존한다. Cause의 문자열을 직렬화하지 않으며 호출자에게 반환하는 원 오류는 유지한다. 모든 JSON 문자열은 잘못된 UTF-8을 대체하지 않고 거부한다. 파일 바이트를 소비자가 변경해도 보존된 내부 캡처를 변경하지 못한다.

small offline 캡처·프레임 파일에는 기존 evidence-replay의 파일당16MiB 한도를 적용하며 4바이트 프레임 헤더도 이 한도에 포함한다. 본문은 헤더의 big-endian 길이와 전체 바이트 수가 정확히 맞을 때만 채택한다. native 요청 payload의48MiB 한도를 offline 파일 한도로 해석하지 않는다. 실제 r2/r4 payload는 기존 strict JSON shape와 canonical Base64를 사용한다. r2에는 네 observation 필드의 존재 자체를 거부하고 r4에는 네 필드를 모두 요구한다. Base64 decode·재인코딩과 decoded query/edit 배열, 편집 version·point·반복 scan의 비용은 같은 eligibilityBudget에서 할당 전에 예약한다. 편집 한도는 version별 상한이며 누적 교체 길이로 바꾸지 않는다.

새 Oracle opt-in·manifest/record/replay·최종 의미 검증과 전체26route/SVC/large qualification은 이 내부 형식과 별도의 완료 조건이다. 캡처 검사만으로 reference 자격·외부 승인·semantic PASS 또는 #216 완료를 선언하지 않는다.

첫 small 생산 경로는 `tsgk oracle record --query-semantics FILE`로 명시적으로 선택한다. 이 문서는 input root 밖의 caller 문서이며 profile/r3의 `query_semantics_registration_sha256`과 정확한 원문을 결속한다. 이 선택은 source witness와 native/r4를 함께 요구한다. 첫 범위는 Windows/Linux의 단일 UTF-8 full-input native-query case이며 SVC·전처리·batch·large·accepted·fact pack과 run-wall 변경은 실행 전에 거부한다. profile/r3에 opt-in 문서가 없으면 과거 생산 방식으로 돌아가지 않는다.

실제 build executable·closure 입력·build 설명은 build 제거 전에 `capture/`의 명시적 manifest member로 보존한다. 실제 invocation의 request frame·plain process·stdout/stderr frame도 같은 capture를 사용한다. `projection/`에는 semantic leaf를 추가하기 전의 case 결과를 저장한다. record/r4의 필수 `query_semantics`는 `phase=PRODUCER_LOCAL`, `claim=BLOCKED`, `code=EXTERNAL_APPROVAL_PENDING`, 등록 digest와 로컬 capture/projection pin을 보존한다. 실행 준비 단계에서 호출에 도달하지 않았다면 capture는 null이다. 완료된 기존 PASS만 이 leaf 때문에 BLOCKED가 되며 기존 FAIL·BLOCKED·미완료 판정과 최초 code를 유지한다. manifest/r5와 Oracle result/r3도 이 producer 판정을 기록한다. 외부 승인이나 최종 의미 PASS를 자체 생성하지 않는다.

보관한 build의 compiler `name`·`version`은 profile의 `ToolIdentity` 선언값 전체와 일치해야 한다. 이 선언은 별도 관측값인 `compiler_version`과 구분하며, 이름이나 선언 version의 일치만으로 compiler 실행이나 관측 version의 진위를 인증하지 않는다.

r5 set 무결성 검사는 producer 판정을 의미 검사 완료로 채택하지 않는다. 필수 projection 필드와 상태·판정 조합, projection에서 pending leaf를 접은 record 및 단일 case의 manifest 집계를 대조한다. 완료 실행에는 raw·capture가 필요하며 plain process의 형식, 실제 보존한 stream 길이·truncation 관계, stdin/stdout frame을 확인한다. `capture/build-manifest.txt`는 원래 build identity를 만든 문구이며 새 identity를 만들지 않는다. 실행 파일·각 closure 입력·build 설명·manifest의 runtime/compiler/version 및 원 argv를 결속하고, cache hit은 `source_steps`를 사용한다.

새 r5의 최초 manifest/record 파싱부터 JSON 값 수·작업·할당 예산을 적용한다. 기존 small 한도를 유지하며 사전 member 수·합계 bytes 검사 뒤 member를 읽는다. 실제 순회는 디렉터리당 최대 128개씩 읽고 파일·디렉터리 수와 깊이·wall을 확인한다. 잘못된 경로·비정규 파일·미등록 파일·디렉터리 읽기/종료 실패를 거부하며 finding은 1,000개로 제한하되 최초 오류와 마지막 원인을 보존한다. r5용 `fs.FS`의 directory file은 bounded `fs.ReadDirFile`을 지원해야 한다. build 보존 실패 뒤에는 driver를 실행하지 않고 build 정리를 수행하며 취소·자원 한도를 일반 I/O 실패로 바꾸지 않는다.


Grammar closure member는 기존 eligibility/r1이 요구하는 `grammar-parser`·`grammar-scanner`·`grammar-header` 역할을 그대로 사용한다. `capture/build-input-NNNNN.bin` 경로와 bytes/hash를 바꾸거나 다른 경로의 같은 hash로 대신하지 않는다. Runtime·driver source closure의 일반 역할은 `build-input`이다. 이 구분은 기존 eligibility 계약을 완화하지 않고 원 member를 연결하기 위한 것이다. 빈 query의 생략 가능한 projection `queries`는 manifest query 목록이 비어 있을 때만 허용하며, 의미 검증의 required=0은 완료 PASS가 아니다. Capture가 없는 사례에는 raw도 없어야 한다. 정리 오류는 별도 finding과 결합 오류로 남기며 이미 확정된 CANCELLED·RESOURCE_LIMIT 상태와 원 오류를 덮지 않는다.

공개 의미 검증과 `oracle-set-r4`는 사전 등록으로 required를 먼저 고정한다. 기존 eligibility/r1의 ABSENT 조기 반환을 유지하되 새 의미 caller는 한 번의 graph/Oracle 검증을 수행하고 전체 required를 BLOCKED로 남긴다. BOUND는 같은 실행 범위에서 실제 checked native와 tuple별 `witnessEvaluation`을 재사용한다. query expectation은 raw의 COMPLETED가 아니라 predicate 평가 후 status/code를 비교한다. 평가값이 없으면 저장 PASS와 모순을 확정하지 않고 base를 BLOCKED로 둔다. producer의 stored fold가 확인된 즉시 기존 FAIL·최초 code를 보존하고, 이후 stream 모순·취소·자원 한도로 덮지 않는다.

완료된 native 응답과 projection의 기본 판정 입력도 대조한다. producer의 ABI·query/API/predicate capability·symbol/field 수·reuse capability와 response status/code, 단계별 route 원값 및 그 값에서 계산한 proven, 단일 실행의 restart 부재, incremental/fresh의 순서 있는 declaration/name span을 같은 retained raw에서 확인한다. provenance 승인과 hash를 다시 맞춰도 projection이 raw의 사실을 바꾸면 FAIL이다. ABI 거부 등 미완료 실행의 원 producer 정보는 이 완료 전용 대조로 덮지 않는다.

producer_fold_valid는 record와 projection의 claim·상태·판정·code 관계가 모두 맞고 pending leaf를 포함한 집계가 올바를 때만 true다. 모순을 finding으로 남겼더라도 이 표시를 true로 유지하지 않는다. 이 판정 뒤의 취소·자원 한도는 이미 확인한 producer 기록과 원 FAIL/code를 지우지 않는다.

## #216 정적 query 슬롯 원장

[query-slot-inventory-r1.json](../../src/contracts/query-slot-inventory-r1.json)은 `tsgk-query-slot-inventory/r1`의 정적 분류다. 기존 qualification 등록부의 원문 path·bytes·SHA-256과 query 정의 파일의 pin을 결속하며, 사례 행과 source를 다시 복제하지 않는다. `STATIC_CLASSIFICATION`과 `assessment=NOT_ASSESSED`는 실행 결과가 아니다. `semantic_required:null`은 이 원장에서 semantic 분모를 등록하지 않았다는 뜻이고, 필드 누락이나 0으로 바꾸지 않는다. 별도로 사전 등록한 JSON24 등의 의미 분모를 이 값으로 대체하지 않는다.

일반 슬롯의 좌표는 `(qualification_inventory.sha256, set, case, query, step, tree_role)`다. 원본 등록부의 모든 일반 route workload에서 모든 case와 모든 query를 선택한다. 일반 사례3,201개 중3,047개는 등록부에 source 원문이 내장되지 않았으므로 후속 witness에는 원문과 독립 expected를 별도로 준비해야 한다. step0은 incremental만 있고 편집 이후 각 step에는 incremental과 fresh를 이 순서로 둔다. 원본 parser가 검사한 set 및 case/query ID의 유일성과 전체 원문 pin을 함께 사용하므로, 정규화한 group과 전개 규칙으로 각 슬롯을 유일하게 복원한다. `case_steps`는 `1 + len(edits)`, `tree_roles`는 `1 + 2*len(edits)`이며 `potential_query_tree_roles`는 query 수와 tree role 수의 곱이다.

현재 등록부는 일반26 route, 3,201사례, 130 query 정의, 편집1,852개, case step5,053개, tree role6,905개, 잠재 query/tree-role 좌표53,104개다. 이 수에는 플랫폼 축이 없으며 semantic 필수 tuple이나 실행 건수도 아니다. 새 semantic 대상은 Windows/Linux로 별도 표시하고 신규 macOS 검증은 제외한다. 기존 baseline의 플랫폼과 extra-role의 적용·유예 사유는 원형대로 유지한다.

각 일반 query는 실제 원문 정의와 등록된 hash가 같아야 한다. 현재130개 원문은 ASCII `#`와 predicate 위치의 legacy `.` 표식을 보수적으로 검사해 `STRUCTURAL_CANDIDATE`, compiled metadata는 `UNVERIFIED`로 기록한다. 고정 query compiler는 `#eq?`뿐 아니라 `.eq?`도 받아들이므로 `#` 부재만으로 predicate 부재를 증명하지 않는다. 괄호 뒤 공백·세미콜론 주석을 건너뛴 `.`는 명시적 분류를 요구하며, capture 이름의 점·sibling anchor·문자열과 주석 안의 점은 구분한다. 이는 컴파일 성공·capture/pattern metadata의 독립 확인이나 EVALUATED를 뜻하지 않는다. 두 표식이 발견되면 정적 분류를 자동 승격하지 않고 독립 검토·명시적 등록을 요구한다. 기존 네 가지 text predicate와 nonempty predicate 등록 조건은 유지한다.

8개 extra 역할은 ID·role·등록 status·reason·platforms·not_applicable·deferred와 원본 workload의 set/profile/route/operation/format·query pin을 보존한다. Workload 부재의 null과 빈 배열을 구별한다. Query가 없는 역할은 `NO_REGISTERED_QUERIES`, query가 있는 별도 context는 `CONTEXT_NOT_EXPANDED`다. SVC69사례의 원문 좌표·segment 및 large 정상3+over-limit1의 paging을 일반 슬롯 곱셈으로 확장하지 않는다. `s06-large-over`의 기대 자원 제한과 각 사례의 전체 입력·편집·기대값은 pin으로 고정한 원본 qualification 등록부를 따른다. Symbol·reuse-accepted의 query0과 EXTERNAL/NOT_RUN/DEFERRED를 의미 PASS로 바꾸지 않는다.

`semantic_required:null`은 이 정적 원장의 값으로 남는다. 별도의 #216 r10 plan은 최대 두 Windows/Linux small full-input predicate probes의 고정 분모만 등록하며 이 53,104개 potential 좌표를 실행 분모로 바꾸지 않는다. r10 결과도 ordinary query workload·SVC·large scope가 미등록인 부분 BLOCKED다. Plan·caller context·approval·결과 fold의 canonical 규칙은 [identity/evidence](identity-and-evidence.md) `#216 small semantic qualification opt-in — r10`을 따른다.

정적 문서 상한은256KiB다. 중복·미지 JSON member, 필수 null/0 필드의 누락, 원문 pin·분류·계수·추가 역할 사유의 변경을 거부한다. 재생성은 기존 `TSGK_WRITE_INVENTORY=1`을 명시한 `TestQuerySlotInventory`가 수행하고, 쓰기 모드를 끈 별도 검사로 원장과 음성 대조를 검증한다. 기본 foundation 검사는 원장 존재·정확한 값과 shape를 요구한다. 이는 기존 small/large runtime budget, producer/reducer 또는 qualification verdict를 변경하지 않으며, #216 r10의 제한된 probe scope 밖인 전체 semantic completion/fan-in gate는 후속 단위다.

## #362 원문 witness 복원과 기대값 규칙 대장 P1

개발용 `go run ./src/dev/query-witness -root .`는 P0 registry의 ordinary selector를 검사하고 고정 qualification r8·slot r1·registry/basis pin에서 26 route의 원문을 복원한다. Case ID로 `routes`, `gaps`, `n461` fixture를 선택하며 내장 query source는 query fixture의 `source_case`와 원래 `routes` case를 함께 대조한다. Dynamic SQL 두 입력은 기존 `dynamic-sql/expected.json`과 실제 파일의 pin에 연결한다. Hash가 같은 다른 case로 출처를 대신하지 않는다. 원래 fixture가 정본이며 export는 `DERIVED_UNREVIEWED` 파생물이다. 이 도구는 native/reference를 실행하지 않는다.

초기 source 3,201개(내장154·복원3,047)의 bytes·SHA-256·길이와 1,852개 편집의 원래 find/replace recipe·순서를 검사하고 기존 `ApplyEdits`로 5,053개 step과 LF 행·byte 열 point를 복원한다. Source마다 65,536 bytes, 읽는 파일과 파생 JSON마다 16 MiB를 넘으면 거부한다. 이는 개발 도구의 상한이며 제품 runtime 예산을 바꾸지 않는다. Export는 stdout으로만 내보내며 보존할 때는 `.work/` 아래에 둔다. 원문·step 복제본은 commit하지 않는다.

`tsgk-query-source-export/r1`은 registry/qualification/slot pin, route·set·UTF-8·full-input context·고정 grammar closure, query ID/hash, case selector·원래 파일 pin/selector/case ID·순서 있는 step hash·edit points를 포함한다. 각 route 안에서 SHA-256·길이·실제 bytes가 같은 source version을 Base64로 한 번 저장하고 case/step은 그 hash를 참조한다. 공유는 route·grammar·encoding·context 경계 안에서만 허용한다. Query/source 묶음은 각 route의 query 목록과 version 목록의 Cartesian product다. 초기 고유 source3,008개, 모든 고유 version3,963개와 query/source31,337개를 검사한다. 두 OS·case·step·tree role의 actual 의무106,208개는 그대로다. 이 파생 export는 승인된 witness나 full-scope evidence reader 입력의 대체물이 아니다.

추적하는 [기대값 규칙 대장](../../src/contracts/query-expectation-rules-r1.json)은 `tsgk-query-expectation-rules/r1`, 최대256 KiB의 `SKELETON_UNREVIEWED` 자료다. 130 query의 ID·원문/hash·원본 파일 pin/selector와 route별 grammar pin·language source provenance를 연결하고 기대 생성 규칙·자동화 상태·적용 범위·수작업 예외 목록·검토 pin을 둔다. `go run ./src/dev/query-witness -root . -rules`는 현재 골격을 stdout으로 재현한다. Pending129개는 선언만 연결했고 CST/API·metadata·ordered captures·fact 규칙 작성은 후속 자료 PR에 남긴다. `manual_count:null`, 예외 분류 미완료와 `independent_review:null`은 수작업0 또는 검토 완료를 뜻하지 않는다.

JSON `q.structure`의 작은 독립 규칙은 표준 라이브러리 `encoding/json`의 `Token`·`InputOffset`으로 유효한 UTF-8 RFC JSON의 object member를 읽는다. 값이 array/object/string인 member의 key string 전체를 각각 `array_key`/`object_key`/`key`로, string 값은 이어서 `value`로 선택한다. 중첩 member·중복 key·escape·Unicode의 원문 byte span을 유지하며 start byte 순서로 돌려준다. 세 query pattern의 match 순서와 capture 위치도 별도로 계산한다. 최대 source65,536 bytes·깊이64이며 comment·multiple root·malformed/recovery는 이 규칙으로 처리하지 않고 오류를 반환한다. Kit reducer·querypolicy나 reference 출력은 기대 생성 입력으로 읽지 않는다. 기존 `json-query-r1` step0의 수작업 anchor13개와 name/type/text 순서를 대조한다. 이 부분 대조는 full CST/API·compiled metadata·12-field capture 또는 JSON 전량 기대값 완료가 아니다. 실제 native/reference 양쪽 대조와 독립 규칙 검토·예외 전수 처분은 후속 의무다.

P1의 파생 원문과 규칙 골격은 P0 root asset 선언을 바꾸지 않는다. Registry의 `content_state=INCOMPLETE`, `full_scope_complete=false`, positive BLOCKED와 슬롯 원장의 `NOT_ASSESSED`를 유지한다. 승인된 source/expected asset을 사전 pin하는 후속 변경 없이 P1 산출물로 completion을 열지 않는다.

## #362 전체 query 의미 의무 등록부 P0

[`query-semantic-registry-r1.json`](../../src/contracts/query-semantic-registry-r1.json)은 기존 `qualification-c1.json` r8과 `query-slot-inventory-r1.json` r1의 원문 bytes·SHA-256 pin을 보존하며, 두 입력에서 선택한 의무를 별도 등록한다. 기존 슬롯 원장의 `semantic_required:null`, `STATIC_CLASSIFICATION`, `NOT_ASSESSED`는 바꾸지 않는다. Root registry는 고정 입력 pin과 qualification의 route/case/query/source/edit selector를 사용하며, 원문·case catalogue·106,208개 행을 복제하지 않는다. 미등록 independent expected asset은 명시적으로 `UNAVAILABLE`이다.

Ordinary 범위는 26개 route의 모든 case·query·step·tree_role을 `ALL`로 전개하고 Windows/Linux를 각각 포함하는 106,208개 `STRUCTURAL_ONLY` tuple이다. Step 0에는 `incremental`만 있고 각 편집 뒤에는 `incremental`, `fresh` 순서로 둔다. Predicate 의미 의무 20개/host, 합계40개 `PREDICATE_REQUIRED`와 host 정책 대조 47개/host, 합계94개 `EXPECTED_REJECTION`은 서로 다른 typed 분모다. Windows large의 알려진 workload도 정상 완료3개 `LARGE_WORKLOAD_POSITIVE`와 기대 자원 제한1개 `LARGE_WORKLOAD_CONTROL`로 별도 등록한다. 알려진 coordinate 합계는106,346개이며 large node/occurrence/page/footer 세부 분모는 `required:null`과 `missing_scopes`로 남는다. SVC segment/range/history/CodeBehind 하위 분모도 null이다. 기존 8개 extra role의 status·reason·platform·적용 불가·유예 사유도 보존한다. 예상 거부 대조는 positive 의미 PASS를 채우지 않고 subject의 원 상태를 바꾸지 않는다.

Coordinate와 obligation ID는 원문 pin, policy, route, host ID/subject, role, set, case, query, step, tree role 및 context discriminator를 포함하는 canonical tuple에서 계산한다. Registry 자기 hash를 ID 안에 넣지 않는다. Caller가 고정한 registry hash는 ID와 별도로 결속하며, count나 digest만 비교해 membership을 인정하지 않는다. Canonical JSON은 Go `encoding/json`(v1) encoder 규칙으로 만든다: UTF-8, struct field·array의 고정 순서, 정렬된 map key, HTML escaping 없음, U+2028/U+2029는 `\u` escape, 마지막 newline 없음. Coordinate digest는 canonical coordinate bytes를 정렬하고 각 행 앞에 uint64 big-endian byte length를 붙여 SHA-256으로 계산한다. Strict decoder는 256 KiB, 깊이64, JSON token 100,000개를 제한하고 duplicate/unknown field, 필수 누락과 잘못된 null/shape를 거부한다. Foundation 검사는 qualification과 슬롯 원장을 독립 경로로 다시 전개해 ID와 coordinate 모두 exact 비교한다. `go run ./src/dev/query-registry -root . -write`는 고정 manifest만 쓰는 명시 생성 단계이며 일반 validation은 generator를 호출하거나 registry를 쓰지 않는다. Generator와 verifier는 internal package이며 public `kit` API를 추가하지 않는다.

Coverage fold는 caller가 제출한 adjudicated state만 집계하며 각 typed set에서 `required = verified + failed + blocked + unprocessed`를 유지한다. 이 P0 fold는 runtime raw claim이나 actual native/reference evidence를 판정하지 않는다. Source witness, independent metadata/expected, full CST/API, capture, current observation, caller approval과 replay asset은 모두 `UNAVAILABLE`이므로 positive coordinate는 VERIFIED가 될 수 없고 known required 분모에서 BLOCKED다. Control은 원 status/code/capture를 보존해 별도 집계한다. 확인된 registry 모순·변조는 FAIL이며, 그 앞에서 이미 확인된 최초 FAIL code를 보존한다. Coverage counter의 합계는 등록된 집합의 합일 뿐이며 미등록 SVC/large 하위 의무를 포함한 전체 분모가 아니다. Namespace는 predicate basis 두 파일(`src/dev/query-reference/check.py`, `src/internal/native/query_policy_test.go`)의 bytes에도 결속된다. 둘 중 하나를 바꾸면 검증이 `REGISTRY_BASIS_PIN`으로 실패하고, registry를 다시 생성하면 모든 obligation ID가 바뀐다. 중단 뒤 판정하지 못한 항목은 unprocessed로 남되, 이미 확인된 필수 asset/input 부재는 blocked로 유지한다. 완전한 빈 expected `[]`와 미확보 expected `null`은 다르다.

P0 registry가 유효해도 독립 기대 자료, 실행 관측, 승인, replay 및 SVC/large 전수 하위 원장이 갖춰지지 않았으므로 `full_scope_complete=false`다. P0 fold는 모든 adjudicated row가 VERIFIED여도 이를 true로 바꾸지 않는다. 이는 #362 전체 완료, r9/r10 승격, release gate, runtime 예산, producer/reducer, CLI 또는 workflow 활성화를 뜻하지 않는다. 전체 완료는 모든 필수 집합의 exact membership, positive·control 검증, 현재 cohort·승인·저장 재계산, SVC/large 전수성 및 미완료 상태 0을 별도 확인한 뒤에만 가능하다.

## #362 predicate 의미 의무와 거부 정책 (범위 3)

P0 registry의 `predicate` 목록은 Windows/Linux 각각 67행이다. 앞 20행이 `PREDICATE_REQUIRED`(host당 20, 합계 40)이고 뒤 47행이 `EXPECTED_REJECTION`(host당 47, 합계 94)이다. 좌표는 `route=owned-plain`, `role=predicate-probe`, `set=predicate/r1`, `case=<행 ID>`, `query=q0`, `step=0`, `tree_role=incremental`이다. 대상은 `eq?`, `not-eq?`, `any-of?`, `not-any-of?` 네 predicate뿐이며 새 predicate·regex·directive·quantifier 지원은 추가하지 않는다. P0 registry 파일(82,475 B, SHA-256 `91e8475f…2a06c`)과 그 ID는 바꾸지 않는다.

세 분류는 서로 섞지 않는다.

- `STRUCTURAL_ONLY`는 compiled metadata의 `predicates`가 명시적 `[]`인 ordinary 의무다. `EVALUATED`는 predicate가 있는 query가 `COMPLETED`로 평가되어 evaluated stream(없으면 `[]`)을 낸 결과다. 둘은 서로의 근거가 될 수 없다. Ordinary 등록은 `Kind != STRUCTURAL_ONLY`(`src/kit/query_ordinary.go:394`)와 nonempty predicate(`query_ordinary_adjudicate.go:250`, `ORDINARY_STRUCTURAL_PREDICATES`)를 거부하므로 r9 ordinary 경로는 이 20 tuple을 받지 않는다. 이 범위는 별도의 typed 경로다.
- 예상 거부 47행은 positive PASS를 채우지 않는다. 각 행은 등록된 `status`·`code`·`evaluation`·`partial`과 evaluated stream `null`(또는 `PRESERVED_STRUCTURAL`), structural stream 보존 여부를 정확히 만족해야 VERIFIED다. `null`을 `[]`로 바꾸거나 거부를 `COMPLETED`/`EVALUATED`로 바꾸면 거부된다.

**독립 expected.** [`query-predicate-expected-r1.json`](../../src/contracts/query-predicate-expected-r1.json)(80,893 B, SHA-256 `b08e2d23454eed94e4527128d7772e01b294f317b1cb3574acc977edbdebf8f4`)은 20 tuple의 compiled metadata, 전체 CST, structural stream, evaluated stream을 담는다. 이 값은 plain grammar(`grammar.json`, 5,325 B, SHA-256 `4a90e2460dc31312361a0fc0cc6e86fc28dd98afe710715b3ca8d538d9efbf17`)의 규칙과 네 predicate의 정의에서 손으로 도출했으며 native·reference·kit 출력을 읽지 않았다. native 첫 실행 전에 이 SHA를 봉인했다. Predicate는 query.c가 평가하지 않는다. C runtime은 pattern별 predicate step 목록만 만들고(`lib/src/query.c`의 `ts_query__parse_predicate`, pattern 단위 slice) 평가는 binding이 한다. 평가 규칙은 match 단위다: 한 match의 모든 predicate가 참이면 그 match의 capture가 모두 남고, 아니면 모두 빠진다. Match 번호는 다시 매기지 않으며, 다른 pattern의 match는 영향받지 않는다(`pattern-isolation`). Source가 비어 있으면 둘 다 `[]`다(`evaluated-empty`). 이 값은 두 신규 제안(`pattern-isolation`, `evaluated-empty`)을 제외하고 기존 `check.py`의 수작업 anchor와 일치한다.

| ID | source → 구조 stream / evaluated stream |
|---|---|
| `eq-0/1`, `not-eq-0/1`, `any-of-0/1`, `not-any-of-0/1` | `x;`/`y;` 두 입력. match 1개. evaluated는 predicate 참일 때만 그 match |
| `capture-pair-0/1` (`#eq? @x @x`) | 항상 참. 두 입력 모두 evaluated = structural |
| `multiple-0/1` | `eq? "x"`와 `not-eq? "z"` 모두 참일 때만 유지 |
| `two-captures-kept/rejected` | 한 node에 `@x @y` 두 capture. 참이면 둘 다, 거짓이면 둘 다 빠진다 |
| `distinct-captures-{eq,not-eq}-{equal,different}` | 두 statement의 `@x`·`@y` 비교. match 1개(capture 2개) |
| `pattern-isolation` | pattern 0(`#eq? @x "z"`)은 탈락, pattern 1(`(number) @n`)은 유지. structural 2, evaluated 1(match 번호 1) |
| `evaluated-empty` | 빈 source. structural `[]`, evaluated `[]`(완료된 빈 stream, `null` 아님) |

**검증.** `kit.ParsePredicateExpected`는 asset을 P0 registry의 정확한 bytes와 고정 roster에 결속한다(registry pin, 20 ID와 순서, source·query·encoding 일치, `null` 거부, 빈 predicate 거부, evaluated가 structural의 완전한 match 부분열인지). `kit.VerifyPredicateTuple`은 저장된 native raw·projection을 기존 `VerifyQueryWitness`로 재계산해 독립 structural·evaluated stream과 대조하고, 독립 full CST를 별도로 비교하며, `Evaluated`가 정확히 1이어야 한다(구조 전용 query는 `PREDICATE_TUPLE_NOT_EVALUATED`). `kit.CheckPredicateControl`은 47행 각각의 정확한 거부를 확인한다. `kit.FoldPredicateScope`는 registry에서 predicate 40·거부 94 좌표를 다시 전개해 host별 `required = verified + failed + blocked + unprocessed`를 유지하며, 삭제·중복·미등록 ID와 positive의 subject 재분류, 거부 행의 PASS 전환을 거부한다. 이 fold는 P0 root의 `INCOMPLETE`·`full_scope_complete=false`를 바꾸지 않는다.

**현재 실행 범위.** Windows host의 20 tuple은 실제 owned native(plain grammar)로 실행해 20/20 VERIFIED이고 47행은 host policy evaluator로 47/47 VERIFIED다. Linux 몫(20+47)은 등록만 되어 있고 실행 결과가 없으므로 BLOCKED다. Reference 독립 실행, r9 replay, 외부 승인은 이 범위에 포함하지 않는다.

## #362 SVC 사례의 독립 등록과 확장 분모 (범위 5)

P0 registry의 `n461-svc` extra role(`s06-csharp-svc`, `SVC-SERVICEHOST-r2`, route `csharp`)은 case 69개, edit 41개, query 2개(`facts.declarations`, `facts.dynamic_sql`)이며 하위 분모는 `required:null`, `UNREGISTERED`와 `missing_scopes`의 `svc-segment-range-history-codebehind`로 남아 있다. 이 숫자는 `qualification-c1.json`의 해당 workload와 정확히 일치하고(case 69, edit 41, 전체 step 110) P0 registry의 값도 같다. P0 registry와 그 ID는 바꾸지 않는다. 별도 추적 계약 [`query-svc-expected-r1.json`](../../src/contracts/query-svc-expected-r1.json)(196,457 B, SHA-256 `bbe9116749abe78cc218a0455cad476d9d4b4b9044447ff35d10a181df7eb746`)이 확장 분모와 독립 expected를 담는다. 이 파일은 registry·qualification inventory·`src/testdata/native/n461/svc.json`의 정확한 bytes/SHA-256에 결속된다.

**확장 분모(host당).** 의무는 asset에서 고정 순서로 다시 전개한다.

| 종류 | 개수 | 내용 | 기대값 |
|---|---:|---|---|
| `SVC_HISTORY` | 110 | case·step마다 원문 version의 byte 수와 SHA-256 | 독립 도출 |
| `SVC_EDIT_POINT` | 41 | edit의 byte와 start/old-end/new-end point | 독립 도출 |
| `SVC_SEGMENT` | 109 | step의 directive·open/close·이름·attribute 범위와 point, diagnostics, Language 상태, CodeBehind 선언, included range, coverage | 독립 도출 |
| `SVC_CODEBEHIND` | 2 | 선언된 참조(`OrderService.svc.cs`, `App_Code/OrderService.cs`)가 실제 관측에서 선언 case로 해석됨 | 독립 도출 |
| `SVC_NEGATIVE` | 1 | VB `Language`의 unsupported-language negative | 원래 계약 보존 |
| `SVC_INLINE_TREE` | 82 | C# inline 구간의 tree(composite step 81 + CodeBehind 참조 C# case 1) | 등록만, BLOCKED |
| `SVC_INLINE_QUERY` | 164 | inline tree 82개 × 두 query | 등록만, BLOCKED |

109개 `SVC_SEGMENT`는 69 case 중 CodeBehind의 대상인 참조 C# case `n461-svc-reference-p-r2`(composite를 만들지 않는 일반 C# case)를 제외한 68 case의 step이다. 참조 case의 원문 history 1건은 `SVC_HISTORY`에, 그 C# tree는 `SVC_INLINE_TREE`에 포함된다.

**독립 expected.** 원문 version과 edit point는 `svc.json`의 find/replace edit에서 도출하고 inventory의 byte-level edit(start/old-end/new-end, old/new bytes)과 서로 교차 검사했다. Directive 관측은 `docs/validation/net461-workload.md`의 `SVC-SERVICEHOST-r2` 형식에서 별도로 구현한 도출기로 만들었으며 native·reference·kit 출력을 읽지 않았다. native 첫 실행 전에 위 SHA를 봉인했다. 도출기는 같은 저자가 형식 문서를 읽고 썼으므로 diagnostics 코드 이름과 일부 경계 규칙은 producer 소스와 같은 규칙을 따른다. 그 한계를 줄이려고 directive 범위·닫는 구분자·inline 시작은 문자열 검색만으로 따로 도출한 anchor로도 확인한다(인용부 안의 `%>`를 가진 case는 제외).

**BLOCKED로 남는 부분.** `SVC_INLINE_TREE`는 실제 C# grammar(`parser.c` 51 MB)가 저장소에 없어 이 host에서 실행하지 못한다. 등록된 `expect`(step 109개)는 inventory의 기존 수작업 기대값이며 이 계약이 새로 만든 값이 아니다. owned plain grammar는 inline 언어의 기계 장치만 확인하는 대역이며 C# tree의 증거가 아니다. `SVC_INLINE_QUERY`는 inventory의 69 case 모두에서 `query_expect`가 비어 있고 `dynamic_sql_expect`가 `null`이므로 query별 expected가 아예 등록되어 있지 않다. 빈 목록 `[]`로 바꾸지 않는다. Linux 몫은 등록되어 있으나 실행 결과가 없어 BLOCKED다. 따라서 SVC 완료는 BLOCKED이며 `full_scope_complete=false`다.

**VB negative.** `n461-svc-language-vb-n-r1`(`Language="VB"`)은 원래 상태 그대로 `BLOCKED`/`SVC_INLINE_UNSUPPORTED`, process 없음, inline tree 없음으로 거부된다(`internal/native/run.go`의 `svcObservationOnly`와 `kit/replay_native.go`의 `svcVerdict`). 관측은 `UNSUPPORTED_LANGUAGE`와 inline coverage `UNSUPPORTED`다. 이 거부를 PASS로 바꾸거나 VB 지원을 추가하지 않는다.

**검증.** `kit.ParseSvcExpected`는 asset을 세 pin과 inventory의 69 case·41 edit·110 step·두 query·CodeBehind 선언에 결속한다. `kit.VerifySvcExpected`는 기존 `ApplyEdits`와 `ObserveServiceHostWithContext`, `svcVerdict`로 의무를 재계산해 독립 expected와 대조한다. `kit.FoldSvcScope`는 host별 `required = verified + failed + blocked + unprocessed`를 유지한다.

## #362 ordinary 사전 등록과 capacity plan P2-A

P2-A는 ordinary 실행 전에 profile·semantic registration·asset registration·shard plan을 strict typed data로 고정하고, P0 ordinary ID를 이 계획에 정확히 연결하는 source-only 단계다. Ordinary execution context는 `ordinary-batch-full-input-utf8/r1`이며 P0 membership coordinate의 `context`와 별도다. `closure_encoding`은 `plain`, `parser-paged`, `binary-paged` 중 하나로 명시한다. P2-A는 producer·guarded reader·replay를 추가하거나 native/reference를 실행하지 않는다.

P0의 canonical registry 자료형·namespace/set digest·coordinate ID 및 membership/coverage 계산은 의존성 없는 내부 `queryregistrycore`가 소유한다. 기존 `queryregistry`는 qualification·slot 입력을 해석해 pinned source view를 core에 전달하고 기존 kit 오류로 바꾸는 adapter로 남는다. 두 소비 경로는 같은 canonical coordinate와 ID를 만든다. 독립 고정값은 registry 82,475 bytes와 SHA-256 `91e8475fd6a207a1c64af50c63e2456601f59097445790f2523b39c265f2a06c`, 모든 P0 ID/set, 기존 Fold 결과다. 이 공용화는 P0 registry bytes, `INCOMPLETE`·`full_scope_complete=false`, asset 부재 상태나 기존 API 동작을 바꾸지 않는다.

`src/kit`의 ordinary planning helpers는 `OrdinaryProfile` (`tsgk-oracle/r4`, `Context OrdinaryContext`), `ParseOrdinaryProfile`, `ParseOrdinaryAssets` (`tsgk-query-ordinary-assets/r1`), `ParseOrdinaryRegistration` (`tsgk-query-semantics-registration/r2`), `BuildOrdinaryShardPlan`, `ParseOrdinaryShardPlan`, `ValidateOrdinaryShardPlan`을 제공한다. Registration은 `registry_pin`, membership namespace, qualification/slot 원문 pin, route/set/platform 및 subject, execution context/closure, candidate/cohort, profile/asset pin, 원래 case selector와 초기 source pin, 순서 있는 전체 edit/version pin/points, query 원문·metadata asset pin, asset basis/review pin, full P0 coordinate·ID, planned members를 연결한다. Policy/limits는 pinned profile을 통해 결속한다. Plan은 registration pin을 단방향으로 참조한다. P0 ID는 기존 membership namespace와 전체 canonical coordinate로만 계산한다. 새 execution context를 P0 `context`에 덮어쓰거나 registry 전체를 다시 hash해 기존 selector를 대체하지 않는다. ID 분리 기준의 context는 P0 membership context다. 같은 source bytes를 쓰는 다른 case·history·host의 기존 membership coordinate는 각각 보존하고, 새 execution context는 그 P0 ID를 바꾸지 않는다.

Shard plan은 ordinary obligation ID를 각 고정 case shard에 정확히 한 번 포함한다. 각 shard는 ID 문자열과 route/platform/subject/case·history, per-step reference role을 기록하며 full coordinate row를 복제하지 않는다. Header에는 profile binding과 route별 query group을 공통화해 저장한다. Validator는 고정 P0 입력에서 full coordinate와 ID를 다시 전개하여 shard route/case를 대조하고 겹침·누락·중복·미지 ID·빈 범위를 거부한다. Shard들의 disjoint union은 P0에서 전개한 Windows/Linux ordinary 집합 106,208개와 정확히 같아야 한다. 각 shard는 file/read/work/logical/entry/wall 한도와 capacity 상태도 기록한다. 모든 planned limits와 inherited limits는 file 16 MiB, total read 256 MiB, work 32,000,000, logical 1 GiB, entries 10,000, wall 120,000 ms로 고정한다. Frame file 상한은 4-byte header를 포함한다. 본문과 검증된 full-output upper bound가 아직 제공되지 않은 현재 P1 skeleton은 각 obligation을 `CAPACITY_UNPROVEN`/BLOCKED로 남긴다. 이 상태도 ID를 빼지 않는다. `UNAVAILABLE` asset은 임의의 `PINNED`로 바꾸지 않는다. 부분 ordinary 계획은 predicate/control/SVC/large 등 다른 P0 범위를 완료하거나 `full_scope_complete`를 바꾸지 않는다.

### #362 shard commitment와 선택 plan 검증 (P2-C)

기존 P2-A `BuildOrdinaryShardPlan`은 pinned P0 registry, qualification inventory와 source-only inputs를 모두 검증하고 106,208개 ordinary ID의 전체 disjoint union을 확인한다. 이 전체 증명을 매 selected pass에서 반복하지 않도록 별도 source-only 단계가 전체 plan JSON에서 `tsgk-query-ordinary-plan-commitment/r1`을 만든다. Commitment는 P2-A 전체 plan의 `path/bytes/SHA-256` pin, shard 배열만 빈 배열로 바꾼 전체 공통 header, shard 수 6,402와 순서가 고정된 SHA-256 Merkle root를 가진다. 각 leaf는 도메인 구분자, 전체 plan pin, header digest, shard 수, 0부터 시작하는 위치와 shard의 정확한 JSON 원문을 길이 접두 방식으로 hash한다. Pair hash도 별도 도메인을 쓴다. 6,402개 leaf의 proof는 최대 13개 sibling digest이며 홀수 층의 자기 복제 sibling을 verifier가 강제한다. Shard ID는 전체 plan에서 엄격히 정렬되고 고유해야 한다.

Commitment 생성은 exact-union planner 뒤에만 허용한다. `src/dev/query-ordinary-plan -commitment-dir`는 기존 full plan 검증에 성공한 뒤 caller가 새로 지정한 디렉터리에 `commitment.json`과 각 `proof-000000.json` 형식의 shard proof를 `O_EXCL`로 쓴다. 각 proof 파일은 16 MiB 이하이며 6,402개 proof와 commitment의 6,403개 항목은 기존 10,000 entries 한도 안이다. Internal `ordinarycommit.Build` 자체는 P0 의미나 exact union을 증명하지 않는다. 따라서 arbitrary/unreviewed root를 생성하는 것만으로 승인이 되지 않는다. Caller는 plan과 commitment 각각의 path/bytes/SHA-256을 외부 `ordinary` approval에 고정해야 한다. 선택된 shard ID의 승인 순서, proof의 증가하는 index, 선택 집합 중복 금지 및 각 shard의 ID 일치도 함께 확인한다. P2-C private binding의 `plan_commitment_pin`은 commitment 문서를 가리킨다. r8 manifest와 run-binding/r2의 기존 `shard_plan_pin`은 계속 완전한 P2-A plan을 가리키며 commitment의 `complete_plan_pin`과 같아야 한다.

선택 pass는 commitment, 선택한 raw shard/proof와 caller approval만 받는다. Commitment와 공통 header는 strict typed shape로 확인하며, 선택 proof는 정확한 shard 원문·index와 함께 root에 포함되는지 검사한다. 선택 shard에서 route query·history마다 canonical coordinate를 다시 만들어 obligation ID 순서와 shard digest를 재계산한다. 전체 106,208개 coordinate와 plan set digest는 재해시하지 않는다. Registration 원문 전체는 strict token/typed-shape 검사로 UTF-8/JSON, duplicate/unknown/missing/null/type 오류를 확인한다. 전역 case/query/obligation ID 유일성과 case/query/step/tree-role tuple의 정확한 union·교차 참조도 얇은 투영으로 유지한다. 비선택 coordinate ID의 암호학적 재계산 및 source/asset·고정 scope의 local 의미 검사는 planner가 이미 전수 검증한 registration pin과 승인 provenance를 상속하며, 각 선택 pass에서 해당 행을 다시 검사한다. 다만 전체 case/query/required typed object는 만들지 않고 선택 case 및 해당 route의 query/required 항목만 typed decode해 결속한다. 원문 전체 read와 global token 검사 비용은 원장에 남는다. Registration의 strict shape·header·`version_pins` count와 span 추출은 한 token scan을 공유하며 비선택 header를 다시 unmarshal하지 않는다. Cached header의 전역 ID·tuple 대조를 기존 의미 검사 순서로 수행하고 구문 오류의 우선순위를 보존한다. 실제 decoder·header 저장소·선택 typed decode의 예약 식은 [eligibility 원장](query-reference-eligibility.md#362-승인된-shard-plan-commitment)을 따른다.

전역 ID 유일성·정확한 union·총계·overlap/gap 보장은 commitment를 외부 caller가 승인한 plan pin에 고정함으로써 상속한다. 근거는 단순 hash 일치가 아니다. 승인된 plan을 생성한 검증 경로가 pinned P0 source를 대상으로 전체 validator를 성공시킨 뒤 commitment를 만들었다는 provenance를 caller가 검토하고 pin해야 한다. Pass는 승인 이후 같은 plan/header/root의 정확한 선택 leaf임을 proof로 확인하고, 선택한 coordinate를 독립 재구성한다. Approval은 caller가 해당 commitment를 선택했다는 기록이지 서명·검토자 인증 자체는 아니다.

| 변조 | 거부 근거 |
|---|---|
| 선택 shard의 bytes/ID 치환 | leaf/root 포함 proof 불일치 또는 승인 selected ID·재구성 ID 불일치 |
| 다른 plan의 shard 혼입 | proof가 다른 plan pin/header/root에 결속됨 |
| 선택 proof 또는 ID 순서 교환 | 증가하는 고유 index와 승인된 ordered selected shard IDs 불일치 |
| 증명 누락·중복 | proof 수가 승인 선택 수와 다르거나 index/ID 중복으로 거부 |
| 전체 plan의 다른 shard 누락·중복·overlap/gap | commitment만으로 새 root를 caller가 승인할 수 없으며, 승인 root는 full planner의 exact-union 결과에만 발급 |
| 같은 digest로 충돌을 시도 | SHA-256 충돌 저항성, leaf/node domain 분리, length-prefix·index·count·plan/header binding 가정. 실제 동일 digest 충돌은 digest 검증만으로 탐지할 수 없음 |

`src/kit`은 offline core의 `TestOfflineClosure` 경계 때문에 `src/internal/ordinarycommit`을 production import하지 않는다. Kit의 private verifier는 CLI 내부 builder와 같은 wire/hash format을 사용하며 고정 검증 vector로 호환성을 확인한다. 이는 공개 kit API를 추가하지 않는다.

### 기존 budget model의 누락과 reference 재계산

아래 353,877,949 B와 SQL 잔액은 P2-A의 full-plan revalidation 전 역사적 진단값이다. 현재 pass는 qualification inventory 전체 bytes/hash·전역 token 검사와 선택 route typed decode(K4), registry 전체 typed decode를 유지하며 plan/registration 검증을 선택형으로 바꾼다. 아래 full inventory+registry typed decode 값은 현재 K4 원장에 그대로 적용하는 고정 비용이 아니다. 예산 재검토는 P2 §5의 잔액 계산에 pass마다 반복되는 qualification inventory 및 registry decode가 빠졌음을 확인했다. 당시 full typed decode 모형의 고정 입력 비용은 ordinary pass마다 **353,877,949 B**였다. 이는 query-slot inventory decode를 포함하지 않는다. Slot 및 새 ordinary documents의 추가 charge는 별도 입력으로 더해지며 아직 미확정이다. 과거 verified-part 재사용 후의 historical core anchor와 알려진 set layer를 그대로 사용해 고정 decode를 차감하면, ordinary body를 읽기 전 잔액은 PostgreSQL −297,750,339 B, TSQL −341,653,347 B다.

| 정적 산술, B | PostgreSQL | TSQL |
|---|---:|---:|
| 작은 inherited logical 한도 | 1,073,741,824 | 1,073,741,824 |
| historical core anchor | 1,017,472,466 | 1,061,314,324 |
| 알려진 set layer | 141,748 | 202,898 |
| pass당 qualification inventory 및 registry decode | 353,877,949 | 353,877,949 |
| body 전 잔액 | **−297,750,339** | **−341,653,347** |

이는 #366 이후 historical core anchor를 쓴 진단 계산이다. PR #366 코드는 P2-A base에 없고, P2-A는 이를 가져오거나 K0–K4·structural 절감액을 적용하지 않는다. 이 계산은 current ordinary closure의 실측 합계라고 주장하지 않는다. 당시 full-decode 모형에서 고정 decode를 포함하고 기존 1 GiB 상한·작은 pass 모델을 유지하면, 위 최소 진단치만으로 두 SQL route의 body 전 여유가 음수이므로 density를 바꿔도 capacity BLOCKED라는 점을 보여준다. 실제 current closure와 모든 output 상한이 입증되지 않은 현재 planner 판정은 `CAPACITY_UNPROVEN`/BLOCKED다. 한도는 올리지 않으며 별도 ordinary 예산도 만들지 않는다.

Reference의 기존 P2 row 모형도 receipt의 Base64 stdout, JSONL row의 두 차례 decode(head/full), structural query에도 적용되는 `reserveReference` 비용을 모두 포함하지 않아 약 세 배 낮게 잡았다.

| U1 최대 step 정적 추정, B | PostgreSQL nominal | TSQL nominal | TSQL dense |
|---|---:|---:|---:|
| 기존 P2 reference row 모형 | 10,809,604 | 58,141,956 | 232,269,572 |
| receipt·두 decode·reservation을 반영한 추정 | 30,249,029 | 177,534,284 | 708,656,264 |

이 값들은 실제 크기나 안전 upper bound의 측정치가 아니다. 특히 `nativeReservation`의 상세 비용, API/fact 본문, 남은 set/registration/binding 비용과 실제 expected output 상한은 여전히 미확정이다. `OrdinaryReferenceEstimate`는 `Density`, `SourceBytes`, `VersionsTextBytes`, `RowBytes`, `ReceiptBytes`, `ReceiptLogical`, `DoubleRowDecodeLogical`, `ReserveReferenceLogical`, `TotalLogical`, `PreviousModelLogical`을 분리해 담는다. 이는 낙관적 이전 모델과 코드 기반 추정의 차이를 검토하기 위한 자료다. `OrdinaryShardCapacity`는 `ClosurePasses=1`, `FixedDocumentDecodeLogical=353877949`, `SourceHistoryBytes`, `DeclaredMemberBytes`, `UpperBound=nil`을 노출한다. Top-level budget에는 고정 pass decode, SQL historical anchor/set 차감 진단, route별 nominal/dense reference maxima를 둔다. Profile binding과 route별 ≤15-query reference groups는 plan에서 한 번 고정해 각 case shard에서는 참조하며, 이 문서 factored metadata는 plan 파일 한도를 지키는 표현이다. 이것은 native/reference output, raw decode 비용이나 logical admission을 줄이는 runtime 구조 절감이 아니다. Capacity plan은 per-pass fixed input decode와 source/member/reference/native 비용을 나눠 기록하되, 이 추정치를 measured upper bound나 admission PASS 근거로 쓰지 않는다. 부족한 상한은 `CAPACITY_UNPROVEN`/BLOCKED로 남긴다.

`src/dev/query-ordinary-plan`은 P1 source export, 검증된 P0 registry, 사전 등록 profile/assets/registration, 그리고 선언된 closure/build/control member bodies를 읽는 source-only 도구다. Native/reference output을 기대값으로 소비하거나 해당 프로세스를 시작하지 않는다. Asset metadata와 제공된 body의 bytes/hash/shape는 검사하지만, P1 rule skeleton에는 독립 full-CST/capture/fact의 완전한 output 상한 계약이 없다. 향후 admission에 필요한 정확한 expected counts와 output caps는 현재 입력에 정의되어 있지 않아 각 shard의 `UpperBound`는 nil이고 모든 ID가 `CAPACITY_UNPROVEN`/BLOCKED다. P2-A는 P1 export의 source bytes를 pinned qualification input 및 embedded source와 대조하고 기존 edit 적용 규칙으로 모든 version bytes/hash/points를 재구성한다. Export의 `Origins` fixture path/selector는 출처를 설명하는 `DERIVED_UNREVIEWED` label이며 fixture 원본 row bytes를 포함하지 않으므로 그 자체로 각 fixture origin을 독립 인증하지 않는다. Registration의 `original_selector`는 변경 불가 witness selector와 같아야 하지만 origin fixture attestation을 대신하지 않는다. Density 추정만으로 capacity PASS를 내지 않는다. TSQL이 현재 `CAPACITY_UNPROVEN`/BLOCKED로 남는 것은 계획의 정상 결과이며 예산 상향이나 별도 ordinary 예산을 뜻하지 않는다. P2-A 산출은 실행 승인이 아니며 실제 guarded charge와 semantic 판단은 후속 범위다. Zero-edit case의 edit points는 witness(nil), registration과 plan(빈 목록)에서 길이 0이면 같다고 본다. 이전에는 registration 검증(`validateOrdinaryRegistration`)이 nil만, 공개 API가 빈 목록만 받아 zero-edit case의 registration이 둘을 동시에 통과할 수 없었다. 비교는 `slices.Equal`이며 길이와 각 원소가 다른 points는 계속 거부한다.

## #362 ordinary guarded reader와 independent adjudicator P2-C

통합 P2-C branch에는 ordinary native build closure 검증, raw base verdict 재계산, root-bound asset resolution, reference/independent adjudicator/runtime fold를 같은 coordinator로 연결했다. `VerifyOrdinaryQuerySemantics`는 exported experimental entry다. Valid HARD receipt를 사용한 focused Windows E2E는 actual owned native producer의 JSON r8을 public API 하나로 읽어 `alt-json-B01-05`의 tuple 1/1을 VERIFIED로 fold하고 base·eligibility·semantic PASS 뒤 seal했다. Test-only runner overlay가 native child를 1 GiB/30 s HARD, `JOB_OBJECT` cleanup, 잔류 자식 0으로 강제했다. 여섯 build-step receipt는 각각 4 GiB HARD/120 s를 사용했으며 제품 기본 4 GiB native operation policy는 변경하지 않았다. 원장은 logical 206,346,731 B, work 7,835,933, read 11,216,999 B였고 focused test는 PASS, SKIP 0, 27.24 s였다. 여섯 public negative call도 모두 통과했다. Approval과 reference row는 기존 test fixture에 기반한 synthetic 자료이며 reference process는 실행하지 않았다. Independent expected는 사전에 작성된 JSON anchor에서 왔지만 합성 reference row도 그 authored anchor에서 만들어지므로 reference와의 독립 실행 대조를 뜻하지 않는다. 선택 proof를 담은 complete-plan scaffold도 전체 P0 source-only exact-union planner provenance가 아니다.

Strict-HARD SQL capacity run도 선택 body 구간을 측정했다. PostgreSQL `alt-postgresql-sql-B04-22`는 actual owned parser-paged r8로 9개 metadata, independent grammar-authored 10-node CST, empty captures/declaration facts 및 synthetic reference rows를 사용해 body/reference/adjudication/fold/identity/seal까지 갔다. 원장은 logical 697,519,126 B, work 14,102,601, read 134,443,515 B였지만 9 tuple은 base fact-reproduction gate 미재계산으로 `ORDINARY_PREREQUISITE_INCOMPLETE`/BLOCKED로 남았다. TSQL `alt-tsql-B01-r001`은 actual owned binary-paged r8 처리 중 `ordinary-base-tree-projection` 단계에서 다음 logical 예약 1,731,872 B에 1,265,854 B가 부족해 `ELIGIBILITY_WORK_LIMIT`으로 멈췄다. 중단 시 logical 1,073,275,806 B, work 31,403,098, read 126,923,139 B였다. TSQL reference 40개는 synthetic placeholders이고 expected assets는 UNAVAILABLE이므로 중단 후의 reference/adjudication/fold/seal 비용은 측정하지 않았다. SQL full-pass admission은 여전히 `CAPACITY_UNPROVEN`/BLOCKED이며 한도는 유지한다.

2026-11-12 ordinary base fact 후속 검증은 위 PostgreSQL의 미재계산 gate를 해소한다. Adapter는 같은 guarded session에서 fact pack·mapping을 한 번 확인한 뒤, P2-B2의 ordered suffix/query SHA와 mapping revision을 결속하고 편집 후 각 source version 및 incremental/fresh 각각의 raw captures로 declaration/dynamic SQL/XML facts를 다시 도출한다. 전체 step·role·query의 저장 `step_facts`와 step0 base `facts` projection을 대조하고, 기존 case claim의 step0 의미를 유지한 재계산 값을 공용 `semanticBase`에 전달한다. 저장 fact 자체는 독립 expected가 아니며 tuple의 metadata/CST/capture/fact 독립 비교를 생략하지 않는다. Legacy single small semantic admission은 fact pack을 받지 않으며 r4–r6와 r10 의미·한도를 바꾸지 않는다.

같은 PostgreSQL shard·실제 owned parser-paged r8·독립 DO expected·합성 approval/reference를 사용한 새 공개 호출은 `BaseVerified=PASS`, semantic/assessment `PASS`, required 9/verified 9/failed 0/blocked 0/unprocessed 0, complete/processing true다. 새 원장은 logical 698,039,669 B, work 14,108,124, read 134,443,451 B다. 이 수치는 선택 shard의 `FULL_BODY_MEASURED_VERIFIED`이며 전체 PostgreSQL route/P0 admission이나 reference 실행 증거가 아니다. 앞의 697,519,126 B 측정은 변경 전 역사적 결과다. 같은 후속 실행의 TSQL은 logical 1,073,723,418 B/work 30,896,698/read 126,923,074 B에서 `projection/steps/0/route`의 다음 logical 예약 32,896 B에 14,490 B가 부족해 중단됐다. Fact context를 base 전에 확인해 이전 측정과 중단 위치가 달라졌으며 full capacity는 여전히 미입증이다.

이 관측은 plain JSON 한 shard와 PostgreSQL/TSQL의 각 선택 SQL shard에 한정되며 P2-C 전체, 세 closure encoding family 전체, P0 full scope, 전체 ordinary SQL 수용 또는 P2-D의 완료가 아니다. Reader cache의 `ready`는 의미 자격이나 외부 승인 완료가 아니다. P2-D의 owned-native/r9 replay CLI 양성 fixture는 synthetic approval/reference overlay를 쓰므로 실제 reference 실행·caller approval과 full current SQL capacity는 별도 미확인이다.

Request에서 필수로 받는 `Selection`은 공개 `OrdinaryQuerySemanticsBinding` alias를 통해 caller-owned pin 묶음을 전달한다. Coordinator는 외부 approval을 확인하기 전에 이 선택을 plan commitment, 순서가 있는 shard proofs, registration과 대조해 selected required 분모를 확정한다. Selection이 비어 있으면 invalid input이다. Approval이 없으면 선택 분모의 모든 ID를 BLOCKED로 보존하며 `Complete=false`, `ProcessingComplete=false`를 반환한다. Valid한 caller selection은 approval을 대신하지 않고 `approval.ordinary`의 모든 pin과 일치해야 eligibility authority가 생긴다.

Producer profile은 실행 `native-query-batch`, UTF-8, tree output, API=true여야 한다. Caller가 보유한 실행 profile, 계획 profile, registration, assets, full shard plan, 별도 plan commitment, run-binding 및 reference-selection bytes는 approval의 `ordinary` block에서 각각 path/bytes/SHA-256까지 pin해야 한다. r8/run-binding/r2는 기존 `shard_plan_pin`으로 full plan을 가리키고, block의 `plan_commitment_pin`은 proof root를 가리킨다. `closure_encoding`, candidate, semantic cohort, run ID와 ordered selected shard IDs도 block과 r8/r2 자료 사이에서 일치해야 한다. 일반 eligibility registration은 기존 Build/Control closure를 재사용하되 ordinary context에서 legacy single-native/observation witness는 제공하지 않는다. 해당 eligibility 경로의 `WitnessRegistration`은 빈 bytes여야 한다.

Ordinary 호출은 하나의 `run`, 1 GiB logical·32,000,000 work·256 MiB read·기타 기존 small 한도를 공유한다. K4 `parseQualificationInventoryRoute`로 선택 route를 strict decode하며 fixed registry/inventory pins를 확인한다. Full plan exact union은 승인된 commitment 생성 시 한 번 증명하고, pass는 commitment/header, 선택 shard inclusion proofs와 선택 coordinate 재구성으로 그 결과를 상속한다. Eligibility가 직접 검증한 같은 root/snapshot 안의 #366/#372 verified-part record와 streamed read를 재사용한다. 이 권한은 같은 approval·registration·owner·cohort·physical identity에만 유효하며, 다른 root/session/cohort의 part를 주입하면 실패해야 한다. Unbound path는 기존 full-read/hash 검증으로 돌아가며 ledger나 limit을 새로 만들지 않는다. `parser-paged`와 `binary-paged`는 part authority만 선택하고 logical 한도를 올리지 않는다. Internal shape/helper tests는 통합된 selected ordinary pass의 actual 원장 측정으로 세지 않는다.

통합 reader는 r8 manifest, r5 case record와 batch transcript의 물리 member, actual request/response frame, ordered frame index·case ID·span·owner·source history 및 reference receipt/row를 한 번의 guarded evidence snapshot에서 대조해야 한다. Case cache는 invocation/frame/case/history/build/runtime/policy에 결속하므로 같은 bytes hash인 다른 case나 step을 섞을 수 없다. `ReferenceSelection`은 `(case, step, query)` selector마다 실제 response ordinal·offset·length·SHA와 source/query pin을 지정한다. 같은 step의 reference row를 incremental/fresh 두 role에서 공유해도 required tuple과 independent expected는 각자 남는다.

Expected metadata의 `predicates`는 명시적인 빈 목록이어야 한다. 별도 authored expected full CST와 ordered captures는 actual full native tree·structural capture·source에 독립 비교되어야 한다. `CompareAPI`는 raw API table의 shape 검증 뒤 native tree에 대한 의미 parity를 확인한다. Fact expected는 별도 expected body를 사용하고 pinned fact pack/mapping을 source version과 capture로 재계산해 대조해야 한다. P2-B `step_facts`의 내부 재도출은 독립 expected가 아니다. 실제 reference stdout은 request/receipt/footer/row와 metadata/CST/capture 대조를 위한 관측이며 기대값으로 사용하지 않는다. metadata FAIL과 field-only CST 차이가 함께 있으면 FAIL을 보존하고 CST 판정은 BLOCKED/`INELIGIBLE`로 남겨야 한다.

통합 coordinator는 guarded r8 reader와 native build closure를 확인한 다음 각 raw case를 기존 `semanticBase` reducer에 전달해 query expectation과 base claim을 재계산하고 저장 projection 및 이전 FAIL과 대조한다. 이 순서를 통해 뒤의 reference 또는 expected asset 읽기가 resource/cancel 오류로 끝나도 이미 판정한 base FAIL이 남는다. 그 다음 `ordinaryReferences`로 승인된 `(case, step, query)` row를 확보하고, guarded asset resolver가 metadata/CST/capture/fact body와 authoring/review pins를 같은 verified graph member bytes에서만 해석한다. 독립 tuple을 판정하고 runtime fold를 수행한다. Selected shard의 `verified + failed + blocked + unprocessed`는 고정 required 수와 같아야 한다. `processing_complete`는 선택 shard 처리가 끝났다는 뜻이고 `full_scope_complete=false`와 registry의 `INCOMPLETE`를 바꾸지 않는다. 정상 완료 경로만 마지막 snapshot 확인, 출력 예산 예약과 결과 봉인을 끝낸다. Approval 부재의 BLOCKED 분모나 budget/cancel/read 오류 부분 결과는 seal되지 않고 `Complete=false`다. Identity 재확인은 관측된 `SameFile`·크기·mtime을 비교하며, 같은 inode에서 내용을 바꾸고 크기/mtime을 원래대로 되돌리는 적대적 변경까지 막는 보장은 기존 [호출자 소유권 범위](public-go-api.md#s01-함수와-소유권)에 설명한 대로 제공하지 않는다. Identity mismatch가 확인되면 해당 pass의 positive judgment를 무효화한다.

`plain`, `parser-paged`, `binary-paged` build closure는 grammar, driver, compiler, runtime, logical descriptor를 caller approval 및 current cohort와 대조한다. Paged part는 같은 verified session의 #366/#372 authority와 streamed chain으로 확인한다. 이 계약의 helper 시험은 실제 build-family positive를 입증하지 않는다. 독립 expected는 producer `step_facts`나 reference 출력이 아니다. P2-D의 owned-native/r9 positive fixture는 선택 경로의 연결을 확인하지만 synthetic approval/reference overlay를 쓰므로 실제 caller-approved end-to-end workload로 집계하지 않는다.

P2-D stored result/reducer와 기존 replay CLI 연결은 아래 절에서 다룬다. Ordinary producer를 선택하는 `oracle record` CLI는 [CLI 계약](cli-and-profile.md#362-ordinary-record-cli)을 따른다. Reference receipt r2, body 크기의 안전한 admission 상한 및 current SQL closure가 아직 입증되지 않았으므로 SQL full ordinary 수용은 `CAPACITY_UNPROVEN`/BLOCKED이며 한도는 유지한다. Full public API와 current SQL capacity 측정의 미완료는 helper-level 결과와 구분한다. 선택 SQL shard의 공개 pass는 PostgreSQL parser-paged `alt-postgresql-sql-B04-22`(base fact 재계산 뒤 9/9 VERIFIED)와 TSQL binary-paged `alt-tsql-B01-r001`(seal 도달, 독립 expected 부재로 72/72 BLOCKED)에서 측정됐고 위 절의 한정을 따른다. Selected JSON public call의 PASS도 전체 P0·다른 route·closure family나 SQL admission을 입증하지 않는다.

## #362 ordinary replay·stored decoder P2-D

P2-D는 selected ordinary pass의 저장 결과를 public `DecodeOrdinaryQuerySemanticsResult`로 다시 읽고, `oracle-set-r9`/revision `r9`를 통해 raw P2-B `manifest/r8`, `record/r5`, batch capture와 outer caller documents를 같은 ordinary core로 연결한다. Outer replay result는 `tsgk-replay-result/r4`이며 ordinary payload field는 `ordinary_query_semantics` (`tsgk-query-semantics-result/r2`)다. Outer `tsgk-replay/r1` profile shape와 이전 reducer의 뜻은 바꾸지 않는다. R7/r8은 각각 V2-B2 runtime-pin replay와 V2-C3 accepted-event/CST replay에 이미 배정되어 있어 새 ordinary reducer에는 쓰지 않는다.

Result의 `selected_obligation_ids`는 선택 shard별 obligation ID의 ordered exact union이다. `coverage_sha256`는 `tsgk-ordinary-shard-members/r1`의 NUL-terminated domain과 각 ID 앞의 8-byte big-endian length를 넣어 해시한 ordered digest다. Replay core는 case·step·query·tree_role coordinate를 재구성해 pinned plan/registration, 이 digest, typed counters와 outer `records`를 대조한다. Stored decoder는 schema/revision·typed counters·budget limits와 selected ID uniqueness·coverage digest·required count 및 completed obligation rows의 selected ID prefix를 검사한다. 이 구조 검사는 저장 결과가 설명하는 native/reference 증거를 다시 읽거나 membership의 caller approval을 재증명하지 않는다. `processing_complete`는 선택 ID의 처리가 끝났는지, `complete`는 API 평가 및 결과 seal까지 끝났는지, `full_scope_complete`는 전체 P0 범위가 끝났는지를 각각 나타낸다. Partial/cancelled/resource-limited result는 확인한 최초 FAIL·code와 counters를 보존하고 `complete=false`로 남긴다.

Replay wrapper는 이미 opened root, `run`, cumulative ordinary ledger와 verified snapshot을 P2-C core에 빌려준다. Wrapper scope/member traversal은 기존 cumulative budget에 비용을 더하므로 API raw totals와 replay totals를 숫자로 같다고 주장하지 않는다. Nested replay의 `bytes_read`/`work`/`logical_bytes`는 outer replay의 실제 totals와 같고 API와 replay의 semantic assessment·최초 code·selected IDs·typed counters는 같아야 한다. 검증 로그는 wrapper delta를 따로 공개한다. Caller document/member를 wrapper에서 재독해 과금하지 않고 reset ledger도 만들지 않는다. Serialized output bound와 wrapper traversal은 같은 budget의 남은 값에 실제 비용을 예약한다. R9 member identity는 기존 `qualificationMembersText`의 merge-sort를 공유한다. `n=len(members)`, `F=Σ(len(path)+len(role)+len(sha256)+32)`, `L=1+ceil(log2(max(1,n)))`일 때 work는 `n + 8×(F+32) + n×(L+2) + Σcomparison(len(left.path)+len(right.path)+1)`, logical은 `3×F+128×n+256`이다. Builder는 `Grow(F+32)`로 반복 확장을 없애며, 실제 비교·merge 이동·출력 복사/해시 및 두 slice·builder capacity·hash input을 test-only 계측해 계상 이상인지 확인한다. Caller identity는 bounded member read가 실제 bytes와 SHA-256을 대조한 뒤 그 verified pin을 재사용한다. Replay 내부 ordinary 결과의 중복 bound/JSON marshal은 생략하고, nested 결과까지 포함하는 최종 outer bound와 exact `sealOutput`에서 동일한 출력 한도를 적용한다. 최종 snapshot identity 확인은 생략하지 않는다. Scope에 verified pin map의 logical `14×256 B`도 예약한다. 한도나 결과의 의미를 바꾸지 않는다. `file`, `read`, `work`, `logical`, `entries`는 기존 small caps이며 새로운 replay allowance나 중간 reset을 두지 않는다. Caller가 저장한 ordinary result는 public decoder로 다시 읽을 수 있다. `oracle record` CLI는 [CLI 계약](cli-and-profile.md#362-ordinary-record-cli)에 따라 같은 source-only 입력을 하나의 누적 원장으로 읽고 계상한다. PostgreSQL `alt-postgresql-sql-B04-22` 양성은 CLI record가 만든 r8을 API·r9 replay·`tsgk replay`가 소비하며 approval·reference는 synthetic이다.

이 reducer의 PASS는 선택 ordinary scope 안에서 raw와 독립 expected를 현재 같은 reader가 다시 계산했다는 결과다. Stored result만 해석하는 것과 차이가 있으며, P0 root의 `INCOMPLETE`, missing SVC/large/predicate scopes, `full_scope_complete=false`는 계속 유지한다. Positive fixture는 실제 owned native producer를 사용하지만 approval/reference는 synthetic overlay이고 repository-tracked full E2E regression과 실제 reference 실행은 아직 별도 경계다. Ordinary SQL pass의 전체 capacity가 확인되지 않은 route는 `CAPACITY_UNPROVEN`/BLOCKED다.

## #216 parser-only paging

일반 작은 source의 `native-query`에서 grammar parser 하나가 16 MiB를 넘을 때만 `full-input-utf8-paged-parser/r1`을 사용한다. 이 context와 `full-input-utf8/r2`에서 source·CST·raw response·driver executable·reference executable/library·다른 closure 입력은 각각 기존 물리 파일 한도를 지킨다. 큰 source/CST, SVC, 전처리, batch, 신규 macOS claim은 이 단위의 대상이 아니다. 기존 small offline 한도 16 MiB/file·256 MiB/shared total·120초·10,000 files·work 32,000,000과 native-query 4 GiB/10초를 유지한다. eligibility 검증(core·public)이 직접 시작한 원장의 logical allocation만 이 context에서 1 GiB 대신 1.5 GiB이고, semantic run의 1 GiB는 그대로다. 별도 large-replay 예산을 이 경로에 적용하지 않는다.

새 context의 producer는 manifest/r6와 기존 profile/r3·record/r4·result/r3를 사용한다. 기존 context `full-input-utf8/r2`와 manifest/r5·`oracle-set-r4`는 기존 의미를 유지한다. 새 `oracle-set-r5`만 paged context/r6를 소비하며 반대 context와의 교차는 거부한다. 공개 의미 결과/r1·replay 결과/r3·eligibility 승인/등록/r1·run-binding/r1·witness/r2는 같은 필드 의미를 유지한다. `OraclePagedSemanticsManifestSchema` 상수로 새 manifest revision을 구분한다. 기존 r5만 허용하는 reader는 r6를 거부한다.

`capture/build-input-%05d.pages.json`은 실제 물리 bytes·SHA-256의 `grammar-paged-descriptor` member다. descriptor `tsgk-query-parser-pages/r1`의 필수 필드는 `schema`, `owner: D`, `logical {closure_index,path,role,bytes,sha256}`, `parts [{index,offset,path,bytes,sha256}]`다. 원본 parser는 `grammar-parser`, 16 MiB 초과·기존 build 100 MiB/file 이하이며 정확히 하나다. 각 part의 역할은 `grammar-paged-part`, 이름은 같은 closure index의 `capture/build-input-%05d.part-%05d.bin`이다. part index는 0부터 연속이고 offset은 index×16 MiB, 마지막 이외 크기는 16 MiB, 마지막은 양수이며 16 MiB 이하이다. 누락·중복·교환·겹침·truncation·불필요한 조각과 대소문자 경로 충돌은 거부한다.

분할은 실제 build 제거 전에 동일 열린 parser에서 수행한다. 원본 크기·전체 해시·읽기 전후 파일 동일성을 확인하며 원본 parser를 다시 합치지 않는다. `build.json`과 plain build manifest는 compiler가 사용한 원래 logical 입력·순서·build identity를 보존한다. offline 검증은 동일 evidence pass에서 이미 확인한 cached part bytes의 개별 해시 및 연결된 전체 해시를 계산하고, 외부 profile의 원본 pin·build closure·plain manifest에 대조한다. descriptor 자신의 전체 해시 선언만으로 승인하지 않는다.

registration의 physical 계획은 build 전에 고정한다. 기존 r5 producer의 hash-bound 등록 bytes는 불투명한 입력으로 유지하며, bounded context 선택만으로 새 형식의 적격이나 PASS를 주장하지 않는다. 새 context로 명시적으로 선택된 경우에만 strict private 사전 검사를 적용한다. 이 검사는 기존 문서 상한 안에서 JSON 문법·UTF-8·중복 key·후행 값·깊이·값 수와 schema/context/planned member shape·parser part 개수/이름을 검사한다. 토큰 사이에 caller 취소 및 유한 deadline을 확인하며 단일 token 처리 도중 강제 중단을 보장하지 않는다. header·planned member 추출도 같은 token pass에서 수행하며 큰 문서 전체를 후속 Unmarshal로 다시 해석하지 않는다. caller 취소와 내부 wall 만료는 구분한다. 나머지 semantic shape 전체가 사전 검증됐다고 주장하지 않는다. Build가 실제 closure/driver pin을 확정하면 driver 실행 전에 계획과 정확히 대조·capture하고, 실제 member 전체는 완결 manifest 전에 재대조한다. O / manifest.json / manifest가 사전 계획에 정확히 하나 있어야 한다. 빈 stderr는 기존 null/no-file을 유지한다. 예상하지 못한 stderr는 원문을 보존하되 계획 불일치로 실패하며 complete marker를 쓰지 않는다. 실패 시 build cleanup 오류도 보존하며, 계획 불일치가 이미 확정된 CANCELLED·RESOURCE_LIMIT·기존 FAIL을 일반 실패나 PASS로 바꾸지 않는다.

Native.Grammar의 원본 parser 경로는 실제 descriptor 물리 pin을 가리킨다. standalone eligibility도 Native.InvocationID가 가리키는 실제 NATIVE_DRIVER node가 descriptor와 모든 part의 owner인지 확인하고 사용하지 않는 paged member도 거부한다. 의미 binding은 graph·D owner·DriverNode·사전 member 계획까지 정확히 대조한다. 승인 ABSENT에서도 동일 set/whole-file 검증을 수행하고 required 분모를 먼저 유지한 채 BLOCKED로 남긴다. caller-approved PASS는 저장 근거 일관성에 한정되며 실행 진위 인증이나 C# 실제 실행 증거가 아니다.

이 계약의 회귀는 합성 저장 자료의 일관성과 경계 동작을 검사하며 실제 reference 실행 증거가 아니다. C# 사전 ABI 기대 symbol_count=654는 SYMBOL_COUNT650+ALIAS_COUNT4의 고정 runtime API에서 도출한 값이며 실행 관측이 아니다. 실제 compiled ABI·driver/reference 크기·독립 reference 실행은 별도 후보의 관측으로 확인해야 한다.

## #216 binary paging

`full-input-utf8-paged-binaries/r1`에서 16 MiB를 넘는 driver executable과 reference library는 descriptor `tsgk-query-binary-pages/r1`과 물리 part로 보존한다. descriptor의 필수 필드는 `schema`, `owner`, `logical {kind,path,bytes,sha256}`, `parts [{index,offset,path,bytes,sha256}]`다. `kind`는 `driver` 또는 `library`이고 logical 크기는 16 MiB 초과·64 MiB 이하다. part index는 0부터 연속이고, offset은 index×16 MiB다. 마지막이 아닌 part는 16 MiB이고 마지막 part는 1 byte 이상이다. 검증은 같은 evidence pass에서 확인한 part bytes로 개별 해시와 연결 해시를 계산한다. part마다 work 1과 logical `2×(bytes+1)`을 charge한다. descriptor의 logical 값은 연결 해시가 일치한 뒤에만 신뢰 출처와 대조한다. descriptor 자신의 물리 SHA-256은 실행 파일이나 library의 identity로 쓰지 않는다.

reference library descriptor는 graph 상대 logical 경로 `L`에 대해 `L.pages.json`, part는 `L.part-%05d.bin`이다. descriptor의 `owner`와 모든 part의 소유 node는 registration `Native.ReferenceLibrary`의 node와 같다. eligibility는 approval에 결속된 이 pin의 descriptor를 실행마다 한 번 재결합하고, current reference argv와 response의 library SHA-256을 재결합 logical 값과 대조한다. 다른 context에서 descriptor role의 reference library는 거부한다. control cohort의 library는 기존 물리 한도를 유지한다.

이 context의 standalone eligibility는 16 MiB 초과 grammar parser를 0개 또는 1개 허용하고, approval에 결속된 registration의 executable·library pin 중 하나 이상에 paged descriptor role을 요구한다. parser만 paged이면 `full-input-utf8-paged-parser/r1`을 사용한다. 이 조건은 native build manifest 검증의 조기 반환 전에 판정하며 historical runtime에서도 위반은 FAIL이다. driver descriptor는 `oracle/capture/driver.pages.json`, part는 `oracle/capture/driver.part-%05d.bin`이며 logical 경로는 `capture/driver.bin`, owner는 `D`다. descriptor와 모든 driver part는 선택된 NATIVE_DRIVER invocation node가 소유해야 하고 미사용 part는 거부한다. 재결합한 logical SHA-256을 approval의 `executable_sha256`과 대조하며 다른 context의 descriptor executable은 거부한다. 새 context의 producer는 manifest/r7과 기존 profile/r3·record/r4·result/r3를 사용한다. 새 `oracle-set-r6`만 binaries context/r7을 소비한다. 기존 두 context와 manifest/r5·r6의 의미는 바뀌지 않는다. 의미 검증 registration은 16 MiB를 넘는 grammar parser를 0개 또는 1개 허용한다. driver 분할 producer와 library 분할 controller는 이 절의 형식을 따른다. oracle set·semantic 연결은 [Binary-paged semantic set](identity-and-evidence.md#binary-paged-semantic-set)을 따른다.

Native driver producer는 binaries context에서 executable이 16 MiB를 넘을 때만 분할하며, 16 MiB 이하는 기존 `capture/driver.bin`과 `driver` role로 보존한다. 분할은 build 제거 전에 한 번 연 동일 파일에서 16 MiB 단위로 수행하며 원본 전체를 다시 합치지 않는다. 읽기 전 일반 파일·build의 예상 크기, 매 part의 caller 취소, 마지막 EOF와 읽기 전후 `SameFile`·크기·`ModTime`·전체 SHA-256을 확인한다. 검증이나 close가 실패하면 부분 결과를 반환하지 않는다. native set 내부 경로는 `capture/driver.pages.json`(`driver-paged-descriptor`)과 `capture/driver.part-%05d.bin`(`driver-paged-part`)이며, graph에서는 위 `oracle/` 접두 경로에 대응한다. descriptor의 logical identity는 원본 build executable의 bytes·SHA-256이고 descriptor member의 물리 identity와 구분한다. logical binary 상한 64 MiB와 각 물리 member의 16 MiB 한도는 유지한다.

Hash-bound registration의 driver 사전 계획은 plain driver 하나 또는 descriptor 하나와 연속 번호 0부터의 part 2~4개만 허용한다. 혼용·경로 오타·알 수 없는 paged driver role·다른 context의 paged driver role은 거부한다. 사전 검사는 part 수의 범위와 경로 shape를 확인하고, 정확한 part 수는 build 뒤 실제 capture와 driver 실행 전에 대조하며 전체 member도 완결 manifest 전에 재대조한다. parser paging은 같은 context에서 큰 grammar parser가 있을 때만 공존하며 parser descriptor·part role은 driver role과 분리한다. reference library의 분할 수집은 controller가 담당하고 native driver producer가 library를 수집하지 않는다. 생산의 명시 선택은 [binary paging opt-in](cli-and-profile.md#216-binary-paging-opt-in), 시험·로컬 compile 근거와 CI 선택 범위는 [검증 계약](../validation/validation.md#216-binary-paging-계약-검증)을 따른다.

공개 의미 검증은 set 검증과 eligibility 검증이 driver part 해시를 각각 한 번씩 계산하므로 driver의 part hash charge가 두 번 들어간다. 공유 work 한도 32,000,000을 유지하며, logical 한도는 원장을 상속하는 semantic·replay·qualification run에서 1 GiB다. eligibility 검증이 원장을 직접 시작하고 approval에 결속된 binaries context일 때만 parser-paged context와 같은 owner·bound 조건으로 1.5 GiB를 적용하며, 공개 API의 logical 최악 상한도 1.5 GiB로 같다. 이 중복 charge를 이유로 한도를 올리지 않는다. reference executable(`build/reference.exe`)과 control cohort의 library는 이 문맥에서도 기존 물리 한도를 지킨다. 큰 source/CST, SVC, 전처리, batch, 신규 macOS claim은 대상이 아니다.

## Windows C# parser paging 첫 실제 표본

[독립 anchor fixture](../../src/testdata/query-semantics/csharp3-anchors.json)는 원래 실행 전 등록에서 필요한 기대값만 보존한다. `class C {}` → `class D {}`의 한 바이트 편집과 `((compilation_unit) @root (#eq? @root "class C {}"))`를 사용한다. 필수 표본은 초기 incremental, 편집 후 incremental·fresh의 세 개이며 structural root는 각각 1개, 평가 capture는 1/0/0이다. ABI 기대값은 15/15/13/654/26이며, fixture의 child anchor는 정적 기대값이다. planner는 이를 별도 파일로 보존하며 수동 assemble·verify 단계는 그 파일을 소비하지 않는다. 이 cohort는 실행 전에 고정한 별도 비교기에서 type/span의 유일성·실제 root→class→identifier 부모 관계와 C/D 원문을 대조했다. 이후 실행도 별도의 독립 대조를 갖춰야 한다. 관측된 child node 번호로 사전 기대값을 만들지 않는다.

로컬 Windows cohort에서 원본 parser 51,472,926바이트를 descriptor와 네 part로 보존하고 native/reference의 전체 7-node CST를 대조했다. 외부의 조건부 `approved-local` 승인 후 API와 `oracle-set-r5` replay는 같은 83,824,089바이트와 여덟 digest를 확인했으며 build9·unit4·control53·assertion36·tuple3을 VERIFIED로 판정했다. 승인 없음은 base PASS·3 BLOCKED, owner·slice 변조는 FAIL이며 원 producer의 COMPLETED/BLOCKED를 보존했다. 이 표본의 API 논리 처리량은 856,861,968바이트로 기존 1 GiB 이내였다. 이는 해당 저장 cohort의 일관성·capacity 관측이며 실행 진위 인증이나 전체26 route·SVC·large·신규 Linux/macOS 의미 검증을 뜻하지 않는다. Native/reference 캡처 소스 `4805c30e`와 이후 offline 조립기 소스 `47dfda68`을 구분하고, 재사용 build의 경로는 원래 영수증의 argv에 결속한다.


## #216 Go3 정의 분류의 수동 연결

[Go3 독립 기대값](../../src/testdata/query-semantics/go3-classification-anchors.json)은 원래 Go query 세 개의 정확한 원문·hash, pattern 수4/2/3, 첫 등장 capture 이름, 명시적 predicate 빈 배열을 보존한다. 고정 grammar의 빈 source는 단일 `source_file` root이며, 세 query의 초기 incremental structural/expected captures는 각각 빈 배열이다. ABI 기대값15/15/13/222/35와 CST 12개 필드는 관측 전에 선택한 정적 기대값이다. 다른 route나 SVC·large의 빈 입력·capture에 일반화하지 않는다.

명시적 local helper는 기존 `driverCapture`와 `b.runCase`로 실제 request·plain process·raw frame·build closure를 보존한다. Semantic producer opt-in과 pending leaf는 만들지 않으며, 기존 nonempty predicate 등록 조건을 유지한다. `plan`과 `assemble`은 caller 승인 문서를 생성하지 않는다. 과거 검증된 build/control의 원래 identity·정확한178 member를 보존해 재사용할 수 있지만, 새로운 current cohort와 graph에는 별도 provenance 검토 및 graph 밖의 새 caller 승인이 필요하다. 실행 전 controller·source·fixture·grammar/runtime·도구·시험 binary 및 유한 예산을 고정해야 한다.

`verify`는 기존 eligibility core의 동일 guarded snapshot에서 native와 각 reference response slice의 full CST를 독립 root 기대값에 따로 대조한다. 같은 snapshot의 실제 stdin/source/query/limits/run·build binding, process Spec, stdout frame/raw와 process member도 확인한다. Raw Spec의 null 환경은 보존하고 비교 DTO의 빈 배열만 구분해 처리한다. 별도의 공개 eligibility API 호출은 같은 고정 caller 문서의 counts·digest 판정을 대조한다. 이 두 offline 호출은 각각 기존 small 한도를 가지며 새로운 공개 API·result revision·semantic 한도를 추가하지 않는다.

Native node의 `run`은 invocation ID `native-go3`이며 별도 관측 run은 preregistration과 observation/cohort에 결속한다. Library compiler의 정확한 request와 도구 hash를 사전에 고정하고, 조립 전과 승인된 graph의 동일 guarded snapshot에서 request·compiler bytes·출력 및 정상 종료·HARD 한도·검증된 정리·잔류0을 확인한다. Native Spec의 `Interactive:false`도 직접 요구한다. 이 추가 검사는 수동 분류 helper의 조건이며 공개 eligibility API의 새 실행 인증이 아니다. 준비 이후 선택된 Go/cc/supervisor/reference의 경로와 hash가 모두 같아야 하며, controller는 별도 진입점의 실행 전 hash 검사를 거친다. Case/native/library/references는 caller가 전용으로 사용하는 실제 디렉터리여야 하고 실행 전 빈 출력 디렉터리·reparse 부재·case 내부 경로를 확인한다. 소유자가 동시에 경로·권한 의미를 바꾸는 경우까지 보호한다는 주장은 하지 않는다.

`assemble`과 `verify`는 caller가 수집 전에 별도로 고정한 plan digest를 `TSGK_QUERY_CLASSIFICATION_PLAN_SHA256`으로 받아 외부 preregistration bytes와 대조한다. 이후 plan·request·graph·승인을 함께 다시 해시해도 이 사전 pin을 바꿀 수는 없다. Guarded graph의 `go-library-build` node도 완료/PASS/NEW_RUN과 compiler·grammar·source·protocol·run 등 정확한 identity를 확인하며, 무결성 검사 자체를 통과한 모순된 node도 분류 결과로 채택하지 않는다.

세 정의가 모두 적격일 때만 별도 수동 결과에 동일한 승인 cohort의 `STRUCTURAL_ONLY`를 기록한다. 이 결과는 caller 승인에 조건부인 저장 일관성이며 실행 진위·서명·EVALUATED 또는 전체 query 슬롯·qualification/fan-in 완료가 아니다. 기본 시험의 수동 helper SKIP도 실제 실행 성공이 아니다. 신규 검증 범위는 Windows Go3로 한정하며 Linux의 새 current 실행과 나머지26 route·SVC·large 의무, macOS 신규 검증 제외를 유지한다.

Go3 anchor의 세 정의는 ordinary `go/s06-go` 원장의 정의와 ID·정확한 UTF-8 원문·계산 hash로 연결한다. 이 정적 연결에는 새 원장 상태나 schema가 필요하지 않으며 r1의 `STRUCTURAL_CANDIDATE`·`compiled_metadata:UNVERIFIED`, `semantic_required:null`·`NOT_ASSESSED`를 유지한다. [Windows Go3 보고서](../reports/issue-216-go3-classification.md)는 당시 grammar/runtime·compiler/driver/reference/library·source snapshot과 외부 승인 graph에 한정한 역사적 관측이다. Query 정의가 같아도 그 실행 context가 달라지거나 확인되지 않으면 현재 compile/metadata 검증으로 승격하지 않는다. CI의 추적 파일 연결 검사는 미추적 raw evidence·caller 승인이나 당시 실행을 재검증하지 않는다.
