package foundation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The route job's time budget for the gate-external collections (#427 P1). routesBaseMinutes is a ceiling chosen by
// hand, not measured by this test or checkable against the workflow: run 43391868784 ran the Windows native
// routes job in 20.1 min (its required step in 18.4 min), and 26 is that plus about 30 % for runner variance. If
// the required steps grow, nothing here notices: raise the constant by hand then. The collection steps and the two
// evidence uploads are bounded by their step limits, and a step timeout cannot be absorbed by the job limit, so
// base + both collection limits + both upload limits + slack must stay below it: 26 + 10 + 12 + 2 + 2 + 4 = 56 of 60.
const (
	routesBaseMinutes = 26
	routesBudgetSlack = 4
)

// routesBudgetFits reports whether the pinned base, the gate-external step limits and the slack fit the job limit.
func routesBudgetFits(job, scope, ordinary, uploads int) bool {
	return routesBaseMinutes+scope+ordinary+uploads+routesBudgetSlack <= job
}

const (
	scopeCollectName   = "      - name: scope 판정 근거 수집 (host별, gate 밖)\n"
	scopeUploadName    = "      - name: scope 판정 근거 보관 (host별, gate 밖)\n"
	ordinaryUploadName = "      - name: ordinary 결과와 원장 보관 (host별, gate 밖)\n"
)

// ordinaryGoTests are the seven go test commands of the ordinary collection, in order: the -run pattern, and the
// completion line a test must print (PASS or FAIL, never SKIP) for the run to count.
var ordinaryGoTests = [7]struct{ run, complete string }{
	{`-run '^(TestVerifyOrdinaryQuerySemanticsOwnedJSONPublicPassAndNegativeCases|TestOrdinaryFullPositiveOwnedJSONZeroEditMultiCase|TestOrdinaryFullPositiveOwnedJSONHistoryMultiCase)$' -count=1 -v -timeout=150s`, `-Pattern "^--- (PASS|FAIL): $test \("`},
	{`-run '^TestOrdinarySQLPublicPassCapacity$/^postgresql-sql$' -count=1 -v -timeout=75s`, `-Pattern '^    --- (PASS|FAIL): TestOrdinarySQLPublicPassCapacity/postgresql-sql \('`},
	{`-run '^TestOrdinaryPostgreSQLExpectedAgainstNative$' -count=1 -v -timeout=75s`, `-Pattern '^--- (PASS|FAIL): TestOrdinaryPostgreSQLExpectedAgainstNative \('`},
	{`-run '^TestOrdinarySQLPublicPassCapacity$/^tsql$' -count=1 -v -timeout=100s`, `-Pattern '^    --- (PASS|FAIL): TestOrdinarySQLPublicPassCapacity/tsql \('`},
	{`-run '^TestOrdinaryTSQLExpectedAgainstNative$' -count=1 -v -timeout=75s`, `-Pattern '^--- (PASS|FAIL): TestOrdinaryTSQLExpectedAgainstNative \('`},
	{`-run '^(TestOrdinarySourcesAreByteIdenticalOnBothHosts|TestOrdinaryScannerLibcExposure)$' -count=1 -v -timeout=45s`, `-Pattern "^--- (PASS|FAIL): $test \("`},
	{`-run '^TestOrdinaryScopeStore$' -count=1 -v -timeout=30s`, `-Pattern '^--- (PASS|FAIL): TestOrdinaryScopeStore \('`},
}

// ordinaryEvidencePolicy pins the gate-external collection of the selected ordinary shards (#427 P1), on the lines of
// queryScopeEvidencePolicy: no new job, no required step, no change of a limit. The collection is a step of its own
// after the scope collection, bounded by its step timeout, unable to fail the required producer, with a cgroup
// delegation of its own; it extends the host directory the scope collection stored and never removes anything in it;
// a failing go test is a result (FAILED tuples are stored before the verdict), while a test that did not run
// (a skip passes go test) ends the step with a warning and leaves what it did not judge BLOCKED.
func ordinaryEvidencePolicy(text string) error {
	routes := workflowJobBody(text, "native-routes")
	_, step, ok := strings.Cut(routes, ordinaryEvidenceCollectHead)
	if !ok {
		return fmt.Errorf("native-routes ordinary collection step missing, unbounded or not continuing on error")
	}
	step, _, _ = strings.Cut(step, "\n      - ")
	// the position inside native-routes (F3 of #435): the host directory is created by the scope collection, extended
	// by the ordinary collection, and only then carried by the scope upload; the ordinary upload follows its collection
	if scopeAt, ordinaryAt := strings.Index(routes, scopeCollectName), strings.Index(routes, ordinaryEvidenceCollectHead); scopeAt < 0 || scopeAt > ordinaryAt {
		return fmt.Errorf("the ordinary collection must follow the scope collection it extends")
	}
	if ordinaryAt, evidenceUploadAt, scopeUploadAt := strings.Index(routes, ordinaryEvidenceCollectHead), strings.Index(routes, ordinaryUploadName), strings.Index(routes, scopeUploadName); evidenceUploadAt < ordinaryAt || scopeUploadAt < 0 || ordinaryAt > scopeUploadAt {
		return fmt.Errorf("the ordinary collection must come before the scope upload that carries its file, and its own upload after it")
	}
	if rest := strings.Replace(routes, step, "", 1); strings.Contains(rest, "TSGK_ORDINARY_") || strings.Contains(rest, "TestOrdinaryScopeStore") {
		return fmt.Errorf("ordinary collection inside another step")
	}
	for _, required := range []string{
		`$scopeOut = Join-Path $env:RUNNER_TEMP "query-scope-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`,
		`$ordinaryOut = Join-Path $env:RUNNER_TEMP "ordinary-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`,
		// it extends what the scope collection stored, and does nothing when that left no host directory
		`if (!(Test-Path -LiteralPath (Join-Path $scopeOut 'run-identity.json') -PathType Leaf)) { throw 'the scope collection stored no host files to extend' }`,
		// its own cgroup delegation, as the scope collection has, under a base name of its own
		`$base = "/sys/fs/cgroup/tsgk-ordinary-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`,
		`sudo mkdir -p "$base/supervisor" "$base/jobs"`, "sudo chown -R \"$(id -u):$(id -g)\" $base",
		`sudo sh -c "echo $PID > '$base/supervisor/cgroup.procs'"`,
		`Set-Content -NoNewline -Path "$base/cgroup.subtree_control" -Value '+memory'`,
		`Set-Content -NoNewline -Path "$base/jobs/cgroup.subtree_control" -Value '+memory'`,
		`$cgroup = "$base/jobs"`,
		"throw 'cgroup create failed'", "throw 'cgroup delegate failed'", "throw 'cgroup move failed'",
		"sudo sh -c \"echo $PID > '$origin/cgroup.procs'\" 2>$null", `sudo rmdir "$base/jobs" "$base/supervisor" $base 2>$null`,
		// the inputs: the compiler of the route step, the prepared closures, the shared result directory
		"$env:TSGK_NATIVE_CC = $env:TSGK_SCOPE_CC", "$env:TSGK_CGROUP_PARENT = $cgroup\n", "$env:TSGK_NATIVE_REQUIRED = '1'",
		"$env:TSGK_ORDINARY_SQL_E2E = '1'", "$env:TSGK_ORDINARY_GRAMMAR_ROOT = Join-Path $prepared 'routes'",
		"$env:TSGK_ORDINARY_E2E_ARTIFACTS = Join-Path $ordinaryOut 'artifacts'", "$env:TSGK_ORDINARY_SCOPE_RESULTS = Join-Path $ordinaryOut 'results'",
		// a test that did not run is named, kept as a warning, and does not discard what the others stored
		"if ($notRun.Count -gt 0) { throw \"ordinary tests did not run: $($notRun -join ', ')\" }",
		// a go test that exits nonzero is visible: a warning naming the failed tests (F1 of #435), never a throw
		"function Write-OrdinaryFailure($log, $status) {", "if ($status -eq 0) { return }",
		`Select-String -LiteralPath $log -CaseSensitive -Pattern '^\s*--- FAIL: (\S+)'`, "$($failed -join ', ')",
		`Write-Output "::warning::ordinary go test exited ${status} (`, "a test that failed stores no result and its tuples stay BLOCKED",
		`Write-Output "::warning::ordinary scope collection failed: $_"`, "$global:LASTEXITCODE = 0\n",
		`$env:PLATFORM -replace '/', '-'`,
	} {
		if !strings.Contains(step, required) {
			return fmt.Errorf("native-routes ordinary collection missing: %s", required)
		}
	}
	// the host files of the scope collection are never removed here, and no other step's cgroup or base is reused
	for _, forbidden := range []string{"Remove-Item", "TSGK_SCOPE_CGROUP", "tsgk-scope-", "TSGK_BENCH_PREPARED", "throw \"ordinary test failed", "-Pattern '^'"} {
		if strings.Contains(step, forbidden) {
			return fmt.Errorf("the ordinary collection must not contain: %s", forbidden)
		}
	}
	// exactly the seven go test commands, each followed by its log and none turning a failing test into a collection
	// failure (the next line shows the log; a FAILED tuple is a result), each with its own pinned time limit
	goTests := regexp.MustCompile(`(?m)^\s*go test (\S+ )?(.*)> \$log 2>&1\n\s*Get-Content -LiteralPath \$log\n\s*Write-OrdinaryFailure \$log \$LASTEXITCODE\n`).FindAllStringSubmatch(step, -1)
	if all := regexp.MustCompile(`(?m)^\s*go test `).FindAllString(step, -1); len(all) != len(ordinaryGoTests) || len(goTests) != len(all) || strings.Count(step, "-timeout") != len(all) {
		return fmt.Errorf("the ordinary collection runs exactly %d go test commands, each logged, each followed by its failure warning and each with its own timeout", len(ordinaryGoTests))
	}
	seconds := 0
	for i, want := range ordinaryGoTests {
		if !strings.Contains(goTests[i][2], want.run) {
			return fmt.Errorf("ordinary go test %d is not %s", i+1, want.run)
		}
		if !strings.Contains(step, want.complete) {
			return fmt.Errorf("ordinary go test %d is not counted only when it ran: %s", i+1, want.complete)
		}
		n, _ := strconv.Atoi(regexp.MustCompile(`-timeout=([1-9][0-9]*)s`).FindStringSubmatch(goTests[i][2])[1])
		seconds += n
	}
	limit := regexp.MustCompile(`(?m)^        timeout-minutes: ([1-9][0-9]*)$`).FindStringSubmatch(ordinaryEvidenceCollectHead)
	stepMinutes, _ := strconv.Atoi(limit[1])
	if seconds+120 > stepMinutes*60 {
		return fmt.Errorf("ordinary go test timeouts add up to %d s, over the step limit %d min less 2 min", seconds, stepMinutes)
	}
	// the order: the process is in its supervisor cgroup before the first native child; the store is the last go test
	// and the only one that knows the scope directory, which is named right before it
	at := func(s string) int { return strings.Index(step, s) }
	goAt := regexp.MustCompile(`(?m)^\s*go test `).FindAllStringIndex(step, -1)
	first, last := goAt[0][0], goAt[len(goAt)-1][0]
	if move := at("$base/supervisor/cgroup.procs"); move < 0 || first < move {
		return fmt.Errorf("the ordinary collection must move itself into its supervisor cgroup before its first go test")
	}
	if strings.Count(step, "TSGK_QUERY_SCOPE_OUT") != 1 || at("$env:TSGK_QUERY_SCOPE_OUT = $scopeOut\n") < at("TestOrdinaryScannerLibcExposure") || at("$env:TSGK_QUERY_SCOPE_OUT = $scopeOut\n") > last || !strings.HasPrefix(strings.TrimLeft(step[last:], " \n"), "go test ./src/kit -run '^TestOrdinaryScopeStore$'") {
		return fmt.Errorf("the scope directory must be named right before the store, the last go test")
	}
	// the delegation state, as the scope collection initialises it: before the try, origin read before the move, the
	// cleanup behind its guard after the catch, outside the try, after the last go test
	if tryAt, init := at("          try {\n            if (!(Test-Path"), at("          $base = ''\n          $origin = ''\n"); tryAt < 0 || init < 0 || init > tryAt {
		return fmt.Errorf("the ordinary collection must initialise $base and $origin before its try")
	}
	if funcAt, tryAt := at("          function Write-OrdinaryFailure("), at("          try {\n"); funcAt < 0 || funcAt > tryAt {
		return fmt.Errorf("the failure warning must be defined before the first go test, outside the try")
	}
	if guardAt, linuxAt := at("            if (!(Test-Path"), at("            if ($IsLinux) {\n"); guardAt < 0 || linuxAt < guardAt {
		return fmt.Errorf("the host-file guard must come before the cgroup delegation")
	}
	originAt := at(`$origin = '/sys/fs/cgroup' + ((Get-Content -LiteralPath /proc/self/cgroup -TotalCount 1) -replace '^0::', '')`)
	if moveAt := at("$base/supervisor/cgroup.procs"); originAt < 0 || originAt > moveAt {
		return fmt.Errorf("the ordinary collection must read its origin cgroup before it moves into the supervisor")
	}
	catchAt, guardAt := at("\n          } catch {\n"), at("\n          if ($base -ne '') {\n")
	backAt, removeAt, exitAt := at("sudo sh -c \"echo $PID > '$origin/cgroup.procs'\" 2>$null"), at("sudo rmdir "), at("$global:LASTEXITCODE = 0\n")
	if catchAt < last || guardAt < catchAt || backAt < guardAt || removeAt < backAt || exitAt < removeAt {
		return fmt.Errorf("the ordinary cleanup must follow the catch block, guarded, after the last go test and before the exit code reset")
	}
	// the budget of the whole job: the base, both collection steps and the slack stay below the job limit
	// (read from the workflow, not from the pinned heads, so that a raised limit of any of these steps is seen here)
	stepLimit := func(name string) (int, error) {
		_, rest, found := strings.Cut(routes, name)
		m := regexp.MustCompile(`^(?:.*\n)??        timeout-minutes: ([1-9][0-9]*)\n`).FindStringSubmatch(strings.SplitN(rest, "\n      - ", 2)[0] + "\n")
		if !found || m == nil {
			return 0, fmt.Errorf("step %q or its limit missing", name)
		}
		return strconv.Atoi(m[1])
	}
	scope, err := stepLimit(scopeCollectName)
	if err != nil {
		return err
	}
	uploads := 0
	for _, name := range []string{scopeUploadName, ordinaryUploadName} {
		n, err := stepLimit(name)
		if err != nil {
			return err
		}
		uploads += n
	}
	jobLimit := regexp.MustCompile(`(?m)^    timeout-minutes: ([1-9][0-9]*)$`).FindStringSubmatch(routes)
	if jobLimit == nil {
		return fmt.Errorf("native-routes job limit missing")
	}
	if job, _ := strconv.Atoi(jobLimit[1]); !routesBudgetFits(job, scope, stepMinutes, uploads) {
		return fmt.Errorf("native-routes job limit %d min: base %d + scope collection %d + ordinary collection %d + uploads %d + slack %d", job, routesBaseMinutes, scope, stepMinutes, uploads, routesBudgetSlack)
	}
	// the upload: evidence apart from the closed host file set, so it must not be that directory, and it must not
	// fail the producer when there is nothing to upload
	_, upload, ok := strings.Cut(routes, ordinaryEvidenceUploadHead)
	if !ok {
		return fmt.Errorf("native-routes ordinary evidence upload missing, unbounded or not continuing on error")
	}
	upload, _, _ = strings.Cut(upload, "\n      - ")
	for _, required := range []string{"name: ordinary-${{ matrix.goos }}-${{ github.run_id }}-${{ github.run_attempt }}", "if-no-files-found: ignore\n", "ordinary-${{ github.run_id }}-${{ github.run_attempt }}/results"} {
		if !strings.Contains(upload, required) {
			return fmt.Errorf("native-routes ordinary evidence upload missing: %s", required)
		}
	}
	if strings.Contains(upload, "query-scope") {
		return fmt.Errorf("the ordinary upload must not carry the closed scope host file set")
	}
	// not in the foundation job (its Linux budget has no room), not in the gate
	if strings.Contains(workflowJobBody(text, "foundation"), "TSGK_ORDINARY_") || strings.Contains(workflowJobBody(text, "integration-gate"), "ordinary-") {
		return fmt.Errorf("the ordinary evidence entered the foundation job or the integration gate")
	}
	return nil
}

func TestWorkflowOrdinaryEvidenceIsGateExternal(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repository(t), ".github/workflows/foundation.yml")))
	if err := ordinaryEvidencePolicy(text); err != nil {
		t.Fatal(err)
	}
	_, body, _ := strings.Cut(text, ordinaryEvidenceCollectHead)
	body, _, _ = strings.Cut(body, "\n      - ")
	block := ordinaryEvidenceCollectHead + body
	_, up, _ := strings.Cut(text, ordinaryEvidenceUploadHead)
	up, _, _ = strings.Cut(up, "\n      - ")
	upload := ordinaryEvidenceUploadHead + up
	inBlock := func(old, replacement string) string {
		return strings.Replace(text, block, strings.Replace(block, old, replacement, 1), 1)
	}
	inUpload := func(old, replacement string) string {
		return strings.Replace(text, upload, strings.Replace(upload, old, replacement, 1), 1)
	}
	routesBody := workflowJobBody(text, "native-routes")
	originLine := "              $origin = '/sys/fs/cgroup' + ((Get-Content -LiteralPath /proc/self/cgroup -TotalCount 1) -replace '^0::', '')\n"
	moveLine := "              sudo sh -c \"echo $PID > '$base/supervisor/cgroup.procs'\"\n"
	moveCheck := "              if ($LASTEXITCODE -ne 0) { throw 'cgroup move failed' }\n"
	cleanup := "          if ($base -ne '') {\n            # best effort: leave the supervisor, then remove the delegated subtree; a leftover dies with the runner\n            sudo sh -c \"echo $PID > '$origin/cgroup.procs'\" 2>$null\n            sudo rmdir \"$base/jobs\" \"$base/supervisor\" $base 2>$null\n          }\n"
	firstGo := "            go test ./src/kit -run '^(TestVerifyOrdinaryQuerySemanticsOwnedJSONPublicPassAndNegativeCases|"
	// the failure warning of each go test (F1 of #435): after the log, before the completion check
	tsqlCheck := "            if (!(Select-String -LiteralPath $log -CaseSensitive -Quiet -Pattern '^    --- (PASS|FAIL): TestOrdinarySQLPublicPassCapacity/tsql"
	tsqlShowAndWarn := "            Get-Content -LiteralPath $log\n            Write-OrdinaryFailure $log $LASTEXITCODE\n" + tsqlCheck
	funcStart := strings.Index(block, "          function Write-OrdinaryFailure(")
	failureFunc := block[funcStart : funcStart+strings.Index(block[funcStart:], "\n          }\n")+len("\n          }\n")]
	// the position (F3 of #435): the step moved behind the upload that carries its file, behind the next uploads, or
	// exchanged with the step it extends or with its own upload, and its own upload moved in front of it
	without := strings.Replace(text, block, "", 1)
	stepEnd := func(s, name string) int {
		i := strings.Index(s, name) + len(name)
		return i + strings.Index(s[i:], "      - name: ")
	}
	place := func(s string, at int, step string) string { return s[:at] + step + "\n" + s[at:] }
	_, scopeBody, _ := strings.Cut(text, scopeEvidenceCollectHead)
	scopeBody, _, _ = strings.Cut(scopeBody, "\n      - ")
	scopeBlock := scopeEvidenceCollectHead + scopeBody
	swap := func(a, b string) string {
		s := strings.Replace(text, a, "\x00", 1)
		return strings.Replace(strings.Replace(s, b, a, 1), "\x00", b, 1)
	}
	for name, broken := range map[string]string{
		"after-scope-upload":         place(without, stepEnd(without, scopeUploadName), block),
		"after-route-upload":         place(without, stepEnd(without, "      - name: route 결과 보관\n"), block),
		"swap-with-ordinary-upload":  swap(block, upload),
		"swap-with-scope-collection": swap(block, scopeBlock),
		"upload-before-collection":   strings.Replace(strings.Replace(text, upload, "", 1), block, upload+"\n"+block, 1),
	} {
		t.Run("position-"+name, func(t *testing.T) {
			if err := ordinaryEvidencePolicy(broken); broken == text || err == nil {
				t.Fatal("ordinary step position control not rejected")
			} else {
				t.Logf("rejected: %v", err)
			}
		})
	}
	for name, broken := range map[string]string{
		// the step itself: present, bounded, continuing, after a success, after the scope collection
		"step-removed":     strings.Replace(text, block, "", 1),
		"unbounded":        inBlock("        timeout-minutes: 12\n", ""),
		"limit-grows":      strings.Replace(text, ordinaryEvidenceCollectHead, strings.Replace(ordinaryEvidenceCollectHead, "timeout-minutes: 12", "timeout-minutes: 55", 1), 1),
		"limit-back-to-25": strings.Replace(text, ordinaryEvidenceCollectHead, strings.Replace(ordinaryEvidenceCollectHead, "timeout-minutes: 12", "timeout-minutes: 25", 1), 1),
		// the job budget (run 43391868784): a raised limit of any gate-external step, or a lowered job limit, is seen
		"scope-limit-grows":        strings.Replace(text, scopeEvidenceCollectHead, strings.Replace(scopeEvidenceCollectHead, "timeout-minutes: 10", "timeout-minutes: 30", 1), 1),
		"scope-upload-limit-grows": strings.Replace(text, scopeEvidenceUploadHead, strings.Replace(scopeEvidenceUploadHead, "timeout-minutes: 2", "timeout-minutes: 25", 1), 1),
		"job-limit-lowered":        strings.Replace(text, routesBody, strings.Replace(routesBody, "    timeout-minutes: 60\n", "    timeout-minutes: 50\n", 1), 1),
		"fails-producer":           strings.Replace(text, ordinaryEvidenceCollectHead, strings.Replace(ordinaryEvidenceCollectHead, "        continue-on-error: true\n", "", 1), 1),
		"runs-after-failure":       strings.Replace(text, ordinaryEvidenceCollectHead, strings.Replace(ordinaryEvidenceCollectHead, "${{ success() }}", "${{ always() }}", 1), 1),
		"before-scope-step":        strings.Replace(strings.Replace(text, block, "", 1), "      - name: scope 판정 근거 수집 (host별, gate 밖)\n", block+"      - name: scope 판정 근거 수집 (host별, gate 밖)\n", 1),
		"throws-uncaught":          inBlock("Write-Output \"::warning::ordinary scope collection failed: $_\"", "throw $_"),
		"fails-job":                inBlock("          $global:LASTEXITCODE = 0\n", ""),
		"in-required-step":         strings.Replace(text, "          $env:TSGK_REUSE_ACCEPTED_OUT = $null\n", "          $env:TSGK_REUSE_ACCEPTED_OUT = $null\n          $env:TSGK_ORDINARY_SQL_E2E = '1'\n", 1),
		"in-foundation":            strings.Replace(text, "      - name: Verify clean validation output\n        timeout-minutes: 1\n        run: |\n          $treeState = git status --porcelain=v1 --untracked-files=all --ignored=matching\n          if ($LASTEXITCODE -ne 0) { throw 'CI worktree query failed' }\n          if ($treeState) { throw 'CI requires a clean worktree including untracked and ignored files' }\n\n  race:", "      - name: ordinary\n        timeout-minutes: 1\n        run: $env:TSGK_ORDINARY_SQL_E2E = '1'\n\n  race:", 1),
		"gate-names-ordinary":      strings.Replace(text, "\n  integration-gate:\n", "\n  integration-gate:\n    # ordinary-evidence\n", 1),
		// it extends the scope collection's host directory and never removes anything in it
		"identity-guard-dropped": inBlock("            if (!(Test-Path -LiteralPath (Join-Path $scopeOut 'run-identity.json') -PathType Leaf)) { throw 'the scope collection stored no host files to extend' }\n", ""),
		"guard-after-delegation": strings.Replace(inBlock("            if (!(Test-Path -LiteralPath (Join-Path $scopeOut 'run-identity.json') -PathType Leaf)) { throw 'the scope collection stored no host files to extend' }\n", ""), "            $prepared = Join-Path", "            if (!(Test-Path -LiteralPath (Join-Path $scopeOut 'run-identity.json') -PathType Leaf)) { throw 'the scope collection stored no host files to extend' }\n            $prepared = Join-Path", 1),
		"removes-scope-files":    inBlock("            Write-Output \"::warning::ordinary scope collection failed: $_\"", "            Write-Output \"::warning::ordinary scope collection failed: $_\"\n            Remove-Item -LiteralPath $scopeOut -Recurse -Force"),
		// the cgroup delegation of its own (#362 N1), under a base name of its own
		"shares-scope-base":         inBlock(`$base = "/sys/fs/cgroup/tsgk-ordinary-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`, `$base = "/sys/fs/cgroup/tsgk-scope-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`),
		"shares-required-base":      inBlock(`$base = "/sys/fs/cgroup/tsgk-ordinary-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`, `$base = "/sys/fs/cgroup/tsgk-$env:GITHUB_RUN_ID-$env:GITHUB_RUN_ATTEMPT"`),
		"cgroup-from-env":           inBlock("            $env:TSGK_CGROUP_PARENT = $cgroup\n", "            $env:TSGK_CGROUP_PARENT = $env:TSGK_SCOPE_CGROUP\n"),
		"supervisor-move-dropped":   inBlock(moveLine+moveCheck, ""),
		"chown-dropped":             inBlock("              sudo chown -R \"$(id -u):$(id -g)\" $base\n              if ($LASTEXITCODE -ne 0) { throw 'cgroup delegate failed' }\n", ""),
		"memory-not-enabled":        inBlock("              Set-Content -NoNewline -Path \"$base/jobs/cgroup.subtree_control\" -Value '+memory'\n", ""),
		"cleanup-dropped":           inBlock("            sudo rmdir \"$base/jobs\" \"$base/supervisor\" $base 2>$null\n", ""),
		"origin-dropped":            inBlock(originLine, ""),
		"origin-read-after-move":    "", // two edits, below
		"islinux-guard-dropped":     inBlock("            if ($IsLinux) {\n", "            if ($true) {\n"),
		"cleanup-guard-dropped":     inBlock("          if ($base -ne '') {\n", "          if ($true) {\n"),
		"state-init-dropped":        inBlock("          $base = ''\n          $origin = ''\n", ""),
		"cleanup-before-go-test":    "", // two edits, below
		"compiler-not-handed":       inBlock("            $env:TSGK_NATIVE_CC = $env:TSGK_SCOPE_CC\n", ""),
		"native-not-required":       inBlock("            $env:TSGK_NATIVE_REQUIRED = '1'\n", ""),
		"prepared-closures-dropped": inBlock("            $env:TSGK_ORDINARY_GRAMMAR_ROOT = Join-Path $prepared 'routes'\n", ""),
		// the six go test commands, their limits and their completion patterns (a skip passes go test)
		"extra-go-test":            inBlock(firstGo, "            go test ./src/internal/native -count=1 -timeout=3000s > $log 2>&1\n            Get-Content -LiteralPath $log\n"+firstGo),
		"json-go-timeout-3000s":    inBlock("-count=1 -v -timeout=150s", "-count=1 -v -timeout=3000s"),
		"tsql-go-timeout-3000s":    inBlock("'^TestOrdinarySQLPublicPassCapacity$/^tsql$' -count=1 -v -timeout=100s", "'^TestOrdinarySQLPublicPassCapacity$/^tsql$' -count=1 -v -timeout=3000s"),
		"tamper-test-not-run":      inBlock("-run '^TestOrdinaryTSQLExpectedAgainstNative$'", "-run '^TestOrdinaryTSQLExpected$'"),
		"postgresql-not-run":       inBlock("^TestOrdinarySQLPublicPassCapacity$/^postgresql-sql$", "^TestOrdinarySQLPublicPassCapacity$/^pg$"),
		"json-single-case-dropped": inBlock("TestVerifyOrdinaryQuerySemanticsOwnedJSONPublicPassAndNegativeCases|", ""),
		"store-not-run":            inBlock("-run '^TestOrdinaryScopeStore$'", "-run '^TestOrdinaryScope$'"),
		"skip-passes-store":        inBlock("-Pattern '^--- (PASS|FAIL): TestOrdinaryScopeStore \\('", "-Pattern '^'"),
		"fail-dropped-tsql":        inBlock("'^    --- (PASS|FAIL): TestOrdinarySQLPublicPassCapacity/tsql \\('", "'^    --- PASS: TestOrdinarySQLPublicPassCapacity/tsql \\('"),
		"skip-passes-json":         inBlock("-Pattern \"^--- (PASS|FAIL): $test \\(\")) { $notRun += $test }\n            }\n            $log = Join-Path $ordinaryOut 'postgresql.log'", "-Pattern '^')) { $notRun += $test }\n            }\n            $log = Join-Path $ordinaryOut 'postgresql.log'"),
		"unlogged-go-test":         inBlock(tsqlShowAndWarn, tsqlCheck),
		"exit-code-throws":         inBlock(tsqlShowAndWarn, "            if ($LASTEXITCODE -ne 0) { throw \"ordinary test failed ($LASTEXITCODE)\" }\n"+tsqlShowAndWarn),
		// a nonzero go test is visible (F1 of #435): each test names itself, the warning keeps its content, a throw is not it
		"failure-warning-dropped-tsql":  inBlock(tsqlShowAndWarn, "            Get-Content -LiteralPath $log\n"+tsqlCheck),
		"failure-warning-dropped-json":  inBlock("            Get-Content -LiteralPath $log\n            Write-OrdinaryFailure $log $LASTEXITCODE\n            foreach ($test in @('TestVerifyOrdinary", "            Get-Content -LiteralPath $log\n            foreach ($test in @('TestVerifyOrdinary"),
		"failure-warning-dropped-store": inBlock("            Write-OrdinaryFailure $log $LASTEXITCODE\n            if (!(Select-String -LiteralPath $log -CaseSensitive -Quiet -Pattern '^--- (PASS|FAIL): TestOrdinaryScopeStore", "            if (!(Select-String -LiteralPath $log -CaseSensitive -Quiet -Pattern '^--- (PASS|FAIL): TestOrdinaryScopeStore"),
		"failure-warning-silent":        inBlock("            if ($status -eq 0) { return }\n", "            return\n"),
		"failure-warning-always-silent": inBlock("            Write-Output \"::warning::ordinary go test exited ${status} (", "            Write-Verbose \"::warning::ordinary go test exited ${status} ("),
		"failure-tests-not-named":       inBlock("failed: $($failed -join ', '); ", ""),
		"failure-pattern-wrong":         inBlock("'^\\s*--- FAIL: (\\S+)'", "'^--- FAIL: (\\S+)'"),
		"failure-claim-not-stated":      inBlock("; a test that failed stores no result and its tuples stay BLOCKED", ""),
		"failure-function-in-try":       strings.Replace(inBlock(failureFunc, ""), "            $log = Join-Path $ordinaryOut 'postgresql.log'\n", strings.ReplaceAll(failureFunc, "          ", "            ")+"            $log = Join-Path $ordinaryOut 'postgresql.log'\n", 1),
		"not-run-ignored":               inBlock("            if ($notRun.Count -gt 0) { throw \"ordinary tests did not run: $($notRun -join ', ')\" }\n", ""),
		"scope-dir-global":              "", // two edits, below
		"scope-dir-not-named":           inBlock("            $env:TSGK_QUERY_SCOPE_OUT = $scopeOut\n", ""),
		"result-dir-dropped":            inBlock("            $env:TSGK_ORDINARY_SCOPE_RESULTS = Join-Path $ordinaryOut 'results'\n", ""),
		"file-check-dropped":            inBlock("            if (!(Test-Path -LiteralPath (Join-Path $scopeOut \"query-scope-ordinary-$($env:PLATFORM -replace '/', '-').json\"))) { throw 'ordinary scope file missing' }\n", ""),
		// the upload
		"upload-requires":       inUpload("if-no-files-found: ignore\n", "if-no-files-found: error\n"),
		"upload-fails-producer": strings.Replace(text, ordinaryEvidenceUploadHead, strings.Replace(ordinaryEvidenceUploadHead, "        continue-on-error: true\n", "", 1), 1),
		"upload-unbounded":      strings.Replace(text, ordinaryEvidenceUploadHead, strings.Replace(ordinaryEvidenceUploadHead, "        timeout-minutes: 2\n", "", 1), 1),
		"upload-removed":        strings.Replace(text, upload, "", 1),
		"upload-scope-dir":      inUpload("ordinary-${{ github.run_id }}-${{ github.run_attempt }}/results", "query-scope-${{ github.run_id }}-${{ github.run_attempt }}"),
	} {
		switch name { // controls that need an edit of two places
		case "origin-read-after-move":
			broken = strings.Replace(inBlock(originLine, ""), moveLine+moveCheck, moveLine+moveCheck+originLine, 1)
		case "cleanup-before-go-test":
			broken = strings.Replace(inBlock(cleanup, ""), "            $env:TSGK_NATIVE_REQUIRED = '1'\n", strings.ReplaceAll(cleanup, "          ", "            ")+"            $env:TSGK_NATIVE_REQUIRED = '1'\n", 1)
		case "scope-dir-global":
			broken = strings.Replace(inBlock("            $env:TSGK_QUERY_SCOPE_OUT = $scopeOut\n", ""), "            $env:TSGK_NATIVE_REQUIRED = '1'\n", "            $env:TSGK_NATIVE_REQUIRED = '1'\n            $env:TSGK_QUERY_SCOPE_OUT = $scopeOut\n", 1)
		}
		t.Run(name, func(t *testing.T) {
			if err := ordinaryEvidencePolicy(broken); broken == text || err == nil {
				t.Fatal("ordinary evidence control not rejected")
			} else {
				t.Logf("rejected: %v", err)
			}
		})
	}
}

// The pinned numbers: the measured base with its variance, and the arithmetic of the budget.
func TestRoutesBudgetPins(t *testing.T) {
	if routesBaseMinutes != 26 || routesBudgetSlack != 4 {
		t.Fatalf("pinned base %d and slack %d changed", routesBaseMinutes, routesBudgetSlack)
	}
	for _, c := range []struct {
		job, scope, ordinary, uploads int
		fits                          bool
	}{
		{60, 10, 12, 4, true},  // the workflow: 26 + 10 + 12 + 4 + 4 = 56
		{60, 10, 25, 4, false}, // the first draft's ordinary limit: 69
		{60, 10, 20, 4, false},
		{60, 30, 12, 4, false},
		{60, 10, 12, 4 + 20, false},
		{50, 10, 12, 4, false},
		{60, 10, 16, 4, true}, // exactly the job limit less nothing: 60
	} {
		if got := routesBudgetFits(c.job, c.scope, c.ordinary, c.uploads); got != c.fits {
			t.Errorf("job %d scope %d ordinary %d uploads %d: fits %v, want %v", c.job, c.scope, c.ordinary, c.uploads, got, c.fits)
		}
	}
}
