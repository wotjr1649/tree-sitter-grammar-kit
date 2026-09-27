# ADR 0001 — 작은 core와 명시적 실행 경계

상태: 채택. profile/tree/adapter 형식의 성숙도는 draft/r0다.

| 결정 | 검토한 대안·이유 | 비용·재검토 조건 |
|---|---|---|
| Go CGO-free, root module, src | Node/Python 선구현 후 포팅·root cmd/internal 대신 사용자 경계와 Windows Native를 유지 | native API는 process protocol 필요; 실제 consumer가 생길 때 공개 API 검토 |
| offline 계산과 OS backend 분리 | 묵시적인 CLI/native 실행은 scanner/JS 위험을 숨김 | capability preflight 필요; 검증된 OS API/의존성 도입 시 영향 리뷰 |
| raw/semantic/정책 identity 분리 | 단일 PASS·버전 문자열만으로 근거를 묶으면 stale 승계 발생 | receipt 양 증가; deterministic field와 consumer 검증 시 형식 revision |
| native C oracle은 별도 process | CGO/FFI 연결 대신 core 배포와 native toolchain 분리 | build closure/감독 필요; S05 CLI adapter → S06 generic driver 단계로 순환 의존 방지 |

각 결정의 실행 계약은 [architecture](../architecture.md), [trust](../../specs/trust-and-execution.md), [identity](../../specs/identity-and-evidence.md), [protocol](../../specs/tree-and-adapter-protocol.md)에 한 번씩 둔다. 제한적인 replay는 과거 계산의 재판정이며 새 성능 측정을 대신하지 않는다.
