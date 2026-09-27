# 문서 지도

현재 구현은 repository foundation 검사다. 제품 명령과 r0 데이터 계약은 개발 계획이며 구현 완료를 뜻하지 않는다.

| 작업 | canonical 소유 문서 |
|---|---|
| 목적·release 후보 범위 | [scope](specs/scope.md) |
| 명령·오류·profile 필드 | [CLI/profile](specs/cli-and-profile.md) |
| fingerprint·manifest·evidence | [identity/evidence](specs/identity-and-evidence.md) |
| untrusted 입력·path/archive·실행 권한 | [trust/execution](specs/trust-and-execution.md) |
| ordered CST·query·adapter | [tree/protocol](specs/tree-and-adapter-protocol.md) |
| OS/arch/capability | [platform](specs/platform-support.md) |
| 구조·의존·상태 전이 | [architecture](design/architecture.md), [선택 근거](design/decisions/0001-core-and-execution.md) |
| 외부 고정 기준·license 관측 | [provenance](provenance/upstream-sources.md) |
| 검사·리뷰·merge | [validation](validation/validation.md), [workload](validation/workload-matrix.md) |
| 세션 범위·Issue/Milestone | [roadmap](roadmap.md) |
| 실제 Session 00 결과 | [foundation 보고](reports/session-00-foundation.md) |

현재 작업에 필요한 문서만 읽는다. 지속 규칙의 진입점은 [AGENTS.md](../AGENTS.md)다.
`docs/prompts/`와 `docs/plans/`는 로컬 실행 자료, `artifacts/`는 보존할 raw와 handoff, `_ref/`는 고정 reference, `.work/`는 재생성 가능한 작업공간이다. 이 경로는 Git에서 제외한다. 공개 acceptance는 이 지도와 Issue에서 독립적으로 이해할 수 있어야 한다.
