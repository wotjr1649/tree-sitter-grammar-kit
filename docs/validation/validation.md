# Validation and merge contract

## Local foundation checks

Only Go 1.27.1, Git, and PowerShell 7 are required. Core checks do not require Node, Python, a C compiler, or WSL. Run from the repository root with PowerShell 7. Check each native command's exit status; do not use global `go env -w` settings.

```powershell
$env:CGO_ENABLED = '0'
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
# Acquire only the go.sum-pinned modules (golang.org/x/sys), then build and test offline.
$env:GOPROXY = 'https://proxy.golang.org'
$env:GOSUMDB = 'sum.golang.org'
go mod download
if ($LASTEXITCODE -ne 0) { throw 'module download failed' }
go mod verify
if ($LASTEXITCODE -ne 0) { throw 'module verify failed' }
$env:GOPROXY = 'off'
$env:GOFLAGS = '-mod=readonly'
$formatted = gofmt -l src
if ($LASTEXITCODE -ne 0 -or $formatted) { throw 'gofmt failed' }
go test ./src/... -count=1 -v -timeout 300s
if ($LASTEXITCODE -ne 0) { throw 'test failed' }
go vet ./src/...
if ($LASTEXITCODE -ne 0) { throw 'vet failed' }
go build ./src/...
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
git diff --check
if ($LASTEXITCODE -ne 0) { throw 'diff check failed' }
```

The dependency test executes `go list -deps -test -json ./src/...` with `CGO_ENABLED=0` and rejects CgoFiles/runtime/cgo in the selected dependency closure. This does not assert that dependency source is cgo-free under every configuration. A separate import AST check rejects C and go-treesitter imports in repository Go source regardless of build tags or OS selection. Zero successful packages/tests do not constitute a pass. Track go.sum when an actual external dependency is introduced. From S01 the CLI build is a separate gate: CI builds `./src/cmd/tsgk` into the runner temporary directory, and `go test` also builds the CLI and an external consumer module outside the checkout (`TestExternalConsumerAndCLI`) and checks that the product closure has no `os/exec`, `net` or `plugin` package (`TestOfflineClosure`). These tests invoke the Go toolchain as development checks; the product itself starts no process.

Filesystem and Git index checks are separate. Source-only checks validate the explicitly supplied root's required files, module, boundaries, canonical local links, AGENTS size/entry point, and ignore/attribute policy without invoking Git. The walk excludes `.git` and designated local paths; within the inspected tree, reject links and special entries before reading contents. These development checks assume a caller-owned, unchanged tree; they are not a sandbox against hostile concurrent replacement. Checkout checks require the root's explicit `.git` marker and do not discover a parent repository. Validate regular modes/stage, required files, and forbidden local paths in the index. Hosted CI must not depend on local prompts.

The Markdown validator checks ordinary `[label](relative/path)` links and image targets for existence. AGENTS must contain an inline link to `docs/README.md`; image references and links inside code spans/fences do not count as entry points. Session-related words are not required. Review anchors, escaping, and complex reference-style Markdown manually. Do not interpret malformed grammar fixtures as documentation. Negative controls must reject boundary, missing-document, AGENTS, ignore, and link violations. A fake parent Git marker verifies that source-only checks never invoke Git.

For explicitly assigned Session 00 campaign work, the local campaign manifest records exact paths and SHA-256 values for eight individual prompts, the master, the shared contract, and the original prompt, together with Issues, Milestones, and dependencies. Verify the original prompt's preservation hash, effective ignore with `git check-ignore`, index exclusion, and ordering locally. This is separate from CI foundation checks and is not required for ordinary development.

Core boundaries have concrete owners: CheckFiles enforces the src/root-module layout, source import restrictions, and ignore/attribute policy; CheckGit checks the index and effective ignores; TestCGOFreeDependencies checks the active dependency closure; the workflow fixes CGO_ENABLED=0 and asserts PowerShell 7. The local-path policy also excludes bin/, dist/, coverage/, go.work, and go.work.sum. AGENTS summarizes these boundaries; its keyword presence does not prove semantic correctness. Changes to the summary or owning policy require the semantic review described below.

## CI and evidence

Mandatory jobs are `foundation (windows-2025)`, `foundation (ubuntu-24.04)`, and `foundation (macos-15)`. Assert Go version/OS/arch and actual checkout SHA; require and record runner ImageOS/ImageVersion, without claiming a pinned image-version assertion. Tests, vet, build, policy, and CGO dependency checks must all succeed. Skipped, cancelled, or missing checks are not PASS. The checkout must match the event's github.sha. PR CI checks the synthetic merge of head and base, not a separate branch-head lane. Record PR head, synthetic checkout SHA, and actual merge commit separately; verify the actual commit after merge.

S04부터 foundation job은 네 가지를 더 한다. Linux에서는 시험 전에 위임 cgroup v2(`/sys/fs/cgroup/tsgk-<run>-<attempt>`, supervisor·jobs leaf)를 만들고 `TSGK_CGROUP_PARENT`로 넘겨 runner의 hard memory cap과 `cgroup.kill`을 실제로 시험한다. cgroup 없이 실행된 hosted Linux 시험은 capability 시험이 실패한다. 이어 Linux에서만 같은 위임 cgroup 아래에서 runner의 process tree 정리 시험(`TestTreeTermination`, `TestPipeHoldingDescendant`, `TestEscapedDescendant`, `TestEscapedQuietDescendant`, `TestFrameEscapedStdoutHolder`, `TestFrameCrashWithStdoutHolder`, `TestCgroupLiveUntilReleased`)을 `-count=20 -failfast`로 반복한다(step 4분, go 200초 상한). cgroup 해제 경쟁은 실제 cgroup v2 host에서만 드러나므로 한 번의 통과보다 반복 표본을 근거로 삼는다. 단언·grace·Verified 요구는 일반 시험과 같다. `src/dev/s04-reproduce/ci-owned.ps1`은 [재현 등록부](../../src/contracts/reproduction-routes.json)의 tree-sitter 0.27.0 release asset을 받아 digest가 맞을 때만 풀고, owned fixture `src/testdata/reproduce`를 JSON mode(`node` 없음)로 두 작업 공간에서 생성해 `expected.json`의 기준 출력과 대조한다. 모든 출력은 runner temp에 둔다. 이 검사는 실제 도구 실행과 세 OS 사이 생성물 동일성의 근거이며 26 route 재현의 세 OS 결과를 대신하지 않는다.

S05부터 foundation job은 `src/dev/s05-native/prepare-routes.ps1 -Routes none`으로 고정 runtime source([manifest](../../src/drivers/native-c/runtime-manifest.json)의 83개 파일 hash 대조)만 받고 `select-compiler.ps1`로 host에 이미 있는 C 컴파일러(Windows MinGW gcc 또는 LLVM clang, Linux `/usr/bin/gcc`, macOS `/usr/bin/clang`; 설치하지 않음)를 골라 경로·hash·version을 기록한 뒤 `TSGK_NATIVE_RUNTIME`·`TSGK_NATIVE_CC`·`TSGK_NATIVE_REQUIRED=1`로 owned native 시험(`src/internal/native`, `src/cmd/tsgk`의 `TestIncrementalCLI`)을 세 OS에서 실행한다. 도구가 없으면 로컬은 skip이고 CI는 실패다. foundation 뒤의 `native prepare (ubuntu-24.04)`는 [route 등록부](../../src/contracts/native-routes.json)의 26 route source를 고정 commit archive에서 받아 채택 6 route의 patch chain을 적용하고(adoption hash 대조) `tsgk reproduce`로 parser를 재생성해(등록 출력 6개 대조) 모든 파일을 hash로 확인한 입력을 artifact로 넘긴다. `native routes (<os>)` 세 job은 그 입력을 다시 hash로 확인하고 `run-routes.ps1`로 route마다 `tsgk incremental`(등록 사례 `src/testdata/native/routes`, `gaps`)과 합성 대용량 C# fixture([등록부](../../src/contracts/native-large-fixtures.json), `real-world-source-r2`)를 실행해 `summary.json`을 artifact로 남긴다. build 실패, 완료되지 않은 사례, incremental equality·route claim FAIL은 job 실패이고, 언어 기대값 FAIL은 grammar 처분 대상으로 기록만 한다. Linux job은 같은 owned native 시험을 AddressSanitizer/UBSan driver build로 한 번 더 실행한다. 대용량 fixture의 hosted 결과가 `real-world-source-r2` 값에 못 미치면 S05-A18 결과로 기록하며 값은 올리지 않는다.

S06부터 `native routes (<os>)`는 `run-routes.ps1 -Large -Oracle`로 실행한다. route마다 S05 `tsgk incremental`(r1) 뒤에 같은 등록 사례와 route의 query 사례(`src/testdata/native/queries`)를 `tsgk oracle record`(r2, `native-query`, API 관측 포함)로 기록한다. C#·T-SQL·PostgreSQL·XML은 [사실 query 세트](../../src/contracts/fact-query-pack.json)를 함께 쓰고, C#·T-SQL은 동적 SQL fixture를 등록 사실과 대조한다. 기록 set은 실행마다 `kit.VerifyOracleSet`으로 검증한다. set 검증 실패, 완료되지 않은 사례, incremental equality·route, query equality, route query 사례의 기대 capture stream(`query_expectations`), 선언 사실 재현(`fact_reproduction`), 동적 SQL claim의 FAIL이 job 실패다. 언어 기대값의 FAIL과, runtime node API가 cursor와 다른 API claim FAIL은 첫 차이를 담아 `summary.json`에 처분 대상으로 기록한다. 대용량 fixture는 windows-2025에서만 `real-world-source-r3`(8 GiB)와 `native-query-large`(선언 query로 S05 선언 사실 재현, 상한을 넘는 입력 하나는 typed `RESOURCE_LIMIT`)로 실행한다. Linux·macOS job은 그 행을 `NOT_APPLICABLE`로 기록한다(사용자 결정 `C1-REAL-WORLD-SOURCE-WINDOWS-R3`).

S08부터 `native routes (<os>)`는 route 실행 뒤 `src/dev/s08-qualify/run-identity.ps1`로 host 실행 identity(`tsgk-run-identity/r1`: 지정한 run 변수, `go env`, checkout만)를 쓰고, `summary.json`과 함께 실행 identity, S06 기록 set(`records/`), 그 workload profile(`profiles/s06-*.json`)을 artifact로 올린다. 이어 `qualification (ubuntu-24.04)` job(`needs: native-routes`, 취소되지 않았으면 route job이 실패해도 실행)이 같은 run·attempt의 세 artifact를 받아 `tsgk qualify --inventory src/contracts/qualification-c1.json --candidate <github.sha>`로 78칸과 추가 역할 행을 만들고 결과를 `s08-qualification-<run>-<attempt>` artifact로 남긴다. `src/dev/s08-qualify/gate.ps1`이 행렬을 출력하고 `mechanism_gate`(완결성, cohort·자격, 칸별 kit 축과 세 host 의미 비교, 실행된 추가 역할 행)가 PASS가 아니면 job을 실패시킨다. 필수 문법 요구의 FAIL과 사례가 없는 의무는 job 실패가 아니라 행렬의 결과이며 지원 claim을 `BLOCKED`로 둔다. job은 heavy job이 아니며(native 실행 없음) route job 세 개가 끝난 뒤에 돈다.

P05 개발 helper의 application discovery 회귀 검사는 각 host의 checkout 밖 task scratch에서 같은 이름의 두 경로를 만들고, 실제 `Get-Command` 결과에서 첫 번째 단일 경로를 선택하며 없는 tool을 거부하는지 검사한다. fixture process는 실행하지 않는다. 이 검사는 upstream acquisition/native 또는 실제 격리 preflight의 성공 근거가 아니다.

같은 self-check는 별도 acquisition/capture 승인 대상과 frozen process 판정의 반례도 검사한다. 빈/이전 승인, running/paused 상태 불일치, 다른 host PID, 추가 child, 위조된 command는 거부한다. 실제 Docker/tmpfs capability는 승인된 hosted preflight에서 별도로 확인한다.

CI rejects staged, unstaged, untracked, and ignored worktree material before and after validation using git status --porcelain=v1 --untracked-files=all --ignored=matching. It fails without deleting files; clean: false does not waive this guard. These snapshots detect residual contamination, not hostile code that mutates and restores a tree between observations. SHA identity alone is not proof of every executed byte. Use the hosted disposable runner and the existing trust boundary; do not treat these checks as a hostile-execution sandbox. Ordinary local development may have unrelated preserved work: bind local evidence and reviews to the exact scoped diff, including intended untracked files, rather than claiming an unchanged commit.

Bind receipts to repository, workflow path, run ID, attempt, event, base/head identity, checkout SHA, job/step status, and URL. Account for API pagination and use the current attempt of the run. Distinguish log retrieval failures from execution failures; missing required evidence means NOT_VERIFIED. Preserve failed runs and do not retry without changed evidence. Do not transfer another SHA's success to the current candidate. A completed PR template or same-name job from an unrelated workflow is not gate evidence; verify the referenced receipt and current candidate.

## Starting ordinary work and completing it locally

Ordinary work does not require session prompts, Milestones, or prior handoffs. Change work, including documentation fixes, requires an Issue and a dedicated task branch. Analysis-only and read-only reviews create neither unless separately requested. At start/resume, scope changes, and PR preparation, reconcile the current request with the Issue's goal, requirements, scope/exclusions, acceptance criteria, validation, dependencies, and this PR's subset. Reuse matching Issues and branches rather than create one for every request. Record scope changes without silently dropping requirements or treating Issue text as authority.

Verify the root, base identity/freshness, branch/PR state, worktree occupancy, and existing staged, unstaged, untracked, and relevant ignored work. Use the current folder when safe; choose an authorized separate worktree for isolation or parallel work. A new worktree must have its required local inputs and checks available before claiming readiness. Preserve user work and active environments; do not force occupied branches or clean/reset/stash unrelated state. After a prior PR merges and its required post-merge checks pass, start a separate task on a new branch from verified main.

Issue creation/updates and other remote effects require authority for the exact destination and effect from the current request or a valid standing authorization. If required Issue linkage is unverified, retain a sanitized local draft and continue analysis; do not claim implementation-ready. If only an update is pending, continue authorized independent local work within verified Issue scope and mark unmatched requirements pending. Local completion requires relevant checks, diff inspection, and reporting observed results, unrun checks, and remaining findings. Do not automatically add PR creation or merge when remote integration was not requested.

Code, product-contract, CI, validation-policy, and semantic development-instruction changes require independent context review before integration into main. Only meaning- and behavior-preserving typos or formatting may use author diff review and relevant automated checks instead. File extension or line count does not determine the exception. Mixed changes follow the higher-impact requirement. Give the reviewer scope, acceptance criteria, canonical contracts, base/head, diff, observed checks, unrun checks, and reference identity. Label a review STATIC_REVIEW when the reviewer did not execute checks.

## Integration into main and Git transaction

Use a task branch based on verified main, then validated work-unit commits, PR, impact-appropriate review, fixes/retests, final-head CI/rule checks, merge commit, and main post-merge CI. Link the tracking Issue and state the completed subset and remaining work; do not use whole-Issue closing semantics for partial work. Campaign-specific Milestones, session branches, and handoffs apply only to that campaign's work. A campaign requiring independent review for every change takes precedence over the ordinary typo exception.

Do not merge with open BLOCKER/MATERIAL findings: resolve, revalidate, and obtain review confirmation. Fix MINOR findings or record impact, rationale, and a tracking Issue. Link each finding to its location, evidence, fix commit, checks, and review. After three identical failures without new evidence, change the mechanism or report a blocker. A required formal approval must be an actual GitHub approval from a separate account; a same-account comment or context review cannot replace it. See [GitHub review rules](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/approving-a-pull-request-with-required-reviews).

Immediately before merge, refresh actual main/head, rulesets, classic protection, checks, conversations, and reviews. If main movement affects the candidate, integrate by merge and refresh checks/review. Request a merge commit with an expected-head-SHA condition. Do not use admin bypass, force-push, or history rewrite. Merge authority excludes branch deletion; separately authorized cleanup follows the conditions below. Recommended protection is PR required, all three OS checks required, resolved conversations, and no force-push. Do not change protection settings in this work.

If remote protection requires no approval/checks, merge authority and this contract's impact-based engineering review and three-OS gates still apply. Missing recommended protection neither grants authority nor removes gates. If actual rules require approval, remain BLOCKED until it exists.

After merge, verify the actual merge SHA and main CI. Recover failures through a separate fix PR and stop dependent work. Pre-merge reports must not claim a future final SHA. Final reports and local receipts own actual merge results. Campaign work additionally requires a handoff and may close only the corresponding Issue/Milestone within authorization.

## Post-merge workspace cleanup

After required post-merge checks pass, assess cleanup by default. Assessment is not deletion authority. Evaluate each local branch, remote branch, remote-tracking ref, and linked worktree separately. Preserve the main worktree, default/protected branches, and work used by another task or process. Local tracking-ref removal does not delete a remote branch. Never assume a missing or locked worktree is abandoned.

When the current request or valid standing authorization includes cleanup of identified task branches, complete eligible local and remote cleanup without asking again. Merge or skill installation alone is not that authorization. Retain only targets with unresolved authority, integration, preservation, use, or policy conditions, and report those conditions individually.

Verify current repository/path identity, PR head and merge result, local and remote tips, all worktree occupancy, and staged/unstaged/untracked/ignored files, including submodules and local-only settings. Check for commits added after merge and unique detached history. For merge commits, verify reachability from the intended base; for squash/rebase, inspect the actual integrated changes and preserve unique history rather than forcing branch deletion. Even after equivalence is established, a native deletion refusal means retain the branch. A clean status, upstream match, or merged PR alone is insufficient. Stop affected cleanup if ownership or active use cannot be established.

Inventory all material before selecting what to archive. Keep unrelated pre-existing or ownership-uncertain material in place and hold removal of its containing worktree. Every non-reproducible item must remain in place or be covered by verified restoration before source removal; only classified reproducible material with explicit disposal authority may be omitted. For completed task-owned work, preserve unique material in an authorized destination before removal. The default candidate is `artifacts/worktree-archive/` in the project's long-lived primary checkout, with a unique task/revision subdirectory. Resolve real paths, reject links/reparse redirects, verify effective Git exclusion, and ensure the destination is outside the removal target. If a suitable checkout or destination is absent, report that prerequisite. Do not automatically archive secrets or private settings; uncertain files remain in place. Reproducible caches may be discarded only when their role and deletion authority are established.

Record original root/revision, preserved refs/commits, file inventory, sizes/hashes, necessary metadata, and restoration steps. Preserve staged and unstaged distinctions when needed for resumption. A Git bundle covers selected Git history, not the complete working state. Check space before copying, do not overwrite an existing archive, and verify a restore into a separate bounded location before deleting the source. A mismatch, copy failure, insufficient space, unsupported file type, or incomplete inventory holds the affected cleanup. Source changes during preservation invalidate that verification.

Immediately before each mutation, recheck identities, tips, worktree contents, and active use. Local branch/worktree removal uses native non-forcing Git; refusal retains the target. For an authorized remote task branch, this contract permits deletion-only compare-and-swap using `git push --force-with-lease=refs/heads/<branch>:<verified-sha> <verified-remote> :refs/heads/<branch>`. Use a fully qualified ref and the explicit, nonempty SHA whose integration and ownership were verified; verify the push destination independently. The command may contain only the intended deletion refspecs, each with its own expected SHA. This narrowly permits conditional deletion, not non-fast-forward updates or history rewriting. Never use `--force`, `+` refspecs, an implicit/empty lease, `--mirror`, or hook/protection bypass. Default/protected or active branches remain excluded. A stale lease or server refusal holds that target: no unconditional-delete fallback or retry with an unreviewed new tip. If higher-priority rules prohibit this operation or its guard, retain the target. See [Git push semantics](https://git-scm.com/docs/git-push).

`TestConditionalRemoteDeletion` exercises matching-SHA deletion, stale-SHA rejection with changed-tip preservation, unrelated-ref preservation, and server deletion refusal in isolated local Git repositories. It does not authorize live deletion or emulate hosted branch protection. Do not use force, clean, reset, or manual directory deletion to bypass a refusal. If verified residual files must be removed first, that exact file removal needs authority and a fresh preservation check; leave uncertain files in place. Reconcile uncertain results and report partial completion without repeating successful deletions blindly.

Keep a local receipt of removed/retained targets, preserved material, checks, failures, and revisit conditions. Record archive size and a review date; age or disk pressure alone never authorizes deletion. Worktree counts are review signals, not hard limits or automatic deletion triggers. `git worktree prune` removes stale administrative entries, not existing workspaces; it is not a substitute for safe removal. Tag creation/push, Release, and package publication remain separate work. This contract defines future cleanup conditions, not permission to delete existing workspaces while editing the contract.

## Campaign 01 준비와 후속 단계

PREPARE는 [준비 보고서](../reports/campaign-01-2026-09-29-preparation.md)의 D1~D13 소유와 [26 route/feature 범위](language-feature-scope.md)를 채택한다. 공급 prompt와 역사적 S00/r2는 서로 다른 revision으로 보존한다. local hash/receipt는 권한이나 성공을 만들어내지 않으며 CI와 공개 API는 local prompt 없이 동작해야 한다.

아래 표가 공개 PREPARE acceptance의 소유자다. 로컬 prompt나 template은 이 정의를 대신하지 않는다. gate status는 관측 receipt에 기록하며 테스트가 ID나 행을 확인한 것만으로 PASS가 되지 않는다.

| ID | PASS에 필요한 근거 | 필수 반례/미충족 판정 |
|---|---|---|
| P01 | 정확한 root/remote/worktree/base와 live 권한, 시작 diff/보존 inventory | 다른 root 또는 관련 없는 기존 변경을 덮어쓰지 않음 |
| P02 | 공급 파일 집합과 역사적 원본의 recorded size/hash 대조 | 누락/변경 prompt를 자체 rehash로 승인하지 않음 |
| P03 | 준비 보고서의 D1~D13 owner와 Session acceptance 정합화 | 폐기한 S05 CLI-only/S06 별도 driver/S08 축소/API 비공개 계약이 활성 상태로 남지 않음 |
| P04 | 26 route의 version/dialect/mode와 완전한 feature inventory 명시 채택 | 빈 목록/global latest/미승인 하한/누락 mode/미완료 feature는 미충족 |
| P05 | 26 source/tool feasibility 행의 관측·unknown·gap과 primary 근거 | repo/README 존재만으로 full support를 주장하지 않음 |
| P06 | 정확한 준비 효과/유한 한도 승인과 선택 probe의 command/input/tool/result | version 조회를 build로 표시하거나 denied effect를 우회하지 않음 |
| P07 | 공개 API·root module·src 경계·ordinary 진입 계약과 source/link 검사 | tracked nested module 또는 ignored prompt에 의존하는 canonical 계약 금지 |
| P08 | 현재 후보의 영향별 foundation/format/test/vet/build/dependency 검사 | 과거 S00 CI 승계/zero tests/미실행을 PASS로 표시하지 않음 |
| P09 | 정확한 diff/head의 분리 context 의미 리뷰와 finding 처분 | open BLOCKER/MATERIAL, self-review 또는 same-account comment를 formal approval로 대체 불가 |
| P10 | 실제 PR의 base/head/synthetic checkout/run/attempt/job와 세 OS 필수 CI | wrong SHA/attempt, skipped/missing job은 NOT_VERIFIED |
| P11 | 허용된 actual merge commit/tree와 main post-merge 검증 | 예정 SHA 또는 PR CI만으로 actual merge 검증 완료 처리 불가 |
| P12 | Parent #1/Issue #3~#10/Milestone 2~9의 intended body/state readback | 동시 사용자 변경 보존, 부분 API 성공이면 pending 유지 |
| P13 | 실제 operational manifest/immutable receipt/승인 budget/hash/tracking의 일치 | template/null approval/변경된 prompt는 S01을 활성화하지 않음 |
| P14 | 한국어 handoff의 실제 판정·미실행·한도·정확한 재개 entry | PREPARE에서 S01 또는 release를 시작하지 않음 |

source 등록부의 개발 검사는 현재 후보 metadata의 필수 값·상태·portable path와 공개 26-route 표의 전수성을 확인한다. feature 개발 검사는 [disposition](language-feature-disposition.md)의 route/ID/처분/case 종류와 [source 위험](source-feature-feasibility.md)의 feature 참조를 확인하고, 미정 scope·없는 feature·재현하지 않은 실패 등급을 넣은 negative control을 거절한다. 문법 목록의 의미상 완전성·사용자 채택·실제 upstream bytes/closure·지원 성공은 독립 리뷰/실제 답변/primary 조사/native receipt가 검증하며, 정적 행 검사가 이를 대신하지 않는다.

필수 입력이 남은 계약 통합은 ADOPTED_PENDING_INPUTS, 필수 외부 격차는 BLOCKED_EXTERNAL, 미처리 finding은 HOLD_FOR_CORRECTION이다. 검증·리뷰가 완료되고 통합만 남으면 READY_FOR_INTEGRATION이다. PREPARATION_READY는 P01~P14와 필수 scope/효과/예산이 확인된 경우만 사용하고 S01 실행은 별도 live 지시를 요구한다. 준비 Issue는 전체 기준 충족 후에만 완료 처리하며 Parent/#3~#10/MS2~9는 PREPARE에서 닫지 않는다.

S01~S03은 공개 offline API/CLI equivalence와 직접 API guard, checkout 밖 별도 consumer module을 검사한다. S04의 한 runner, S05 최소 native producer, S06 동일 producer 확장, S07 등록 reducer를 영향별 회귀로 검증한다. S08 최종 78칸은 새 현재 후보의 실제 세 host native 근거가 필요하다. PR synthetic tree와 actual merge tree가 동일하면 관계를 기록하고 필수 post-merge 검증을 수행하며, 다르면 영향 검사를 다시 수행한다. source-export/versioned local module-proxy 소비도 S08에 확인한다.

시간/입력/출력/저장/지원되는 memory·process 한도와 campaign/session 소모량을 등록한다. local heavy는 1, campaign heavy CI는 겹치는 workflow 전체를 합쳐 최대 3이다. 현재 Foundation은 workflow 전체의 공통 concurrency group과 matrix max-parallel3으로 직렬 run을 보장한다. 이후 native workflow도 같은 공유 lane을 사용하거나 동등한 scheduler 근거를 갖춰야 한다. 무변경 재시도는 원인이 확인된 일시적 infrastructure 오류에 한해 1회이며 제품 수정은 새 후보·남은 예산으로 검증한다. null/0을 무제한으로 해석하거나 한도·golden·필수 범위를 자동 완화하지 않는다.

### Campaign 01 Session 진행 규칙 — 2026-10-03 PREPARE 보완

* kit 결함은 다음 Session 진입을 막는다. grammar 지원 gap은 기록·처분한 뒤 S08 판정으로 넘기며 S06/S07 진입을 막지 않는다. kit 결함은 kit 코드·계약이 입력을 잘못 읽거나 판정·guard·회수·결과를 틀리게 내는 경우이고, grammar gap은 kit가 충실히 보고한 결과에서 grammar가 유효한 source를 기대 구조로 파싱하지 못하는 경우다. generator 같은 도구 실패는 kit runner가 잘못 다루면 kit 결함이고, upstream 도구 자체의 실패는 gap처럼 기록·처분한다. 필수 mainstream gap은 S08의 지원 claim을 막는다.
* patch 사전 승인: 채택 6 route의 등록 실패 사례를 고치는 저장소 안 patch subject 수정과, 26 route 전부의 build 이식성 수정(compiler 옵션·플랫폼 shim·build 스크립트)은 미리 승인됐다. build 이식성의 platform shim은 kit 쪽 compat header·build 옵션·스크립트이며 비채택 route의 scanner.c 직접 수정은 포함하지 않는다. 조건은 실패 사례·원문·기대값 선기록, 분리 context 리뷰 BLOCKER/MATERIAL 0, 세 OS 필수 CI 통과, 기존 등록 사례 회귀 없음, native는 Session envelope·운영 상한 안, 결과의 tracking·registry 기록이다. upstream 변경, 기대값·comparator 완화, provider/candidate 교체, 상한 상향, tag/Release/package publication은 제외이며 비채택 20 route의 grammar 동작 patch는 별도 승인이 필요하다.
* campaign 예비분(wall 115200초, CI 720 job-분, download 1 GiB, 유료 0원)은 목표가 아니다. Session envelope 합계 밖의 추가분으로 본다(PREPARE 보완의 기록된 가정). Session이 envelope를 넘으면 사유와 소비를 ledger에 기록하고 차감한다. 소진되면 멈추고 사용자에게 묻는다.
* S04 runner는 Windows Job Object와 프로세스 제어에 `golang.org/x/sys`를 쓸 수 있다(2026-10-03 승인). 채택 시점 안정 버전을 고정하고 `go.sum` checksum을 확인하며 `CGO_ENABLED=0`을 유지한다. 취득은 S04 이후 x/sys를 요구하는 모든 CI run의 별도 step에서 `GOPROXY=https://proxy.golang.org`·`GOSUMDB=sum.golang.org`로 `go.sum`에 고정된 module만 내려받고, 이후 build·test는 `GOPROXY=off`와 `-mod=readonly`로 실행한다. 로컬 검증도 같은 순서를 따르며, S04는 위 로컬 명령 블록과 workflow를 같은 작업 단위에서 갱신한다.
* 사용자 PR #47은 S01 전에 사용자가 처리한다. 열려 있으면 entry에서 사용자의 처리나 명시적 보류를 확인한다.
* 비공개 corpus와 실사용 source 값은 [NET461 등록부](net461-workload.md)를 따른다.

## Session 00 gates — 보존된 완료 기준

All gates are required: G00-01 target/authority; 02 pinned references; 03 src/module/local boundaries; 04 AGENTS at most 60 nonblank lines; 05 design; 06 observed foundation/negative/CGO-free checks; 07 exact three-OS CI; 08 Issue/Milestone work program; 09 local prompts/hashes; 10 independent review with zero open BLOCKER/MATERIAL findings; 11 merge/post-merge; 12 handoff.

Use FOUNDATION_READY only when all are observed; READY_FOR_MERGE when only merge remains; BLOCKED_EXTERNAL when a mandatory external gate is unavailable; HOLD_FOR_CORRECTION for unresolved design, validation, or findings. Never declare all gates PASS in advance. The [workload matrix](workload-matrix.md) owns feature-specific gates.
