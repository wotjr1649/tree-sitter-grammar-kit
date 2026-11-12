package kit

import "reflect"

// Reference observations are compared with independent assets, never used as expected output.
func (b *eligibilityBudget) ordinaryReferenceExpected(id ordinaryTupleIdentity, ref *instrumentedReference, metadataData, treeData, captureData []byte) (ordinaryTupleJudgment, *Error) {
	j := &ordinaryJudgmentBuilder{}
	if ref == nil || len(metadataData) == 0 || len(treeData) == 0 || len(captureData) == 0 {
		j.block("ORDINARY_REFERENCE_EXPECTED_MISSING", "reference의 독립 기대 자산이 없다")
		return j.value(), nil
	}
	// The decoded assets are shared with the tuple judgment and read-only here.
	metadata, _, e := ordinaryExpectedDecode[QueryWitnessMetadata](b, "ordinary-reference-metadata", metadataData)
	if e != nil {
		return j.value(), e
	}
	actualMetadata := QueryWitnessMetadata{Patterns: ref.Query.Patterns, CaptureNames: ref.Query.CaptureNames, Predicates: ref.Query.Predicates}
	if !reflect.DeepEqual(*metadata, actualMetadata) || len(ref.Query.Predicates) != 0 {
		j.fail("ORDINARY_REFERENCE_METADATA", "reference metadata가 독립 structural-only 등록과 다르다")
	}
	tree, _, e := ordinaryExpectedDecode[ordinaryExpectedTree](b, "ordinary-reference-expected-tree", treeData)
	if e != nil {
		return j.value(), e
	}
	if !ordinaryTreeIdentity(tree.Schema, tree.Case, tree.Step, tree.TreeRole, tree.Source, id) {
		j.fail("ORDINARY_REFERENCE_TREE_BINDING", "reference expected tree가 선택 tuple에 연결되지 않는다")
	}
	captures, _, e := ordinaryExpectedDecode[ordinaryExpectedCaptures](b, "ordinary-reference-expected-captures", captureData)
	if e != nil {
		return j.value(), e
	}
	if !ordinaryCaptureIdentity(captures.Schema, captures.Case, captures.Query, captures.Step, captures.TreeRole, captures.Source, id) {
		j.fail("ORDINARY_REFERENCE_CAPTURE_BINDING", "reference expected capture가 선택 tuple에 연결되지 않는다")
	}
	if e := b.charge(uint64(len(tree.Tree)+len(captures.Ordered)+len(ref.Tree.Nodes)+len(ref.Query.Structural))+1, 0, "ordinary-reference-independent-compare"); e != nil {
		return j.value(), e
	}
	if fieldOnly, changed := ordinaryTreeFieldOnlyDifference(ref.Tree.Nodes, tree.Tree); changed {
		if fieldOnly {
			j.block("REFERENCE_CST_INELIGIBLE", "유효한 reference CST field가 독립 full CST와 다르다")
		} else {
			j.fail("ORDINARY_REFERENCE_CST", "reference full CST가 독립 기대값과 다르다")
		}
	}
	if captures.Matches != ref.Query.Matches || CompareCaptures(captures.Ordered, ref.Query.Structural) != nil {
		j.fail("ORDINARY_REFERENCE_CAPTURES", "reference의 ordered capture가 독립 기대값과 다르다")
	}
	return j.value(), nil
}
