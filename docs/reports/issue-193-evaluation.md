# #193 성능·정확도·소비자 도입 평가

이 보고서는 기존 [grammar 지원 안내](../grammar-support.md)의 지원 판정과 별도로, 실제 측정 범위와 제품 경계를 기록한다. 전체 26 route의 정확도 비율이나 경쟁 제품 대비 속도 순위를 제공하지 않는다. 단계별 후속 결과는 같은 보고서에 추가한다.

## 1. 재현 가능한 성능 기준선

2026-10-13 Windows/amd64, Intel Core Ultra 9 285H에서 측정했다. 제품 기준 commit은 `4b19b369897fad554c707f76e4afcb7f038d140c`다. `CGO_ENABLED=0`, `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`와 Go 1.27.1을 사용했다. 각 benchmark는 CPU 1 설정으로 직렬 실행했다. 이는 호스트 전체의 다른 작업을 차단하는 설정이 아니다.

Offline benchmark는 21개 조건마다 100ms 이상 × 5 sample, native는 JSON의 3개 크기 × 4개 mode마다 3 iteration × 5 sample이다. 아래 값은 5 sample의 중앙값이다. 작은 sample 수와 호스트 scheduling 영향 때문에 수치의 대소를 제품 우위나 통계적 유의성으로 해석하지 않는다. 원시 log와 최소·최대값은 로컬 `artifacts/followup-20261013/`에 보존한다.

입력은 공개 가능한 합성 데이터다. Tree는 root 하나와 `identifier` sibling 64/1,024/16,384개, schema는 서로 다른 node 선언 64/1,024/4,096개, Identity는 단일 `grammar.js` 4KiB/256KiB/4MiB다. Identity는 준비 단계에서 이미 읽은 파일을 반복하므로 warm-cache 경로다. Schema diff의 MB/s는 baseline 문서 크기를 분모로 사용하며 두 문서 처리량이나 소스 파싱 처리량이 아니다.

Native JSON은 `[12345,...,0]`이며 number 수 65/513/2,049, source 크기 387/3,075/12,291 bytes다. `direct-fresh`는 C API의 fresh parse만 수행한다. `kit-tree`는 full CST를 기록·decode·검사한다. `kit-edit`는 첫 number `12345`를 `9`로 바꾸고 initial + incremental + fresh parse 3회와 CST 비교를 수행한다. `kit-api`는 fresh parse 1회와 등록 API audit를 수행한다. 서로 다른 작업량이므로 네 mode의 시간 비율을 동일 기능의 성능 비교로 사용하지 않는다.

Direct executable은 kit와 **같은 고정 runtime/parser object**에 링크했다. 준비 source는 main CI `38344498469`, attempt 1의 artifact `11788328665`에서 canonical hash를 검증한 JSON/runtime 87개 파일만 추출했다. 기존 reference checkout은 변경하지 않았다. Compile·fixture 생성은 측정 loop에서 제외했다. 각 run은 입력 hash·bytes·node 수·step 수·edit 범위·fresh equality를 확인한다.

`ns/op`는 Go 호출부터 process supervision, output decode 및 검증까지의 전체 비용이다. `process-ms/op`는 child wall time, `direct-parse-ns/op`는 C 단조 시계로 잰 parse 구간만이다. Kit의 기존 `ParseMillis`는 정수 millisecond이므로 `kit-reported-parse-ms/op` 그대로 기록하며 ns 정밀도로 바꾸지 않는다. 이는 CST serialization이나 API audit 시간의 독립 측정값이 아니다. Go `B/op`·`allocs/op`는 native heap을 포함하지 않는다. Native peak는 관측된 run에서만 기록하며 이 Windows 실행의 지표는 Job committed memory, enforcement는 HARD다. RSS와 동일한 값으로 비교하지 않는다.

| Offline 조건 | 전체 ms/op | Go B/op | Go allocs/op |
|---|---:|---:|---:|
| `CompareTrees/nodes=16385/last-difference=false` | 5.523 | 2,077,904 | 261,360 |
| `CompareTrees/nodes=16385/last-difference=true` | 15.165 | 2,077,971 | 261,361 |
| `TreeDigest/nodes=16385` | 1.266 | 280 | 7 |
| `TreeJSON/nodes=16385` | 18.551 | 3,907,700 | 4 |
| `Schema/nodes=4096/diff=false` | 6.429 | 3,105,909 | 49,274 |
| `Schema/nodes=4096/diff=true` | 18.494 | 6,269,778 | 98,553 |
| `Identity/bytes=4096` | 1.039 | 1,065,203 | 168 |
| `Identity/bytes=262144` | 1.804 | 1,323,372 | 170 |
| `Identity/bytes=4194304` | 13.583 | 2,109,896 | 171 |

| JSON number 수 / mode | 전체 ms/op | Child ms/op | parse 구간 | Go B/op | 관측 native peak 최대 bytes |
|---|---:|---:|---|---:|---:|
| `numbers=65/mode=direct-fresh` | 60.904 | 59.33 | 173133.00 ns | 113,581 | 688,128 |
| `numbers=65/mode=kit-tree` | 31.276 | 29.33 | 0.00 ms | 186,082 | 716,800 |
| `numbers=65/mode=kit-edit` | 63.562 | 60.33 | 0.67 ms | 342,066 | 692,224 |
| `numbers=65/mode=kit-api` | 32.127 | 31.00 | 0.33 ms | 258,618 | 679,936 |
| `numbers=513/mode=direct-fresh` | 48.005 | 46.33 | 496667.00 ns | 113,581 | 761,856 |
| `numbers=513/mode=kit-tree` | 53.045 | 51.00 | 1.00 ms | 666,850 | 1,044,480 |
| `numbers=513/mode=kit-edit` | 54.627 | 51.33 | 1.33 ms | 1,805,957 | 1,458,176 |
| `numbers=513/mode=kit-api` | 81.382 | 76.00 | 0.67 ms | 1,220,994 | 1,179,648 |
| `numbers=2049/mode=direct-fresh` | 44.308 | 43.33 | 1576267.00 ns | 113,581 | 1,323,008 |
| `numbers=2049/mode=kit-tree` | 85.700 | 78.33 | 3.67 ms | 2,269,898 | 2,363,392 |
| `numbers=2049/mode=kit-edit` | 52.827 | 43.00 | 3.00 ms | 7,286,829 | 4,005,888 |
| `numbers=2049/mode=kit-api` | 128.789 | 99.33 | 3.33 ms | 5,554,544 | 2,899,968 |

모든 native sample에서 peak는 3/3회 관측되었다. 마지막 열은 각 sample의 최대값 중 다시 최대값이며 중앙값이 아니다.

재실행은 repository root에서 아래와 같이 한다. Native 환경값은 절대 경로여야 하며 source와 compiler identity를 고정한다. Linux에는 기존 native runner 계약의 delegated cgroup parent가 필요하다. 이 실행에서 Linux/macOS 성능 측정은 하지 않았다.

```powershell
$env:CGO_ENABLED = '0'
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOFLAGS = '-mod=readonly'
go test ./src/kit -run '^TestBenchmarkFixtures$' -bench '^Benchmark(CompareTrees|TreeDigest|Schema|Identity|TreeJSON)$' -benchmem -benchtime=100ms -count=5 -cpu=1 -timeout=180s
if ($LASTEXITCODE -ne 0) { throw 'offline benchmark failed' }
# Set TSGK_BENCH_PREPARED to the canonical runtime/routes source root and TSGK_NATIVE_CC to the compiler.
go test ./src/internal/native -run '^TestNativeBenchmarkControls$' -bench '^BenchmarkNativeJSON$' -benchmem -benchtime=3x -count=5 -cpu=1 -timeout=180s
if ($LASTEXITCODE -ne 0) { throw 'native benchmark failed' }
```

Offline fixture controls는 마지막 node 변경 감지, digest 변경, JSON round trip, schema 추가 감지, 동일 크기 파일 교체의 identity 변경을 확인했다. Native controls는 잘못된 JSON, 빈 입력, 64KiB 초과 입력의 거절과 process cleanup을 확인했다. 최종 두 실행은 각각 exit 0으로 끝났고, 세 benchmark source의 실행 전후 SHA-256이 일치했다.

| 측정 source | SHA-256 |
|---|---|
| `src/kit/benchmark_test.go` | `37055d8cf6eed325d5b7c22ccae957fbdb441da762c436b2f35497009bb3d2b9` |
| `src/internal/native/benchmark_test.go` | `4962014db2e5a769d473b9c6f33b6f206309876812a1da9dbb8f4ce25487a454` |
| `src/dev/benchmark/native/parse.c` | `eaee12213963a61b81f8db660aef79b8f2c86b3de741c017e1274ffa85786a8a` |

이 표는 측정 당시 source cohort의 hash다. 전체 회귀가 검출한 build helper 우회는 이후 기존 `testBuildRequest` 재사용으로 수정하여 sanitizer/cgroup 설정을 같은 경로에서 받도록 했다. 당시 source snapshot과 측정값은 로컬 증거에 보존했다. 측정은 sanitizer 없는 조건이었으며 compiler/runtime/parser object와 측정 loop의 작업은 이 helper 수정으로 바뀌지 않았다. 이후 실행 검증은 수정된 helper의 새 검증 근거로 구분한다.

Right pane의 `codex --no-daemon`에 `herdr`로 전체 변경 source를 전달하여 두 차례 정적 리뷰했다. 첫 리뷰의 단위·메모리 관측·fixture 검증·negative control 지적을 수정했고, 두 번째는 `NO_ACTIONABLE_FINDINGS`였다. 실행 검증은 controller가 수행했으며 정적 리뷰를 독립 실행으로 세지 않는다.

현재 기준선은 offline API 5종과 Windows JSON native의 제한된 workload다. CLI 전체, regeneration, corpus, replay, qualification, cold-cache IO, compiler 준비 비용, 26 route 전부, 호스트 간 속도 비교는 측정하지 않았다. 이 구분은 기능의 미지원 판정이 아니라 성능 근거의 범위다.

## 2. 독립 구문 oracle와 오류 복구 평가

평가 입력은 [고정 JSON holdout](../../src/testdata/native/json-holdout.json) 36개다. Native 출력 관측 전에 Go 1.27.1 `encoding/json.Valid`로 선언 label을 전부 확인했다. JSON object·array·scalar·Unicode·escape·number·whitespace의 유효 입력 16개와 누락 delimiter·comma·colon, escape·number·comment 등의 잘못된 입력 20개다. 입력 목록 SHA-256은 `741fb047ea5bf3081204cb9bef92d29e70398c86b54978ee2734ba375d69fdf8`이다. 기존 native corpus의 기대값을 oracle로 사용하지 않았다. 이 저장소의 기존 grammar 수정에 대해 blind 평가를 했다는 뜻은 아니다.

Windows의 같은 pinned JSON/runtime 후보에서 TP 16, TN 20, false acceptance 0, false rejection 0을 관측했다. 각 결과는 source SHA·bytes와 full recovery CST의 구조를 확인했다. 여기서 acceptance는 `has_error=false`이며 키 중복의 의미 정책, 숫자의 표현 범위, 언어 전체의 정확도, 26 route의 compiler/engine 적합성은 평가하지 않았다. 작은 인위적 사례 집합의 전부 일치를 실사용 정확도 100%로 확대하지 않는다.

| 편집 사례 | 독립 구문 oracle 일치 | Incremental = fresh CST | Incremental route |
|---|---|---|---|
| `[1` → `[1]` | 일치 | PASS | BLOCKED |
| `[1]` → `[1` | 일치 | PASS | BLOCKED |
| `[12345,2]` → `[9,2]` | 일치 | PASS | PASS |
| `{"a" 1}` → `{"a":1}` | 일치 | PASS | BLOCKED |

세 BLOCKED는 edit가 관측되었지만 오류 tree 구간에서 재사용 node가 0이었던 경우다. 정확한 code는 `INCREMENTAL_ROUTE_UNOBSERVABLE_ERROR_TREE_STEP_1`이며 전체 사례 판정도 BLOCKED로 유지했다. 오류 tree에도 재사용이 가능한 다른 사례가 있으므로 `has_error`만으로 모든 edit를 BLOCKED로 분류하지 않는다. [기존 route 계약](../specs/tree-and-adapter-protocol.md)의 equality·reuse·syntax 축을 분리한 결과다. 처음 평가 테스트의 모든 edit에 PASS를 요구한 잘못된 가정을 수정했고, 실제 관측과 그 초기 실패 log를 보존했다. 최종 테스트는 네 사례의 route 기대값도 명시해 다른 판정을 조용히 수용하지 않는다.

```powershell
# Same explicit native preparation/compiler configuration as stage 1.
go test ./src/internal/native -run '^Test(NativeJSONHoldout|JSONHoldoutLabels)$' -count=1 -v -timeout=180s
if ($LASTEXITCODE -ne 0) { throw 'JSON holdout evaluation failed' }
```

`TestJSONHoldoutLabels`는 native 도구 없이 실행된다. Native 평가 환경이 없으면 `TestNativeJSONHoldout`는 SKIP이고 위 결과를 새 호스트에 전이하지 않는다. 평가 test의 exit 0은 관측과 고정 사례 회귀 검사가 끝났다는 뜻이며 BLOCKED route를 PASS로 바꾸지 않는다.

Foundation의 Windows native job은 같은 run에서 준비한 source와 compiler로 holdout 및 native negative controls를 실행하고 `json-holdout.log`를 별도 `json-holdout-<run>-<attempt>` artifact에 보존한다. Qualification host artifact의 닫힌 파일 목록에 추가하지 않는다. 처음 후보 CI에서 그 목록 안에 log를 넣어 `HOST_FILE_UNREGISTERED`로 완결성이 실패한 근거는 보존한다. 속도 benchmark는 호스트 변동 때문에 CI 합격 조건으로 쓰지 않는다. 이 CI 연결의 통과 여부는 해당 후보의 실제 run 완료 후에 기록한다.

## 3. 실제 소비자 도입 실험

사용자가 지정한 [go-treesitter](https://github.com/wotjr1649/go-treesitter)의 개발 상태를 확인했다. 일시 중지된 branch와 기존 release worktree를 보존하고 별도 `validation/grammar-kit-193` worktree에 전용 CLI 검사 도구를 만들었다. 이는 kit의 임시 external consumer test와 별도로 실제 소비자 repository 안에서 수행한 로컬 pilot이다. 소비자 commit과 원래 상태의 대조 기록은 로컬 receipt로 보존한다.

소비자의 기존 `tools/runtime-bundle/grammars/go.json`을 caller-trusted 등록부로 삼아 Go source ZIP과 derived product blob을 `tsgk identity`로 읽고 기존 SHA-256과 대조했다. 등록부 자체도 같은 관측 set에 결속했다. 실제 artifact 대조, 복사본의 동일 크기 blob 변조 거절, 잘못된 CLI executable hash 거절, 동시 stdout/stderr의 합계 64KiB 출력 상한 및 상한+1 byte 거절 control이 통과했다. 처음 parent의 잘못된 output encoding으로 발생한 JSON decode 실패를 UTF-8 byte 수집/strict decode로 수정했고 실패 log를 보존했다.

원래 parser·internal runtime·oracle epoch·grammar baseline·Go module dependency는 변경하지 않았다. 도구의 PASS는 기존 등록부와 선택한 두 artifact의 byte identity 일치다. 구문 정확도·C oracle parity·release readiness는 주장하지 않는다. 현 상태는 **PILOT_LOCAL**이며 소비자 default branch·CI·배포에 채택했다는 뜻이 아니다. 소비자의 중지된 제품 개발을 재개하거나 원래 baseline을 kit의 다른 grammar identity로 자동 교체하지 않았다.

도입 비용은 PowerShell 검사 도구 4파일·143줄이며 Go dependency 추가 0개다. 실행 입력은 기존 source ZIP 및 product blob과 작은 등록부 3파일, 실행 도구는 고정 kit CLI 하나다. 로컬 CLI build는 약 2.5초, 실제 소비자·음성 대조·출력 상한을 포함한 최종 전체 pilot test는 약 1.8초였다. Native compile이나 제품 parser 재구축은 수행하지 않았다. 이 일회 관측을 cold install 비용이나 모든 소비자의 도입 시간으로 일반화하지 않는다.

## 4. 관측된 comparator 비용 개선

기준선에서 `CompareTrees`는 같은 node에도 공개 필드 12개의 양쪽 값을 매번 문자열로 만들었다. 이제 같은 순서로 typed value를 비교하고 실제 첫 차이에 대해서만 기존 표현으로 문자열을 만든다. API·첫 차이의 순서·nil과 빈 field 구분·진단 text·tree digest·source/grammar identity·운영 상한은 유지한다. [정확한 진단 회귀](../../src/kit/tree_test.go)는 12개 필드, node 우선순위, 같은 node의 field 우선순위, nil/빈 field, escape와 invalid UTF-8의 기존 표현을 확인한다.

원래 benchmark fixture와 함수는 변경하지 않고 같은 CPU/100ms/count 5 조건으로 한 번 측정했다. 호스트 noise와 실행 순서가 있으므로 아래는 이 합성 workload의 관측이며 전체 CLI/native 속도 개선율이 아니다.

| 조건 | 이전 중앙값 ms/op (min–max) | 이후 중앙값 ms/op (min–max) | 이전 → 이후 B/op | 이전 → 이후 allocs/op |
|---|---:|---:|---:|---:|
| `nodes=65/last-difference=false` | 0.0324 (0.0287–0.0329) | 0.0003 (0.0003–0.0004) | 5,024 → 0 | 520 → 0 |
| `nodes=65/last-difference=true` | 0.0359 (0.0318–0.0414) | 0.0005 (0.0005–0.0006) | 5,088 → 96 | 521 → 3 |
| `nodes=1025/last-difference=false` | 1.2219 (1.1765–1.3701) | 0.0057 (0.0047–0.0061) | 111,824 → 0 | 15,600 → 0 |
| `nodes=1025/last-difference=true` | 1.2780 (1.1800–1.4515) | 0.0051 (0.0049–0.0053) | 111,888 → 96 | 15,601 → 3 |
| `nodes=16385/last-difference=false` | 5.5229 (5.2856–22.6658) | 0.0886 (0.0836–0.0899) | 2,077,904 → 0 | 261,360 → 0 |
| `nodes=16385/last-difference=true` | 15.1653 (7.6591–16.8862) | 0.0672 (0.0615–0.0840) | 2,077,971 → 96 | 261,361 → 3 |

Equal 경로의 Go 할당 제거가 확인되어 이 제한된 변경을 유지한다. CST serialization, runner startup, native parse, API audit 비용을 줄였다는 주장은 없다. Native·replay 호출자는 같은 comparator를 사용하므로 해당 회귀와 전체 Go suite·vet·build 및 현재 후보 CI를 별도로 확인한다.

## 5. Experimental 계약과 유지보수

[공개 API 계약](../specs/public-go-api.md)은 module·문서·schema·grammar version의 구분과 실제 소비자 채택의 경계를 명시한다. [Platform 계약](../specs/platform-support.md)은 experimental 연산의 호환성, native 실행과 offline replay의 효과 차이, query predicate 미평가, host별 supervision/memory 차이, local patch의 갱신·제거 근거를 연결한다. 이 평가에서 새 stable 보장이나 release를 추가하지 않았다. Source/patch closure의 canonical 소유권은 기존 [provenance](../provenance/upstream-sources.md)와 등록부에 유지한다.

## 비슷한 도구와 역할 비교

2026-10-13 각 프로젝트의 공식 문서를 확인했다. 이 표는 기능의 주된 목적을 비교하며 제품별 전체 기능 inventory, 성능 benchmark 또는 적합성 인증이 아니다.

| 도구 | 공식 문서에서 확인한 주된 역할 | Grammar-kit와 함께 사용할 때의 구분 |
|---|---|---|
| Tree-sitter CLI | parser corpus test, source parse, edit 적용, parse 시간·통계 출력 ([test](https://tree-sitter.github.io/tree-sitter/cli/test.html), [parse](https://tree-sitter.github.io/tree-sitter/cli/parse.html)) | Parse/edit/test 기능은 겹친다. Kit는 별도로 고정 source·producer·policy와 검증 결과를 결속하고 replay·qualification 계약으로 집계한다. |
| ast-grep | syntax tree에 따른 구조 검색·lint·rewrite ([공식 소개](https://ast-grep.github.io/guide/introduction.html)) | Kit의 목적은 grammar 후보와 검증 근거의 평가다. 코드 검색·수정 제품의 역할을 대체하지 않는다. |
| Semgrep Code | 규칙을 이용한 정적 코드 분석과 보안 finding 관리 ([공식 개요](https://docs.semgrep.dev/semgrep-code/overview)) | Kit의 syntax/CST/API 검증은 보안 취약점 검출 정확도나 프로그램 의미 분석의 근거가 아니다. |

따라서 현재 확인할 수 있는 차별점은 `CGO_ENABLED=0` offline 검사, source/generated/runtime identity의 추적, incremental/fresh·API·query·실행/미관측의 분리, 저장된 근거의 재계산, 등록 26 route의 세 host qualification을 하나의 계약으로 연결한 것이다. 각 개별 기능이 다른 도구에 없다는 주장이 아니다. 정확도와 속도의 상대 우위는 동일한 입력·작업·grammar/runtime/compiler·기능 범위를 고정한 별도 비교가 있어야 한다. 이번 측정으로 그 순위를 정하지 않는다.
