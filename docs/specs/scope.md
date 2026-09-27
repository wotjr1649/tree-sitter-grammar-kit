# 제품 범위

`tree-sitter-grammar-kit`의 CLI `tsgk`는 Tree-sitter grammar의 파일·생성물·구조·실행 결과와 그 근거를 재현 가능한 identity로 검증한다. grammar 작성자와 runtime 소비자가 같은 입력·정책을 사용했는지 설명하는 작은 Go CLI를 목표로 한다.

첫 release 후보의 최소 범위는 검증된 offline `inspect`, `identity`, `verify`, `schema`다. Session 01~08 campaign은 그 위에 reproduction·incremental·native oracle·replay·qualification을 순차 개발한다. campaign 완료는 release 승인이 아니다. 현재는 foundation 검사만 구현했다.

사용 사례는 legacy grammar inventory, 생성물 drift 탐지, 정적 node schema 변경 분류, incremental/fresh 불일치 재현, 고정 raw의 재판정, 플랫폼별 의미 결과 대조다. self-generated fingerprint는 신뢰된 출처 인증이 아니다.

compiler/runtime/parser 재구현, LSP/IDE, daemon/DB/web UI/plugin registry, 자동 설치·배포, 언어 사양의 자동 정답 판정, 임의 grammar의 완전한 sandbox, 자동 SemVer 판정은 범위 밖이다. `go-treesitter`는 별도 consumer이며 이번 campaign에 adapter 구현이나 core dependency 추가를 포함하지 않는다.

지원 claim은 [platform](platform-support.md)의 OS·arch·capability별 근거에 한정한다. BrightScript의 검증 성공은 kit의 성공 근거가 아니다. CLI 계약은 [명령과 profile](cli-and-profile.md), 작업 단위는 [roadmap](../roadmap.md)에 둔다.
