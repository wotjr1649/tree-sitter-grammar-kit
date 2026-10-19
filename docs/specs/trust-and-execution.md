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

portable path는 UTF-8 상대 경로, `/` separator, 빈 segment/`.`/`..`/제어문자/절대 경로/drive/UNC/역슬래시/ADS `:`를 거부한다. 대소문자 접기 충돌, Windows device basename(CON/PRN/AUX/NUL/COM1~9/LPT1~9/COM¹²³/LPT¹²³/CONIN$/CONOUT$, 확장자 형태와 첫 마침표 앞 공백 형태 `CON .txt` 포함; [Windows 이름 규칙](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file)), trailing dot/space도 거부한다. Unicode 정규화 충돌은 S02가 고정한 이름 정책 `portable-names-r1`(아래)이 ASCII 밖 이름을 명시 거부하는 방식으로 다룬다.

grammar 선택의 `Selection.Grammar`와 source 등록부의 `grammar_subdirectory`는 전체 값 `"."`에 한해 별도 root sentinel을 허용한다. 검증된 root를 가리키며 정규화로 다른 문자열을 sentinel로 바꾸지 않는다. 파일/member path와 `./child`·`child/.`·`child/../other`에는 이 예외를 적용하지 않는다.

입력 root 자체의 실제 경로를 먼저 확정하고 내부 member와 구분한다. macOS `/var` → `/private/var` root alias를 archive 탈출로 오판하지 않는다. 내부 symlink/hardlink/junction/reparse point는 r0에서 거부한다. count/file/total/depth/output 제한은 [profile](cli-and-profile.md)이 소유한다. 파일 검사 후 교체 race를 막을 handle 기반 접근이 없으면 그런 capability를 제공한다고 주장하지 않는다.

### CLI 입력 경계의 directory identity 검사

CLI의 output, 외부 신뢰 문서, reproduce/native workspace 검사는 경로 조상과 입력 root만 비교하지 않는다. root 전체의 directory identity를 출력·문서·workspace parent 및 그 조상의 identity와 대조하여, root 하위 directory가 외부 이름으로 보이는 `subst`/bind-mount 형태도 검사한다. identity는 열린 directory의 `File.Stat`에서 얻는다. Windows의 path-based `os.Stat`은 identity 조회를 `os.SameFile`까지 지연시키고 조회 실패를 false로 숨길 수 있으므로 membership 근거로 쓰지 않는다. 이 검사는 OS의 file identity 의미를 사용하며 filesystem identity가 정상 제공되는 caller-owned local tree를 전제로 한다.

root를 포함해 최대 10000 entries, root 기준 directory depth 64, destination ancestor 256, directory batch 128, 검사 한 쌍당 협력적 deadline 10초를 적용한다. file 내용은 읽지 않으며 root 내부 symlink와 Windows name-surrogate reparse alias를 따라가지 않는다. 이 alias·읽기/identity 오류·한도 초과는 미확인이다. bind mount 자체를 식별하여 제외하는 검사는 아니며, mount를 포함한 directory tree에 동일한 identity/탐색 한도를 적용한다. root가 큰 경우에도 확인한 일부만으로 outside를 승인하지 않는다. 동기 filesystem syscall을 중단시키는 hard timeout은 아니며, 열린 directory 수는 depth에 비례한다. 서로 다른 경로 검사마다 한도가 적용되므로 전체 명령의 wall cap과 같은 의미가 아니다.

미확인 결과는 trust/workspace 검사에서 `INPUT_PATH_UNVERIFIED`(exit 2), output publication에서 `OUTPUT_PATH_UNVERIFIED`(exit 4), qualify의 사전 output 검사에서 `OUTPUT_PATH_UNVERIFIED`(exit 2)로 거부한다. 확인된 내부 경로의 기존 `*_INSIDE_INPUT` code와 output의 `OUTPUT_PARENT_ALIAS`·no-clobber publication은 유지한다. directory subject가 없는 archive/schema 명령은 입력 directory 검사를 적용하지 않고 기존 destination 거부를 유지한다. namespace가 적대적으로 동시에 교체되는 상황에 대한 TOCTOU 방어·native sandbox는 이 검사로 제공하지 않는다.

archive inspection은 extraction과 분리한다. 절대·상위·중복·case collision·file/directory 겹침·link·비정상 metadata·압축/해제 size 한도 초과를 거부하고 실제 읽은 bytes에도 한도를 적용한다. extraction은 필요할 때만 새 caller-owned 빈 directory에서 수행하고 멤버별 no-follow/open 경계를 검증한다. 이름 정규화만으로 TOCTOU를 해결했다고 하지 않는다. 다른 process가 workspace를 적대적으로 변경하는 상황을 지원하려면 OS별 안전한 접근을 별도로 증명해야 한다.

### #215 출력 handle 결속과 미완료 경계

완료 report의 CLI publication은 output parent와 모든 input root를 `os.Root`로 열고, 검사부터 임시 파일의 배타 생성·동기화·no-clobber 게시·종료까지 유지한다. Linux·macOS는 `Link`와 임시 이름 정리를 사용한다. Windows는 `NtCreateFile(FILE_CREATE)`로 `DELETE` 권한과 delete sharing 없는 임시 file handle을 확보하고, 그 handle의 `FileRenameInformation`(`ReplaceIfExists=false`, 열린 parent 상대 단일 basename)으로 게시한다. 임시 이름은 별도 short alias가 생기지 않는 8.3 형식이며 최종 이름과 첫 글자를 분리한다. 게시 전 실패는 같은 handle의 `FileDispositionInformation`과 Close로 정리하며 임시 이름을 다시 열거나 삭제하지 않는다. 게시 뒤 Close 실패는 게시 사실과 함께 실패로 반환하고 최종 파일을 삭제하지 않는다. Windows final basename은 reserved·ADS·separator·끝 dot/space·control 이름을 거부한다. 추가 권한이나 filesystem 기능이 없으면 pathname 방식·ACL 변경으로 우회하지 않는다. `qualify`는 첫 host뿐 아니라 모든 host root를 최종 검사한다. directory 탐색의 child lookup도 해당 root handle 상대 연산을 사용하며, 검사와 열기 사이의 identity 변경이나 관측된 alias를 거부한다. Windows의 irregular entry는 pathname `FindFirstFile` fallback 없이 parent handle 상대 reparse-tag 조회로 분류한다. 이 변경은 입력 내용 읽기 전체를 namespace snapshot으로 만드는 기능이 아니다.

native CLI는 containment를 검사한 output parent와 case·grammar input root handle을 호출 종료까지 유지한다. internal/native에는 검사한 parent와 단일 basename의 `OutputTarget`을 빌려주며, 생성은 그 parent에서만 수행하고 닫힌 target·생성 오류에서 pathname 방식으로 fallback하지 않는다. 이때 요청의 `Out`은 표시용이며 native는 caller의 parent를 닫지 않는다. CLI는 소유한 두 input root를 `InputRoots`로 함께 native에 빌려준다. non-nil binding은 case·grammar 양쪽의 열린 handle을 요구하며, 누락·종료된 handle에서 문자열 경로로 fallback하지 않는다. direct·batch·SVC reference·Oracle의 case 읽기와 build의 grammar 복사는 각 역할의 root 상대 연산을 사용한다. component의 link·special 및 관측된 identity 교체를 거부하고, leaf는 Linux·macOS의 `O_NOFOLLOW|O_NONBLOCK` 또는 Windows의 `FILE_OPEN_REPARSE_POINT`로 열어 regular file인지, 열린 handle의 hard link 수가 1인지 확인한다(core guard와 같은 r0 hardlink 거부, `SOURCE_LINK_REJECTED`). 실제 열린 file과 전후 entry의 identity를 대조하고, 같은 buffer의 hash·size를 검증한 뒤 그 buffer를 실행 입력·grammar 사본에 사용한다. cache hit 전에도 grammar 검증을 실행한다. 중간 component의 전후 대조는 모든 순간의 alias 부재나 namespace snapshot을 증명하지 않는다. binding 없는 기존 internal 호출과 별도 runtime source 읽기는 이 보완의 대상이 아니다.

native `incremental`·`oracle record`는 output과 고정 child directory를 배타 생성하고, 획득한 directory handle에서 member를 `O_CREATE|O_EXCL`로 쓴다. 기존 output을 수용하는 `MkdirAll`과 member truncate를 사용하지 않는다. Windows는 `NtCreateFile(FILE_DIRECTORY_FILE, FILE_CREATE)`의 생성 handle을 delete sharing 없이 유지하면서 `os.Root`와 immediate `File.Stat` identity를 대조한다. 이 보호는 해당 handle로 directory를 list/read-attributes 할 권한도 요구하며, 권한 오류를 약한 생성 방식이나 ACL 변경으로 우회하지 않는다. member 기록 후 offline set 검사는 output root에 결속한다.

`incremental`은 최종 `result.json`의 배타 file handle을 먼저 확보하고 child·output directory를 모두 닫은 뒤, 그 종료 결과를 직렬화하여 확보한 file handle로 기록한다. directory 종료 실패는 저장·반환 report 모두 `FAILED`와 `OUTPUT_CLOSE_FAILED`를 가지며 기존 `PASS` assessment는 `NOT_ASSESSED`가 된다. 최종 file의 write·close 실패는 `EVIDENCE_WRITE_FAILED`와 반환 I/O 오류이며, 이미 기록한 bytes에 마지막 close 실패를 사후 반영한다고 주장하지 않는다. 호출자는 반환 오류·CLI 종료 상태까지 확인해야 하며 파일 존재나 내부 `PASS`만으로 실행 전체의 성공을 채택하지 않는다. 이 마지막 쓰기 동안 directory 이름이 고정된다는 보장은 없고, 쓰기는 이미 확보한 file 객체에 결속한다. `oracle record`의 set 구조 유효성도 호출 전체의 정상 종료와 구분한다. CLI가 소유한 경로 handle을 닫는 오류는 native가 이미 저장한 report에 사후 반영되지 않으며, stdout report의 실패와 nonzero exit로 반환한다. native stdout의 Write 오류·short write도 `OUTPUT_WRITE_FAILED`와 exit 4로 반환한다.

**#215는 이 부분 구현만으로 완료되지 않는다.** Linux·macOS의 `Mkdir` 성공부터 첫 `OpenRoot`까지 원래 생성한 directory임을 증명하는 공백은 남는다. random staging·추가 stat·협조하지 않는 process에 대한 lockfile을 해결 근거로 삼지 않는다. namespace 전체의 원자적 snapshot과 Linux·macOS의 임시 file basename 자체 교체에 대한 객체 지정 cleanup은 아직 증명하지 않는다. native CLI의 output parent 및 case·grammar root pathname 재개방은 위 handle 전달로 대체하지만, input 내부의 모든 동시 교체를 원자적으로 거부한다고 주장하지 않는다. 기존 보장 기준을 유지하며 이 경계는 미완료로 남긴다. `os.Root`는 bind mount 자체를 차단하지 않고, 열린 directory의 input 아래 이동·쓰기 뒤 이동·동일 UID 적대자나 mount 권한자를 격리하지 않는다. 이 한계를 넓은 동시 변경 안전성 주장으로 대체하지 않는다.

### S02 구현 — archive profile `zip-r1`과 이름 정책 `portable-names-r1`

`kit.Verify`/`tsgk verify --archive`는 ZIP을 풀지 않고 직접 읽는다. 지원 범위(`zip-r1`)는 단일 디스크, 압축 방식 0(stored)·8(deflate), data descriptor(general purpose bit 3)다. ZIP64, 다중 디스크, 암호화·strong encryption·masked header, 다른 압축 방식은 `UNSUPPORTED`(`ZIP64_UNSUPPORTED`, `ZIP_MULTIDISK_UNSUPPORTED`, `ZIP_ENCRYPTED_UNSUPPORTED`, `ZIP_METHOD_UNSUPPORTED`)로 끝나며 건너뛰지 않는다. tar·gzip 같은 다른 형식은 EOCD가 없어 `ZIP_EOCD_MISSING`이다.

구조 검사는 member 데이터를 읽기 전에 한다. EOCD는 comment 길이가 파일 끝과 정확히 맞아야 하고, central directory는 EOCD 바로 앞에서 끝나야 한다(`ZIP_STRUCTURE_INVALID`). entry 수는 entry를 할당하기 전에 `ArchiveLimits.Entries`와 비교한다. 각 local header는 central entry와 flags·method·이름 bytes가 같아야 하고, descriptor가 없으면 CRC·두 크기도 같아야 한다. descriptor가 있으면 local 값은 0이거나 같아야 하며 descriptor 값은 central과 같아야 한다(`ZIP_LOCAL_CENTRAL_MISMATCH`, `ZIP_DESCRIPTOR_MISMATCH`). local header·data·descriptor 영역이 서로 겹치거나 central directory와 겹치면 `ZIP_OVERLAP`이다. stored member는 두 크기가 같아야 한다.

member 이름은 원래 bytes를 진단용으로 보존하고 고쳐 쓰지 않는다. directory entry(이름 끝 `/`)는 데이터를 가질 수 없다(`ZIP_DIRECTORY_HAS_DATA`). external attribute 상위 16 bit의 Unix mode type은 entry가 주장하는 host와 관계없이 검사한다(Unix·OS X writer와 st_mode를 복사하는 도구가 쓰고 extractor가 따른다). symlink type이면 `ARCHIVE_LINK_REJECTED`, regular·directory가 아닌 type은 `ARCHIVE_SPECIAL_REJECTED`, type과 이름의 file/directory 종류가 다르면 `ZIP_ENTRY_TYPE_INCONSISTENT`다. MS-DOS/FAT·NTFS·VFAT host(0·10·14)의 속성은 reparse point면 `ARCHIVE_LINK_REJECTED`, directory 속성인데 이름이 `/`로 끝나지 않으면 `ZIP_ENTRY_TYPE_INCONSISTENT`다. 이름이 `/`로 끝나는데 directory 속성이 없는 entry(예: Java jar)는 허용한다. 같은 이름은 `ARCHIVE_DUPLICATE_MEMBER`, ASCII 대소문자만 다른 이름은 `ARCHIVE_CASE_COLLISION`, 한 entry가 file이고 다른 entry가 그 아래를 쓰는 경우(대소문자 무시)는 `ARCHIVE_FILE_DIRECTORY_CONFLICT`다.

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
