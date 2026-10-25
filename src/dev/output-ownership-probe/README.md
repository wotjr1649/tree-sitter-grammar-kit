# Linux 출력 소유권 API probe (#215)

이 개발 도구는 Linux API의 제한된 관측을 남긴다. **현재 제품의 출력 경로를 바꾸지 않으며 모든 결과의 `support_claim`은 `BLOCKED`다.** #215 해결·namespace snapshot·권한 격리·현재/다른 kernel의 일반 보장을 증명하지 않는다. macOS용 해법이나 Go `os.Root` 내부 descriptor의 Close 오류 관측도 제공하지 않는다.

기존 Go 1.27.2와 고정 `golang.org/x/sys`만 사용한다. native library·compiler·network·새 capability·설치가 필요 없다. 기존 parent 아래 새 `probe-215-*` directory 하나와 세 하위 실험 directory만 만들고 보존한다. 기존 directory를 결과로 수용하거나 기존 entry를 삭제하지 않는다. task 전용 parent에서 다른 process가 건드리지 않는 조건으로 실행한다. 실제 다른 UID 공격을 수행하는 시험은 아니다.

```text
CGO_ENABLED=0 go build -o <task-directory>/probe ./src/dev/output-ownership-probe
<task-directory>/probe -parent <existing-absolute-task-directory>
```

Linux 전용이다. shell에서 환경변수 지정 구문은 실행 shell에 맞게 사용한다. standalone process는 전체10초 watchdog을 가지며 만료 exit124는 관측 성공이 아니다. 출력·stderr를 파일로 받고 caller도 process timeout을 적용한다. watchdog은 process를 종료하고 남은 task entry를 보존한다. `go test`는 package timeout을 별도로 지정한다.

- `directory`: 새 parent inode에 `FAN_CREATE|FAN_ONDIR|FAN_EVENT_ON_CHILD`를 mark한다. fresh FID queue, self PID/name/parent·child FID, filesystem fsid·handle type/length/bytes를 대조한다. 첫 mkdir 뒤 descriptor를 얻는 창에 대한 일반 증명은 아직 아니다.
- `replacement`: 같은 process에서 child를 생성·rename하고 다른 child를 만들어 결정적으로 교체한다. 두 생성의 모호함 또는 identity 차이를 거부하는 좁은 대조다. 서로 다른 UID·병렬 process·모든 event 병합·FID 재사용 검증을 대신하지 않는다.
- `publication`: 열린 parent 상대 O_TMPFILE, 같은 file descriptor의 write/fsync, 열린 proc fd directory 상대 linkat와 두 번째 EEXIST, final bytes를 관측한다. procfs filesystem 종류 확인은 현재 process fd table에 대한 완전한 신뢰 증명이 아니므로 성공한 primitive 관측도 `UNVERIFIED`다. fallback이나 이름 기반 임시 cleanup을 사용하지 않는다.

FID buffer는128 bytes로 고정한다. queue는 event64개·총256 KiB·2초를 동시에 제한하며 nonblocking read에서 EAGAIN이면 종료한다. 기다리면서 새 예산을 부여하지 않는다. 지원 부재·권한 부족은 `UNSUPPORTED`, 누락·overflow·잘못된 event·예산 초과는 `UNVERIFIED`, 모호하거나 다른 생성은 `REJECTED`, 좁은 정상 대조는 `CANDIDATE_MATCH`다. 관측 또는 Close 오류는 `FAILED`다. JSON의 kernel/filesystem/privilege context가 다른 결과를 같은 근거로 합산하지 않는다.

Exit0는 도구가 실패 없이 관측을 기록했다는 뜻이다. `UNSUPPORTED`·`UNVERIFIED`도 exit0일 수 있으며 #215 또는 지원 판정의 PASS가 아니다. 잘못된 설정·실패는 nonzero다. CI의 `TestLinuxCapabilityProbe`는 JSON을 test log에 남기고 skip하지 않는다. 그 test의 PASS는 도구의 결과 분류 검사이며 API 가용성·소유권 해결 PASS가 아니다. 원래 runtime·CLI·qualification 필수 검사는 그대로 유지한다.

syscall 오류는 operation과 원 errno 정수(`OPERATION:errno=N`)를 함께 보존한다. fanotify mark의 zero-fsid `ENODEV`·subvolume `EXDEV`, FID 미지원·고정 buffer 초과, O_TMPFILE/procfs/linkat의 문서화된 지원·권한 제한을 호출 맥락별 `UNSUPPORTED`로 나눈다. 파일·메모리·저장공간 등의 resource 부족은 `UNVERIFIED`다. `EINVAL`은 잘못된 호출과 기능 부재를 구분할 수 없으므로 `FAILED`/nonzero로 보존하고 성공으로 숨기지 않는다. errno가 아닌 실제 zero-fsid 검사 실패는 별도 `ZERO_FSID` code다. 이 분류도 지원 claim을 만들지 않는다.
