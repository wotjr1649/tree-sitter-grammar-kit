# tree-sitter-grammar-kit

Tree-sitter grammar의 생성물·구조·검증 근거·runtime 결과를 identity에 연결하는 Go 기반 CGO-free CLI `tsgk`를 준비하는 저장소다.

현재 구현은 repository 경계·canonical 문서·Git 추적 정책을 검사하는 Session 00 foundation이다. `tsgk` 제품 명령과 native 실행 기능은 아직 구현하지 않았다.

Go 1.27.1과 Git이 필요하다. Windows Native에서 PowerShell 7을 사용하며 core 검사는 `CGO_ENABLED=0`이다. 설치할 Go dependency는 없고 Node/Python/C compiler/WSL을 요구하지 않는다.

[문서 지도](docs/README.md)에서 설계와 작업 계약을 찾고, [검사 절차](docs/validation/validation.md)에 따라 test/vet/build를 실행한다. [실제 검증 보고](docs/reports/session-00-foundation.md)는 foundation의 범위와 미실행 기능을 구분한다. 세 OS CI 결과는 제품/native 지원 선언이 아니다.
