package kit

// PreprocessSelectionDisjoint reports an unobservable selection switch: no unchanged
// active byte remains active, and at least one unchanged byte switches active/inactive.
// The caller must recompute both steps from the source and validate the edit payload.
// Replacement bytes and directive lines are not evidence of a selection switch.
func PreprocessSelectionDisjoint(old, cur PreprocessStep, edit Edit, oldBytes, newBytes uint64) bool {
	if old.Status != PreprocessOK || cur.Status != PreprocessOK || oldBytes > uint64(ppDefaultLimits.Bytes) || newBytes > uint64(ppDefaultLimits.Bytes) ||
		edit.OldEndByte < edit.StartByte || edit.NewEndByte < edit.StartByte || uint64(edit.OldEndByte) > oldBytes || uint64(edit.NewEndByte) > newBytes {
		return false
	}
	delta := int64(edit.NewEndByte) - int64(edit.OldEndByte)
	if int64(oldBytes)+delta != int64(newBytes) {
		return false
	}
	overlaps := func(a, b []Span) bool {
		for _, x := range a {
			for _, y := range b {
				if x.StartByte < y.EndByte && y.StartByte < x.EndByte {
					return true
				}
			}
		}
		return false
	}
	valid := func(s PreprocessStep, size uint64) bool {
		if len(s.Active) == 0 || len(s.Active) > ppDefaultLimits.Ranges || len(s.Inactive) > ppDefaultLimits.Directives+1 {
			return false
		}
		for _, spans := range [][]Span{s.Active, s.Inactive} {
			var end uint32
			for _, r := range spans {
				if r.StartByte < end || r.StartByte >= r.EndByte || uint64(r.EndByte) > size {
					return false
				}
				end = r.EndByte
			}
		}
		return !overlaps(s.Active, s.Inactive)
	}
	if !valid(old, oldBytes) || !valid(cur, newBytes) {
		return false
	}
	// Map only the unchanged prefix/suffix; an insertion has two adjacent old cuts.
	mappedOverlap := func(before, after []Span) bool {
		for _, r := range before {
			parts := [2][2]int64{
				{int64(r.StartByte), min(int64(r.EndByte), int64(edit.StartByte))},
				{max(int64(r.StartByte), int64(edit.OldEndByte)) + delta, int64(r.EndByte) + delta},
			}
			for _, p := range parts {
				if p[0] >= p[1] {
					continue
				}
				for _, s := range after {
					if p[0] < int64(s.EndByte) && int64(s.StartByte) < p[1] {
						return true
					}
				}
			}
		}
		return false
	}
	return !mappedOverlap(old.Active, cur.Active) && (mappedOverlap(old.Active, cur.Inactive) || mappedOverlap(old.Inactive, cur.Active))
}

// selectionRouteBlocked uses the source-derived selection and the registered edit. Raw
// projection/observation validation remains mandatory after this route calculation.
func (x *replayEnv) selectionRouteBlocked(c *rcCase, step int, want *IncrementalCase) bool {
	if want == nil || step < 1 || step >= len(c.Steps) || len(x.selectionSteps) != len(c.Steps) || len(x.selectionBytes) != len(c.Steps) || step > len(want.Edits) {
		return false
	}
	old, cur := c.Steps[step-1], c.Steps[step]
	r, e := cur.Route, cur.Edit
	full := func(t *rcTree) bool {
		return t != nil && t.Status == StatusCompleted && t.Form == "full" && t.Tree != nil
	}
	if r == nil || !r.EditHasChanges || r.Proven || r.ReusedNodes != 0 || r.FreshReusedNodes != 0 || !full(old.Incremental) || !full(cur.Incremental) || !full(cur.Fresh) ||
		e == nil || e.StartByte == nil || e.OldEndByte == nil || e.NewEndByte == nil {
		return false
	}
	registered := want.Edits[step-1]
	if *e.StartByte != registered.StartByte || *e.OldEndByte != registered.OldEndByte || *e.NewEndByte != registered.NewEndByte {
		return false
	}
	return PreprocessSelectionDisjoint(x.selectionSteps[step-1], x.selectionSteps[step], registered, x.selectionBytes[step-1], x.selectionBytes[step])
}
