# 계측 query reference 계약 (#216)

이 도구는 고정 Tree-sitter 0.27.1의 별도 계측 reference다. Stock CLI full-field parity는 계속 **BLOCKED**이며, 이 도구의 빌드·owned 검사 성공은 전체 `query_semantics` qualification 완료가 아니다. 기존 oracle/replay/qualification revision과 보장 범위는 유지한다.

## 독립성 및 identity

Upstream은 `tree-sitter/tree-sitter`의 `10eb5ac119eec9cb4267a8e035066473983f5f3a`다. [instrumentation.patch](../../src/dev/query-reference/instrumentation.patch)는 Rust binding에 읽기 전용 query pointer와 별도 raw capture iterator만 추가한다. 기존 iterator의 기본값은 predicate 평가를 유지한다. 별도 iterator는 predicate 탈락 match를 제거하지 않으며, emitter가 변경하지 않은 upstream `QueryMatch::satisfies_text_predicates`에 원래 전체 match와 원 source를 넘긴다. Kit 평가기를 호출하거나 kit 결과를 읽지 않는다.

원본 workspace·library manifest를 유지하고 emitter member 하나를 더한다. 원본 lock의 기존 package·version·checksum·dependency를 바꾸지 않는다. `build.py`는 새 package 하나만 추가됐는지 확인한 뒤 `--offline --frozen`으로 선택 package만 빌드한다. 전체 CLI·WASM·bindgen 빌드는 이 경로에 포함하지 않는다. 실제 selected dependency/features의 전체 `selected-tree.json` receipt hash와 종료 전 불변성, metadata, 실행 요청·결과, 원본 archive·patch·emitter·manifest·lock·도구·binary hash를 별도로 보존한다. Build 도구 목록은 실제 Python interpreter의 절대 경로·파일 hash·`sys.version`도 포함하며 build 전후 hash가 같아야 한다. Cargo offline은 의존성 취득 방식이며 network sandbox를 뜻하지 않는다.

`tsgk-instrumented-reference-build/r1`의 `complete`는 빌드 절차 완료다. Caller가 독립적으로 선택한 upstream·도구 취득 근거 및 등록 hash와 연결해야 한다. Evidence가 자체 제출한 identity/hash만으로 reference 자격을 만들지 않는다. 공개 source, compiler/standard-library provenance, 실제 feature closure와 실행 문맥의 신뢰 검토는 자격 검증에 계속 필요하다.

## 결과와 완료 조건

Emitter 인자는 절대 library path, symbol, source path, query path, query ID, 사전 등록 library SHA-256이다. Source는 UTF-8 32 MiB 이하, query는 UTF-8 64 KiB 이하, library는 64 MiB 이하의 regular file이다. Source·query는 읽은 bytes에서 hash와 결과를 계산한다. Library는 실행 전 예상 hash와 비교하고 실행 후 다시 비교한다. 이 검사는 악의적인 동시 파일 교체나 native code 자체를 격리한다는 보장이 아니다. 검토·고정한 생성 grammar만 별도 process에서 실행한다. 출력 소유권의 #215 미완료 보장을 이 경로의 hash 검사로 대체하지 않는다.

`tsgk-instrumented-query-reference/r1`은 다음을 제공한다.

- Fresh parse의 전체 preorder CST: parent·field·type, named·extra·is_error·has_error·is_missing, byte·point 전체 필드.
- 실제 compiled query의 pattern 수·capture 이름·predicate 단계와 quantifier.
- 원 capture 순서와 pattern/capture 번호를 보존한 structural·evaluated 전체 stream. Capture의 필드는 실제 captured node에서 읽고 독립 CST의 node identity 및 모든 필드와 대조한다. Span만으로 node를 찾지 않는다.
- 원 runtime match ID와 최초 structural 등장 순서의 match 번호 mapping. Predicate 탈락 뒤 번호를 다시 매기지 않는다.
- 실제 사용 한도, cursor 종료·match-limit 초과 여부 및 `complete`.

각 match의 전체 capture signature·pattern·predicate 결정이 재등장 때 같아야 한다. 각 capture **occurrence index**는 정확히 한 번 반환돼야 하며, 종료 시 누락이 있으면 실패한다. 서로 다른 occurrence의 동일한 capture 값은 보존한다. 단순 개수 합계나 정렬·중복 제거로 이 검사를 대체하지 않는다.

성공은 모든 검사가 끝났고 cursor가 소진됐으며 한도를 넘지 않았을 때만 `COMPLETED`, `complete: true`다. 100,000 node·50,000 stream capture·200,000 group capture·100,000 match·내부 wall 3초를 유지한다. Predicate는 `eq?`, `not-eq?`, `any-of?`, `not-any-of?`의 지원 operand 형태와 `ONE` capture만 허용한다. 그 밖의 연산·directive·quantifier·encoding은 이 기준의 부적격이며 범위를 넓혀 통과시키지 않는다. 원본 C query cursor의 기본 range가 byte0에서 끝나는 capture를 제외하는 동작은 빈 source 대조에 반영한다.

## 실행 및 검사

Python 3.11 이상과 이미 준비한 절대 경로의 Cargo/Rust 1.97.1(`-vV`의 유일한 `release` 필드와 정확히 일치), C compiler·archiver·Git, 고정 source archive와 checksummed vendor directory가 필요하다. Global 설치나 설정을 바꾸지 않는다. 새 task directory에서만 source를 추출·계측하며 기존 reference checkout을 수정하지 않는다. Source archive는 해당 upstream commit의 `git archive --format=tar` bytes이며 SHA-256은 `6025ffd838c63bb011eeec9b259891ae0e051a3db9aa24c8812679f7462a50a9`다.

먼저 `CGO_ENABLED=0 go build -o <supervisor> ./src/dev/query-reference-run`으로 기존 runner를 사용하는 supervisor를 만든다. Shell 대신 정확한 argv·최소 환경으로 실행하고 stdout/stderr·wall·메모리·process tree 종료를 기록한다. Windows Job Object와 delegated Linux cgroup은 hard memory cap을 사용한다. macOS는 기존 runner의 sampled memory·process group 문맥을 그대로 표시하며 hard cap으로 보고하지 않는다. Linux에서는 두 Python 명령 모두 검증한 delegated `--cgroup-parent`가 필요하다.

```text
python src/dev/query-reference/build.py --archive <absolute-tar> --vendor <absolute-vendor> --cargo <absolute-cargo> --rustc <absolute-rustc> --cc <absolute-compiler> --ar <absolute-archiver> --git <absolute-git> --supervisor <absolute-supervisor> --work <new-absolute-build-directory> --target <target>
python src/dev/query-reference/check.py --reference <absolute-built-binary> --identity <build-directory/identity.json> --supervisor <absolute-supervisor> --cc <absolute-compiler> --work <new-absolute-control-directory>
```

대상은 `x86_64-pc-windows-gnu`, `x86_64-unknown-linux-gnu`, `aarch64-apple-darwin`이다. Build는 2 compiler job·4 GiB·15분 안에서, owned reference 실행은 1 GiB·10초 안에서 감독한다. 실패·출력 잘림·정리 미확인은 성공 receipt를 만들지 않는다. 입력이나 도구가 빌드 중 변경됐으면 identity 완료 전에 거부한다. 의도한 patch·manifest·lock 변경 뒤 source와 vendor 전체 파일 snapshot을 selected-tree 실행 전에 고정하고, 실행 직후와 build·unit 이후 다시 비교한다. 원본 binding hash가 정확할 때만 zero-context patch를 적용한다. 두 Python 진입점은 `-O`·`PYTHONOPTIMIZE` 실행을 인자 처리 전에 거부한다.

Rust unit은 유지·탈락 match 모두에서 capture 누락·중복·범위 오류를 검사한다. `check.py`는 owned grammar의 전체 7-node CST와 전체 capture·metadata를 손으로 고정해 네 연산의 유지·탈락, capture 간 비교, 복수 predicate, structural, 두 capture의 유지·탈락, 정상 중복 보존, 빈 source, 미지원 연산 거부와 잘못된 library hash 거부를 확인한다. Capture 간 비교는 서로 다른 두 node의 값이 같을 때와 다를 때 `eq?`·`not-eq?`의 유지·탈락도 확인한다. 수정한 source는 별도 fresh parse다. Kit의 incremental/fresh 실행을 했다는 근거로 세지 않는다. 각 실행의 stdout·stderr·runner 결과와 최종 요약은 새 directory에 남는다.

선택한 Rust unit은 정확히 1개 실행·1개 성공·미실행 0개를 요구한다. Owned parser·header는 읽은 bytes와 hash를 고정한 task copy에서 컴파일하며 최종 copy·binary·identity·도구·검사 source를 재확인한다. 거부 대조도 정상 process 종료·정확한 exit1·잘림 없는 출력·정리 완료를 요구하므로, 오류 JSON을 먼저 출력한 timeout은 성공한 거부가 아니다. `python src/dev/query-reference/test_pipeline.py -v`는 최적화 실행·0개 unit·입력 변경을 검사한다. 실제 supervisor의 절대 경로를 `TSGK_REFERENCE_SUPERVISOR`로 지정하면 정상 exit1, 오류 출력 뒤 wall 초과, stderr 잘림을 별도 process로 검사하며, 미지정 시 그 시험은 SKIP으로 남는다. Linux에서는 `TSGK_REFERENCE_CGROUP_PARENT`도 필요하다.

## 전체 지원 대상 연결의 자격 기준

새 qualification은 등록된 필수 `(route, platform, role, case, query, step, tree_role)`를 결과 전에 고정해야 한다. 필수 tuple 0개·claim 누락·미실행·partial·reference 부적격·source witness 누락은 BLOCKED다. Raw/projection/reference·identity 모순이나 변조는 FAIL이다. Predicate 의미 PASS는 완료된 EVALUATED 결과, 독립 full CST·structural stream·metadata 일치, 수작업 anchor와 전체 evaluated capture 기대값, 저장 witness의 재계산을 모두 요구한다.

Native의 field patch 때문에 이 reference와 CST 또는 capture가 달라지면 **INELIGIBLE/BLOCKED**로 별도 집계한다. Kit 값으로 reference의 필드를 채우거나 stock CLI와 같다는 이유로 kit 계약을 완화하지 않는다. 전체 26 route × 3 host와 등록된 추가 role, producer의 opt-in source 보관, 새 claim/reducer/inventory/fan-in 연결 및 실제 main 검증은 후속 완료 조건이다. 이 도구나 owned 대조 하나로 해당 조건을 충족한 것으로 처리하지 않는다.

후속 연결 검증의 내부 준비 경로는 기존 `VerifyEvidence`가 해석한 graph와 동일 guard 읽기에서 hash가 확인된 요청 member bytes만 보관한다. 보관 대상을 골라도 다른 member·graph·policy·한도·취소 검사는 그대로 실행하며, 일반 member에 replay-result용 record 한도를 추가하지 않는다. 빈 member는 누락과 구분하고 실패한 member의 bytes나 이전 호출의 snapshot을 재사용하지 않는다. 이 내부 준비만으로 reference eligibility API나 qualification 연결이 구현된 것은 아니다.

입력 identity JSON은 1 MiB, reference binary는 64 MiB 한도를 파일 열기 전 stat과 열린 regular file의 fstat에서 확인한다. JSON은 상한+1까지만 읽고 binary는 스트리밍 hash를 사용한다. 실행 중 파일 크기가 늘어나도 상한을 넘으면 거부한다. `complete`는 JSON boolean `true`만 허용하며 truthy 문자열·숫자로 완료를 대신하지 않는다. Owned control은 identity가 결속한 selected-tree receipt도 직접 확인하고 종료 직전 보존 bytes를 재확인한다. `test_pipeline.py`는 1 GiB sparse 입력의 사전 거부와 완료 값 타입 반례를 포함한다.

Owned control의 compiler 출력 library도 hash 계산 전에 64 MiB 한도와 regular-file 검사를 적용한다. 실제 build/check 진입점 fault 회귀는 `TSGK_REFERENCE_SUPERVISOR`·`TSGK_REFERENCE_CC`·`TSGK_REFERENCE_AR`·`TSGK_REFERENCE_GIT`·`TSGK_REFERENCE_ARCHIVE`가 모두 지정됐을 때 실행하며, 없으면 해당 회귀는 SKIP으로 기록한다. 고정 버전의 접미사·다른 patch 버전, selected-tree 중 source/vendor 변경, compiler의 상한 초과 library를 실제 supervisor로 실행해 후속 단계와 완료 identity 생성 전에 거부하는지 확인한다.
