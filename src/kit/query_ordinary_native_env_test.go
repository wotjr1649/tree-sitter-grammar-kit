package kit

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryRunHost is the registry's platform ID and subject of the host this run executes on, for example
// windows-amd64 and windows/amd64. The ordinary registrations and plan rows of the native regressions bind it,
// so each host judges its own rows of the two-host denominator (#427).
func ordinaryRunHost() (platform, subject string) {
	return runtime.GOOS + "-" + runtime.GOARCH, runtime.GOOS + "/" + runtime.GOARCH
}

// ordinaryHostPlan is the one rule the ordinary native regressions share (the queryHostPlan rule of the predicate and SVC
// tests, #362): a host the registry does not judge skips, and a registered host other than windows-amd64 runs them
// only while the opt-in collection (QueryScopeOrdinaryResultsEnv) asks for their results, so the required foundation
// run on Linux does not gain their time or their failures. Windows ran them before and still does.
func ordinaryHostPlan(platform string, collecting bool) (skip string) {
	registered := slices.ContainsFunc(queryregistrycore.Platforms(), func(p queryregistrycore.Platform) bool { return p.ID == platform })
	switch {
	case !registered:
		return "the registry judges windows-amd64 and linux-amd64 only; " + platform + " is not registered"
	case platform != "windows-amd64" && !collecting:
		return "on " + platform + " the ordinary owned-native regressions run only when " + QueryScopeOrdinaryResultsEnv + " collects their results"
	}
	return ""
}

func ordinaryHostPlatform() string { p, _ := ordinaryRunHost(); return p }

func ordinaryHostSubject() string { _, s := ordinaryRunHost(); return s }

// The ordinary owned-native regressions run the real native producer on the tools a preparation step hands in:
// TSGK_NATIVE_RUNTIME (the pinned runtime source root) and TSGK_NATIVE_CC (the C compiler), used exactly as
// given. Their identity is not decided here by comparing paths. The producer checks every runtime file against
// the embedded runtime manifest (internal/native/build.go: copyVerified, RUNTIME_MISMATCH) and the compiler
// against the identity the profile pins (internal/native/build.go: TOOL_IDENTITY_MISMATCH), and a grammar file
// against its registered pin (SOURCE_MISMATCH).
//
// Like the other native suites, a missing tool skips, except under TSGK_NATIVE_REQUIRED=1 (CI), where it fails.
// The ordinary registrations of these regressions bind this host's platform and subject (ordinaryRunHost) and the
// producer refuses a registration whose subject is not its host (internal/native/oracle_ordinary.go), so a host
// the registry does not judge skips (#427).
func ordinaryNativeTools(t *testing.T) (runtimeRoot, compiler string) {
	t.Helper()
	platform, _ := ordinaryRunHost()
	if skip := ordinaryHostPlan(platform, os.Getenv(QueryScopeOrdinaryResultsEnv) != ""); skip != "" {
		t.Skip(skip)
	}
	runtimeRoot, compiler = os.Getenv("TSGK_NATIVE_RUNTIME"), os.Getenv("TSGK_NATIVE_CC")
	if runtimeRoot == "" || compiler == "" {
		if os.Getenv("TSGK_NATIVE_REQUIRED") == "1" {
			t.Fatal("TSGK_NATIVE_REQUIRED=1 but TSGK_NATIVE_RUNTIME/TSGK_NATIVE_CC are not set")
		}
		t.Skip("native tools not configured (TSGK_NATIVE_RUNTIME, TSGK_NATIVE_CC)")
	}
	return runtimeRoot, compiler
}

// ordinaryPreparedRoutes returns the prepared grammar routes directory (one subdirectory per route, for
// example postgresql-sql and tsql) that the opt-in SQL runs need: TSGK_ORDINARY_GRAMMAR_ROOT. The large SQL
// grammars cannot be tracked, so an opt-in run without it fails with this reason.
func ordinaryPreparedRoutes(t *testing.T) string {
	t.Helper()
	root := os.Getenv("TSGK_ORDINARY_GRAMMAR_ROOT")
	if root == "" {
		t.Fatal("TSGK_ORDINARY_GRAMMAR_ROOT must name the prepared grammar routes directory (one subdirectory per route) for this opt-in run")
	}
	return root
}

// ordinaryPreparedRoute returns one route's prepared grammar root below TSGK_ORDINARY_GRAMMAR_ROOT.
func ordinaryPreparedRoute(t *testing.T, route string) string {
	t.Helper()
	return filepath.Join(ordinaryPreparedRoutes(t), route)
}

// ordinaryPreparedRepro returns the prepared reproduction outputs of a route: the sibling "repro" directory of
// the routes directory.
func ordinaryPreparedRepro(t *testing.T, route string) string {
	t.Helper()
	return filepath.Join(filepath.Dir(ordinaryPreparedRoutes(t)), "repro", route)
}
