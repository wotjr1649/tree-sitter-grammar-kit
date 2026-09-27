# 보안 경계

현재 구현은 이 저장소의 개발용 foundation 검사다. 임의 grammar를 안전하게 실행하는 sandbox를 제공하지 않는다.
향후 offline 명령과 generator/native/adapter 실행 경계는 [실행 계약](docs/specs/trust-and-execution.md)이 소유한다. 환경 정리·private cache·resource cap만으로 악성 native 코드를 격리했다고 주장하지 않는다.

민감하지 않은 결함은 재현 범위와 함께 Issue로 보고할 수 있다. secret이나 비공개 입력은 공개 Issue에 올리지 않는다. 별도 private reporting 채널의 활성 상태는 확인하지 않았다.
