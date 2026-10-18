# Security policy

## Current security boundary

구현 범위는 저장소 foundation 검사, offline inspection/identity/verification/schema API와 CLI, 그리고 experimental reproduction/native/evidence/qualification 기능이다. 세부 범위는 [문서 map](docs/README.md)과 [platform 계약](docs/specs/platform-support.md)을 따른다. 경로 검사는 caller-owned tree와 namespace가 검사·사용 중 변경되지 않는다는 전제를 갖는다. 이 프로젝트는 임의 grammar나 악성 native 코드의 보안 sandbox가 아니며, CI 통과는 실행 안전성 인증이 아니다.

[trust와 실행 계약](docs/specs/trust-and-execution.md)이 offline 데이터 처리와 opt-in generator/native 실행의 경계를 소유한다. 구현 범위와 아직 계획 단계인 기능은 문서 map에서 구분한다. 환경 정리, private cache와 resource cap만으로 악성 native 코드를 격리할 수 없다. source/tool identity와 보존 evidence는 추적 근거이며, 자체 생성 fingerprint는 입력 출처를 인증하지 않는다.

CLI report publication과 native output의 일부 handle 결속 보호 및 아직 남은 생성·namespace·cleanup 경계는 [trust 계약의 #215 절](docs/specs/trust-and-execution.md)에 구분한다. 이 부분 구현은 namespace 전체에 대한 동시 변경 안전성이나 임의 native 코드 격리를 보증하지 않는다.

## Reporting a concern

Report non-sensitive defects through [GitHub Issues](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues). Include:

- the affected commit or tag and the relevant command;
- OS/architecture and tool versions;
- expected and observed behavior, with a minimal non-sensitive reproduction;
- the affected boundary and impact, if known.

Do not post secrets, credentials, private inputs, personal information, or sensitive exploit details in public Issues or attachments. Private vulnerability reporting, a security email address, and a response SLA are not established by this policy. Before sharing sensitive material, ask the maintainer for a private reporting route using only non-sensitive context; keep the material private until that route is confirmed.

## Support and disclosure limits

지원 범위와 검증 상태는 [platform](docs/specs/platform-support.md) 및 [validation](docs/validation/validation.md) 계약에 정의한다. 모든 입력·플랫폼·미래 revision이나 security response deadline을 보장하지 않는다. 보고서는 영향받은 revision에 연결하고 관측 실패와 미검증 위험을 구분한다.
