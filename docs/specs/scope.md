# 제품 범위

`tree-sitter-grammar-kit`의 CLI `tsgk`와 작은 공개 offline Go API는 같은 코어로 grammar의 파일·생성물·구조·실행 결과와 그 근거를 검사·비교한다. 첫 사용자는 `go-treesitter`의 grammar 도입·업데이트 담당자다. 제품은 독립 도구이며 소비자의 Go 변환·scanner 이식·runtime·최종 채택 판단을 소유하지 않는다. 일반 application 파싱 경로에 kit 실행을 요구하지 않는다.

첫 release 후보의 최소 범위는 검증된 offline `inspect`, `identity`, `verify`, `schema`와 해당 공개 API다. Session 01~08 campaign은 그 위에 reproduction·incremental·native oracle·bounded replay·qualification을 순차 개발한다. 독립 offline 기능의 사용 가능성과 campaign 전체 목표 충족은 구분한다. campaign 완료는 release 승인이 아니다. 현재는 foundation 검사만 구현했다.

파일·결과 코어는 불필요한 C 종속을 갖지 않으며 첫 실행 backend는 표준 Tree-sitter다. `parser.c`는 native build가 요구하는 입력이고 inventory/schema/기록 비교의 일률적 선행 조건이 아니다. repository snapshot, 선택 grammar, 공유 source/dependency closure를 구분한다.

필수 qualification은 [언어·feature 등록부](../validation/language-feature-scope.md)의 26개 route 전체와 Windows amd64/Linux amd64/macOS arm64의 78개 요약 칸이다. maintained BrightScript, 제한된 historical audit, 자체 scannerless/stateful 및 경계 fixture는 별도 역할로 유지한다. 소비자가 26개 grammar를 모두 설치해야 한다는 뜻은 아니다.

언어의 공식 구문 근거와 검토된 구조 기대값으로 correctness를 판단하고 native 결과는 회귀·비교에 사용한다. upstream corpus와 ERROR 없는 tree만으로 feature 지원을 확정하지 않는다. required grammar gap은 kit의 검출 성공과 별도로 남으며 historical expected failure로 옮겨 지원 목표를 통과시키지 않는다. framework·서버·DB 실행과 미등록 확장은 구문 범위에 포함되지 않는다.

사용 사례는 legacy grammar inventory, 생성물 drift 탐지, 정적 node schema 변경 분류, incremental/fresh 불일치 재현, 고정 raw의 재판정, 플랫폼별 의미 결과 대조다. self-generated fingerprint는 신뢰된 출처 인증이 아니다.

compiler/runtime/parser 재구현, LSP/IDE, daemon/DB/web UI/plugin registry, 자동 설치·배포, 언어 사양의 자동 정답 판정, 임의 grammar의 완전한 sandbox, 자동 SemVer 판정은 범위 밖이다. `go-treesitter`는 별도 consumer이며 이번 campaign에 adapter 구현이나 core dependency 추가를 포함하지 않는다.

지원 claim은 [platform](platform-support.md)의 OS·arch·capability별 근거에 한정한다. BrightScript의 검증 성공은 kit의 성공 근거가 아니다. CLI 계약은 [명령과 profile](cli-and-profile.md), 작업 단위는 [roadmap](../roadmap.md)에 둔다.
