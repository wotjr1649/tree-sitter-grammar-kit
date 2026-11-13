package kit

import (
	"encoding/json"
	"slices"
	"strings"
)

// The views of the r2 expected asset for the two audits of the differential-oracle alternatives (#233).
//
// The position audit may know the witness of an alternative and nothing else; the type audit the expected types and nothing
// else (the decision of 2026-11-13, R-4: the two stay independent of what the other compares). The independence is made
// structural here: this file is the only place where the r2 asset is read for the audits, and it hands each audit a narrow
// value (G1Audit76Input, G1TypeAudit76Input) with no member for the other's data. The audit files take those values and
// nothing more, so a function of an audit cannot name, and the separation test shows it cannot reach, the expected asset,
// the correction records or the kind map. The view takes the old value of a correction from the correction record, so
// the record is read here and only here.

func g1ViewFault(code string) error { return fail(KindInvalidInput, code, CSharpG1Audit76Path, nil) }

// g1ViewDifferential returns the r2 alternatives of the differential-oracle stratum, by id, with the corrections.
func g1ViewDifferential(expected []byte) (CSharpG1Expected, []G1ExpAlt, error) {
	x, err := DecodeCSharpG1Expected(expected)
	if err != nil {
		return x, nil, err
	}
	if x.Schema != CSharpG1ExpectedSchemaR2 {
		return x, nil, g1ViewFault("G1_AUDIT76_EXPECTED")
	}
	var alts []G1ExpAlt
	for _, a := range x.Alternatives {
		if a.Status == G1ExpExpected && a.Basis == G1BasisDiff {
			alts = append(alts, a)
		}
	}
	slices.SortFunc(alts, func(a, b G1ExpAlt) int { return strings.Compare(a.ID, b.ID) })
	return x, alts, nil
}

// g1ViewInitialWitness is the witness of the alternative before the corrections of the r2 asset that moved or relaxed it.
func g1ViewInitialWitness(item string, w G1ExpWitness, cs []G1Correction) (G1ExpWitness, error) {
	for _, c := range cs {
		if c.Item != item {
			continue
		}
		switch c.Field {
		case G1FieldEvidence:
			var o g1cEvidence
			if err := json.Unmarshal([]byte(c.Old), &o); err != nil {
				return w, g1ViewFault("G1_AUDIT76_CORRECTION")
			}
			w = o.Witness
		case G1FieldWitnessMatch:
			var m string
			if err := json.Unmarshal([]byte(c.Old), &m); err != nil {
				return w, g1ViewFault("G1_AUDIT76_CORRECTION")
			}
			w.Match = m
		}
	}
	return w, nil
}

// g1ViewInitialTS is the ts of the alternative before the ts corrections of the r2 asset.
func g1ViewInitialTS(item string, ts []string, cs []G1Correction) ([]string, error) {
	for _, c := range cs {
		if c.Item == item && c.Field == G1FieldTS {
			var old []string
			if err := json.Unmarshal([]byte(c.Old), &old); err != nil {
				return ts, fail(KindInvalidInput, "G1_TYPEAUDIT76_CORRECTION", CSharpG1TypeAudit76Path, nil)
			}
			ts = old
		}
	}
	return ts, nil
}

func g1ViewWitness(w G1ExpWitness) G1Audit76Witness {
	return G1Audit76Witness{Source: w.Source, Match: w.Match, Start: w.Start, End: w.End}
}

// G1Audit76View is the input of the position audit made from the r2 asset: the witnesses (as they are and as they were
// before the corrections) and the text of the sources.
func G1Audit76View(expected []byte, texts map[string][]byte) (G1Audit76Input, error) {
	out := G1Audit76Input{Texts: texts}
	x, alts, err := g1ViewDifferential(expected)
	if err != nil {
		return out, err
	}
	for _, a := range alts {
		if len(a.Witnesses) != 1 {
			return out, g1ViewFault("G1_AUDIT76_WITNESS")
		}
		initial, err := g1ViewInitialWitness(a.ID, a.Witnesses[0], x.Corrections)
		if err != nil {
			return out, err
		}
		out.Alts = append(out.Alts, G1Audit76Alt{ID: a.ID, Witness: g1ViewWitness(a.Witnesses[0]), Initial: g1ViewWitness(initial)})
	}
	return out, nil
}

// G1TypeAudit76View is the input of the type audit made from the r2 asset: the expected types (as they are and as they were
// before the ts corrections).
func G1TypeAudit76View(expected []byte) (G1TypeAudit76Input, error) {
	var out G1TypeAudit76Input
	x, alts, err := g1ViewDifferential(expected)
	if err != nil {
		return out, err
	}
	for _, a := range alts {
		initial, err := g1ViewInitialTS(a.ID, a.TS, x.Corrections)
		if err != nil {
			return out, err
		}
		out.Alts = append(out.Alts, G1TypeAudit76Alt{ID: a.ID, TS: slices.Clone(a.TS), InitialTS: slices.Clone(initial)})
	}
	return out, nil
}
