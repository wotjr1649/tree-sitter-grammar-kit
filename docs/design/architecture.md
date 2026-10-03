# 아키텍처

root `go.mod` 하나와 `src/` 제품·검증 경계를 사용한다. 현재 Go package는 공개 entry `github.com/wotjr1649/tree-sitter-grammar-kit/src/kit`(S01 offline core: discovery·identity·encoding 판별·guard·E0·corpus inventory), CLI `src/cmd/tsgk/`, 개발 검사 `src/internal/foundation/`이다. 내부 import는 실제 책임이 생길 때 `src/internal/...`로 둔다. API와 CLI는 동일한 offline core/guard를 호출한다. [공개 API](../specs/public-go-api.md)가 함수·타입·오류·소유권을 소유한다. 실제 package는 담당 Session에서 만든다. Go internal 가시성에 따라 내부 구현을 쓰는 도구/테스트는 `src/` 아래에 둔다. [Go module layout](https://go.dev/doc/modules/layout)

## 책임과 데이터 흐름

CLI/API → 공유 offline operation의 OS 중립 계산으로 의존한다. CLI는 인자·표현·exit mapping만 담당하고 API가 CLI를 subprocess로 호출하지 않는다. profile과 pathguard는 입력 경계를 소유한다. 외부 실행이 필요한 orchestration만 runner/toolchain/reproduce/incremental/oracle로 연결한다. runner는 OS backend를 선택하며 offline package에서 runner/network를 import하지 않는다. package는 실제 책임이 생길 때 만들며 범용 utils/common/model이나 plugin registry를 먼저 만들지 않는다.

`DISCOVER → VALIDATE_PROFILE → SNAPSHOT → PLAN → CHECK_CAPABILITIES → EXECUTE → COMPARE → WRITE_EVIDENCE`가 실행형 작업의 상태다. 읽기 전용 작업은 EXECUTE를 생략하고 자체 계산만 한다. profile/identity/capability 검증 실패는 외부 실행 전에 끝난다. source snapshot은 불변이며 output은 선택 입력 집합 밖 새 workspace에만 쓴다.

source와 generated artifacts, tool closure, policy와 evidence를 별도 identity로 모델링한다. tree/query comparator는 host 관측을 처리하지 않는다. OS backend가 platform metric과 cleanup receipt를 전달하면 orchestration이 profile의 required capability를 평가한다. 데이터 contract는 `src/contracts/`, 작고 독립적인 regression fixture는 `src/testdata/`에 둔다. byte/archives/malformed fixture는 Git normalization을 끈다.

native C는 `src/drivers/native-c/`에 분리해 Go package build에 포함되지 않게 한다. native oracle은 별도 process이고 core에는 FFI/CGO dependency가 없다. S01은 E0, S04는 한 runner, S05는 최소 native driver/ordered tree/edit 비교, S06은 같은 producer의 query/API 기록 확장, S07은 같은 evidence의 검증과 제한 replay를 소유한다. 실제 dependency가 필요할 때만 승인 검토 후 go.sum을 만든다. `src/go.mod`, go.work, root pkg/runtime bundle은 만들지 않는다. embed가 필요해지면 package-relative 소유로 설계한다.

외부 소비자 시험은 `src/testdata/consumer/`의 template을 checkout 밖 임시 module로 구성한다. 실제 중첩 `go.mod`는 추적하지 않는다. 초기 local replace 시험과 S08의 versioned local module-proxy/source-export 시험을 구분하며 unpublished tag나 개발자 경로에 제품이 의존하지 않는다.

## 단순성과 자원

stdlib, streaming hash/record, 명시적 size bound, iterative traversal을 우선한다. deep tree 재귀와 대형 raw 전체 복제를 피한다. benchmark는 serial 또는 고정 concurrency로 수행한다. 보안 OS API를 unsafe 코드로 새로 만들기보다 필요한 경우 `x/sys`의 버전·license·CGO·호출 효과를 검토하고 최소 의존성으로 채택한다. 현재 외부 Go dependency는 없다.

fixture는 자체 scannerless/stateful-scanner를 작게 작성하며 외부 grammar 전체를 vendor하지 않는다. artifact/source ZIP 배포는 [identity 계약](../specs/identity-and-evidence.md), 권한은 [trust](../specs/trust-and-execution.md), 순서는 [roadmap](../roadmap.md)이 소유한다.
