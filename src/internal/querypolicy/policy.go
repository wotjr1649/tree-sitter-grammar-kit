// Package querypolicy implements the data-only tsgk-predicates/r1 filter.
package querypolicy

import (
	"bytes"
	"slices"
)

const (
	Revision         = "tsgk-predicates/r1"
	InvalidMetadata  = "QUERY_PREDICATE_INVALID"
	InvalidCapture   = "QUERY_PREDICATE_CAPTURE_INVALID"
	AbsentCapture    = "QUERY_PREDICATE_CAPTURE_ABSENT"
	AmbiguousCapture = "QUERY_PREDICATE_CAPTURE_AMBIGUOUS"
)

type Predicate struct {
	Pattern uint32 `json:"pattern"`
	Steps   []Step `json:"steps"`
}

type Step struct {
	Kind       string  `json:"kind"`
	Value      string  `json:"value"`
	Quantifier *string `json:"quantifier"`
}

// Capture is the view needed for filtering; callers retain the complete stream.
type Capture struct {
	Match, Pattern     uint32
	Name               string
	StartByte, EndByte uint32
}

func validMetadata(patterns uint32, predicates []Predicate) bool {
	for _, p := range predicates {
		if p.Pattern >= patterns || len(p.Steps) == 0 || p.Steps[0].Kind != "string" {
			return false
		}
		for _, step := range p.Steps {
			switch step.Kind {
			case "string":
				if step.Quantifier != nil {
					return false
				}
			case "capture":
				if step.Quantifier == nil || step.Value == "" {
					return false
				}
				switch *step.Quantifier {
				case "ZERO", "ZERO_OR_ONE", "ZERO_OR_MORE", "ONE", "ONE_OR_MORE":
				default:
					return false
				}
			default:
				return false
			}
		}
	}
	return true
}

// Support validates metadata before examining policy operands. Caller owns envelope,
// stream completeness, query/source identity and outer byte/count limits.
func Support(patterns uint32, predicates []Predicate, encoding string) string {
	if !validMetadata(patterns, predicates) {
		return InvalidMetadata
	}
	single := func(s Step) bool { return s.Kind == "capture" && *s.Quantifier == "ONE" }
	for _, p := range predicates {
		op, args := p.Steps[0].Value, p.Steps[1:]
		unsupported := "QUERY_PREDICATE_UNSUPPORTED:" + op
		switch op {
		case "eq?", "not-eq?":
			if len(args) != 2 || !single(args[0]) || (args[1].Kind == "capture" && !single(args[1])) {
				return unsupported
			}
		case "any-of?", "not-any-of?":
			if len(args) < 2 || !single(args[0]) {
				return unsupported
			}
			for _, a := range args[1:] {
				if a.Kind != "string" {
					return unsupported
				}
			}
		default:
			return unsupported
		}
		if encoding != "UTF-8" {
			return "QUERY_PREDICATE_ENCODING"
		}
	}
	return ""
}

// Filter returns original indices without sorting or renumbering. Invalid evidence
// returns no indices; a completed empty stream returns a non-nil empty slice.
func Filter(patterns uint32, predicates []Predicate, captures []Capture, encoding string, source []byte) ([]int, string) {
	reason := Support(patterns, predicates, encoding)
	if reason == InvalidMetadata {
		return nil, reason
	}
	if captures == nil {
		return nil, InvalidCapture
	}
	matches := map[uint32][]Capture{}
	order := []uint32{}
	for _, capture := range captures {
		if capture.Pattern >= patterns || capture.Name == "" || capture.StartByte > capture.EndByte || uint64(capture.EndByte) > uint64(len(source)) {
			return nil, InvalidCapture
		}
		group := matches[capture.Match]
		if len(group) == 0 {
			order = append(order, capture.Match)
		} else if group[0].Pattern != capture.Pattern {
			return nil, InvalidCapture
		}
		matches[capture.Match] = append(group, capture)
	}
	if reason != "" {
		return nil, reason
	}
	byPattern := map[uint32][]Predicate{}
	for _, p := range predicates {
		byPattern[p.Pattern] = append(byPattern[p.Pattern], p)
	}
	keep := map[uint32]bool{}
	for _, id := range order {
		group := matches[id]
		text := func(name string) ([]byte, string) {
			var value []byte
			found := false
			for _, c := range group {
				if c.Name != name {
					continue
				}
				if found {
					return nil, AmbiguousCapture
				}
				found, value = true, source[c.StartByte:c.EndByte]
			}
			if !found {
				return nil, AbsentCapture
			}
			return value, ""
		}
		ok := true
		for _, p := range byPattern[group[0].Pattern] {
			op, args := p.Steps[0].Value, p.Steps[1:]
			left, code := text(args[0].Value)
			if code != "" {
				return nil, code
			}
			var result bool
			switch op {
			case "eq?", "not-eq?":
				right := []byte(args[1].Value)
				if args[1].Kind == "capture" {
					right, code = text(args[1].Value)
					if code != "" {
						return nil, code
					}
				}
				result = bytes.Equal(left, right) == (op == "eq?")
			default:
				member := slices.ContainsFunc(args[1:], func(a Step) bool { return a.Value == string(left) })
				result = member == (op == "any-of?")
			}
			ok = ok && result
		}
		keep[id] = ok
	}
	indices := []int{}
	for i, c := range captures {
		if keep[c.Match] {
			indices = append(indices, i)
		}
	}
	return indices, ""
}
