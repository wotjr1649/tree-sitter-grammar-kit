package native

import (
	"os"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/runner"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func TestQueryClassificationPostgreSQL9StaticProfile(t *testing.T) {
	spec, ok := classificationCPPProfileSpecFor("postgresql9-empty")
	if !ok || spec.label != "PostgreSQL9" || spec.route != "postgresql-sql" || spec.symbol != "tree_sitter_postgres" || spec.rootType != "source_file" ||
		len(spec.querySpecs) != 9 || spec.factQueries != 1 || spec.grammarCount != 5 || spec.symbolCount != 1280 || spec.fieldCount != 0 {
		t.Fatal("closed PostgreSQL9 paged profile missing or changed")
	}
	profile, err := classificationCaptureProfileFor("postgresql9-empty")
	if err != nil || profile.anchorFile != spec.anchorFile || profile.anchorSHA != spec.anchorSHA || profile.route != spec.route || profile.symbol != spec.symbol ||
		profile.queryCount != 9 || profile.grammarCount != 5 || !profile.paged {
		t.Fatalf("PostgreSQL9 profile is not closed and paged: %#v, %v", profile, err)
	}
	for _, name := range []string{"", "positive-batch", "c4-empty", "java5-empty", "rust4-empty", "csharp15-empty", "cpp4-empty", "kotlin5-empty", "swift8-empty", "dart7-empty", "php5-empty", "javascript4-empty", "powershell3-tiny"} {
		other, err := classificationCaptureProfileFor(name)
		wantPaged := name == "csharp15-empty" || name == "cpp4-empty" || name == "kotlin5-empty" || name == "swift8-empty"
		if err != nil || other.paged != wantPaged {
			t.Fatalf("PostgreSQL9 paging changed another profile: %q, %#v, %v", name, other, err)
		}
		if other, ok := classificationCPPProfileSpecFor(name); ok && other.factQueries != 0 {
			t.Fatalf("fact queries leaked into the %q paged profile", name)
		}
	}
	for _, alias := range []string{"postgresql-sql", "postgresql9", "postgresql9-empty ", "POSTGRESQL9-empty", "postgres9-empty"} {
		if _, ok := classificationCPPProfileSpecFor(alias); ok {
			t.Fatalf("near-alias paged profile %q accepted", alias)
		}
		if _, err := classificationCaptureProfileFor(alias); err == nil {
			t.Fatalf("near-alias capture profile %q accepted", alias)
		}
	}
	if _, ok := classificationCJRSpecFor("postgresql9-empty"); ok || classificationCJRProfile("postgresql9-empty") || classificationEmptyProfileName("postgresql9-empty") {
		t.Fatal("PostgreSQL9 entered an unpaged closed family")
	}
	if _, ok := classificationSmall17ProfileFor("postgresql9-empty"); ok {
		t.Fatal("PostgreSQL9 entered the Small17 family")
	}
	if !classificationGoCaptureEntrySkipsProfile("postgresql9-empty") {
		t.Fatal("generic Go capture entry would take the PostgreSQL9 profile")
	}
	data, anchors := classificationPagedReadAnchors(t, "postgresql9-empty")
	if sha(data) != spec.anchorSHA {
		t.Fatal("PostgreSQL9 anchor pin differs")
	}
	if err := classificationPagedAnchorsValid("postgresql9-empty", anchors); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryValid("postgresql9-empty", anchors); err != nil {
		t.Fatal(err)
	}
	queryData, err := os.ReadFile("../../testdata/native/queries/postgresql-sql.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedQueryRegistryBytesValid("postgresql9-empty", anchors, append(slices.Clone(queryData), ' ')); err == nil {
		t.Fatal("PostgreSQL9 query registry byte mutation accepted")
	}
	// Eight ordinary queries come from the query file; the ninth must come from the pinned fact query pack.
	if err := classificationPagedQueryRegistryBytesValid("postgresql9-empty", anchors, queryData); err != nil {
		t.Fatal(err)
	}
	factData, err := os.ReadFile(classificationSmall17FactPackPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedFactPackValid("postgresql9-empty", anchors, factData); err != nil {
		t.Fatal(err)
	}
	if err := classificationPagedFactPackValid("postgresql9-empty", anchors, append(slices.Clone(factData), ' ')); err == nil {
		t.Fatal("PostgreSQL9 fact query pack byte mutation accepted")
	}
	if err := classificationPagedFactPackValid("postgresql9-empty", anchors, queryData); err == nil {
		t.Fatal("PostgreSQL9 accepted the query file as the fact query pack")
	}
	if err := classificationPagedFactPackValid("swift8-empty", anchors, factData); err == nil {
		t.Fatal("a profile without fact queries accepted a fact query pack")
	}
	for name, change := range map[string]func(*classificationLocalAnchors){
		"fact-source": func(a *classificationLocalAnchors) { a.Queries[8].Source += " " },
		"fact-id":     func(a *classificationLocalAnchors) { a.Queries[8].ID = "facts.dynamic_sql" },
		"fact-count":  func(a *classificationLocalAnchors) { a.Queries = a.Queries[:8] },
	} {
		changed := anchors
		changed.Queries = slices.Clone(anchors.Queries)
		change(&changed)
		if classificationPagedFactPackValid("postgresql9-empty", changed, factData) == nil {
			t.Fatalf("detached PostgreSQL9 fact anchor %s accepted", name)
		}
	}
	// Registry file queries are never matched against the fact anchor, and the registry alone does not cover nine queries.
	detached := anchors
	detached.Queries = slices.Clone(anchors.Queries)
	detached.Queries[0].Source += "\n"
	if classificationPagedQueryRegistryBytesValid("postgresql9-empty", detached, queryData) == nil {
		t.Fatal("PostgreSQL9 anchor query source detached from the pinned registry accepted")
	}
	// The four paged profiles never accept each other's anchors.
	for _, other := range []string{"cpp4-empty", "kotlin5-empty", "swift8-empty"} {
		if classificationPagedAnchorsValid(other, anchors) == nil || classificationPagedQueryRegistryValid(other, anchors) == nil {
			t.Fatalf("the %s validator accepted the PostgreSQL9 anchors", other)
		}
		_, otherAnchors := classificationPagedReadAnchors(t, other)
		if classificationPagedAnchorsValid("postgresql9-empty", otherAnchors) == nil || classificationPagedQueryRegistryValid("postgresql9-empty", otherAnchors) == nil {
			t.Fatalf("the PostgreSQL9 validator accepted the %s anchors", other)
		}
	}
	if classificationCJRAnchorsValid("postgresql9-empty", anchors) == nil {
		t.Fatal("CJR validator accepted the PostgreSQL9 profile")
	}
}

func TestQueryClassificationPostgreSQL9AnchorGuards(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*classificationLocalAnchors)
	}{
		{"route", func(a *classificationLocalAnchors) { a.Route = "tsql" }},
		{"case", func(a *classificationLocalAnchors) { a.Case = "swift8-empty-definitions" }},
		{"api", func(a *classificationLocalAnchors) { a.API = true }},
		{"source", func(a *classificationLocalAnchors) { a.Source.UTF8, a.Source.Bytes = "\n", 1 }},
		{"root-type", func(a *classificationLocalAnchors) { a.Nodes[0].Type = "program" }},
		{"root-named", func(a *classificationLocalAnchors) { a.Nodes[0].Named = false }},
		{"abi-symbols", func(a *classificationLocalAnchors) { a.ABI.SymbolCount = 1279 }},
		{"abi-fields", func(a *classificationLocalAnchors) { a.ABI.FieldCount = 1 }},
		{"query-order", func(a *classificationLocalAnchors) { a.Queries[0], a.Queries[1] = a.Queries[1], a.Queries[0] }},
		{"query-source", func(a *classificationLocalAnchors) { a.Queries[2].Source += " " }},
		{"fact-query-source", func(a *classificationLocalAnchors) { a.Queries[8].Source += " " }},
		{"patterns", func(a *classificationLocalAnchors) { a.Queries[2].Metadata.Patterns = 4 }},
		{"fact-patterns", func(a *classificationLocalAnchors) { a.Queries[8].Metadata.Patterns = 15 }},
		{"captures", func(a *classificationLocalAnchors) {
			a.Queries[2].Metadata.CaptureNames = []string{"listen_channel", "notify_channel"}
		}},
		{"predicates-nil", func(a *classificationLocalAnchors) { a.Queries[0].Metadata.Predicates = nil }},
		{"query-count", func(a *classificationLocalAnchors) { a.Queries = a.Queries[:8] }},
		{"required", func(a *classificationLocalAnchors) { a.Required = a.Required[:8] }},
		{"required-null", func(a *classificationLocalAnchors) { a.Required[0].Expected = nil }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, anchors := classificationPagedReadAnchors(t, "postgresql9-empty")
			mutate.edit(&anchors)
			if err := classificationPagedAnchorsValid("postgresql9-empty", anchors); err == nil {
				t.Fatal("PostgreSQL9 anchor mutation accepted")
			}
		})
	}
}

func TestQueryClassificationPostgreSQL9GrammarGuards(t *testing.T) {
	spec, _ := classificationCPPProfileSpecFor("postgresql9-empty")
	symbol, files := classificationCPPRouteFiles(t, spec.route)
	if symbol != spec.symbol || !classificationPagedGrammarValid("postgresql9-empty", files) {
		t.Fatal("PostgreSQL9 grammar closure differs from the frozen native-routes row")
	}
	// Six full 16 MiB parts plus a short tail, within the 100 MiB build-file ceiling.
	if files[0].Bytes <= 16<<20 || files[0].Bytes > 100<<20 || (files[0].Bytes+(16<<20)-1)/(16<<20) != 7 || files[0].Bytes-6*(16<<20) != 3622435 {
		t.Fatal("PostgreSQL9 parser is not the expected seven-part paged parser")
	}
	if classificationCPPGrammarValid(files) || classificationCJRGrammarValid("postgresql9-empty", files) || classificationPagedGrammarValid("kotlin5-empty", files) || classificationPagedGrammarValid("swift8-empty", files) {
		t.Fatal("another closed grammar validator accepted the PostgreSQL9 closure")
	}
	for _, mutate := range []func([]kit.NativeInput) []kit.NativeInput{
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Bytes--; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[0].Path = "src/parser.c"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].SHA256 = f[2].SHA256; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[2].Path = "postgres/src/alloc.h"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { f[1].Role = "parser"; return f },
		func(f []kit.NativeInput) []kit.NativeInput { return f[:4] },
		func(f []kit.NativeInput) []kit.NativeInput { f[0], f[1] = f[1], f[0]; return f },
	} {
		if classificationPagedGrammarValid("postgresql9-empty", mutate(slices.Clone(files))) {
			t.Fatal("PostgreSQL9 grammar mutation accepted")
		}
	}
}

func TestQueryClassificationPostgreSQL9EmptyStderrCaptureMember(t *testing.T) {
	processResult, ownerResult := &runner.Result{}, &runner.Result{}
	result := &CaseResult{Process: processResult}
	capture := &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult}
	classificationNormalizeCaptureStderr("postgresql9-empty", result, capture)
	for _, process := range []*runner.Result{processResult, ownerResult} {
		if process.StderrBytes != 0 || process.Stderr == nil || len(process.Stderr) != 0 {
			t.Fatal("PostgreSQL9 zero-byte stderr was not made explicit in both process records")
		}
	}
	files, err := capture.files("capture/postgresql9-empty-definitions")
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
	for _, file := range files {
		if file.pin.Role == "stderr" {
			rows++
			if file.pin.Bytes != 0 || file.pin.SHA256 != sha(nil) || file.data == nil || len(file.data) != 0 {
				t.Fatal("PostgreSQL9 empty stderr member differs from the process")
			}
		}
	}
	if rows != 1 {
		t.Fatal("PostgreSQL9 empty stderr member was not serialized exactly once")
	}
	stderr := []byte{0x41}
	processResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	ownerResult = &runner.Result{StderrBytes: 1, Stderr: slices.Clone(stderr)}
	classificationNormalizeCaptureStderr("postgresql9-empty", &CaseResult{Process: processResult}, &driverCapture{stage: "RUN_RETURNED", spec: &runner.Spec{}, result: ownerResult})
	if !slices.Equal(processResult.Stderr, stderr) || !slices.Equal(ownerResult.Stderr, stderr) {
		t.Fatal("PostgreSQL9 nonempty stderr was rewritten")
	}
}
