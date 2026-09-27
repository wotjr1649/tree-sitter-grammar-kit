# 아키텍처

root `go.mod` 하나와 `src/` 제품·검증 경계를 사용한다. 현재 실제 Go package는 `src/internal/foundation/`뿐이다. 이후 CLI는 `src/cmd/tsgk/`, 내부 import는 `github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/...`가 된다. Go internal 가시성에 따라 이를 쓰는 도구/테스트도 `src/` 아래에 둔다. CLI가 생기는 S01 이후 root에서 `go build ./src/cmd/tsgk`가 가능해야 한다. [Go module layout](https://go.dev/doc/modules/layout)

## 책임과 데이터 흐름

CLI → app orchestration → inventory/identity/manifest/schema/tree/evidence/replay/report의 OS 중립 계산으로 의존한다. profile과 pathguard는 입력 경계를 소유한다. 외부 실행이 필요한 orchestration만 runner/toolchain/reproduce/incremental/oracle/parity로 연결한다. runner는 OS backend를 선택하며 offline package에서 runner/network를 import하지 않는다. 범용 utils/common/model이나 plugin registry를 먼저 만들지 않는다.

`DISCOVER → VALIDATE_PROFILE → SNAPSHOT → PLAN → CHECK_CAPABILITIES → EXECUTE → COMPARE → WRITE_EVIDENCE`가 실행형 작업의 상태다. 읽기 전용 작업은 EXECUTE를 생략하고 자체 계산만 한다. profile/identity/capability 검증 실패는 외부 실행 전에 끝난다. source snapshot은 불변이며 output은 선택 입력 집합 밖 새 workspace에만 쓴다.

source와 generated artifacts, tool closure, policy와 evidence를 별도 identity로 모델링한다. tree/query comparator는 host 관측을 처리하지 않는다. OS backend가 platform metric과 cleanup receipt를 전달하면 orchestration이 profile의 required capability를 평가한다. 데이터 contract는 `src/contracts/`, 작고 독립적인 regression fixture는 `src/testdata/`에 둔다. byte/archives/malformed fixture는 Git normalization을 끈다.

native C는 `src/drivers/native-c/`에 분리해 Go package build에 포함되지 않게 한다. native oracle은 별도 process이고 core에는 FFI/CGO dependency가 없다. 실제 dependency가 필요할 때만 go.sum을 만든다. `src/go.mod`, go.work, 공개 pkg/API/runtime bundle은 기본 구조가 아니다. embed가 필요해지면 package-relative 소유로 설계하며 상위 경로 embed를 가정하지 않는다.

## 단순성과 자원

stdlib, streaming hash/record, 명시적 size bound, iterative traversal을 우선한다. deep tree 재귀와 대형 raw 전체 복제를 피한다. benchmark는 serial 또는 고정 concurrency로 수행한다. 보안 OS API를 unsafe 코드로 새로 만들기보다 필요한 경우 `x/sys`의 버전·license·CGO·호출 효과를 검토하고 최소 의존성으로 채택한다. 현재 외부 Go dependency는 없다.

fixture는 자체 scannerless/stateful-scanner를 작게 작성하며 외부 grammar 전체를 vendor하지 않는다. artifact/source ZIP 배포는 [identity 계약](../specs/identity-and-evidence.md), 권한은 [trust](../specs/trust-and-execution.md), 순서는 [roadmap](../roadmap.md)이 소유한다.
