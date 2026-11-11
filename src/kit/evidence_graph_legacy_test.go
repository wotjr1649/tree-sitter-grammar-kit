package kit

// Pinned 1c18f93 graph oracle preserves finding order for the indexed validator.

import (
	"fmt"
	"slices"
)

func legacyEvidenceGraph(x *replayEnv, nodes map[string]*EvidenceNode, pol evidencePolicy, kept map[string][]byte, snapshot *evidenceSnapshot) *Error {
	add := func(code, path, msg string) { x.finding(code, path, msg) }
	markNode := func(id string) {
		if snapshot != nil && snapshot.wantedNodes[id] {
			snapshot.invalidNodes[id] = true
		}
	}
	nodeBad := func(code, id, msg string) {
		markNode(id)
		add(code, id, msg)
	}
	// references, relations and cycles
	allowed := map[string]map[string][]string{
		"run":    {"supersedes": {"run", "record", "carry"}},
		"record": {"supersedes": {"run", "record"}},
		"replay": {"subject": {"run", "record"}},
		"carry":  {"carries": {"run"}},
	}
	for _, id := range sortedKeys(nodes) {
		n := nodes[id]
		counts := map[string]int{}
		for _, ref := range n.Refs {
			t := nodes[ref.Node]
			switch {
			case t == nil:
				nodeBad("REF_UNRESOLVED", id, "참조한 node가 없다: "+ref.Node)
			case !slices.Contains(allowed[n.Kind][ref.Relation], t.Kind):
				nodeBad("REF_RELATION_INVALID", id, fmt.Sprintf("%s는 %s 관계로 %s를 참조할 수 없다", n.Kind, ref.Relation, t.Kind))
			default:
				counts[ref.Relation]++
			}
		}
		if n.Kind == "replay" && counts["subject"] != 1 {
			nodeBad("REPLAY_SUBJECT_REQUIRED", id, "replay는 subject를 정확히 하나 참조한다")
		}
		if n.Kind == "carry" && counts["carries"] != 1 {
			nodeBad("CARRY_ORIGIN_REQUIRED", id, "승계는 원 결과를 정확히 하나 참조한다")
		}
	}
	if cyc := legacyFindCycle(nodes); cyc != "" {
		add("GRAPH_CYCLE", cyc, "참조가 순환한다")
		if snapshot != nil {
			for id := range snapshot.wantedNodes {
				if legacyFindCycleFrom(nodes, []string{id}) != "" {
					markNode(id)
				}
			}
		}
	}
	sameRun := func(a, b *EvidenceNode) bool {
		return a.Identities["run"] == b.Identities["run"] && a.Identities["attempt"] == b.Identities["attempt"] && a.Identities["platform"] == b.Identities["platform"]
	}
	ids := sortedKeys(nodes)
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			na, nb := nodes[a], nodes[b]
			// a run and a historical record of the same run identity are one execution
			// labelled twice, as are two runs
			if na.Kind != "replay" && na.Kind != "carry" && nb.Kind != "replay" && nb.Kind != "carry" && sameRun(na, nb) {
				markNode(a)
				markNode(b)
				add("RUN_IDENTITY_DUPLICATE", a+","+b, "같은 run·attempt·platform이 두 결과로 기록됐다(relabel)")
			}
		}
	}
	for _, id := range ids {
		n := nodes[id]
		for _, ref := range n.Refs {
			t := nodes[ref.Node]
			if t == nil {
				continue
			}
			switch ref.Relation {
			case "supersedes":
				if sameRun(n, t) {
					nodeBad("RUN_RELABELED", id, "새 결과가 이전 결과와 같은 run identity로 그 결과를 대체한다")
				}
			case "subject":
				if n.Identities["run"] == t.Identities["run"] && n.Identities["attempt"] == t.Identities["attempt"] {
					nodeBad("REPLAY_RELABELS_SUBJECT", id, "replay가 subject run과 같은 실행 identity를 쓴다")
				}
				for _, role := range []string{"source", "input", "workload", "platform"} {
					if n.Identities[role] != t.Identities[role] {
						nodeBad("REPLAY_SUBJECT_MISMATCH", id, "replay와 subject의 identity가 다르다: "+role)
					}
				}
				if e := x.checkReplayFile(n, t, kept, add); e != nil {
					return e
				}
			case "carries":
				legacyCheckCarry(n, t, pol, nodeBad)
			}
		}
		if n.Kind == "carry" {
			authorized := false
			for _, rule := range pol.CarryForward {
				authorized = authorized || rule.Node == id
			}
			if !authorized {
				nodeBad("CARRY_FORWARD_UNAUTHORIZED", id, "승계를 허가한 policy 규칙이 없다(기본 거부)")
			}
		}
	}
	for _, req := range pol.Required {
		n := nodes[req.Node]
		if n == nil || n.Kind != req.Kind {
			nodeBad("REQUIRED_NODE_MISSING", req.Node, "필수 node가 없거나 종류가 다르다")
			continue
		}
		for _, role := range sortedKeys(req.Identities) {
			if n.Identities[role] != req.Identities[role] {
				nodeBad("IDENTITY_MISMATCH", req.Node, "필수 node의 identity가 policy와 다르다(stale 또는 혼합): "+role)
			}
		}
	}
	if el := pol.Eligibility; el != nil {
		for _, id := range el.Nodes {
			n := nodes[id]
			if n == nil {
				nodeBad("ELIGIBILITY_NODE_MISSING", id, "자격 판정 대상 node가 없다")
			} else if !slices.Contains(el.Modes, n.EvidenceMode) {
				nodeBad("ELIGIBILITY_REJECTED", id, "이 자격은 "+fmt.Sprint(el.Modes)+" evidence만 받는다: "+n.EvidenceMode)
			}
		}
	}
	return nil
}

// checkCarry applies the adopted relation: authorized for this node and origin, every
// dependency identity unchanged and non-empty where the origin binds one, the origin a
// completed new run, the assessment unchanged.
func legacyCheckCarry(n, origin *EvidenceNode, pol evidencePolicy, add func(code, path, msg string)) {
	for _, rule := range pol.CarryForward {
		if rule.Node != n.ID {
			continue
		}
		if rule.Origin != origin.ID || rule.Relation != CarryUnchangedDependency || rule.AuthorizedBy == "" {
			add("CARRY_RELATION_UNSUPPORTED", n.ID, "승계 규칙의 원 결과·관계·허가가 채택 관계와 다르다")
		}
	}
	for _, role := range dependencyRoles {
		if n.Identities[role] != origin.Identities[role] {
			add("CARRY_DEPENDENCY_CHANGED", n.ID, "원 결과와 dependency identity가 다르다: "+role)
		}
	}
	if origin.ExecutionStatus != StatusCompleted || origin.EvidenceMode != ModeNewRun {
		add("CARRY_ORIGIN_INCOMPLETE", n.ID, "원 결과가 완료된 새 실행이 아니다")
	}
	if n.Assessment != origin.Assessment {
		add("CARRY_ASSESSMENT_CHANGED", n.ID, "승계가 원 결과의 판정을 바꿨다")
	}
}

// legacyFindCycle returns a node on a reference cycle, or "".
func legacyFindCycle(nodes map[string]*EvidenceNode) string {
	return legacyFindCycleFrom(nodes, sortedKeys(nodes))
}

func legacyFindCycleFrom(nodes map[string]*EvidenceNode, roots []string) string {
	state := map[string]int{}
	var visit func(id string) string
	visit = func(id string) string {
		switch state[id] {
		case 1:
			return id
		case 2:
			return ""
		}
		state[id] = 1
		if n := nodes[id]; n != nil {
			for _, r := range n.Refs {
				if nodes[r.Node] != nil {
					if c := visit(r.Node); c != "" {
						return c
					}
				}
			}
		}
		state[id] = 2
		return ""
	}
	for _, id := range roots {
		if c := visit(id); c != "" {
			return c
		}
	}
	return ""
}
