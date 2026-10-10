package kit

import "testing"

func TestQualifyRetainsSegmentProcessObservations(t *testing.T) {
	for _, top := range []string{"absent", "smaller", "larger"} {
		t.Run(top, func(t *testing.T) {
			f := newQfx(t)
			host := func(target bool) qfxMut {
				return qfxMut{record: func(set string, recs []map[string]any) {
					if set != "s06-fxa" {
						return
					}
					for _, c := range recs {
						c["process"] = nil
						if !target || top != "absent" {
							wall, peak := 7, 7
							if target && top == "larger" {
								wall, peak = 2222, 3333
							}
							c["process"] = map[string]any{"wall_ms": wall, "memory": map[string]any{"peak_bytes": peak}}
						}
						wallA, peakA, wallB, peakB := 1, 1, 2, 2
						if target {
							wallA, peakA, wallB, peakB = 999, 77, 888, 999
						}
						c["segments"] = []any{map[string]any{"process": map[string]any{"wall_ms": wallA, "memory": map[string]any{"peak_bytes": peakA}}}, map[string]any{"process": map[string]any{"wall_ms": wallB, "memory": map[string]any{"peak_bytes": peakB}}}, map[string]any{"process": nil}}
					}
				}}
			}
			r := f.run(t, f.all(t, map[string]qfxMut{"linux-amd64": host(true), "windows-amd64": host(false), "darwin-arm64": host(false)}))
			c := cellOf(r, "fxa", "linux-amd64")
			wall, peak := "999", "999"
			if top == "larger" {
				wall, peak = "2222", "3333"
			}
			if c.Set.Host["process_wall_ms_max"] != wall || c.Set.Host["memory_peak_bytes_max"] != peak || c.Comparison != AssessPass {
				t.Fatalf("segment observations omitted or compared: %+v comparison %s", c.Set.Host, c.Comparison)
			}
			if c.Mechanism != AssessFail {
				t.Fatal("unregistered segments must still fail the independent mechanism gate")
			}
		})
	}
}
