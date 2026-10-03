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

portable path는 UTF-8 상대 경로, `/` separator, 빈 segment/`.`/`..`/제어문자/절대 경로/drive/UNC/역슬래시/ADS `:`를 거부한다. 대소문자 접기 충돌, Windows device basename(CON/PRN/AUX/NUL/COM1~9/LPT1~9 및 확장자 형태), trailing dot/space도 거부한다. Unicode 정규화 충돌은 S02가 고정한 이름 정책 `portable-names-r1`(아래)이 ASCII 밖 이름을 명시 거부하는 방식으로 다룬다.

grammar 선택의 `Selection.Grammar`와 source 등록부의 `grammar_subdirectory`는 전체 값 `"."`에 한해 별도 root sentinel을 허용한다. 검증된 root를 가리키며 정규화로 다른 문자열을 sentinel로 바꾸지 않는다. 파일/member path와 `./child`·`child/.`·`child/../other`에는 이 예외를 적용하지 않는다.

입력 root 자체의 실제 경로를 먼저 확정하고 내부 member와 구분한다. macOS `/var` → `/private/var` root alias를 archive 탈출로 오판하지 않는다. 내부 symlink/hardlink/junction/reparse point는 r0에서 거부한다. count/file/total/depth/output 제한은 [profile](cli-and-profile.md)이 소유한다. 파일 검사 후 교체 race를 막을 handle 기반 접근이 없으면 그런 capability를 제공한다고 주장하지 않는다.

archive inspection은 extraction과 분리한다. 절대·상위·중복·case collision·file/directory 겹침·link·비정상 metadata·압축/해제 size 한도 초과를 거부하고 실제 읽은 bytes에도 한도를 적용한다. extraction은 필요할 때만 새 caller-owned 빈 directory에서 수행하고 멤버별 no-follow/open 경계를 검증한다. 이름 정규화만으로 TOCTOU를 해결했다고 하지 않는다. 다른 process가 workspace를 적대적으로 변경하는 상황을 지원하려면 OS별 안전한 접근을 별도로 증명해야 한다.

### S02 구현 — archive profile `zip-r1`과 이름 정책 `portable-names-r1`

`kit.Verify`/`tsgk verify --archive`는 ZIP을 풀지 않고 직접 읽는다. 지원 범위(`zip-r1`)는 단일 디스크, 압축 방식 0(stored)·8(deflate), data descriptor(general purpose bit 3)다. ZIP64, 다중 디스크, 암호화·strong encryption·masked header, 다른 압축 방식은 `UNSUPPORTED`(`ZIP64_UNSUPPORTED`, `ZIP_MULTIDISK_UNSUPPORTED`, `ZIP_ENCRYPTED_UNSUPPORTED`, `ZIP_METHOD_UNSUPPORTED`)로 끝나며 건너뛰지 않는다. tar·gzip 같은 다른 형식은 EOCD가 없어 `ZIP_EOCD_MISSING`이다.

구조 검사는 member 데이터를 읽기 전에 한다. EOCD는 comment 길이가 파일 끝과 정확히 맞아야 하고, central directory는 EOCD 바로 앞에서 끝나야 한다(`ZIP_STRUCTURE_INVALID`). entry 수는 entry를 할당하기 전에 `ArchiveLimits.Entries`와 비교한다. 각 local header는 central entry와 flags·method·이름 bytes가 같아야 하고, descriptor가 없으면 CRC·두 크기도 같아야 한다. descriptor가 있으면 local 값은 0이거나 같아야 하며 descriptor 값은 central과 같아야 한다(`ZIP_LOCAL_CENTRAL_MISMATCH`, `ZIP_DESCRIPTOR_MISMATCH`). local header·data·descriptor 영역이 서로 겹치거나 central directory와 겹치면 `ZIP_OVERLAP`이다. stored member는 두 크기가 같아야 한다.

member 이름은 원래 bytes를 진단용으로 보존하고 고쳐 쓰지 않는다. directory entry(이름 끝 `/`)는 데이터를 가질 수 없다(`ZIP_DIRECTORY_HAS_DATA`). Unix 생성 entry의 mode가 symlink이거나 DOS/NTFS 생성 entry에 reparse point 속성이 있으면 `ARCHIVE_LINK_REJECTED`, regular·directory가 아닌 mode는 `ARCHIVE_SPECIAL_REJECTED`, mode와 이름의 file/directory 종류가 다르면 `ZIP_ENTRY_TYPE_INCONSISTENT`다. 같은 이름은 `ARCHIVE_DUPLICATE_MEMBER`, ASCII 대소문자만 다른 이름은 `ARCHIVE_CASE_COLLISION`, 한 entry가 file이고 다른 entry가 그 아래를 쓰는 경우(대소문자 무시)는 `ARCHIVE_FILE_DIRECTORY_CONFLICT`다.

이름 정책 `portable-names-r1`은 위 portable path 규칙에 ASCII만 허용하는 조건을 더한다. Unicode 정규화 충돌을 표 없이 검출할 수 없어 정규화 표 dependency를 들이지 않고, ASCII 밖 이름은 `..._NOT_ASCII`(`UNSUPPORTED`)로 명시 거부한다. 거부 사유 code는 `EMPTY`, `BACKSLASH`(UNC `\\server` 포함), `ABSOLUTE`(`//server` 포함), `DRIVE`, `ADS`, `NOT_ASCII`, `CONTROL`, `EMPTY_SEGMENT`, `TRAVERSAL`, `TRAILING_DOT_SPACE`, `DEVICE`이고 archive member는 `ARCHIVE_PATH_` 접두, expected path는 `EXPECTED_PATH_` 접두로 보고한다. 이 정책은 verify의 archive member와 expected path에 적용한다. 디렉터리 discovery의 S01 규칙(UTF-8 portable path)은 바꾸지 않는다. 디렉터리 subject에서 ASCII 밖 이름의 actual 파일은 expected에 있을 수 없으므로 `UNEXPECTED_FILE`이 된다.

member 데이터는 선택 범위 안에서만 streaming으로 읽는다. 선언한 압축 해제 크기가 단일 파일 한도(등록된 대용량 identity 예외 포함)를 넘으면 읽기 전에 `FILE_BYTES_LIMIT`이다. 읽는 동안 선언 크기를 넘는 순간 `ZIP_SIZE_MISMATCH`로 멈추고, 누적 해제량이 `total_bytes`를 넘으면 `TOTAL_BYTES_LIMIT`이다. 그래서 압축 폭탄도 1 MiB 읽기 buffer 밖의 메모리를 쓰지 않는다. 끝에서 크기·CRC·남은 압축 bytes를 확인한다(`ZIP_TRUNCATED`, `ZIP_CRC_MISMATCH`, `ZIP_COMPRESSED_SIZE_MISMATCH`, 손상된 deflate는 `ZIP_DATA_CORRUPT`). path depth는 `depth` 한도, 선택 file 수는 `files` 한도, archive 파일 크기는 `ArchiveLimits.Bytes`(`ARCHIVE_BYTES_LIMIT`)를 따른다. archive 파일 자체는 root처럼 caller가 명시한 alias를 따라갈 수 있지만 local 일반 파일이어야 하고, 읽는 동안 크기·수정 시각이 바뀌면 `SOURCE_CHANGED`다.

중첩 archive는 자동으로 찾지 않는다. 호출자가 `Nested`(`--nested`)로 이름을 준 member만 열고, 그 member 기록 대신 `<member path>/<내부 path>` 이름의 member들이 scope에 들어간다. 중첩 member의 bytes(단일 파일 한도 안)는 메모리에 두고, 그 해제량과 내부 member 해제량은 모두 같은 `total_bytes`에, 내부 entry 수는 같은 `ArchiveLimits.Entries`에 누적한다. 깊이는 `ArchiveLimits.Depth`(바깥 archive가 1)를 넘으면 `ARCHIVE_DEPTH_LIMIT`, 지정한 member가 없으면 `NESTED_MEMBER_NOT_FOUND`다.

extraction은 구현하지 않는다(UNAVAILABLE). 따라서 이 build의 archive 경로는 쓰기·덮어쓰기·정리 효과가 없다.

## 실행·실패·검토

caller-owned 새 workspace에서 snapshot을 읽고 실행한다. cache key는 source/tool/compiler/options/policy identity 전체를 포함하고 불일치 cache를 거부한다. timeout/output cap/메모리 종류/process-tree cleanup을 각 backend에서 실제 검증한다. parent 종료만으로 child 종료를 주장하지 않는다. cap/cleanup 확인이 불가능하면 엄격 profile은 BLOCKED다.

환경 정리와 private cache는 보안 sandbox가 아니다. 신뢰하지 못하는 native 코드 실행은 별도 격리 환경과 정확한 승인이 필요하다. offline 경로와 실행 backend, archive/path 처리, dependency 도입 변경은 full boundary review와 정상/adversarial 검사를 요구한다. raw 실패·취소·partial 출력은 failure evidence로 남기고 성공 manifest를 쓰지 않는다.

## Campaign 준비와 실행값

source/tool 확보와 offline 제품 호출을 분리한다. 승인 기록은 공급 URL/provider, immutable revision/digest, 실제 executable/dependency/header/compiler closure, 대상 위치, 허용 fetch/unpack/lifecycle/build/run 효과, 유한 한도와 cleanup 책임을 구분한다. 다운로드 승인은 script/native 실행 승인으로 확대되지 않는다. 기존 reference와 다른 저장소는 읽기 전용이며 새 복사본은 정확한 준비 승인 안에서만 만든다.

새 Go dependency는 필요한 이유·source/version/license·전이 dependency·CGO·script·호환성과 실행 효과를 검토한 뒤 준비 목록에 승인 등록한다. `x/sys`도 자동 허용하지 않지만 dependency 수를 줄이기 위해 위험한 unsafe OS API를 새로 구현하지 않는다. 언어별 SDK/DB server·전역 PATH·WSL·PowerShell 5·유료 모델 호출은 기본 준비 효과가 아니다.

모든 연산은 input files/bytes/depth, decoded/archive 누적량, output/records/nodes/captures, wall time, storage와 지원 가능한 memory/process control을 명시한다. 0/null은 무제한 승인이나 capability 증거가 아니다. sampled memory 관측은 hard cap이 아니며 필요한 cap이 없으면 해당 strict operation이 BLOCKED다. 상세 실행값과 소모량은 승인된 준비/Session receipt가 소유하며 공개 제품 동작은 local prompt를 읽지 않는다.
