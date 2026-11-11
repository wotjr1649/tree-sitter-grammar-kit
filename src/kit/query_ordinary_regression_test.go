package kit

import (
	"bytes"
	"testing"
)

func TestOrdinaryMetadataFailurePrecedesResourceInterruption(t *testing.T) {
	id, actual, source, metadata, _, captures := ordinaryAdjudicatorFixture(t)
	actual.Queries[0].Predicates = []QueryPredicate{{Pattern: 0, Steps: []QueryPredicateStep{}}}
	oversizedTree := bytes.Repeat([]byte(" "), int(MaxDocumentBytes)+1)
	judgment, e := adjudicateOrdinaryTuple(ordinaryTestBudget(t), id, actual, source, ordinaryJSON(t, metadata), oversizedTree, ordinaryJSON(t, captures), nil, nil)
	if e == nil || e.Kind != KindResourceLimit || judgment.State != ordinaryTupleFailed || judgment.Code != "ORDINARY_STRUCTURAL_PREDICATES" {
		t.Fatalf("resource interruption erased known metadata failure: %+v %v", judgment, e)
	}
}

func TestOrdinaryCaptureOrderAndDuplicateCannotSelfApprove(t *testing.T) {
	for _, mutation := range []string{"reverse", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			id, actual, source, metadata, tree, captures := ordinaryAdjudicatorFixture(t)
			if mutation == "reverse" {
				actual.Queries[0].Captures[0], actual.Queries[0].Captures[1] = actual.Queries[0].Captures[1], actual.Queries[0].Captures[0]
				captures.Ordered[0], captures.Ordered[1] = captures.Ordered[1], captures.Ordered[0]
			} else {
				actual.Queries[0].Captures = append(actual.Queries[0].Captures, actual.Queries[0].Captures[1])
				captures.Ordered = append(captures.Ordered, captures.Ordered[1])
			}
			got := ordinaryAdjudicateTest(t, id, actual, source, metadata, tree, captures)
			if got.State != ordinaryTupleFailed {
				t.Fatalf("internally matching malformed capture stream was accepted: %+v", got)
			}
		})
	}
}
