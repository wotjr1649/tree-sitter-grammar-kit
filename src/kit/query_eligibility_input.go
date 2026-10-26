package kit

import (
	"fmt"
	"strings"
)

func (s *eligibilitySession) input(inv *eligibilityInvocation, name, kind string, index *uint64, literal, sha string, bytes *uint64) (bool, *Error) {
	x, ok := inv.Inputs[name]
	if !ok {
		s.finding("ELIGIBILITY_INPUT_MISSING", inv.ID+"/"+name, false)
		return false, nil
	}
	good := x.Kind == kind
	if kind == "NOT_APPLICABLE" {
		if !good {
			s.finding("ELIGIBILITY_INPUT_APPLICABILITY", inv.ID+"/"+name, true)
		}
		return good, nil
	}
	if x.ArgvIndex == nil || index == nil || *x.ArgvIndex != *index || x.PathLiteral != literal {
		good = false
	}
	if kind == "PINNED" {
		_, present, e := s.member(x.MemberPin)
		if e != nil {
			return false, e
		}
		good = good && present
		if x.MemberPin == nil || sha != "" && x.MemberPin.Member.SHA256 != sha || bytes != nil && x.MemberPin.Member.Bytes != *bytes {
			good = false
		}
	}
	if !good {
		s.finding("ELIGIBILITY_INPUT_BINDING", inv.ID+"/"+name, true)
	}
	return good, nil
}

func (s *eligibilitySession) inputSlots(inv *eligibilityInvocation, allowed map[string]bool) bool {
	good := true
	for name := range inv.Inputs {
		if !allowed[name] {
			s.finding("ELIGIBILITY_INPUT_UNKNOWN", inv.ID+"/"+name, true)
			good = false
		}
	}
	return good
}

func eligibilityIndex(i int) *uint64 { n := uint64(i); return &n }

func (s *eligibilitySession) fixedInputs(inv *eligibilityInvocation, args []string, paths map[string]string) (bool, *Error) {
	allowed := map[string]bool{"source": true, "library": true}
	good := true
	if len(args) == 0 {
		return false, nil
	}
	if inv.CatalogueID == "compile" {
		if ok, e := s.input(inv, "source", "NOT_APPLICABLE", nil, "", "", nil); e != nil {
			return false, e
		} else {
			good = good && ok
		}
		if ok, e := s.input(inv, "library", "NOT_APPLICABLE", nil, "", "", nil); e != nil {
			return false, e
		} else {
			good = good && ok
		}
		allowed["grammar-parser"], allowed["output-library"] = true, true
		parser := s.catalogue.GrammarInputs["parser.c"]
		if ok, e := s.input(inv, "grammar-parser", "PINNED", eligibilityIndex(5), args[5], parser.SHA256, &parser.Bytes); e != nil {
			return false, e
		} else {
			good = good && ok
		}
		if ok, e := s.input(inv, "output-library", "PINNED", eligibilityIndex(7), args[7], paths["LH"], nil); e != nil {
			return false, e
		} else {
			good = good && ok
		}
	} else {
		batch := args[0] == "--batch"
		sourceIndex, libraryIndex := 2, 0
		if batch {
			sourceIndex, libraryIndex = 3, 1
		}
		base := paths["C"]
		sep := "/"
		if s.cohorts[inv.CohortID].Platform == "windows/amd64" {
			sep = "\\"
		}
		sourceName := strings.TrimPrefix(args[sourceIndex], base+sep)
		source, found := s.catalogue.Inputs[sourceName]
		if !found {
			return false, nil
		}
		if ok, e := s.input(inv, "source", "PINNED", eligibilityIndex(sourceIndex), args[sourceIndex], source.SHA256, &source.Bytes); e != nil {
			return false, e
		} else {
			good = good && ok
		}
		if ok, e := s.input(inv, "library", "PINNED", eligibilityIndex(libraryIndex), args[libraryIndex], paths["LH"], nil); e != nil {
			return false, e
		} else {
			good = good && ok
		}
		queryIndices := []int{3}
		if batch {
			queryIndices = nil
			for k := 6; k < len(args); k += 2 {
				queryIndices = append(queryIndices, k)
			}
		}
		for n, k := range queryIndices {
			name := fmt.Sprintf("query-%d", n)
			allowed[name] = true
			path := args[k]
			queryName := strings.TrimPrefix(path, base+sep)
			if queryName == "missing.scm" {
				if ok, e := s.input(inv, name, "EXPECTED_ABSENT", eligibilityIndex(k), path, "", nil); e != nil {
					return false, e
				} else {
					good = good && ok
				}
				continue
			}
			row, found := s.catalogue.Inputs[queryName]
			if !found {
				return false, nil
			}
			if ok, e := s.input(inv, name, "PINNED", eligibilityIndex(k), path, row.SHA256, &row.Bytes); e != nil {
				return false, e
			} else {
				good = good && ok
			}
		}
	}
	return s.inputSlots(inv, allowed) && good, nil
}
