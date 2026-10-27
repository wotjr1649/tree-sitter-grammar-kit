// Package querypolicy implements the data-only tsgk-predicates/r1 filter.
package querypolicy

import (
	"bytes"
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

func validMetadata(check func() error, patterns uint32, predicates []Predicate) (bool, error) {
	for _, p := range predicates {
		if check != nil {
			if err := check(); err != nil {
				return false, err
			}
		}
		if p.Pattern >= patterns || len(p.Steps) == 0 || p.Steps[0].Kind != "string" {
			return false, nil
		}
		for _, step := range p.Steps {
			if check != nil {
				if err := check(); err != nil {
					return false, err
				}
			}
			switch step.Kind {
			case "string":
				if step.Quantifier != nil {
					return false, nil
				}
			case "capture":
				if step.Quantifier == nil || step.Value == "" {
					return false, nil
				}
				switch *step.Quantifier {
				case "ZERO", "ZERO_OR_ONE", "ZERO_OR_MORE", "ONE", "ONE_OR_MORE":
				default:
					return false, nil
				}
			default:
				return false, nil
			}
		}
	}
	return true, nil
}

// Support validates metadata before examining policy operands. Caller owns envelope,
// stream completeness, query/source identity and outer byte/count limits.
func Support(patterns uint32, predicates []Predicate, encoding string) string {
	code, _ := SupportChecked(nil, patterns, predicates, encoding)
	return code
}

// SupportChecked checks cancellation and budgets inside metadata and operand scans.
func SupportChecked(check func() error, patterns uint32, predicates []Predicate, encoding string) (string, error) {
	valid, err := validMetadata(check, patterns, predicates)
	if err != nil {
		return "", err
	}
	if !valid {
		return InvalidMetadata, nil
	}
	single := func(s Step) bool { return s.Kind == "capture" && *s.Quantifier == "ONE" }
	for _, p := range predicates {
		if check != nil {
			if err := check(); err != nil {
				return "", err
			}
		}
		op, args := p.Steps[0].Value, p.Steps[1:]
		unsupported := "QUERY_PREDICATE_UNSUPPORTED:" + op
		switch op {
		case "eq?", "not-eq?":
			if len(args) != 2 || !single(args[0]) || (args[1].Kind == "capture" && !single(args[1])) {
				return unsupported, nil
			}
		case "any-of?", "not-any-of?":
			if len(args) < 2 || !single(args[0]) {
				return unsupported, nil
			}
			for _, a := range args[1:] {
				if check != nil {
					if err := check(); err != nil {
						return "", err
					}
				}
				if a.Kind != "string" {
					return unsupported, nil
				}
			}
		default:
			return unsupported, nil
		}
		if encoding != "UTF-8" {
			return "QUERY_PREDICATE_ENCODING", nil
		}
	}
	return "", nil
}

// Filter returns original indices without sorting or renumbering. Invalid evidence
// returns no indices; a completed empty stream returns a non-nil empty slice.
func Filter(patterns uint32, predicates []Predicate, captures []Capture, encoding string, source []byte) ([]int, string) {
	indices, code, _ := FilterChecked(nil, patterns, predicates, captures, encoding, source)
	return indices, code
}

// FilterChecked preserves Filter's policy and original indices, with cancellation
// checks inside match/predicate scans rather than only between complete queries.
func FilterChecked(checkpoint func() error, patterns uint32, predicates []Predicate, captures []Capture, encoding string, source []byte) ([]int, string, error) {
	work := 0
	check := func() error {
		work++
		if checkpoint != nil && (work%1024 == 0 || work == 1) {
			return checkpoint()
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, "", err
	}
	reason, err := SupportChecked(check, patterns, predicates, encoding)
	if err != nil {
		return nil, "", err
	}
	if reason == InvalidMetadata {
		return nil, reason, nil
	}
	if captures == nil {
		return nil, InvalidCapture, nil
	}
	matches := map[uint32][]Capture{}
	order := []uint32{}
	for _, capture := range captures {
		if err := check(); err != nil {
			return nil, "", err
		}
		if capture.Pattern >= patterns || capture.Name == "" || capture.StartByte > capture.EndByte || uint64(capture.EndByte) > uint64(len(source)) {
			return nil, InvalidCapture, nil
		}
		group := matches[capture.Match]
		if len(group) == 0 {
			order = append(order, capture.Match)
		} else if group[0].Pattern != capture.Pattern {
			return nil, InvalidCapture, nil
		}
		matches[capture.Match] = append(group, capture)
	}
	if reason != "" {
		return nil, reason, nil
	}
	byPattern := map[uint32][]Predicate{}
	for _, p := range predicates {
		byPattern[p.Pattern] = append(byPattern[p.Pattern], p)
	}
	keep := map[uint32]bool{}
	for _, id := range order {
		group := matches[id]
		text := func(name string) ([]byte, string, error) {
			var value []byte
			found := false
			for _, c := range group {
				if err := check(); err != nil {
					return nil, "", err
				}
				if c.Name != name {
					continue
				}
				if found {
					return nil, AmbiguousCapture, nil
				}
				found, value = true, source[c.StartByte:c.EndByte]
			}
			if !found {
				return nil, AbsentCapture, nil
			}
			return value, "", nil
		}
		ok := true
		for _, p := range byPattern[group[0].Pattern] {
			if err := check(); err != nil {
				return nil, "", err
			}
			op, args := p.Steps[0].Value, p.Steps[1:]
			left, code, err := text(args[0].Value)
			if err != nil {
				return nil, "", err
			}
			if code != "" {
				return nil, code, nil
			}
			var result bool
			switch op {
			case "eq?", "not-eq?":
				right := []byte(args[1].Value)
				if args[1].Kind == "capture" {
					right, code, err = text(args[1].Value)
					if err != nil {
						return nil, "", err
					}
					if code != "" {
						return nil, code, nil
					}
				}
				result = bytes.Equal(left, right) == (op == "eq?")
			default:
				member := false
				for _, a := range args[1:] {
					if err := check(); err != nil {
						return nil, "", err
					}
					if a.Value == string(left) {
						member = true
						break
					}
				}
				result = member == (op == "any-of?")
			}
			ok = ok && result
		}
		keep[id] = ok
	}
	indices := []int{}
	for i, c := range captures {
		if err := check(); err != nil {
			return nil, "", err
		}
		if keep[c.Match] {
			indices = append(indices, i)
		}
	}
	if checkpoint != nil {
		if err := checkpoint(); err != nil {
			return nil, "", err
		}
	}
	return indices, "", nil
}
