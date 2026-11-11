package queryregistry

import (
	"errors"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type Subject struct {
	Status     string
	Code       string
	Evaluation string
	Partial    bool
	Structural string
	Expected   string
}

type Judgment struct {
	ID         string
	Coordinate Coordinate
	State      string
	Code       string
	Subject    *Subject
}

type Shard struct {
	ID             string
	RegistrySHA256 string
	Judgments      []Judgment
}

type Counter struct {
	Kind        string `json:"kind"`
	Scope       string `json:"scope"`
	Platform    string `json:"platform"`
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

type Coverage struct {
	Counters          []Counter `json:"counters"`
	Verdict           string    `json:"verdict"`
	Code              string    `json:"code"`
	FullScopeComplete bool      `json:"full_scope_complete"`
}

// checkShards checks coordinates as well as hashes. Missing IDs are allowed only for partial folding.
func (v *Validated) checkShards(shards []Shard, exact bool) (map[string]Judgment, error) {
	seenShards := map[string]bool{}
	seen := map[string]Judgment{}
	for _, shard := range shards {
		if shard.ID == "" || seenShards[shard.ID] || shard.RegistrySHA256 != v.sha {
			return nil, invalid("REGISTRY_SHARD_BINDING", nil)
		}
		seenShards[shard.ID] = true
		for _, j := range shard.Judgments {
			c, exists := v.byID[j.ID]
			if !exists || c != j.Coordinate || c.ID(v.registry.Namespace) != j.ID {
				return nil, invalid("REGISTRY_UNDECLARED_COORDINATE", nil)
			}
			if _, duplicate := seen[j.ID]; duplicate {
				return nil, invalid("REGISTRY_SHARD_OVERLAP", nil)
			}
			seen[j.ID] = j
		}
	}
	if exact && len(seen) != len(v.byID) {
		return nil, invalid("REGISTRY_SHARD_GAP", nil)
	}
	return seen, nil
}

// VerifyExact requires a disjoint union equal to the original required ID/coordinate set.
func (v *Validated) VerifyExact(shards []Shard) error {
	_, err := v.checkShards(shards, true)
	return err
}

// Fold consumes adjudicated states, not runtime claims. P0 never opens the full completion gate.
// missingInput blocks every unseen known obligation; an interruption leaves unseen inputs unprocessed.
func (v *Validated) Fold(shards []Shard, missingInput bool) (Coverage, error) {
	var out Coverage
	// A contradiction or tampering is a FAIL that still reports the first failure already
	// confirmed by a well-formed judgment before it, never an empty coverage (#362 review M2).
	fail := func(err error) (Coverage, error) {
		code := v.firstConfirmedFailure(shards)
		var ke *kit.Error
		if code == "" && errors.As(err, &ke) {
			code = ke.Code
		}
		return Coverage{Verdict: "FAIL", Code: code}, err
	}
	judgments, err := v.checkShards(shards, false)
	if err != nil {
		return fail(err)
	}
	indices := map[string]int{}
	for _, set := range v.registry.RequiredSets {
		key := set.Kind + "/" + set.Scope + "/" + set.Platform
		indices[key] = len(out.Counters)
		out.Counters = append(out.Counters, Counter{Kind: set.Kind, Scope: set.Scope, Platform: set.Platform, Required: set.Required})
	}
	out.Verdict, out.Code = "BLOCKED", "FULL_SCOPE_INCOMPLETE"
	// Preserve the first confirmed failure in consumption order, including after interruption.
	for _, shard := range shards {
		for _, judgment := range shard.Judgments {
			if judgment.State == "FAILED" && out.Verdict != "FAIL" {
				out.Verdict, out.Code = "FAIL", judgment.Code
			}
		}
	}
	missingAssets := v.missingAssets()
	for _, coordinate := range v.coordinates {
		counter := &out.Counters[indices[setKey(coordinate)]]
		j, exists := judgments[coordinate.ID(v.registry.Namespace)]
		if !exists {
			if missingInput || missingAssets && coordinate.Kind != Control {
				counter.Blocked++
			} else {
				counter.Unprocessed++
			}
			continue
		}
		if err := v.checkJudgment(coordinate, j, missingAssets); err != nil {
			return fail(err)
		}
		switch j.State {
		case "VERIFIED":
			counter.Verified++
		case "FAILED":
			counter.Failed++
			if out.Verdict != "FAIL" {
				out.Verdict, out.Code = "FAIL", j.Code
			}
		case "BLOCKED":
			counter.Blocked++
		case "UNPROCESSED":
			if missingInput || missingAssets && coordinate.Kind != Control {
				counter.Blocked++
			} else {
				counter.Unprocessed++
			}
		}
	}
	return out, nil
}

func (v *Validated) missingAssets() bool {
	for _, asset := range v.registry.Assets {
		if asset.State == "UNAVAILABLE" {
			return true
		}
	}
	return false
}

// checkJudgment validates one judgment's state, code and subject against its coordinate.
func (v *Validated) checkJudgment(coordinate Coordinate, j Judgment, missingAssets bool) error {
	switch j.State {
	case "VERIFIED":
		// shortcut: large subjects are blocked in P0; add subject checks with the large adjudicator.
		if missingAssets && coordinate.Kind != Control || j.Code != "" {
			return invalid("REGISTRY_UNAVAILABLE_VERIFIED", nil)
		}
		if coordinate.Kind == Control {
			var probe *Probe
			for i := range v.registry.Probes {
				if v.registry.Probes[i].ID == coordinate.Case {
					probe = &v.registry.Probes[i]
					break
				}
			}
			if probe == nil || j.Subject == nil || *j.Subject != (Subject{probe.Status, probe.Code, probe.Evaluation, probe.Partial, probe.Structural, probe.Expected}) {
				return invalid("REGISTRY_CONTROL_SUBJECT", nil)
			}
		} else if j.Subject != nil {
			return invalid("REGISTRY_POSITIVE_RECLASSIFIED", nil)
		}
	case "FAILED":
		if j.Code == "" {
			return invalid("REGISTRY_FAILURE_CODE", nil)
		}
	case "BLOCKED":
		if j.Code == "" {
			return invalid("REGISTRY_BLOCKED_CODE", nil)
		}
	case "UNPROCESSED":
	default:
		return invalid("REGISTRY_COVERAGE_STATE", nil)
	}
	return nil
}

// firstConfirmedFailure is the code of the first FAILED judgment, in consumption order, whose
// shard binding, coordinate, state, code and subject are valid and follow only valid judgments;
// anything after the first defect is not confirmed.
func (v *Validated) firstConfirmedFailure(shards []Shard) string {
	seenShards := map[string]bool{}
	seen := map[string]bool{}
	missingAssets := v.missingAssets()
	for _, shard := range shards {
		if shard.ID == "" || seenShards[shard.ID] || shard.RegistrySHA256 != v.sha {
			return ""
		}
		seenShards[shard.ID] = true
		for _, j := range shard.Judgments {
			c, exists := v.byID[j.ID]
			if !exists || c != j.Coordinate || c.ID(v.registry.Namespace) != j.ID || seen[j.ID] || v.checkJudgment(c, j, missingAssets) != nil {
				return ""
			}
			seen[j.ID] = true
			if j.State == "FAILED" && j.Code != "" {
				return j.Code
			}
		}
	}
	return ""
}
