# Windows native 테스트 빌드 직렬화 비용 (#210)

## 실패와 원인 범위

PR #209 통합 뒤 main `1b6287f75733cc4b2b080b94ee9e72974a402804`의 [Foundation run 38653072957, attempt 1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38653072957)은 Windows `src/internal/native` package의 300초 timeout으로 실패했다. 실제 package 시간은 300.246초였고, 남은 compiler/NewBuild 호출이 중단됐다. downstream native·qualification skip은 PASS가 아니다.

직전 PR 실행은 같은 코드·workflow·Go 1.27.1·GCC 15.2.0 binary에서 243.242초로 통과했다. Windows image는 `20260925.250.1`에서 `20261007.264.1`로 달랐다. host 부하 등은 통제하지 못했으므로 image를 단독 원인이나 일시적 인프라 오류로 확정하지 않는다.

확인한 개선 대상은 테스트 helper의 구조다. 기존 `fixtureBuild`는 cache miss의 전체 C 컴파일 동안 전역 mutex를 점유했고, 서로 다른 define도 직렬로 빌드했다. fault 4개 및 reuse 관측 10개 child도 순차 실행했다. 이는 독립 작업을 직렬화한 비용이며 parser 실행 속도나 제품 API의 결함을 뜻하지 않는다.

## 변경

fixture마다 key별 `sync.Once`가 실제 `NewBuild` 결과와 오류를 함께 공유한다. compiler·fixture 준비의 `t.Fatal`은 Once 밖에서 호출하고 오류는 각 호출자의 test에 보고한다. map과 공통 work root만 mutex로 보호하고, 서로 다른 빌드는 고유 directory에서 실행한다. fixture 컴파일 동시성은 4로 제한한다. 기존 benchmark 전용 cache와 코드는 유지한다.

fault와 reuse의 부모 test는 순차로 유지하며 독립 child만 `t.Parallel`로 실행한다. 따라서 이 그룹은 다른 순차 timing test와 겹치지 않는다. 각 child가 입력·edit·request를 소유한다. reuse의 후속 실행 수는 child 10개와 Go test 병렬도에 의해 제한되며, 컴파일 상한 4가 전체 native 실행 상한 4를 뜻하지 않는다.

제품 `NewBuild`, compiler flags, runtime·grammar source, fault 목록과 assertion, package 300초·foundation job 10분·각 process 제한은 변경하지 않는다. 신규 실제 native 동시요청 검사는 동일 key의 Build 포인터 공유와 다른 define의 directory·identity 분리를 확인한다. 최대 동시성이나 실패 cache 전파의 독립 동적 관측은 이 검사만으로 입증하지 않는다.

## 관측과 검증

로컬 Go 1.27.2, CGO_ENABLED=0, GCC 16.2.0에서 같은 runtime·compiler로 별도 cold test binary를 실행했다. fault 단계 3 paired 표본의 process wall은 기존 33.006/28.672/27.597초, 첫 후보 12.201/10.888/10.162초였다. 중앙값은 28.672→10.888초다. 이 첫 후보는 reuse 병렬화를 포함하지 않았다. 측정 순서는 baseline/candidate, candidate/baseline, baseline/candidate였고 두 binary 모두 parallel=4였다.

reuse 병렬 후보의 단계 2 paired process-wall 표본은 기존 69.955/82.800초, 후보 26.954/30.841초였다.

최종 코드의 로컬 `go test` native package는 150.140초로 300초 제한 안에서 통과했다. Go 명령을 포함한 process wall은 150.924초였다. foundation 전체는 process wall 57.235초, vet 0.898초, build 1.015초로 통과했다. 다른 test-bearing package는 r3 실행에서 통과한 동일 byte 결과를 집계했고, 수정한 foundation은 새 전체 실행으로 대체했다. 이는 하나의 전체 명령 성공을 주장하는 방식이 아니다. `src/drivers`는 test file이 없으며, 플랫폼·benchmark 도구 조건에 따른 기존 skip은 보존한다.

native 도구 optional 미설정(둘 다 없음·runtime만·compiler만)은 신규 테스트 부모 전체가 SKIP·성공 종료했다. REQUIRED=1인데 둘 다 없으면 예상대로 FAIL·exit 1이었다. 도구가 설정된 실제 동시요청 검사는 통과했다. STATIC 리뷰에서 발견한 optional 경로 결함을 부모 시작의 `nativeTools(t)`로 수정했다. 문서 LF와 source-only fixture의 명시적 report 목록 누락 때문에 실패했던 중간 실행은 보존하며, LF 및 fixture 경로를 수정한 집중 검사와 전체 foundation 결과로 해결을 확인했다.

right pane의 두 차례 최종 코드 STATIC_REVIEW에서 optional 결함 해결과 추가 finding 없음을 확인했다. 이는 독립 실행이나 공식 GitHub 승인이 아니다. 새 PR의 세 OS 필수 CI와 actual-main post-merge 검증은 아직 미실행이며, 로컬 통과로 대체하지 않는다.

로컬 미실행 항목은 route schema 입력이 없는 `TestFactMappingAgainstSchemas`, C# prepared closure가 없는 CLI SVC 관측, JSON prepared closure가 없는 holdout·benchmark control·reuse 검사다. case-sensitive 이름·executable bit·FIFO 검사는 이 Windows filesystem에서 SKIP했다. 이들을 로컬 PASS로 계산하지 않는다.

이 결과를 parser throughput 개선, 컴파일 횟수 감소, 다른 host에서의 보장 또는 전체 300초 문제 해결로 확대하지 않는다. fixture cache warm-up으로 단계 사이에 비용이 이동할 수 있으므로 전체 package 결과도 확인한다. 기존 실패 실행은 보존한다.
