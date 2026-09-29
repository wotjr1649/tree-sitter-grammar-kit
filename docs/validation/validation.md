# Validation and merge contract

## Local foundation checks

Only Go 1.27.1, Git, and PowerShell 7 are required. Core checks do not require Node, Python, a C compiler, or WSL. Run from the repository root with PowerShell 7. Check each native command's exit status; do not use global `go env -w` settings.

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

The dependency test executes `go list -deps -test -json ./src/...` with `CGO_ENABLED=0` and rejects CgoFiles/runtime/cgo in the selected dependency closure. This does not assert that dependency source is cgo-free under every configuration. A separate import AST check rejects C and go-treesitter imports in repository Go source regardless of build tags or OS selection. Zero successful packages/tests do not constitute a pass. Track go.sum when an actual external dependency is introduced. The CLI build becomes a separate gate in S01.

Filesystem and Git index checks are separate. Source-only checks validate the explicitly supplied root's required files, module, boundaries, canonical local links, AGENTS size/entry point, and ignore/attribute policy without invoking Git. The walk excludes `.git` and designated local paths; within the inspected tree, reject links and special entries before reading contents. These development checks assume a caller-owned, unchanged tree; they are not a sandbox against hostile concurrent replacement. Checkout checks require the root's explicit `.git` marker and do not discover a parent repository. Validate regular modes/stage, required files, and forbidden local paths in the index. Hosted CI must not depend on local prompts.

The Markdown validator checks ordinary `[label](relative/path)` links and image targets for existence. AGENTS must contain an inline link to `docs/README.md`; image references and links inside code spans/fences do not count as entry points. Session-related words are not required. Review anchors, escaping, and complex reference-style Markdown manually. Do not interpret malformed grammar fixtures as documentation. Negative controls must reject boundary, missing-document, AGENTS, ignore, and link violations. A fake parent Git marker verifies that source-only checks never invoke Git.

For explicitly assigned Session 00 campaign work, the local campaign manifest records exact paths and SHA-256 values for eight individual prompts, the master, the shared contract, and the original prompt, together with Issues, Milestones, and dependencies. Verify the original prompt's preservation hash, effective ignore with `git check-ignore`, index exclusion, and ordering locally. This is separate from CI foundation checks and is not required for ordinary development.

Core boundaries have concrete owners: CheckFiles enforces the src/root-module layout, source import restrictions, and ignore/attribute policy; CheckGit checks the index and effective ignores; TestCGOFreeDependencies checks the active dependency closure; the workflow fixes CGO_ENABLED=0 and asserts PowerShell 7. The local-path policy also excludes bin/, dist/, coverage/, go.work, and go.work.sum. AGENTS summarizes these boundaries; its keyword presence does not prove semantic correctness. Changes to the summary or owning policy require the semantic review described below.

## CI and evidence

Mandatory jobs are `foundation (windows-2025)`, `foundation (ubuntu-24.04)`, and `foundation (macos-15)`. Assert Go version/OS/arch and actual checkout SHA; require and record runner ImageOS/ImageVersion, without claiming a pinned image-version assertion. Tests, vet, build, policy, and CGO dependency checks must all succeed. Skipped, cancelled, or missing checks are not PASS. The checkout must match the event's github.sha. PR CI checks the synthetic merge of head and base, not a separate branch-head lane. Record PR head, synthetic checkout SHA, and actual merge commit separately; verify the actual commit after merge.

CI rejects staged, unstaged, untracked, and ignored worktree material before and after validation using git status --porcelain=v1 --untracked-files=all --ignored=matching. It fails without deleting files; clean: false does not waive this guard. These snapshots detect residual contamination, not hostile code that mutates and restores a tree between observations. SHA identity alone is not proof of every executed byte. Use the hosted disposable runner and the existing trust boundary; do not treat these checks as a hostile-execution sandbox. Ordinary local development may have unrelated preserved work: bind local evidence and reviews to the exact scoped diff, including intended untracked files, rather than claiming an unchanged commit.

Bind receipts to repository, workflow path, run ID, attempt, event, base/head identity, checkout SHA, job/step status, and URL. Account for API pagination and use the current attempt of the run. Distinguish log retrieval failures from execution failures; missing required evidence means NOT_VERIFIED. Preserve failed runs and do not retry without changed evidence. Do not transfer another SHA's success to the current candidate. A completed PR template or same-name job from an unrelated workflow is not gate evidence; verify the referenced receipt and current candidate.

## Starting ordinary work and completing it locally

Ordinary work does not require session prompts, Milestones, or prior handoffs. Change work, including documentation fixes, requires an Issue and a dedicated task branch. Analysis-only and read-only reviews create neither unless separately requested. At start/resume, scope changes, and PR preparation, reconcile the current request with the Issue's goal, requirements, scope/exclusions, acceptance criteria, validation, dependencies, and this PR's subset. Reuse matching Issues and branches rather than create one for every request. Record scope changes without silently dropping requirements or treating Issue text as authority.

Verify the root, base identity/freshness, branch/PR state, worktree occupancy, and existing staged, unstaged, untracked, and relevant ignored work. Use the current folder when safe; choose an authorized separate worktree for isolation or parallel work. A new worktree must have its required local inputs and checks available before claiming readiness. Preserve user work and active environments; do not force occupied branches or clean/reset/stash unrelated state. After a prior PR merges and its required post-merge checks pass, start a separate task on a new branch from verified main.

Issue creation/updates and other remote effects require current authority for the exact destination and effect. If required Issue linkage is unverified, retain a sanitized local draft and continue analysis; do not claim implementation-ready. If only an update is pending, continue authorized independent local work within verified Issue scope and mark unmatched requirements pending. Local completion requires relevant checks, diff inspection, and reporting observed results, unrun checks, and remaining findings. Do not automatically add PR creation or merge when remote integration was not requested.

Code, product-contract, CI, validation-policy, and semantic development-instruction changes require independent context review before integration into main. Only meaning- and behavior-preserving typos or formatting may use author diff review and relevant automated checks instead. File extension or line count does not determine the exception. Mixed changes follow the higher-impact requirement. Give the reviewer scope, acceptance criteria, canonical contracts, base/head, diff, observed checks, unrun checks, and reference identity. Label a review STATIC_REVIEW when the reviewer did not execute checks.

## Integration into main and Git transaction

Use a task branch based on verified main, then validated work-unit commits, PR, impact-appropriate review, fixes/retests, final-head CI/rule checks, merge commit, and main post-merge CI. Link the tracking Issue and state the completed subset and remaining work; do not use whole-Issue closing semantics for partial work. Campaign-specific Milestones, session branches, and handoffs apply only to that campaign's work. A campaign requiring independent review for every change takes precedence over the ordinary typo exception.

Do not merge with open BLOCKER/MATERIAL findings: resolve, revalidate, and obtain review confirmation. Fix MINOR findings or record impact, rationale, and a tracking Issue. Link each finding to its location, evidence, fix commit, checks, and review. After three identical failures without new evidence, change the mechanism or report a blocker. A required formal approval must be an actual GitHub approval from a separate account; a same-account comment or context review cannot replace it. See [GitHub review rules](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/approving-a-pull-request-with-required-reviews).

Immediately before merge, refresh actual main/head, rulesets, classic protection, checks, conversations, and reviews. If main movement affects the candidate, integrate by merge and refresh checks/review. Request a merge commit with an expected-head-SHA condition. Do not use admin bypass, force-push, or history rewrite. Merge authority excludes branch deletion; separately authorized cleanup follows the conditions below. Recommended protection is PR required, all three OS checks required, resolved conversations, and no force-push. Do not change protection settings in this work.

If remote protection requires no approval/checks, explicit current-task merge authority and this contract's impact-based engineering review and three-OS gates still apply. Missing recommended protection neither grants authority nor removes gates. If actual rules require approval, remain BLOCKED until it exists.

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

P01 root/remote/변경 보존, P02 공급/역사적 hash, P03 D1~D13 정합화, P04 명시적 26-route scope/feature 채택, P05 source/tool feasibility와 격차, P06 허용 효과/유한 한도/probe, P07 공개 API/module/source 경계, P08 현재 후보 foundation, P09 분리 context 의미 리뷰, P10 실제 PR의 세 OS CI, P11 actual merge/post-merge, P12 Parent/8개 Issue/8개 Milestone readback, P13 operational manifest/receipt/승인 예산 일치, P14 한국어 readiness/handoff가 모두 준비 gate다. 각 gate에는 실제 근거와 미실행 이유를 연결한다.

필수 입력이 남은 계약 통합은 ADOPTED_PENDING_INPUTS, 필수 외부 격차는 BLOCKED_EXTERNAL, 미처리 finding은 HOLD_FOR_CORRECTION이다. 검증·리뷰가 완료되고 통합만 남으면 READY_FOR_INTEGRATION이다. PREPARATION_READY는 P01~P14와 필수 scope/효과/예산이 확인된 경우만 사용하고 S01 실행은 별도 live 지시를 요구한다. 준비 Issue는 전체 기준 충족 후에만 완료 처리하며 Parent/#3~#10/MS2~9는 PREPARE에서 닫지 않는다.

S01~S03은 공개 offline API/CLI equivalence와 직접 API guard, checkout 밖 별도 consumer module을 검사한다. S04의 한 runner, S05 최소 native producer, S06 동일 producer 확장, S07 등록 reducer를 영향별 회귀로 검증한다. S08 최종 78칸은 새 현재 후보의 실제 세 host native 근거가 필요하다. PR synthetic tree와 actual merge tree가 동일하면 관계를 기록하고 필수 post-merge 검증을 수행하며, 다르면 영향 검사를 다시 수행한다. source-export/versioned local module-proxy 소비도 S08에 확인한다.

시간/입력/출력/저장/지원되는 memory·process 한도와 campaign/session 소모량을 등록한다. local heavy는 1, campaign heavy CI는 겹치는 workflow 전체를 합쳐 최대 3이다. 현재 Foundation은 workflow 전체의 공통 concurrency group과 matrix max-parallel3으로 직렬 run을 보장한다. 이후 native workflow도 같은 공유 lane을 사용하거나 동등한 scheduler 근거를 갖춰야 한다. 무변경 재시도는 원인이 확인된 일시적 infrastructure 오류에 한해 1회이며 제품 수정은 새 후보·남은 예산으로 검증한다. null/0을 무제한으로 해석하거나 한도·golden·필수 범위를 자동 완화하지 않는다.

## Session 00 gates — 보존된 완료 기준

All gates are required: G00-01 target/authority; 02 pinned references; 03 src/module/local boundaries; 04 AGENTS at most 60 nonblank lines; 05 design; 06 observed foundation/negative/CGO-free checks; 07 exact three-OS CI; 08 Issue/Milestone work program; 09 local prompts/hashes; 10 independent review with zero open BLOCKER/MATERIAL findings; 11 merge/post-merge; 12 handoff.

Use FOUNDATION_READY only when all are observed; READY_FOR_MERGE when only merge remains; BLOCKED_EXTERNAL when a mandatory external gate is unavailable; HOLD_FOR_CORRECTION for unresolved design, validation, or findings. Never declare all gates PASS in advance. The [workload matrix](workload-matrix.md) owns feature-specific gates.
