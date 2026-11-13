package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
)

// The P1 source-only witness export (query-witness build) carries the version bytes of every ordinary case.
// The measurement reads initial sources from it so that it can pick any case of the denominator; the tracked
// route fixtures hold only part of them. TSGK_MEASURE_WITNESS names the exported file. The parsed export is
// cached by sync.OnceValues so that no test helper assigns a package variable.
var ordinaryMeasureWitness = sync.OnceValues(func() (map[string]map[string]string, error) {
	data, err := os.ReadFile(os.Getenv("TSGK_MEASURE_WITNESS"))
	if err != nil {
		return nil, err
	}
	var export struct {
		Routes []struct {
			Route    string `json:"route"`
			Versions []struct {
				SHA256 string `json:"sha256"`
				Source []byte `json:"source_base64"`
			} `json:"source_versions"`
			Cases []struct {
				ID    string   `json:"id"`
				Steps []string `json:"step_source_sha256"`
			} `json:"cases"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, err
	}
	routes := map[string]map[string]string{}
	for _, r := range export.Routes {
		byHash := map[string]string{}
		for _, v := range r.Versions {
			byHash[v.SHA256] = string(v.Source)
		}
		cases := map[string]string{}
		for _, c := range r.Cases {
			if len(c.Steps) == 0 {
				continue
			}
			text, ok := byHash[c.Steps[0]]
			if !ok {
				return nil, fmt.Errorf("witness export has no initial version of %s/%s", r.Route, c.ID)
			}
			cases[c.ID] = text
		}
		routes[r.Route] = cases
	}
	return routes, nil
})

func ordinaryMeasureSources(t *testing.T, route string) (map[string]string, bool) {
	t.Helper()
	if os.Getenv("TSGK_MEASURE_WITNESS") == "" {
		return nil, false
	}
	routes, err := ordinaryMeasureWitness()
	if err != nil {
		t.Fatal(err)
	}
	cases, ok := routes[route]
	return cases, ok
}
