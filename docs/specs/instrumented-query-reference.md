# 계측 query reference 계약 (#216)

이 도구는 고정 Tree-sitter 0.27.1의 별도 계측 reference다. Stock CLI full-field parity는 계속 **BLOCKED**이며, 이 도구의 빌드·owned 검사 성공은 전체 `query_semantics` qualification 완료가 아니다. 기존 oracle/replay/qualification revision과 보장 범위는 유지한다.

## 독립성 및 identity

Upstream은 `tree-sitter/tree-sitter`의 `10eb5ac119eec9cb4267a8e035066473983f5f3a`다. [instrumentation.patch](../../src/dev/query-reference/instrumentation.patch)는 Rust binding에 읽기 전용 query pointer와 별도 raw capture iterator만 추가한다. 기존 iterator의 기본값은 predicate 평가를 유지한다. 별도 iterator는 predicate 탈락 match를 제거하지 않으며, emitter가 변경하지 않은 upstream `QueryMatch::satisfies_text_predicates`에 원래 전체 match와 원 source를 넘긴다. Kit 평가기를 호출하거나 kit 결과를 읽지 않는다.

원본 workspace·library manifest를 유지하고 emitter member 하나를 더한다. 원본 lock의 기존 package·version·checksum·dependency를 바꾸지 않는다. `build.py`는 새 package 하나만 추가됐는지 확인한 뒤 `--offline --frozen`으로 선택 package만 빌드한다. 전체 CLI·WASM·bindgen 빌드는 이 경로에 포함하지 않는다. 실제 selected dependency/features의 전체 `selected-tree.json` receipt hash와 종료 전 불변성, metadata, 실행 요청·결과, 원본 archive·patch·emitter·manifest·lock·도구·binary hash를 별도로 보존한다. Build 도구 목록은 실제 Python interpreter의 절대 경로·파일 hash·`sys.version`도 포함하며 build 전후 hash가 같아야 한다. Cargo offline은 의존성 취득 방식이며 network sandbox를 뜻하지 않는다.

`tsgk-instrumented-reference-build/r1`의 `complete`는 빌드 절차 완료다. Caller가 독립적으로 선택한 upstream·도구 취득 근거 및 등록 hash와 연결해야 한다. Evidence가 자체 제출한 identity/hash만으로 reference 자격을 만들지 않는다. 공개 source, compiler/standard-library provenance, 실제 feature closure와 실행 문맥의 신뢰 검토는 자격 검증에 계속 필요하다.

Build와 owned controls는 기존 wire encoder로 요청 bytes를 한 번 만들고 supervisor stdin에 전달하는 동일 bytes를 실행 전에 `*-request.json`으로 저장한다. `batch-truncated-request.json`은 `StdoutBytes=1024`로 바꾼 요청을 보존하며 원래 `batch-sixteen-request.json`은 변경하지 않는다. 요청 저장이 실패하면 그 실행을 시작하지 않는다. 이 보존은 producer의 송신 bytes와 저장 bytes의 일치이며 실행 진위·후속 변조 방지나 reference 자격을 인증하지 않는다. 과거 누락 요청을 재생성해 원자료로 승격하지 않으며 `build.py` 변경 뒤에는 새 입력 hash에 맞는 build identity가 필요하다.

Supervisor receipt는 명시적 `tsgk-reference-process/r1`이며 정확히 `schema`, `execution`, `stdout_base64`, `stderr_base64`를 갖는다. 두 stream은 UTF-8 문자열 변환 없이 원 bytes를 표준 padding 있는 canonical base64로 보존하고 nil·빈 stream은 빈 문자열이다. 소비자는 receipt 원문을 strict UTF-8로 읽고 모든 깊이의 중복 JSON key를 거부한 뒤 schema·필드·base64 type/형식·canonical 재인코딩과 byte count를 검사한다. 비잘림 stream은 원 bytes 길이가 execution count와 같아야 하며 잘린 stream의 보존 prefix는 관측 count 이하이고 실행 완료로 채택하지 않는다. Rust JSON/NDJSON·unit/tool text를 해석할 때만 strict UTF-8로 decode하며 replacement decode나 UTF-16/32 자동 인식을 사용하지 않는다. stderr 진단은 원 bytes와 구분한다. 과거 익명 `stdout`/`stderr` string receipt는 이 raw closure의 근거로 수용하지 않으며 새 supervisor·Python 입력 identity로 build와 controls를 다시 실행해야 한다. Process·stream·small reference/replay 한도는 유지하고 base64 envelope가 offline 파일 한도를 넘으면 부적격으로 남긴다. 이 receipt 보존은 전체 closure·실행 진위·query_semantics 자격 완료를 뜻하지 않는다.

## 결과와 완료 조건

Emitter 인자는 절대 library path, symbol, source path, query path, query ID, 사전 등록 library SHA-256이다. Source는 UTF-8 32 MiB 이하, query는 UTF-8 64 KiB 이하, library는 64 MiB 이하의 regular file이다. Source·query는 읽은 bytes에서 hash와 결과를 계산한다. Library는 실행 전 예상 hash와 비교하고 실행 후 다시 비교한다. 이 검사는 악의적인 동시 파일 교체나 native code 자체를 격리한다는 보장이 아니다. Build archive는 128 MiB 상한의 단일 buffer로 읽어 pinned SHA-256을 확인하고 동일 bytes에서만 tar를 해석한다. 완료 identity 직전 원 archive 경로도 다시 hash하여 남아 있는 변경을 거부한다. 검토·고정한 생성 grammar만 별도 process에서 실행한다. 출력 소유권의 #215 미완료 보장을 이 경로의 hash 검사로 대체하지 않는다.

`tsgk-instrumented-query-reference/r1`은 다음을 제공한다.

- Fresh parse의 전체 preorder CST: parent·field·type, named·extra·is_error·has_error·is_missing, byte·point 전체 필드.
- 실제 compiled query의 pattern 수·capture 이름·predicate 단계와 quantifier.
- 원 capture 순서와 pattern/capture 번호를 보존한 structural·evaluated 전체 stream. Capture의 필드는 실제 captured node에서 읽고 독립 CST의 node identity 및 모든 필드와 대조한다. Span만으로 node를 찾지 않는다.
- 원 runtime match ID와 최초 structural 등장 순서의 match 번호 mapping. Predicate 탈락 뒤 번호를 다시 매기지 않는다.
- 실제 사용 한도, cursor 종료·match-limit 초과 여부 및 `complete`.

각 match의 전체 capture signature·pattern·predicate 결정이 재등장 때 같아야 한다. 각 capture **occurrence index**는 정확히 한 번 반환돼야 하며, 종료 시 누락이 있으면 실패한다. 서로 다른 occurrence의 동일한 capture 값은 보존한다. 단순 개수 합계나 정렬·중복 제거로 이 검사를 대체하지 않는다.

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

수동 workflow `Reference Linux`(`.github/workflows/reference-linux.yml`, `workflow_dispatch`, 공유 heavy lane)는 hosted `ubuntu-24.04`에서 이 취득, Rust 1.97.1 toolchain, supervisor build, delegated cgroup v2를 준비한 뒤 `build.py --target x86_64-unknown-linux-gnu`의 build·Rust unit, `test_pipeline.py`(SKIP 없음), owned `check.py` controls를 실행한다. build identity의 vendor snapshot이 위 값과 다르면 실패하며, 요청·결과 receipt만 `reference-linux-<run>-<attempt>` artifact로 30일 보관한다. 이 실행은 필수 PR check가 아니고 전체 reference 자격·3-host qualification·26 route 대조를 대신하지 않는다. 2026-10-25 사용자 결정으로 macOS(`aarch64-apple-darwin`) 신규 검증은 수행하지 않으며 #243은 NOT_PLANNED로 종료했다. 기존 macOS CI와 과거 근거는 유지하고 신규 claim은 macOS SUPPORTED로 승격하지 않는다.

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

PASS는 외부 승인 기준에 연결한 저장 데이터의 일관성만 뜻한다. 복사한 승인 header를 붙인 fabricated response의 실제 실행 진위는 증명하지 못한다. Manifest의 전체 selected-tree/tool/source/vendor receipt closure 재검증, 독립 실행 진위, qualification eligibility·claim/reducer/inventory/fan-in은 별도 미완료 영역이다. Stock CLI full-field parity의 BLOCKED와 #215의 생성 순간 소유권 공백도 유지한다.

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

정적 문서 상한은256KiB다. 중복·미지 JSON member, 필수 null/0 필드의 누락, 원문 pin·분류·계수·추가 역할 사유의 변경을 거부한다. 재생성은 기존 `TSGK_WRITE_INVENTORY=1`을 명시한 `TestQuerySlotInventory`가 수행하고, 쓰기 모드를 끈 별도 검사로 원장과 음성 대조를 검증한다. 기본 foundation 검사는 원장 존재·정확한 값과 shape를 요구한다. 이는 기존 small/large runtime budget, producer/reducer 또는 qualification verdict를 변경하지 않으며, 독립 predicate probe·reference·전체 semantic completion/fan-in gate는 후속 단위다.

## #216 parser-only paging

일반 작은 source의 `native-query`에서 grammar parser 하나가 16 MiB를 넘을 때만 `full-input-utf8-paged-parser/r1`을 사용한다. source·CST·raw response·driver executable·reference executable/library·다른 closure 입력은 각각 기존 물리 파일 한도를 지킨다. 큰 source/CST, SVC, 전처리, batch, 신규 macOS claim은 이 단위의 대상이 아니다. 기존 small offline 한도 16 MiB/file·256 MiB/shared total·120초·10,000 files·work 32,000,000·logical allocation 1 GiB와 native-query 4 GiB/10초를 유지한다. 향후 별도 large-replay 예산을 이 경로에 적용하지 않는다.

새 context의 producer는 manifest/r6와 기존 profile/r3·record/r4·result/r3를 사용한다. 기존 context `full-input-utf8/r2`와 manifest/r5·`oracle-set-r4`는 기존 의미를 유지한다. 새 `oracle-set-r5`만 paged context/r6를 소비하며 반대 context와의 교차는 거부한다. 공개 의미 결과/r1·replay 결과/r3·eligibility 승인/등록/r1·run-binding/r1·witness/r2는 같은 필드 의미를 유지한다. `OraclePagedSemanticsManifestSchema` 상수로 새 manifest revision을 구분한다. 기존 r5만 허용하는 reader는 r6를 거부한다.

`capture/build-input-%05d.pages.json`은 실제 물리 bytes·SHA-256의 `grammar-paged-descriptor` member다. descriptor `tsgk-query-parser-pages/r1`의 필수 필드는 `schema`, `owner: D`, `logical {closure_index,path,role,bytes,sha256}`, `parts [{index,offset,path,bytes,sha256}]`다. 원본 parser는 `grammar-parser`, 16 MiB 초과·기존 build 100 MiB/file 이하이며 정확히 하나다. 각 part의 역할은 `grammar-paged-part`, 이름은 같은 closure index의 `capture/build-input-%05d.part-%05d.bin`이다. part index는 0부터 연속이고 offset은 index×16 MiB, 마지막 이외 크기는 16 MiB, 마지막은 양수이며 16 MiB 이하이다. 누락·중복·교환·겹침·truncation·불필요한 조각과 대소문자 경로 충돌은 거부한다.

분할은 실제 build 제거 전에 동일 열린 parser에서 수행한다. 원본 크기·전체 해시·읽기 전후 파일 동일성을 확인하며 원본 parser를 다시 합치지 않는다. `build.json`과 plain build manifest는 compiler가 사용한 원래 logical 입력·순서·build identity를 보존한다. offline 검증은 동일 evidence pass에서 이미 확인한 cached part bytes의 개별 해시 및 연결된 전체 해시를 계산하고, 외부 profile의 원본 pin·build closure·plain manifest에 대조한다. descriptor 자신의 전체 해시 선언만으로 승인하지 않는다.

registration의 physical 계획은 build 전에 고정한다. 기존 r5 producer의 hash-bound 등록 bytes는 불투명한 입력으로 유지하며, bounded context 선택만으로 새 형식의 적격이나 PASS를 주장하지 않는다. 새 context로 명시적으로 선택된 경우에만 strict private 사전 검사를 적용한다. 이 검사는 기존 문서 상한 안에서 JSON 문법·UTF-8·중복 key·후행 값·깊이·값 수와 schema/context/planned member shape·parser part 개수/이름을 검사한다. 토큰 사이에 caller 취소 및 유한 deadline을 확인하며 단일 token 처리 도중 강제 중단을 보장하지 않는다. header·planned member 추출도 같은 token pass에서 수행하며 큰 문서 전체를 후속 Unmarshal로 다시 해석하지 않는다. caller 취소와 내부 wall 만료는 구분한다. 나머지 semantic shape 전체가 사전 검증됐다고 주장하지 않는다. Build가 실제 closure/driver pin을 확정하면 driver 실행 전에 계획과 정확히 대조·capture하고, 실제 member 전체는 완결 manifest 전에 재대조한다. O / manifest.json / manifest가 사전 계획에 정확히 하나 있어야 한다. 빈 stderr는 기존 null/no-file을 유지한다. 예상하지 못한 stderr는 원문을 보존하되 계획 불일치로 실패하며 complete marker를 쓰지 않는다. 실패 시 build cleanup 오류도 보존하며, 계획 불일치가 이미 확정된 CANCELLED·RESOURCE_LIMIT·기존 FAIL을 일반 실패나 PASS로 바꾸지 않는다.

Native.Grammar의 원본 parser 경로는 실제 descriptor 물리 pin을 가리킨다. standalone eligibility도 Native.InvocationID가 가리키는 실제 NATIVE_DRIVER node가 descriptor와 모든 part의 owner인지 확인하고 사용하지 않는 paged member도 거부한다. 의미 binding은 graph·D owner·DriverNode·사전 member 계획까지 정확히 대조한다. 승인 ABSENT에서도 동일 set/whole-file 검증을 수행하고 required 분모를 먼저 유지한 채 BLOCKED로 남긴다. caller-approved PASS는 저장 근거 일관성에 한정되며 실행 진위 인증이나 C# 실제 실행 증거가 아니다.

이 계약의 회귀는 합성 저장 자료의 일관성과 경계 동작을 검사하며 실제 reference 실행 증거가 아니다. C# 사전 ABI 기대 symbol_count=654는 SYMBOL_COUNT650+ALIAS_COUNT4의 고정 runtime API에서 도출한 값이며 실행 관측이 아니다. 실제 compiled ABI·driver/reference 크기·독립 reference 실행은 별도 후보의 관측으로 확인해야 한다.

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
