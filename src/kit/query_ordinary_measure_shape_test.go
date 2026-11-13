package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ordinaryMeasureShape is the tree shape of one measured case: the node count, the depth sum and maximum, and
// the named-node count per source step (incremental tree). It is read from the stored phase-A native r8.
type ordinaryMeasureShape struct {
	Route, Case string
	Nodes       []int
	DepthSum    []int
	DepthMax    []int
	Named       []int
	Fields      []int
}

// TestOrdinaryMeasureShape re-reads the stored phase-A r8 of every measured case below TSGK_MEASURE_SHAPE_ROOT
// (out/<run>/measure) and writes shape.json beside its measure.json. No native run.
func TestOrdinaryMeasureShape(t *testing.T) {
	root := os.Getenv("TSGK_MEASURE_SHAPE_ROOT")
	if root == "" {
		t.Skip("set TSGK_MEASURE_SHAPE_ROOT")
	}
	ordinaryPreparedRoutes(t)
	paths, err := filepath.Glob(filepath.Join(root, "*", "*", "phase-a", "owned-producer"))
	if err != nil {
		t.Fatal(err)
	}
	for _, producer := range paths {
		caseDir := filepath.Dir(filepath.Dir(producer))
		if _, err := os.Stat(filepath.Join(caseDir, "shape.json")); err == nil {
			continue
		}
		route, caseID := filepath.Base(filepath.Dir(caseDir)), filepath.Base(caseDir)
		t.Run(route+"/"+caseID, func(t *testing.T) {
			in := ordinaryMeasureInputsFor(t, route, caseID, OrdinaryAssetRegistration{}, ordinaryMeasureClosure(route))
			checked, versions, _ := ordinaryMeasureActual(t, in, producer)
			shape := ordinaryMeasureShape{Route: route, Case: caseID}
			for step := range versions {
				nodes := checked.Steps[step].Incremental.Nodes
				depth := make([]int, len(nodes))
				sum, maxd, named, fields := 0, 0, 0, 0
				for i, n := range nodes {
					if n.Parent >= 0 {
						depth[i] = depth[n.Parent] + 1
					}
					sum += depth[i]
					maxd = max(maxd, depth[i])
					if n.Named {
						named++
					}
					if n.Field != nil {
						fields++
					}
				}
				shape.Nodes, shape.DepthSum, shape.DepthMax = append(shape.Nodes, len(nodes)), append(shape.DepthSum, sum), append(shape.DepthMax, maxd)
				shape.Named, shape.Fields = append(shape.Named, named), append(shape.Fields, fields)
			}
			data, err := json.Marshal(shape)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(caseDir, "shape.json"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}
