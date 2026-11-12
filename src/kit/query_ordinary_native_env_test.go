package kit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The ordinary owned-native regressions run the real native producer on the tools a preparation step hands in:
// TSGK_NATIVE_RUNTIME (the pinned runtime source root) and TSGK_NATIVE_CC (the C compiler), used exactly as
// given. Their identity is not decided here by comparing paths. The producer checks every runtime file against
// the embedded runtime manifest (internal/native/build.go: copyVerified, RUNTIME_MISMATCH) and the compiler
// against the identity the profile pins (internal/native/build.go: TOOL_IDENTITY_MISMATCH), and a grammar file
// against its registered pin (SOURCE_MISMATCH).
//
// Like the other native suites, a missing tool skips, except under TSGK_NATIVE_REQUIRED=1 (CI), where it fails.
// The ordinary registrations of these regressions bind the subject windows/amd64 and the producer refuses a
// registration whose subject is not its host (internal/native/oracle_ordinary.go), so other hosts skip.
func ordinaryNativeTools(t *testing.T) (runtimeRoot, compiler string) {
	t.Helper()
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skipf("the ordinary owned-native regressions register the windows/amd64 subject; this host is %s/%s", runtime.GOOS, runtime.GOARCH)
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
