package kit

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
)

func graphDifferential(t *testing.T, nodes []EvidenceNode, pol evidencePolicy) {
	t.Helper()
	indexed := map[string]*EvidenceNode{}
	wanted := map[string]bool{"absent": true}
	for i := range nodes {
		indexed[nodes[i].ID] = &nodes[i]
		wanted[nodes[i].ID] = true
	}
	old, current := &replayEnv{}, &replayEnv{}
	oldSnapshot := &evidenceSnapshot{wantedNodes: wanted, invalidNodes: map[string]bool{}}
	snapshot := &evidenceSnapshot{wantedNodes: wanted, invalidNodes: map[string]bool{}}
	oldErr := legacyEvidenceGraph(old, indexed, pol, nil, oldSnapshot)
	err := checkEvidenceGraph(current, indexed, pol, nil, snapshot)
	if !reflect.DeepEqual(err, oldErr) || !reflect.DeepEqual(current.findings, old.findings) || !reflect.DeepEqual(snapshot.invalidNodes, oldSnapshot.invalidNodes) {
		t.Fatalf("graph differential: errors %v / %v; findings %#v / %#v; owners %v / %v", err, oldErr, current.findings, old.findings, snapshot.invalidNodes, oldSnapshot.invalidNodes)
	}
}

func graphShape(size int, shape string) []EvidenceNode {
	nodes := make([]EvidenceNode, size)
	for i := range nodes {
		id := fmt.Sprintf("n%05d", i)
		nodes[i] = EvidenceNode{ID: id, Kind: "run", ExecutionStatus: StatusCompleted, EvidenceMode: ModeNewRun, Assessment: AssessPass, Identities: ident("p", id)}
		if i > 0 {
			target := i - 1
			if shape == "fan-out" {
				target = 0
			}
			nodes[i].Refs = []EvidenceRef{{Relation: "supersedes", Node: fmt.Sprintf("n%05d", target)}}
		}
	}
	if shape == "cycle" {
		nodes[0].Refs = []EvidenceRef{{Relation: "supersedes", Node: nodes[len(nodes)-2].ID}}
	}
	if shape == "near-cycle" {
		nodes[0].Refs = []EvidenceRef{{Relation: "supersedes", Node: "absent"}}
	}
	return nodes
}

func TestEvidenceGraphDifferential(t *testing.T) {
	for _, shape := range []string{"chain", "fan-out", "near-cycle", "cycle"} {
		for _, size := range []int{2, 128, 512} {
			t.Run(fmt.Sprintf("%s-%d", shape, size), func(t *testing.T) { graphDifferential(t, graphShape(size, shape), evidencePolicy{}) })
		}
	}
	rng := rand.New(rand.NewPCG(362, 380))
	for sample := range 400 {
		nodes := graphShape(2+rng.IntN(18), "fan-out")
		pol := evidencePolicy{}
		for i := range nodes {
			n := &nodes[i]
			n.Kind = []string{"run", "record", "carry", "replay", "unknown"}[rng.IntN(5)]
			n.Refs = nil
			for range rng.IntN(5) {
				target := fmt.Sprintf("n%05d", rng.IntN(len(nodes)+2))
				n.Refs = append(n.Refs, EvidenceRef{Relation: []string{"supersedes", "subject", "carries", "unknown"}[rng.IntN(4)], Node: target})
			}
			for _, role := range []string{"run", "attempt", "platform", "source"} {
				n.Identities[role] = fmt.Sprint(rng.IntN(3))
			}
			if rng.IntN(3) == 0 {
				pol.CarryForward = append(pol.CarryForward, evidenceCarryRule{Node: n.ID, Origin: nodes[rng.IntN(len(nodes))].ID, Relation: CarryUnchangedDependency, AuthorizedBy: "test"})
			}
		}
		t.Run(fmt.Sprintf("random-%d", sample), func(t *testing.T) { graphDifferential(t, nodes, pol) })
	}
}

func TestEvidenceGraphRunIdentity(t *testing.T) {
	for _, role := range []string{"run", "attempt", "platform"} {
		t.Run(role, func(t *testing.T) {
			nodes := graphShape(4, "fan-out")
			for i := range nodes {
				nodes[i].Refs = nil
				nodes[i].Identities = ident("p", "same")
			}
			nodes[1].Identities[role] = "different"
			graphDifferential(t, nodes, evidencePolicy{})
		})
	}
}

func TestEvidenceGraphCycleOwners(t *testing.T) {
	nodes := graphShape(8, "fan-out")
	nodes[0].Refs = []EvidenceRef{{Relation: "supersedes", Node: nodes[1].ID}}
	nodes[4].Refs = nil
	nodes[5].Refs = []EvidenceRef{{Relation: "supersedes", Node: nodes[4].ID}}
	nodes[6].Refs = []EvidenceRef{{Relation: "supersedes", Node: nodes[5].ID}, {Relation: "supersedes", Node: nodes[2].ID}}
	graphDifferential(t, nodes, evidencePolicy{})
}

func TestEvidenceGraphReservation(t *testing.T) {
	nodes := graphShape(8, "fan-out")
	for i := range nodes {
		nodes[i].Identities["run"] = fmt.Sprintf("%08d", i)
	}
	unique := semanticTestBudget(t)
	if e := unique.reserveGraph(nodes, evidencePolicy{}); e != nil {
		t.Fatal(e)
	}
	for i := range nodes {
		nodes[i].Identities["run"] = "00000000"
	}
	duplicates := semanticTestBudget(t)
	if e := duplicates.reserveGraph(nodes, evidencePolicy{}); e != nil {
		t.Fatal(e)
	}
	if duplicates.work-unique.work != 64*28 {
		t.Fatal("duplicate pair visits omitted", unique.work, duplicates.work)
	}
	for _, remaining := range []uint64{duplicates.work - 1, duplicates.work} {
		b := semanticTestBudget(t)
		b.work = eligibilityWorkLimit - remaining
		e := b.reserveGraph(nodes, evidencePolicy{})
		if (remaining == duplicates.work) != (e == nil) {
			t.Fatalf("reservation boundary: remaining=%d error=%v", remaining, e)
		}
	}
	pol := evidencePolicy{CarryForward: []evidenceCarryRule{{Node: "absent"}, {Node: "absent"}}}
	nodes[1].Refs = []EvidenceRef{{Relation: "carries", Node: nodes[0].ID}, {Relation: "carries", Node: nodes[0].ID}}
	unmatched := semanticTestBudget(t)
	if e := unmatched.reserveGraph(nodes, pol); e != nil {
		t.Fatal(e)
	}
	for i := range pol.CarryForward {
		pol.CarryForward[i].Node = nodes[1].ID
	}
	matched := semanticTestBudget(t)
	if e := matched.reserveGraph(nodes, pol); e != nil {
		t.Fatal(e)
	}
	if matched.work-unmatched.work != 64*4 {
		t.Fatal("matching carry rule visits omitted", unmatched.work, matched.work)
	}
	nodes[1].Files = make([]ReplayMember, 7)
	before := semanticTestBudget(t)
	if e := before.reserveGraph(nodes, pol); e != nil {
		t.Fatal(e)
	}
	nodes[1].Refs[0].Relation = "subject"
	after := semanticTestBudget(t)
	if e := after.reserveGraph(nodes, pol); e != nil {
		t.Fatal(e)
	}
	if after.work+64*2-before.work != 64*7 {
		t.Fatal("replay file scan visits omitted", before.work, after.work)
	}
	graphDifferential(t, nodes, pol)
}
