package kit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/queryregistrycore"
)

// ordinaryCLIInputs is the owned PostgreSQL case `alt-postgresql-sql-B04-22` with a registration whose
// history pins are exact for every case of the route, so that the real source-only planner accepts it
// against the real P0 registry and the P1 witness. Unlike the synthetic complete-plan scaffold of the
// other owned fixtures, its shard plan is the plan BuildOrdinaryShardPlan returns for these inputs.
type ordinaryCLIInputs struct {
	PlanReq           OrdinaryPlanRequest
	Plan              OrdinaryShardPlan
	PlanBytes         []byte
	Request           OrdinaryQuerySemanticsRequest
	Registration      OrdinaryRegistration
	RegistrationBytes []byte
	Profile           []byte
	FactPack          []byte
	FactMapping       []byte
	Source            []byte
	GrammarRoot       string
	AssetBodies       map[string][]byte
	ExpectedTree      ordinarySQLExpectedSet
	Metadata          map[string]QueryWitnessMetadata
	ShardID           string
	CaseID            string
}

func ordinaryCLIRealInputs(t *testing.T, witness []byte) ordinaryCLIInputs {
	t.Helper()
	req, profileBytes, source, registration, _, planningProfile, assetData, grammarRoot, packBytes, mapping, assetBodies, expectedTree, metadata := ordinarySQLOwnedInputs(t, "postgresql-sql", nil)
	_, inventory := ordinaryP0ForPlanTest(t)
	var export ordinarySourceExport
	if err := ordinaryDecode(witness, &export); err != nil {
		t.Fatal(err)
	}
	exported := export.Routes[ordinaryRouteIndex(inventory, "postgresql-sql")]
	versions := map[string][]byte{}
	for _, version := range exported.Versions {
		versions[version.SHA256] = version.Source
	}
	for i := range registration.Cases {
		index := slices.IndexFunc(exported.Cases, func(c ordinarySourceCase) bool { return c.ID == registration.Cases[i].ID })
		if index < 0 {
			t.Fatalf("witness has no case %s", registration.Cases[i].ID)
		}
		wc := exported.Cases[index]
		pins := make([]OrdinaryPin, len(wc.Steps))
		for step, hash := range wc.Steps {
			pins[step] = OrdinaryPin{Path: "source-witness/" + hash + ".utf8", Bytes: uint64(len(versions[hash])), SHA256: hash}
		}
		registration.Cases[i].VersionPins, registration.Cases[i].Points, registration.Cases[i].InitialSource = pins, append([]EditPoints{}, wc.Points...), pins[0]
	}
	registrationBytes := ordinaryJSON(t, registration)
	read := func(path string) []byte {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	bodies := map[string][]byte{}
	for path, body := range assetBodies {
		bodies[path] = body
	}
	queryTexts := ordinaryRouteQueries(t, "postgresql-sql")
	for _, query := range registration.Queries {
		bodies[query.Source.Path] = []byte(queryTexts[query.ID])
	}
	for _, member := range registration.PlannedMembers {
		bodies[member.Pin.Path] = []byte("synthetic reference library; never loaded")
	}
	planReq := OrdinaryPlanRequest{Registry: req.Registry, Qualification: req.QualificationInventory, Slots: req.SlotInventory, Witness: witness,
		Profile: planningProfile, Assets: assetData, RegistrySHA256: ordinaryRegistrySHA, Registrations: [][]byte{registrationBytes}, Bodies: bodies,
		Basis: map[string][]byte{"src/dev/query-reference/check.py": read("../dev/query-reference/check.py"), "src/internal/native/query_policy_test.go": read("../internal/native/query_policy_test.go")}}
	// A zero-edit case has nil points in the witness and empty points in the plan and in this registration.
	plan, err := BuildOrdinaryShardPlan(planReq)
	if err != nil {
		t.Fatalf("BuildOrdinaryShardPlan: %v", err)
	}
	const caseID = "alt-postgresql-sql-B04-22"
	selected := slices.IndexFunc(plan.Shards, func(s OrdinaryPlanShard) bool {
		return s.Route == "postgresql-sql" && s.Platform == "windows-amd64" && s.Case == caseID
	})
	if selected < 0 {
		t.Fatalf("real plan has no shard for %s", caseID)
	}
	planBytes := ordinaryJSON(t, plan)
	parsed, err := ParseOrdinaryShardPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateOrdinaryShardPlan(parsed, planReq); err != nil {
		t.Fatalf("stored plan does not validate against its inputs: %v", err)
	}
	commitment, selection := ordinarySelectionForTest(t, plan, []int{selected})
	selection.Binding.ClosureEncoding = "parser-paged"
	selection.Binding.Profile = queryregistrycore.FilePin("profile/executable.json", profileBytes)
	request := req
	request.Selection = OrdinaryQuerySemanticsBinding(selection.Binding)
	request.PlanningProfile, request.Registration, request.AssetRegistration = planningProfile, registrationBytes, assetData
	request.PlanCommitment, request.ShardProofs = commitment, selection.Proofs
	request.Profile = profileBytes
	return ordinaryCLIInputs{PlanReq: planReq, Plan: plan, PlanBytes: planBytes, Request: request, Registration: registration, RegistrationBytes: registrationBytes,
		Profile: profileBytes, FactPack: packBytes, FactMapping: mapping, Source: source, GrammarRoot: grammarRoot, AssetBodies: assetBodies,
		ExpectedTree: expectedTree, Metadata: metadata, ShardID: plan.Shards[selected].ID, CaseID: caseID}
}

// ordinaryCLIWriteBundle lays the inputs of `tsgk oracle record` out under dir: the ordinary root (the
// pinned P0 documents, the planning profile, assets, registration and every declared body), the case
// root, and the trusted documents outside both roots.
func ordinaryCLIWriteBundle(t *testing.T, dir string, in ordinaryCLIInputs, witness []byte) {
	t.Helper()
	put := func(rel string, data []byte) {
		t.Helper()
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("ordinary/"+queryregistrycore.Path, in.PlanReq.Registry)
	put("ordinary/"+queryregistrycore.QualificationPin.Path, in.PlanReq.Qualification)
	put("ordinary/"+queryregistrycore.SlotPin.Path, in.PlanReq.Slots)
	for path, data := range in.PlanReq.Basis {
		put("ordinary/"+path, data)
	}
	put("ordinary/src/contracts/fact-mapping.json", in.FactMapping)
	put("ordinary/"+in.Plan.Profile.Path, in.PlanReq.Profile)
	put("ordinary/"+in.Plan.Assets.Path, in.PlanReq.Assets)
	put("ordinary/"+in.Plan.Registrations[0].Path, in.RegistrationBytes)
	for path, data := range in.PlanReq.Bodies {
		put("ordinary/"+path, data)
	}
	put("cases/cases/"+in.CaseID+".sql", in.Source)
	put("witness.json", witness)
	put("plan.json", in.PlanBytes)
	put("oracle-profile.json", in.Profile)
	put("fact-pack.json", in.FactPack)
}
