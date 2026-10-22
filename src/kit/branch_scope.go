package kit

// branchSelectionRoutes are the routes whose default parser picks a conditional-compilation
// section without evaluating symbols (#218): the registered C# grammar (C2 patch).
var branchSelectionRoutes = map[string]bool{"csharp": true}

// branchSelectionConditions are the C# nodes whose "condition" field selects a section.
var branchSelectionConditions = map[string]bool{"preproc_if": true, "preproc_elif": true, "preproc_if_in_attribute_list": true}

// branchSelectionUnclaimed reports whether a recorded case's branch selection is outside the
// claim: some recorded tree (incremental, fresh or SVC inline, every step) has a condition
// whose subtree names an identifier, or a recorded tree whose nodes are not there to
// inspect. A tree the case does not have (fresh at step 0, no inline C#) is skipped. No
// mark does not show that a literal condition was evaluated correctly.
func branchSelectionUnclaimed(c *rcCase) bool {
	for _, s := range c.Steps {
		for _, t := range []*rcTree{s.Incremental, s.Fresh} {
			if t != nil && (t.Tree == nil || conditionNamesSymbol(t.Tree.Nodes)) {
				return true
			}
		}
		if s.Composite != nil && s.Composite.Inline != nil && (s.Composite.Inline.Tree == nil || conditionNamesSymbol(s.Composite.Inline.Tree.Nodes)) {
			return true
		}
	}
	return false
}

// conditionNamesSymbol walks a preorder node table once: a node is inside a condition when
// it is the condition field of a conditional node or a descendant of one.
func conditionNamesSymbol(nodes []TreeNode) bool {
	inside := make([]bool, len(nodes))
	for i, n := range nodes {
		if n.Parent >= 0 && n.Parent < int64(i) {
			p := n.Parent
			inside[i] = inside[p] || n.Field != nil && *n.Field == "condition" && branchSelectionConditions[nodes[p].Type]
		}
		if inside[i] && n.Type == "identifier" {
			return true
		}
	}
	return false
}
