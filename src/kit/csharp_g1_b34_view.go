package kit

import (
	"slices"
	"strings"
)

// The views of the G1-3b expected asset for its two audits (#233). The position audit may know the witnesses and the text of the
// sources, the type audit the expected types, and neither knows what the other compares. This file is the only place where the
// expected asset is read for the audits; it hands each audit a narrow value with no member for the other's data (csharp_g1_b34_audit.go
// takes those values and nothing more, and the separation test shows it reaches no other G1 file).

func g1b34ViewFault(code string) error { return fail(KindInvalidInput, code, CSharpG1B34Path, nil) }

// G1B34AuditView is the input of the position audit: the witnesses of the EXPECTED alternatives, sorted by id, and the source text.
func G1B34AuditView(expected []byte, texts map[string][]byte) (G1B34AuditInput, error) {
	out := G1B34AuditInput{Texts: texts}
	x, err := DecodeCSharpG1B34(expected)
	if err != nil {
		return out, err
	}
	for _, a := range x.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		if len(a.Witnesses) != 1 {
			return out, g1b34ViewFault("G1_B34_WITNESS")
		}
		w := a.Witnesses[0]
		alt := G1B34AuditAlt{ID: a.ID, Group: a.Group, Witness: G1Audit76Witness{Source: w.Source, Match: w.Match}}
		switch a.Group {
		case G1ClassStructure:
			if len(w.Span) != 2 {
				return out, g1b34ViewFault("G1_B34_WITNESS")
			}
			alt.Witness.Start, alt.Witness.End = w.Span[0], w.Span[1]
		case G1ClassLiteral:
			for _, an := range a.Anchors {
				alt.Texts = append(alt.Texts, an.Text)
			}
			alt.Tokens = w.Tokens
		default:
			return out, g1b34ViewFault("G1_B34_WITNESS")
		}
		out.Alts = append(out.Alts, alt)
	}
	slices.SortFunc(out.Alts, func(a, b G1B34AuditAlt) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// G1B34TypeView is the input of the type audit: the expected types of the EXPECTED B alternatives, sorted by id, and the types of each
// anchor lexeme (the union over the A alternatives that have it).
func G1B34TypeView(expected []byte) (G1B34TypeInput, error) {
	var out G1B34TypeInput
	x, err := DecodeCSharpG1B34(expected)
	if err != nil {
		return out, err
	}
	anchors := map[string][]string{}
	for _, a := range x.Alternatives {
		if a.Status != G1ExpExpected {
			continue
		}
		switch a.Group {
		case G1ClassStructure:
			out.Alts = append(out.Alts, G1B34TypeAlt{ID: a.ID, TS: slices.Clone(a.TS)})
		case G1ClassLiteral:
			for _, an := range a.Anchors {
				anchors[an.Text] = append(anchors[an.Text], an.TS...)
			}
		}
	}
	slices.SortFunc(out.Alts, func(a, b G1B34TypeAlt) int { return strings.Compare(a.ID, b.ID) })
	for _, t := range g1SortedKeys(anchors) {
		ts := anchors[t]
		slices.Sort(ts)
		out.Anchors = append(out.Anchors, G1B34AnchorTS{Text: t, TS: slices.Compact(ts)})
	}
	return out, nil
}
