package native

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// S06-A08: a missing or wrong scanner, a changed header, a grammar symbol the closure does
// not define and an ABI the runtime does not accept all fail before any tree, query or API
// observation is interpreted; nothing falls through to another grammar or runtime.
func TestClosureRejections(t *testing.T) {
	// independent builds in their own directories: run after the sequential (timing) tests
	t.Parallel()
	rt, cc := nativeTools(t)
	sum, n := digestOf(t, cc)
	compiler := kit.ToolIdentity{Name: "cc", Version: "test", SHA256: sum, Bytes: n}
	work := t.TempDir()
	build := func(root string, g []kit.NativeInput, symbol string) (*Build, error) {
		r := testBuildRequest(work, rt, root, g, cc, compiler)
		r.Symbol = symbol
		// each build has its own deadline: the parallel subtests outlive this function
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return NewBuild(ctx, r)
	}
	withRole := func(g []kit.NativeInput, role string, keep bool) []kit.NativeInput {
		var out []kit.NativeInput
		for _, f := range g {
			if (f.Role == role) == keep {
				out = append(out, f)
			}
		}
		return out
	}
	t.Run("missing-scanner", func(t *testing.T) {
		t.Parallel()
		g := withRole(fixtureGrammar(t, "stateful"), "scanner", false)
		if b, err := build(fixtureRoot(t, "stateful"), g, "tree_sitter_tsgk_stateful"); codeOf(err) != "BUILD_FAILED" || b.Steps[len(b.Steps)-1].Name != "link" {
			t.Fatalf("parser without its scanner: %v", err)
		}
	})
	t.Run("wrong-scanner", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		copyTree(t, fixtureRoot(t, "stateful"), root)
		p := filepath.Join(root, "src", "scanner.c")
		data, _ := os.ReadFile(p)
		os.WriteFile(p, bytes.ReplaceAll(data, []byte("tree_sitter_tsgk_stateful_external_scanner"), []byte("tree_sitter_tsgk_other_external_scanner")), 0o644)
		g := fixtureGrammar(t, "stateful")
		for i := range g {
			if g[i].Role == "scanner" {
				g[i].SHA256, g[i].Bytes = digestOf(t, p)
			}
		}
		if _, err := build(root, g, "tree_sitter_tsgk_stateful"); codeOf(err) != "BUILD_FAILED" {
			t.Fatalf("another grammar's scanner: %v", err)
		}
	})
	t.Run("scanner-identity", func(t *testing.T) {
		t.Parallel()
		// a changed scanner builds only under a new identity: the scanner is in the closure
		base := fixtureBuild(t, "stateful")
		root := t.TempDir()
		copyTree(t, fixtureRoot(t, "stateful"), root)
		p := filepath.Join(root, "src", "scanner.c")
		data, _ := os.ReadFile(p)
		os.WriteFile(p, append(data, "\n/* changed */\n"...), 0o644)
		g := fixtureGrammar(t, "stateful")
		if _, err := build(root, g, "tree_sitter_tsgk_stateful"); codeOf(err) != "SOURCE_MISMATCH" {
			t.Fatalf("changed scanner under the old identity: %v", err)
		}
		for i := range g {
			if g[i].Role == "scanner" {
				g[i].SHA256, g[i].Bytes = digestOf(t, p)
			}
		}
		b, err := build(root, g, "tree_sitter_tsgk_stateful")
		if err != nil || b.Identity == base.Identity {
			t.Fatalf("changed scanner must build under a new identity: %v", err)
		}
		b.Remove()
	})
	t.Run("changed-header", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		copyTree(t, fixtureRoot(t, "plain"), root)
		h := filepath.Join(root, "src", "tree_sitter", "parser.h")
		data, _ := os.ReadFile(h)
		os.WriteFile(h, append(data, "\n#define TSGK_CHANGED 1\n"...), 0o644)
		if _, err := build(root, fixtureGrammar(t, "plain"), "tree_sitter_tsgk_plain"); codeOf(err) != "SOURCE_MISMATCH" {
			t.Fatalf("changed header under the recorded identity: %v", err)
		}
	})
	t.Run("undefined-symbol", func(t *testing.T) {
		t.Parallel()
		if _, err := build(fixtureRoot(t, "plain"), fixtureGrammar(t, "plain"), "tree_sitter_tsgk_stateful"); codeOf(err) != "BUILD_FAILED" {
			t.Fatalf("a symbol the closure does not define: %v", err)
		}
	})
	t.Run("abi", func(t *testing.T) {
		t.Parallel()
		for _, abi := range []string{"99", "12"} {
			root := t.TempDir()
			copyTree(t, fixtureRoot(t, "plain"), root)
			p := filepath.Join(root, "src", "parser.c")
			data, _ := os.ReadFile(p)
			os.WriteFile(p, bytes.Replace(data, []byte("#define LANGUAGE_VERSION 15"), []byte("#define LANGUAGE_VERSION "+abi), 1), 0o644)
			g := fixtureGrammar(t, "plain")
			g[0].SHA256, g[0].Bytes = digestOf(t, p)
			b, err := build(root, g, "tree_sitter_tsgk_plain")
			if err != nil {
				t.Fatalf("abi %s build: %v", abi, err)
			}
			r := runOracleCase(t, b, oracleContext(true, "(identifier) @i"), "a = 1;", nil)
			if r.ExecutionStatus != kit.StatusFailed || r.Code != "LANGUAGE_INCOMPATIBLE" || len(r.Steps) != 0 || r.Producer == nil || r.Producer.LanguageVersion == 15 {
				t.Fatalf("abi %s: %s %s %d", abi, r.ExecutionStatus, r.Code, len(r.Steps))
			}
			b.Remove()
		}
	})
}
