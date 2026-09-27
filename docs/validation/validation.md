# 검증과 merge 계약

## 로컬 foundation

Go 1.27.1, Git, PowerShell 7만 필요하다. 제품 core 검사에는 Node/Python/C compiler/WSL이 필요하지 않다. root에서 PowerShell 7로 실행한다. native command exit를 각각 확인하고 전역 `go env -w`를 사용하지 않는다.

```powershell
$env:CGO_ENABLED = '0'
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$formatted = gofmt -l src
if ($LASTEXITCODE -ne 0 -or $formatted) { throw 'gofmt failed' }
go test ./src/... -count=1 -v -timeout 120s
if ($LASTEXITCODE -ne 0) { throw 'test failed' }
go vet ./src/...
if ($LASTEXITCODE -ne 0) { throw 'vet failed' }
go build ./src/...
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
git diff --check
if ($LASTEXITCODE -ne 0) { throw 'diff check failed' }
```

foundation의 dependency test는 `CGO_ENABLED=0`에서 `go list -deps -test -json ./src/...`를 실제 실행하고 선택된 전체 closure의 CgoFiles/runtime/cgo를 거부한다. 이는 모든 설정에서 dependency source에 cgo가 없다는 주장이 아니다. 별도 import AST 검사는 build tag/OS 선택에 숨은 저장소 Go source의 C import와 go-treesitter import도 거부한다. 성공한 package/test가 0개인 결과를 통과시키지 않는다. go.sum은 실제 외부 dependency가 생길 때 추적한다. CLI build는 S01부터 별도 gate다.

filesystem 검사와 Git index 검사는 별도다. source-only는 명시한 root의 필수 문서·module·경계·canonical local 링크·AGENTS 길이/진입점·ignore/attribute 정책을 검사하고 Git을 호출하지 않는다. 내용을 읽기 전 link/special entry를 거부한다. 이 개발 검사는 caller가 소유한 변경 중이지 않은 tree를 대상으로 하며 적대적인 동시 파일 교체를 막는 sandbox는 아니다. checkout 검사는 root의 `.git` marker를 먼저 요구하며 상위 저장소 자동 발견을 허용하지 않는다. Git index에서 regular mode/stage, 필수 파일과 금지 local 경로를 검사한다. hosted CI에 local prompt가 있다고 가정하지 않는다.

개발용 link 검사는 canonical Markdown의 일반 `[label](relative/path)` 파일 링크를 대상으로 한다. anchor·복잡한 reference-style Markdown의 의미 검증은 리뷰에서 한다. 제품 grammar의 malformed fixture는 문서 입력으로 해석하지 않는다. 최소 negative 대조는 경계/누락 문서/AGENTS/ignore/link 위반을 실제 실패시키며 source-only 아래 가짜 Git marker로 Git 비호출을 검증한다.

local campaign manifest는 8개 개별 prompt·master·공통 계약·원본 prompt의 정확한 경로와 SHA-256, Issue/Milestone/의존성을 기록한다. 원본 prompt 보존 hash, `git check-ignore` 및 index 비추적, 순서 mapping을 로컬에서 확인한다. 이 검사는 CI foundation과 별개다.

## CI와 근거

필수 job은 `foundation (windows-2025)`, `foundation (ubuntu-24.04)`, `foundation (macos-15)`다. OS/arch assertion, Go version, runner image, 실제 checkout SHA, test/vet/build/정책/CGO dependency 검사가 모두 성공해야 한다. skip/cancel/missing은 PASS가 아니다. checkout은 event의 github.sha와 일치해야 한다. PR CI는 branch head 자체를 별도로 실행하는 lane이 아니라 그 head와 base의 synthetic merge를 검사한다. PR head와 synthetic checkout SHA, 실제 merge commit을 따로 기록하고 merge 후 실제 commit도 다시 검사한다.

receipt는 repo, workflow path, run ID, attempt, event, head SHA, checkout SHA, job/step status와 URL을 결속한다. API pagination을 확인하고 같은 run의 현재 attempt를 사용한다. 로그 실패는 실행 실패와 구분하되 필요한 근거가 없으면 해당 검증은 NOT_VERIFIED다. 실패 run을 보존하고 원인 변화 없는 재시도를 하지 않는다. 다른 SHA의 성공을 현재 후보에 옮기지 않는다.

## Review와 Git transaction

Issue/Milestone → 최신 main의 세션 branch → 검증한 work-unit commit → PR → 분리 context review → fix/retest → 최종 head CI/규칙 확인 → merge commit → main post-merge CI 순서다. review에는 scope/acceptance/canonical/base/head/diff/관측 검사/미실행/reference identity만 전달한다. 직접 실행하지 않은 reviewer는 STATIC_REVIEW로 기록한다.

BLOCKER/MATERIAL은 해결·재검증·리뷰 확인 전 merge하지 않는다. MINOR는 수정하거나 영향·이유·추적 Issue를 남긴다. finding별 위치/근거/수정 commit/검사/review를 연결한다. 세 번 같은 실패에 새 근거가 없으면 mechanism을 바꾸거나 blocker로 남긴다. 정식 required approval은 별도 계정의 실제 GitHub 승인이다. 같은 계정의 comment와 context review는 이를 대신하지 못한다. [GitHub review 규칙](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/approving-a-pull-request-with-required-reviews)

merge 직전 actual main/head/ruleset/classic protection/checks/conversation/review를 재조회한다. main 이동이 영향을 주면 merge로 통합하고 검증/review를 갱신한다. expected head SHA 조건으로 merge commit을 요청한다. admin bypass·force-push·history rewrite·원격 branch 삭제를 하지 않는다. 보호 권장값은 PR required, 필수 세 OS check, conversation 해결, force-push 금지이며 이번에 설정을 변경하지 않는다.

원격 required approval/check 보호 규칙이 없다면 명시적인 현재 세션 merge 권한과 이 문서의 engineering review/세 OS gate를 적용한다. 권장 보호 설정의 부재만으로 권한을 만들거나 gate를 생략하지 않는다. 실제 required approval 규칙이 있으면 해당 승인이 올 때까지 BLOCKED다.

post-merge에서 actual merge SHA와 main CI가 모두 확인된 뒤 S00 Issue/Milestone만 닫는다. 실패하면 별도 수정 PR로 복구하고 후속 세션은 중단한다. pre-merge 보고서는 미래 최종 SHA를 담지 않는다. 최종 공개 comment와 local receipt/handoff가 실제 merge 결과를 소유한다.

## Session 00 gate

G00-01 대상/권한, 02 고정 reference, 03 src/module/local 경계, 04 AGENTS ≤60 비공백 줄, 05 설계, 06 실제 foundation/negative/CGO-free, 07 세 OS exact CI, 08 Issue/Milestone 작업 프로그램, 09 local prompt/hash, 10 분리 review와 열린 BLOCKER/MATERIAL 0, 11 merge/post-merge, 12 handoff가 모두 필요하다.
모두 관측하면 FOUNDATION_READY, merge만 남으면 READY_FOR_MERGE, 외부 필수 gate 불가면 BLOCKED_EXTERNAL, 설계/검증/finding 미해결이면 HOLD_FOR_CORRECTION이다. 전체 PASS를 미리 선언하지 않는다. 상세 기능별 gate는 [workload](workload-matrix.md)가 소유한다.
