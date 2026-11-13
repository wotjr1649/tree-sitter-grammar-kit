package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ordinarycommit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// Pinned digests of the source-only output for the pinned P0 inputs and the test candidate. A change
// of the registry, the inventory, the query texts, the runtime manifest or the generator moves them.
const (
	testCandidate  = "candidate-427-p3"
	testCohort     = "cohort-427-p3"
	witnessSHA     = "cb8503c58c29a5e7661077605edf53fc7a0e32dbc6d05a6605f22ef00fc25318"
	profileSHA     = "2b9bfb66788e996f6a0c1ccdfec13b8187e1a748c7765a3f7ba683124174a0a7"
	assetsSHA      = "741e32ae5a338c7f1e7fa65218832751fe40d3baf916128ec74d4249c3c75a47"
	planSHA        = "e9d7f3a4cc6bc30a785e92a7c4ce2d8b26273ceb9c6b369590046c192a1f2ce1"
	commitmentRoot = "04a7bb4257328735e182628f5fa88b423be67b66bd6d6b80c573d3f28a2ae0a9"
	registrationsD = "35e7290b2da240c00c278aee7c3685f91b3634c3ea1d877ccc960fbfc87854b5"
	// errorRecoveryTuples is the per-host count of tuples whose step expects syntax ERROR in the inventory.
	errorRecoveryTuples = 13238
)

// hostTuples is the per-route, per-host ordinary tuple count of the registry: queries x tree roles
// (1 at step 0, 2 after each edit) summed over the cases of the route.
var hostTuples = map[string]int{
	"csharp": 9600, "go": 393, "python": 1352, "javascript": 552, "jsx": 342, "typescript": 931, "tsx": 214, "java": 520, "kotlin": 510,
	"c": 376, "cpp": 412, "rust": 244, "swift": 1600, "dart": 707, "php": 700, "ruby": 392, "r": 219, "bash": 134, "powershell": 261,
	"html": 100, "css": 3366, "json": 71, "yaml": 71, "xml": 196, "tsql": 23856, "postgresql-sql": 5985,
}

var hosts = []string{"windows-amd64", "linux-amd64"}

type fixtureData struct {
	req kit.OrdinaryGenerateRequest
	out Output
	inv kit.QualificationInventory
	err error
}

var (
	fixtureOnce sync.Once
	fixture     fixtureData
)

func repoRoot(t testing.TB) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// loadFixture derives the P1 witness with the source-only query-witness tool, then generates once.
func loadFixture(t testing.TB) fixtureData {
	t.Helper()
	fixtureOnce.Do(func() {
		root := repoRoot(t)
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "run", "./src/dev/query-witness", "-root", root)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		witness, err := cmd.Output()
		if err != nil {
			fixture.err = err
			return
		}
		witnessPath := filepath.Join(t.TempDir(), "witness.json")
		if fixture.err = os.WriteFile(witnessPath, witness, 0o600); fixture.err != nil {
			return
		}
		if fixture.req, fixture.err = loadRequest(root, witnessPath, testCandidate, testCohort); fixture.err != nil {
			return
		}
		if fixture.inv, fixture.err = kit.ParseQualificationInventory(fixture.req.Qualification); fixture.err != nil {
			return
		}
		fixture.out, fixture.err = generate(fixture.req)
	})
	if fixture.err != nil {
		t.Fatal(fixture.err)
	}
	return fixture
}

func digest(data []byte) string { return queryregistrycore.Digest(data) }

func parseRegistration(t testing.TB, data []byte) kit.OrdinaryRegistration {
	t.Helper()
	var r kit.OrdinaryRegistration
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPinnedOutputAndInputs(t *testing.T) {
	f := loadFixture(t)
	if got := digest(f.req.Witness); got != witnessSHA {
		t.Fatalf("the derived P1 witness changed: %s", got)
	}
	keys := []string{}
	for path := range f.out.Files {
		keys = append(keys, path)
		if strings.Contains(path, "approval") {
			t.Fatalf("an approval document was generated: %s", path)
		}
		if len(f.out.Files[path]) > 16<<20 {
			t.Fatalf("%s exceeds the 16 MiB file limit", path)
		}
	}
	// 1 profile + 1 assets + 52 registrations + 130 query bodies + plan + commitment, and nothing else.
	if len(keys) != 2+52+130+2 {
		t.Fatalf("generated %d files", len(keys))
	}
	for path, want := range map[string]string{"profile/tsgk-oracle-r4.json": profileSHA, "profile/ordinary-assets-r1.json": assetsSHA, "plan.json": planSHA} {
		if got := digest(f.out.Files[path]); got != want {
			t.Fatalf("%s digest %s, want %s", path, got, want)
		}
	}
	if f.out.Commitment.Root != commitmentRoot || f.out.Commitment.ShardCount != 6402 || f.out.Commitment.Plan != (ordinarycommit.Pin{Path: f.out.Commitment.Plan.Path, Bytes: uint64(len(f.out.Files["plan.json"])), SHA256: planSHA}) {
		t.Fatalf("commitment changed: %+v", f.out.Commitment)
	}
	sort.Strings(keys)
	var joined strings.Builder
	for _, path := range keys {
		if strings.HasPrefix(path, "registrations/") {
			joined.WriteString(digest(f.out.Files[path]))
		}
	}
	if got := digest([]byte(joined.String())); got != registrationsD {
		t.Fatalf("registrations digest %s", got)
	}
	again, err := kit.GenerateOrdinaryRegistration(f.req)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Profile) != string(f.out.Files["profile/tsgk-oracle-r4.json"]) || string(again.Assets) != string(f.out.Files["profile/ordinary-assets-r1.json"]) || len(again.Registrations) != 52 {
		t.Fatal("a second generation differs from the first")
	}
	for key, data := range again.Registrations {
		if string(data) != string(f.out.Files["registrations/"+key+".json"]) {
			t.Fatalf("a second generation of %s differs", key)
		}
	}
}

func TestFullPlanExactUnion(t *testing.T) {
	f := loadFixture(t)
	plan := f.out.Plan
	if plan.Required != 106208 || plan.Admitted != 0 || plan.Blocked != 106208 || len(plan.Shards) != 6402 || plan.FullScopeComplete {
		t.Fatalf("plan counters: required=%d admitted=%d blocked=%d shards=%d", plan.Required, plan.Admitted, plan.Blocked, len(plan.Shards))
	}
	if len(f.inv.Routes) != 26 || len(hostTuples) != 26 {
		t.Fatalf("routes: inventory %d pinned %d", len(f.inv.Routes), len(hostTuples))
	}
	cases := map[string]int{}
	totalCases := 0
	for _, route := range f.inv.Routes {
		cases[route.Route] = len(route.Workload.Cases)
		totalCases += len(route.Workload.Cases)
		// An independent count from the inventory: queries x (1 + 2 per edit) per case.
		derived := 0
		for _, c := range route.Workload.Cases {
			derived += len(route.Workload.Queries) * (1 + 2*len(c.Edits))
		}
		if derived != hostTuples[route.Route] {
			t.Fatalf("%s: inventory derives %d tuples per host, pinned %d", route.Route, derived, hostTuples[route.Route])
		}
	}
	if totalCases != 3201 {
		t.Fatalf("cases %d", totalCases)
	}
	type count struct{ shards, ids int }
	got := map[string]*count{}
	seen := map[string]string{}
	for _, shard := range plan.Shards {
		key := shard.Route + "/" + shard.Platform
		if got[key] == nil {
			got[key] = &count{}
		}
		got[key].shards++
		got[key].ids += len(shard.ObligationIDs)
		for _, id := range shard.ObligationIDs {
			if prev, dup := seen[id]; dup {
				t.Fatalf("obligation %s is in shards %s and %s", id, prev, shard.ID)
			}
			seen[id] = shard.ID
		}
	}
	if len(seen) != 106208 || len(got) != 52 {
		t.Fatalf("union %d ids over %d route-host groups", len(seen), len(got))
	}
	for route, want := range hostTuples {
		for _, host := range hosts {
			c := got[route+"/"+host]
			if c == nil || c.shards != cases[route] || c.ids != want {
				t.Fatalf("%s/%s: %+v, want %d shards and %d tuples", route, host, c, cases[route], want)
			}
		}
	}
	// The registrations carry the same ids as the plan: the plan was built from them and from the registry.
	rows := 0
	for key, data := range f.out.PlanRequestRegistrations() {
		r := parseRegistration(t, data)
		for _, row := range r.Required {
			if _, ok := seen[row.ID]; !ok {
				t.Fatalf("%s: required id %s is not in the plan", key, row.ID)
			}
			rows++
		}
	}
	if rows != 106208 {
		t.Fatalf("registrations hold %d required rows", rows)
	}
}

func (o Output) PlanRequestRegistrations() map[string][]byte {
	out := map[string][]byte{}
	for path, data := range o.Files {
		if key, ok := strings.CutPrefix(path, "registrations/"); ok {
			out[strings.TrimSuffix(key, ".json")] = data
		}
	}
	return out
}

// hostFields is the closed list of fields that may differ between the two hosts of one route. The
// obligation id embeds the platform and subject, so it differs with them.
var hostFields = []string{
	"registration: platform_id", "registration: subject_platform", "registration.required[]: obligation_id",
	"registration.required[].coordinate: platform", "registration.required[].coordinate: subject",
	"profile.bindings[]: platform_id", "profile.bindings[]: subject_platform",
	"plan.shards[]: id", "plan.shards[]: platform_id", "plan.shards[]: subject_platform", "plan.shards[]: binding_id",
	"plan.shards[]: obligation_ids", "plan.shards[]: obligation_ids_sha256",
}

func TestHostsDifferOnlyInPlatformFields(t *testing.T) {
	f := loadFixture(t)
	t.Logf("allowed host-dependent fields: %s", strings.Join(hostFields, "; "))
	regs := f.out.PlanRequestRegistrations()
	for _, route := range f.inv.Routes {
		w, l := parseRegistration(t, regs[route.Route+"/windows-amd64"]), parseRegistration(t, regs[route.Route+"/linux-amd64"])
		if w.Platform == l.Platform || w.Subject == l.Subject || len(w.Required) != len(l.Required) {
			t.Fatalf("%s: hosts are not two distinct hosts with one row set", route.Route)
		}
		for i := range w.Required {
			if w.Required[i].ID == l.Required[i].ID || w.Required[i].Coordinate.Platform == l.Required[i].Coordinate.Platform {
				t.Fatalf("%s row %d: host fields do not differ", route.Route, i)
			}
			// Align the rows on every field except the allowed ones.
			l.Required[i].ID, l.Required[i].Coordinate.Platform, l.Required[i].Coordinate.Subject = w.Required[i].ID, w.Required[i].Coordinate.Platform, w.Required[i].Coordinate.Subject
		}
		l.Platform, l.Subject = w.Platform, w.Subject
		if !reflect.DeepEqual(w, l) {
			t.Fatalf("%s: the registrations differ outside the allowed host fields", route.Route)
		}
	}
	profile, err := kit.ParseOrdinaryProfile(f.out.Files["profile/tsgk-oracle-r4.json"])
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]kit.OrdinaryBinding{}
	for _, b := range profile.Bindings {
		bindings[b.Route+"/"+b.Platform] = b
	}
	if len(bindings) != 52 {
		t.Fatalf("%d bindings", len(bindings))
	}
	for _, route := range f.inv.Routes {
		w, l := bindings[route.Route+"/windows-amd64"], bindings[route.Route+"/linux-amd64"]
		l.Platform, l.Subject = w.Platform, w.Subject
		if !reflect.DeepEqual(w, l) {
			t.Fatalf("%s: the profile bindings differ outside the allowed host fields", route.Route)
		}
	}
	shards := map[string]kit.OrdinaryPlanShard{}
	for _, s := range f.out.Plan.Shards {
		shards[s.Route+"/"+s.Case+"/"+s.Platform] = s
	}
	for key, w := range shards {
		if w.Platform != "windows-amd64" {
			continue
		}
		l, ok := shards[strings.TrimSuffix(key, "windows-amd64")+"linux-amd64"]
		if !ok || w.ID == l.ID || w.ObligationIDSHA256 == l.ObligationIDSHA256 {
			t.Fatalf("%s: no distinct linux shard", key)
		}
		l.ID, l.Platform, l.Subject, l.BindingID, l.ObligationIDs, l.ObligationIDSHA256 = w.ID, w.Platform, w.Subject, w.BindingID, w.ObligationIDs, w.ObligationIDSHA256
		if !reflect.DeepEqual(w, l) {
			t.Fatalf("%s: the shards differ outside the allowed host fields", key)
		}
	}
}

func TestErrorRecoveryTuplesStayInTheDenominatorAsBlocked(t *testing.T) {
	f := loadFixture(t)
	errorStep := map[string]bool{}
	for _, route := range f.inv.Routes {
		for _, c := range route.Workload.Cases {
			for _, e := range c.Expect {
				if e.Syntax == "ERROR" {
					errorStep[route.Route+"/"+c.ID+"/"+strconv.Itoa(e.Step)] = true
				}
			}
		}
	}
	blocked := map[string]bool{}
	for _, shard := range f.out.Plan.Shards {
		if shard.Admission != (kit.OrdinaryPlanAdmission{State: "BLOCKED", Code: "CAPACITY_UNPROVEN"}) {
			t.Fatalf("shard %s is %+v, not the registered BLOCKED state", shard.ID, shard.Admission)
		}
		for _, id := range shard.ObligationIDs {
			blocked[id] = true
		}
	}
	perHost := map[string]int{}
	for key, data := range f.out.PlanRequestRegistrations() {
		r := parseRegistration(t, data)
		for _, row := range r.Required {
			if errorStep[r.Route+"/"+row.Coordinate.Case+"/"+strconv.Itoa(int(row.Coordinate.Step))] {
				if !blocked[row.ID] {
					t.Fatalf("%s: error-recovery tuple %s is not in a BLOCKED shard", key, row.ID)
				}
				perHost[r.Platform]++
			}
		}
	}
	t.Logf("error-recovery tuples per host: %v", perHost)
	if perHost["windows-amd64"] != errorRecoveryTuples || perHost["linux-amd64"] != errorRecoveryTuples {
		t.Fatalf("error-recovery tuples per host: %v, want %d", perHost, errorRecoveryTuples)
	}
	if f.out.Plan.Blocked != f.out.Plan.Required || len(blocked) != 106208 {
		t.Fatal("the denominator is not fully BLOCKED")
	}
}

// reject asserts that stored plan bytes are refused by the strict decoder, by the full revalidation
// against the caller's inputs, and that they no longer hash to the pinned commitment root.
func reject(t *testing.T, name string, plan kit.OrdinaryShardPlan, req kit.OrdinaryPlanRequest, strictDecode bool) {
	t.Helper()
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if strictDecode {
		if _, err := kit.ParseOrdinaryShardPlan(data); err == nil {
			t.Errorf("%s: ParseOrdinaryShardPlan accepted the plan", name)
		}
	}
	if err := kit.ValidateOrdinaryShardPlan(plan, req); err == nil {
		t.Errorf("%s: ValidateOrdinaryShardPlan accepted the plan", name)
	}
	if c, _, err := ordinarycommit.Build(data); err == nil && c.Root == commitmentRoot {
		t.Errorf("%s: the tampered plan still has the pinned commitment root", name)
	}
}

func TestTamperedPlanIsRejected(t *testing.T) {
	f := loadFixture(t)
	req := f.out.PlanRequest
	if err := kit.ValidateOrdinaryShardPlan(f.out.Plan, req); err != nil {
		t.Fatalf("the generated plan does not validate against its inputs: %v", err)
	}
	clone := func() kit.OrdinaryShardPlan {
		p := f.out.Plan
		p.Shards = append([]kit.OrdinaryPlanShard(nil), p.Shards...)
		p.Registrations = append([]kit.OrdinaryPin(nil), p.Registrations...)
		return p
	}

	missing := clone()
	missing.Shards = append(missing.Shards[:7:7], missing.Shards[8:]...)
	reject(t, "missing shard", missing, req, true)

	missingAdjusted := clone()
	dropped := missingAdjusted.Shards[7]
	missingAdjusted.Shards = append(missingAdjusted.Shards[:7:7], missingAdjusted.Shards[8:]...)
	missingAdjusted.Required -= uint64(len(dropped.ObligationIDs))
	missingAdjusted.Blocked = missingAdjusted.Required
	reject(t, "missing shard with adjusted counters", missingAdjusted, req, true)

	duplicated := clone()
	duplicated.Shards[3].ObligationIDs = append([]string(nil), duplicated.Shards[3].ObligationIDs...)
	duplicated.Shards[3].ObligationIDs[0] = duplicated.Shards[4].ObligationIDs[0]
	reject(t, "duplicated tuple", duplicated, req, true)

	for name, mutate := range map[string]func(*kit.OrdinaryShardPlan){
		"registration pin": func(p *kit.OrdinaryShardPlan) { p.Registrations[0].SHA256 = strings.Repeat("0", 64) },
		"profile pin":      func(p *kit.OrdinaryShardPlan) { p.Profile.SHA256 = strings.Repeat("0", 64) },
		"asset pin":        func(p *kit.OrdinaryShardPlan) { p.Assets.SHA256 = strings.Repeat("0", 64) },
		"witness pin":      func(p *kit.OrdinaryShardPlan) { p.Witness.SHA256 = strings.Repeat("0", 64) },
		"candidate":        func(p *kit.OrdinaryShardPlan) { p.Candidate += "x" },
	} {
		changed := clone()
		mutate(&changed)
		reject(t, name, changed, req, false)
	}

	// Tampered registrations: the planner refuses to rebuild, so no plan and no commitment exist for them.
	for name, mutate := range map[string]func(*kit.OrdinaryRegistration){
		"registration drops a required row": func(r *kit.OrdinaryRegistration) { r.Required = r.Required[1:] },
		"registration repeats a required row": func(r *kit.OrdinaryRegistration) {
			r.Required = append(append([]kit.OrdinaryRequired(nil), r.Required[:len(r.Required)-1]...), r.Required[0])
		},
		"registration changes a pin": func(r *kit.OrdinaryRegistration) { r.Profile.SHA256 = strings.Repeat("1", 64) },
	} {
		tampered := req
		tampered.Registrations = append([][]byte(nil), req.Registrations...)
		r := parseRegistration(t, tampered.Registrations[0])
		mutate(&r)
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		tampered.Registrations[0] = data
		if _, err := kit.BuildOrdinaryShardPlan(tampered); err == nil {
			t.Errorf("%s: the planner accepted the registrations", name)
		}
	}
}
