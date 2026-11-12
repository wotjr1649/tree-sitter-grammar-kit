package kit

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

func ordinaryJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsonv2.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ordinaryRegistryFixture(t *testing.T) (OrdinaryPlanRequest, *queryregistrycore.Validated, QualificationInventory) {
	t.Helper()
	read := func(path string) []byte {
		b, err := os.ReadFile(filepath.Join("../..", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	r := OrdinaryPlanRequest{Registry: read(queryregistrycore.Path), RegistrySHA256: ordinaryRegistrySHA, Qualification: read(queryregistrycore.QualificationPin.Path), Slots: read(queryregistrycore.SlotPin.Path), Basis: map[string][]byte{}}
	for _, path := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		r.Basis[path] = read(path)
	}
	v, inv, err := ordinaryRegistry(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Coordinates()) != 106346 || len(queryregistrycore.Sets(v.Coordinates())) != 58 || len(r.Registry) != 82475 || v.SHA256() != ordinaryRegistrySHA {
		t.Fatal("P0 adapter drift")
	}
	return r, v, inv
}

func ordinaryProfileFixture() OrdinaryProfile {
	return OrdinaryProfile{Schema: "tsgk-oracle/r4", Context: OrdinaryContext, Candidate: "candidate-362", SemanticCohort: "cohort-362", NativePolicy: queryregistrycore.FilePin("policy/native.txt", []byte(NativePolicyText(NativeOperations()["native-query-batch"], OutputTree))), Limits: OrdinaryPlanLimits(), InheritedLimits: OrdinaryPlanLimits(), Hosts: []OrdinaryHost{{"windows-amd64", "windows/amd64"}, {"linux-amd64", "linux/amd64"}}, Bindings: []OrdinaryBinding{}}
}

func TestOrdinaryStrictDocuments(t *testing.T) {
	p := ordinaryProfileFixture()
	b := ordinaryJSON(t, p)
	if _, err := ParseOrdinaryProfile(b); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOracleProfile(b); err == nil {
		t.Fatal("ordinary planning profile accepted by executable oracle parser")
	}
	mutants := map[string]func(map[string]any){
		"unknown": func(m map[string]any) { m["ignored"] = true },
		"missing": func(m map[string]any) { delete(m, "bindings") },
		"null":    func(m map[string]any) { m["bindings"] = nil },
		"host":    func(m map[string]any) { m["hosts"] = []any{} },
		"cap": func(m map[string]any) {
			m["limits"].(map[string]any)["logical"] = float64(OrdinaryPlanLimits().Logical + 1)
		},
		"inherited-cap": func(m map[string]any) { m["inherited_limits"].(map[string]any)["logical"] = float64(1536 << 20) },
		"policy":        func(m map[string]any) { m["native_policy"].(map[string]any)["sha256"] = strings.Repeat("0", 64) },
	}
	for name, mutate := range mutants {
		t.Run(name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			mutate(m)
			bad, _ := json.Marshal(m)
			if _, err := ParseOrdinaryProfile(bad); err == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
	for _, data := range [][]byte{append(append([]byte{}, b...), b...), []byte(strings.Replace(string(b), `"schema":`, `"schema":"duplicate","schema":`, 1))} {
		if _, err := ParseOrdinaryProfile(data); err == nil {
			t.Fatal("accepted duplicate or trailing JSON")
		}
	}
	asset := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: OrdinaryPin{Path: ordinaryRegistryPath, Bytes: 82475, SHA256: ordinaryRegistrySHA}, Candidate: p.Candidate, SemanticCohort: p.SemanticCohort, Assets: []OrdinaryAsset{{ID: "m", Role: "metadata", State: "UNAVAILABLE"}}}
	if _, err := ParseOrdinaryAssets(ordinaryJSON(t, asset)); err != nil {
		t.Fatal(err)
	}
	asset.Assets[0].State = "PINNED"
	if _, err := ParseOrdinaryAssets(ordinaryJSON(t, asset)); err == nil {
		t.Fatal("upgraded missing asset")
	}
	member := OrdinaryMember{Owner: "owner", Path: "p.json", Role: "grammar-paged-descriptor", CaseOrBatch: "batch", Pin: queryregistrycore.FilePin("p.json", []byte("{}"))}
	if err := ordinaryMembers([]OrdinaryMember{member}, "plain"); err == nil {
		t.Fatal("paged role in plain closure")
	}
	if err := ordinaryMembers([]OrdinaryMember{member}, "parser-paged"); err != nil {
		t.Fatal(err)
	}
	member.Role = "library-paged-descriptor"
	if err := ordinaryMembers([]OrdinaryMember{member}, "parser-paged"); err == nil {
		t.Fatal("binary descriptor as parser")
	}
	metadata := OrdinaryAsset{Role: "metadata"}
	if err := ordinaryValidateAssetBody(metadata, []byte(`{"patterns":1,"capture_names":[],"predicates":[]}`)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"patterns":1,"capture_names":[],"predicates":null}`, `{"patterns":1,"capture_names":[],"predicates":[{"pattern":0,"steps":[]}]}`, `{"patterns":1,"capture_names":[]}`} {
		if err := ordinaryValidateAssetBody(metadata, []byte(body)); err == nil {
			t.Fatal("accepted missing or predicate metadata")
		}
	}
	var descriptor semanticParserDescriptor
	descriptor.Schema = "tsgk-query-parser-pages/r1"
	descriptor.Owner = "owner"
	descriptor.Logical.Role = "grammar-scanner"
	descriptor.Logical.Path = "src/parser.c"
	descriptor.Logical.Bytes = 16<<20 + 1
	descriptor.Logical.SHA256 = strings.Repeat("a", 64)
	member.Role = "grammar-paged-descriptor"
	if err := ordinaryValidateMemberBody(member, ordinaryJSON(t, descriptor)); err == nil {
		t.Fatal("parser descriptor with wrong logical role")
	}
}

func TestOrdinaryRegistrationBinding(t *testing.T) {
	t.Parallel()
	_, v, inv := ordinaryRegistryFixture(t)
	var route QualRoute
	for _, r := range inv.Routes {
		if r.Route == "go" {
			route = r
			break
		}
	}
	if route.Route == "" {
		t.Fatal("missing go route")
	}
	p := ordinaryProfileFixture()
	p.Bindings = []OrdinaryBinding{{Route: route.Route, Platform: "windows-amd64", Subject: "windows/amd64", ClosureEncoding: "plain", Grammar: route.Workload.Grammar, Runtime: queryregistrycore.FilePin("runtime.json", []byte("runtime")), Members: []OrdinaryMember{}}}
	pb := ordinaryJSON(t, p)
	assets := OrdinaryAssetRegistration{Schema: "tsgk-query-ordinary-assets/r1", Registry: OrdinaryPin{Path: ordinaryRegistryPath, Bytes: 82475, SHA256: ordinaryRegistrySHA}, Candidate: p.Candidate, SemanticCohort: p.SemanticCohort, Assets: []OrdinaryAsset{}}
	for _, x := range []struct{ id, role string }{{"m", "metadata"}, {"t", "expected-tree"}, {"c", "expected-captures"}, {"f", "expected-facts"}} {
		assets.Assets = append(assets.Assets, OrdinaryAsset{ID: x.id, Role: x.role, State: "UNAVAILABLE"})
	}
	ab := ordinaryJSON(t, assets)
	w := ordinarySourceExport{Registry: assets.Registry, Qualification: queryregistrycore.QualificationPin, Slots: queryregistrycore.SlotPin}
	s := ordinarySourceRoute{Route: route.Route, Set: route.Workload.Set, Queries: route.Workload.Queries}
	r := OrdinaryRegistration{Schema: "tsgk-query-semantics-registration/r2", Registry: w.Registry, Namespace: v.Namespace(), Inventory: w.Qualification, Slots: w.Slots, Profile: queryregistrycore.FilePin("profile.json", pb), Assets: queryregistrycore.FilePin("assets.json", ab), Route: route.Route, Set: route.Workload.Set, Platform: "windows-amd64", Subject: "windows/amd64", MembershipContext: "qualification/r8", Context: OrdinaryContext, ClosureEncoding: "plain", Candidate: p.Candidate, SemanticCohort: p.SemanticCohort, PlannedMembers: []OrdinaryMember{}}
	for i, c := range route.Workload.Cases {
		sc := ordinarySourceCase{ID: c.ID, Selector: fmt.Sprintf("qualification_inventory#/routes/0/workload/cases/%d", i), Points: make([]EditPoints, len(c.Edits))}
		rc := OrdinaryCase{ID: c.ID, OriginalSelector: sc.Selector, Edits: c.Edits, Points: sc.Points}
		for step := 0; step <= len(c.Edits); step++ {
			hash := digestHex([]byte(fmt.Sprintf("%s/%d", c.ID, step)))
			pin := OrdinaryPin{Path: "source-witness/" + hash + ".utf8", Bytes: 1, SHA256: hash}
			sc.Steps = append(sc.Steps, hash)
			s.Versions = append(s.Versions, ordinarySourceVersion{SHA256: hash, Bytes: 1})
			rc.VersionPins = append(rc.VersionPins, pin)
		}
		rc.InitialSource = rc.VersionPins[0]
		s.Cases = append(s.Cases, sc)
		r.Cases = append(r.Cases, rc)
	}
	w.Routes = []ordinarySourceRoute{s}
	for _, q := range route.Workload.Queries {
		r.Queries = append(r.Queries, OrdinaryQuery{ID: q.ID, Source: OrdinaryPin{Path: "queries/" + q.ID, Bytes: 1, SHA256: q.SHA256}, MetadataAsset: "m", Kind: queryregistrycore.Structural})
	}
	for _, c := range v.Coordinates() {
		if c.Route == r.Route && c.Platform == r.Platform && c.Kind == queryregistrycore.Structural && c.Role == "primary" {
			r.Required = append(r.Required, OrdinaryRequired{ID: c.ID(v.Namespace()), Coordinate: c, TreeAsset: "t", CaptureAsset: "c"})
		}
	}
	check := func(r OrdinaryRegistration) error {
		return validateOrdinaryRegistration(r, p, assets, w, inv, pb, ab, v)
	}
	if err := check(r); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOrdinaryRegistration(ordinaryJSON(t, r)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"coordinate", "source", "sourcepath", "version-bytes", "edit", "point", "query", "metadata", "selector", "gap", "overlap", "asset", "profile"} {
		t.Run(name, func(t *testing.T) {
			var changed OrdinaryRegistration
			if err := jsonv2.Unmarshal(ordinaryJSON(t, r), &changed); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "coordinate":
				changed.Required[0].Coordinate.Context = OrdinaryContext
			case "source":
				changed.Cases[0].VersionPins[0].SHA256 = strings.Repeat("0", 64)
			case "sourcepath":
				changed.Cases[0].VersionPins[0].Path = "different/source.utf8"
			case "version-bytes":
				changed.Cases[0].VersionPins[0].Bytes++
			case "edit":
				changed.Cases[0].Edits = append(changed.Cases[0].Edits, Edit{})
			case "point":
				changed.Cases[0].Points = append(changed.Cases[0].Points, EditPoints{})
			case "query":
				changed.Queries[0].Source.SHA256 = strings.Repeat("0", 64)
			case "metadata":
				changed.Queries[0].MetadataAsset = "t"
			case "selector":
				changed.Cases[0].OriginalSelector = "different"
			case "gap":
				changed.Required = changed.Required[1:]
			case "overlap":
				changed.Required[0] = changed.Required[1]
			case "asset":
				changed.Required[0].TreeAsset = "c"
			case "profile":
				changed.Profile.SHA256 = strings.Repeat("0", 64)
			}
			if err := check(changed); err == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
}
