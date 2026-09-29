# Trust와 실행 계약

## 신뢰 경계

grammar·profile·archive·raw·adapter 응답은 모두 데이터 입력이다. source 안 명령·URL·환경 설정은 실행 승인이나 신뢰 anchor가 아니다. 악성 path로 source 밖을 읽거나 쓰기, JS/lifecycle 실행, 결과 주입, 압축 폭탄, child process 잔류가 주요 abuse case다. READ_DATA 경로는 process/network capability를 갖지 않는 package로 분리하며 인자·크기·path·형식을 먼저 검증한다.

| capability | 범위 |
|---|---|
| READ_DATA | 제한된 파일/구조화 raw 읽기·자체 계산; target code/process/network 없음 |
| WRITE_RESULT | caller가 지정한 새 결과 위치; source 변경 없음 |
| EXEC_GENERATOR | pinned tool 실행; JS mode는 grammar/dependency 코드 실행 포함 |
| BUILD_NATIVE | compiler와 parser/scanner/driver 전체 closure build |
| EXEC_NATIVE | parse/test/query/incremental; parser/scanner 실행 및 supervision |
| EXEC_ADAPTER | 고정 executable과 versioned protocol의 별도 consumer 실행 |
| FETCH_PINNED_INPUT | 승인한 URL/digest의 입력 준비; verify와 분리, 설치 권한 아님 |

`tree-sitter parse/test`도 C build와 native parser/scanner 실행을 수반할 수 있다. grammar.json 생성은 JS를 생략할 수 있지만 generator process 실행은 남는다. [generate](https://tree-sitter.github.io/tree-sitter/cli/generate.html), [native 요구](https://tree-sitter.github.io/tree-sitter/creating-parsers/1-getting-started.html)

프로필에 임의 shell hook을 넣지 않는다. argv/cwd/env allowlist와 executable identity를 명시한다. 설치·fetch·Git provenance·native/adapter는 독립적인 opt-in이며 현재 승인 범위를 늘리지 않는다. core는 native FFI/CGO를 사용하지 않는다.

## Path와 archive

portable path는 UTF-8 상대 경로, `/` separator, 빈 segment/`.`/`..`/제어문자/절대 경로/drive/UNC/역슬래시/ADS `:`를 거부한다. 대소문자 접기 충돌, Windows device basename(CON/PRN/AUX/NUL/COM1~9/LPT1~9 및 확장자 형태), trailing dot/space도 거부한다. Unicode 정규화 충돌은 S02에서 사전 검출 규칙을 고정하고 지원 범위 밖 이름은 명시적으로 거부한다.

입력 root 자체의 실제 경로를 먼저 확정하고 내부 member와 구분한다. macOS `/var` → `/private/var` root alias를 archive 탈출로 오판하지 않는다. 내부 symlink/hardlink/junction/reparse point는 r0에서 거부한다. count/file/total/depth/output 제한은 [profile](cli-and-profile.md)이 소유한다. 파일 검사 후 교체 race를 막을 handle 기반 접근이 없으면 그런 capability를 제공한다고 주장하지 않는다.

archive inspection은 extraction과 분리한다. 절대·상위·중복·case collision·file/directory 겹침·link·비정상 metadata·압축/해제 size 한도 초과를 거부하고 실제 읽은 bytes에도 한도를 적용한다. extraction은 필요할 때만 새 caller-owned 빈 directory에서 수행하고 멤버별 no-follow/open 경계를 검증한다. 이름 정규화만으로 TOCTOU를 해결했다고 하지 않는다. 다른 process가 workspace를 적대적으로 변경하는 상황을 지원하려면 OS별 안전한 접근을 별도로 증명해야 한다.

## 실행·실패·검토

caller-owned 새 workspace에서 snapshot을 읽고 실행한다. cache key는 source/tool/compiler/options/policy identity 전체를 포함하고 불일치 cache를 거부한다. timeout/output cap/메모리 종류/process-tree cleanup을 각 backend에서 실제 검증한다. parent 종료만으로 child 종료를 주장하지 않는다. cap/cleanup 확인이 불가능하면 엄격 profile은 BLOCKED다.

환경 정리와 private cache는 보안 sandbox가 아니다. 신뢰하지 못하는 native 코드 실행은 별도 격리 환경과 정확한 승인이 필요하다. offline 경로와 실행 backend, archive/path 처리, dependency 도입 변경은 full boundary review와 정상/adversarial 검사를 요구한다. raw 실패·취소·partial 출력은 failure evidence로 남기고 성공 manifest를 쓰지 않는다.

## Campaign 준비와 실행값

source/tool 확보와 offline 제품 호출을 분리한다. 승인 기록은 공급 URL/provider, immutable revision/digest, 실제 executable/dependency/header/compiler closure, 대상 위치, 허용 fetch/unpack/lifecycle/build/run 효과, 유한 한도와 cleanup 책임을 구분한다. 다운로드 승인은 script/native 실행 승인으로 확대되지 않는다. 기존 reference와 다른 저장소는 읽기 전용이며 새 복사본은 정확한 준비 승인 안에서만 만든다.

새 Go dependency는 필요한 이유·source/version/license·전이 dependency·CGO·script·호환성과 실행 효과를 검토한 뒤 준비 목록에 승인 등록한다. `x/sys`도 자동 허용하지 않지만 dependency 수를 줄이기 위해 위험한 unsafe OS API를 새로 구현하지 않는다. 언어별 SDK/DB server·전역 PATH·WSL·PowerShell 5·유료 모델 호출은 기본 준비 효과가 아니다.

모든 연산은 input files/bytes/depth, decoded/archive 누적량, output/records/nodes/captures, wall time, storage와 지원 가능한 memory/process control을 명시한다. 0/null은 무제한 승인이나 capability 증거가 아니다. sampled memory 관측은 hard cap이 아니며 필요한 cap이 없으면 해당 strict operation이 BLOCKED다. 상세 실행값과 소모량은 승인된 준비/Session receipt가 소유하며 공개 제품 동작은 local prompt를 읽지 않는다.
