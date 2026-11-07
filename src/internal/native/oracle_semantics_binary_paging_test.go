package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func semanticBinaryTestPlan(parts int) *semanticCapturePlan {
	p := &semanticCapturePlan{Context: semanticPagedBinariesContext, Members: []semanticCaptureMember{
		{Owner: "O", Path: "manifest.json", Role: "manifest"},
		{Owner: "D", Path: "capture/driver.pages.json", Role: "driver-paged-descriptor"},
	}}
	for i := range parts {
		p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: fmt.Sprintf("capture/driver.part-%05d.bin", i), Role: "driver-paged-part"})
	}
	return p
}

func semanticBinaryTestRegistration(t *testing.T, p *semanticCapturePlan) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"schema": "tsgk-query-semantics-registration/r1", "context": p.Context, "planned_members": p.Members})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func semanticBinaryTestError(t *testing.T, err error, code string) {
	t.Helper()
	if code == "" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var ne *Error
	wantKind := kit.KindInvalidInput
	if code == "SEMANTICS_MEMBER_LIMIT" {
		wantKind = kit.KindResourceLimit
	}
	if !errors.As(err, &ne) || ne.Code != code || ne.Kind != wantKind {
		t.Fatalf("want %s/%s, got %v", wantKind, code, err)
	}
}

func TestSemanticCapturePlanDriverShapeBounds(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"parts-2", "parts-3", "parts-4", "plain", "parts-1", "parts-5", "gap", "no-descriptor", "two-descriptors", "descriptor-path", "mixed", "descriptor-only", "missing-driver", "two-drivers", "plain-and-part", "part-path", "case-collision", "duplicate-part", "unknown-paged-role"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			code := "SEMANTICS_MEMBER_PLAN"
			plain := semanticCaptureMember{Owner: "D", Path: "capture/driver.bin", Role: "driver"}
			switch name {
			case "parts-2", "parts-3", "parts-4":
				p = semanticBinaryTestPlan(int(name[len(name)-1] - '0'))
				code = ""
			case "plain":
				p.Members = append(p.Members[:1], plain)
				code = ""
			case "parts-1":
				p = semanticBinaryTestPlan(1)
			case "parts-5":
				p = semanticBinaryTestPlan(5)
			case "gap":
				p.Members[3].Path = "capture/driver.part-00002.bin"
			case "no-descriptor":
				p.Members = append(p.Members[:1], p.Members[2:]...)
			case "two-descriptors":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/other.pages.json", Role: "driver-paged-descriptor"})
			case "descriptor-path":
				p.Members[1].Path = "capture/drivers.pages.json"
			case "mixed":
				p.Members = append(p.Members, plain)
			case "descriptor-only":
				p.Members = p.Members[:2]
			case "missing-driver":
				p.Members = p.Members[:1]
			case "two-drivers":
				p.Members = append(p.Members[:1], plain, semanticCaptureMember{Owner: "D", Path: "capture/other.bin", Role: "driver"})
			case "plain-and-part":
				p.Members = append(p.Members[:1], plain, p.Members[2])
			case "part-path":
				p.Members[2].Path = "capture/driver.part-0000.bin"
			case "case-collision":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/DRIVER.part-00000.bin", Role: "driver-paged-part"})
			case "duplicate-part":
				p.Members = append(p.Members, p.Members[2])
			case "unknown-paged-role":
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/future.bin", Role: "driver-paged-future"})
			}
			registered, err := semanticCaptureRegistration(context.Background(), semanticBinaryTestRegistration(t, p))
			if name == "case-collision" || name == "duplicate-part" {
				semanticBinaryTestError(t, err, code)
				return
			}
			semanticBinaryTestError(t, err, "")
			semanticBinaryTestError(t, registered.driverShape(), code)
		})
	}
}

func TestSemanticCapturePlanContextSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		context string
		paged   bool
	}{
		{"binaries", semanticPagedBinariesContext, true},
		{"parser", semanticPagedContext, true},
		{"revision", "full-input-utf8-paged-binaries/r2", false},
		{"trailing-space", semanticPagedBinariesContext + " ", false},
		{"case", strings.ToUpper(semanticPagedBinariesContext), false},
		{"opaque", "opaque", false},
	} {
		for _, stage := range []string{"registration", "selector"} {
			t.Run(stage+"-"+tc.name, func(t *testing.T) {
				t.Parallel()
				p := semanticBinaryTestPlan(2)
				p.Context = tc.context
				data := semanticBinaryTestRegistration(t, p)
				if stage == "selector" {
					got, err := semanticPagedSelected(data, func() error { return nil })
					if err != nil || got != tc.paged {
						t.Fatalf("selected=%v want=%v: %v", got, tc.paged, err)
					}
					return
				}
				got, err := semanticCaptureRegistration(context.Background(), data)
				semanticBinaryTestError(t, err, "")
				wantContext := "full-input-utf8/r2"
				if tc.paged {
					wantContext = tc.context
				}
				if got.Context != wantContext || got.paged() != tc.paged || got.binaries() != (tc.context == semanticPagedBinariesContext) {
					t.Fatalf("wrong plan: %+v", got)
				}
			})
		}
	}
}

func TestSemanticPagedBinariesContextMatchesKit(t *testing.T) {
	t.Parallel()
	if semanticPagedBinariesContext != "full-input-utf8-paged-binaries/r1" || kit.OraclePagedBinariesSemanticsManifestSchema != "tsgk-oracle-manifest/r7" {
		t.Fatal("native context or kit manifest schema differs from the binary paging contract")
	}
}

func TestSemanticCapturePlanDriverRolesOtherContexts(t *testing.T) {
	t.Parallel()
	for _, context := range []string{semanticPagedContext, "full-input-utf8/r2"} {
		for _, role := range []string{"driver-paged-descriptor", "driver-paged-part", "driver-paged-future", "driver"} {
			t.Run(context+"/"+role, func(t *testing.T) {
				t.Parallel()
				path := "capture/driver.bin"
				if role == "driver-paged-descriptor" {
					path = "capture/driver.pages.json"
				} else if role == "driver-paged-part" {
					path = "capture/driver.part-00000.bin"
				}
				p := &semanticCapturePlan{Context: context, Members: []semanticCaptureMember{{Owner: "D", Path: path, Role: role}}}
				code := "SEMANTICS_MEMBER_PLAN"
				if role == "driver" {
					code = ""
				}
				semanticBinaryTestError(t, p.driverShape(), code)
			})
		}
	}
}

func TestSemanticCapturePlanBuildOnlyDriverRoles(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"driver", "driver-paged-descriptor", "driver-paged-part", "driver-future", "driver-paged-future"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			p := &semanticCapturePlan{Members: []semanticCaptureMember{{Owner: "D", Path: "capture/driver", Role: role}}}
			included := role == "driver" || role == "driver-paged-descriptor" || role == "driver-paged-part"
			files := []captureFile{{pin: kit.ReplayMember{Path: "capture/driver", Role: role}}}
			if p.matches(files, true) != included || p.matches(nil, true) == included || !p.matches(files, false) {
				t.Fatal("buildOnly must include exactly the three driver roles")
			}
		})
	}
}

func TestSemanticCapturePlanGrammarShapeUnderBinaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		size    uint64
		parsers int
		pages   bool
		code    string
	}{
		{"small-plain", kit.MaxDocumentBytes, 1, false, ""},
		{"no-parser", 0, 0, false, ""},
		{"small-with-pages", kit.MaxDocumentBytes, 1, true, "SEMANTICS_MEMBER_PLAN"},
		{"no-parser-with-pages", 0, 0, true, "SEMANTICS_MEMBER_PLAN"},
		{"one-large", kit.MaxDocumentBytes + 1, 1, true, ""},
		{"build-file-boundary", kit.BuildFileBytes, 1, true, ""},
		{"above-build-file", kit.BuildFileBytes + 1, 1, true, "SEMANTICS_MEMBER_LIMIT"},
		{"two-large", kit.MaxDocumentBytes + 1, 2, true, "SEMANTICS_MEMBER_PLAN"},
		{"large-without-pages", kit.MaxDocumentBytes + 1, 1, false, "SEMANTICS_MEMBER_PLAN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			var grammar []kit.NativeInput
			for range tc.parsers {
				grammar = append(grammar, kit.NativeInput{Role: "parser", Bytes: tc.size})
			}
			if tc.pages {
				p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/build-input-00007.pages.json", Role: "grammar-paged-descriptor"})
				count := (tc.size + kit.MaxDocumentBytes - 1) / kit.MaxDocumentBytes
				for i := uint64(0); i < count; i++ {
					p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: fmt.Sprintf("capture/build-input-00007.part-%05d.bin", i), Role: "grammar-paged-part"})
				}
			}
			semanticBinaryTestError(t, p.grammarShape(grammar), tc.code)
		})
	}
	for _, role := range []string{"grammar-paged-descriptor", "grammar-paged-part", "grammar-paged-future"} {
		t.Run(role+"-without-parser", func(t *testing.T) {
			t.Parallel()
			p := semanticBinaryTestPlan(2)
			p.Members = append(p.Members, semanticCaptureMember{Owner: "D", Path: "capture/unused", Role: role})
			semanticBinaryTestError(t, p.grammarShape(nil), "SEMANTICS_MEMBER_PLAN")
		})
	}
}
