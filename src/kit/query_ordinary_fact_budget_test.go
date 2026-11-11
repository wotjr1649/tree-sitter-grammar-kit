package kit

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/querywire"
)

func ordinaryFactBudgetQuery(caps []Capture) querywire.Query {
	q := querywire.Query{Captures: [][]int64{}}
	names, types := map[string]int{}, map[string]int{}
	for _, c := range caps {
		var flags int64
		if c.Named {
			flags |= 1
		}
		name, ok := names[c.Name]
		if !ok {
			name = len(q.CaptureNames)
			names[c.Name] = name
			q.CaptureNames = append(q.CaptureNames, c.Name)
		}
		typeID, ok := types[c.Type]
		if !ok {
			typeID = len(q.Types)
			types[c.Type] = typeID
			q.Types = append(q.Types, c.Type)
		}
		q.Captures = append(q.Captures, []int64{int64(c.Match), 0, int64(name), c.Node, int64(typeID), flags, int64(c.StartByte), int64(c.EndByte), 0, 0, 0, 0})
	}
	return q
}

func TestOrdinaryBaseFactReservationIgnoresUnusedSource(t *testing.T) {
	for _, kind := range []string{FactDeclarations, FactXMLStructure} {
		t.Run(kind, func(t *testing.T) {
			q := ordinaryFactBudgetQuery([]Capture{{Name: "long." + strings.Repeat("한", 4096), Type: "node"}})
			reserve := func(source []byte) (uint64, uint64) {
				b := semanticTestBudget(t)
				if e := b.ordinaryBaseFactReservation(FactQuery{Facts: kind}, FactPackRoute{}, q, source); e != nil {
					t.Fatal(e)
				}
				return b.work, b.logical
			}
			w, l := reserve(nil)
			if w2, l2 := reserve([]byte(strings.Repeat("unused", 100000))); w != w2 || l != l2 {
				t.Fatalf("unused source changed reservation: %d/%d -> %d/%d", w, l, w2, l2)
			}
		})
	}
}

func TestOrdinaryBaseDeclarationBudgetStopsBeforeHugeDepth(t *testing.T) {
	b := semanticTestBudget(t)
	caps := []Capture{{Name: "c.0." + strconv.Itoa(int(^uint(0)>>1)) + ".0"}}
	_, e := deriveDeclarations([]NativeDeclaration{{Name: "node"}}, caps, b)
	if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" || b.work > 1024 {
		t.Fatalf("huge depth was not refused before its loop: work=%d error=%v", b.work, e)
	}
}

func ordinaryFactBranchingDeclarations(depth int) ([]NativeDeclaration, []Capture) {
	items := []NativeDeclaration{{Fact: "decl", Node: "node", Name: strings.TrimSuffix(strings.Repeat("children:node/", depth), "/")}}
	caps := []Capture{{Name: "c.0.0.0", Node: 1, EndByte: 1}}
	match := uint32(1)
	parents := []int64{1}
	for level := 1; level <= depth; level++ {
		children := []int64{int64(2 * level), int64(2*level + 1)}
		for _, p := range parents {
			for _, c := range children {
				caps = append(caps, Capture{Match: match, Name: fmt.Sprintf("c.0.%d.%d", level, level-1), Node: p, EndByte: 1}, Capture{Match: match, Name: fmt.Sprintf("c.0.%d.%d", level, level), Node: c, EndByte: 1})
				match++
			}
		}
		parents = children
	}
	return items, caps
}

func TestOrdinaryBaseDeclarationBudgetStopsFrontierExpansion(t *testing.T) {
	items, caps := ordinaryFactBranchingDeclarations(24)
	b := semanticTestBudget(t)
	b.work = eligibilityWorkLimit - 200000
	_, e := deriveDeclarations(items, caps, b)
	if e == nil || e.Kind != KindResourceLimit || e.Code != "ELIGIBILITY_WORK_LIMIT" || b.work > eligibilityWorkLimit {
		t.Fatalf("branching frontier escaped its budget: work=%d error=%v", b.work, e)
	}
}
