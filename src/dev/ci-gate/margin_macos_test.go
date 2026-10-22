package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarginFixedMainHistory(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ci-gate", "margin-main-39068412883-1.json"))
	if err != nil || digest(b) != "24ccddb1f7df38cd7d31e1df672298b2be78ef99f17d67bd3d6aca9ec65e88f4" {
		t.Fatal("original hosted margin fixture changed", err)
	}
	var past marginReport
	if err := decodeMarginReport(b, &past); err != nil {
		t.Fatal(err)
	}
	current, _, _ := marginFixture(t, "900")
	if failed, err := evaluateMargins(&current, []marginReport{past}); err != nil || failed {
		t.Fatalf("original main report rejected: failed=%v err=%v", failed, err)
	}
	for _, m := range current.Metrics {
		if m.Samples != 1 {
			t.Fatal("original main configuration increased current samples")
		}
	}
	if _, err := evaluateMargins(&past, nil); err == nil {
		t.Fatal("original main report accepted as current evidence")
	}
}

func TestMarginMacLegacyHistory(t *testing.T) {
	current, _, _ := marginFixture(t, "900")
	legacy, jobs, core := marginFixture(t, "1")
	for i := range legacy.Configs {
		c := &legacy.Configs[i]
		c.Source = strings.Repeat("e", 64)
		if c.Run.Job == "native-routes-macos" {
			c.Run.Job = "native-routes"
		}
	}
	for i := range legacy.Metrics {
		m := &legacy.Metrics[i]
		m.ID = strings.Replace(m.ID, "native-routes-macos/", "native-routes/", 1)
		for _, c := range legacy.Configs {
			if strings.HasPrefix(m.ID, c.Run.Job+"/"+c.Run.GOOS+"/") {
				m.Config = marginKey(c, m.ID, m.Limit)
			}
		}
	}
	// Exercise the original serialized layout without rewriting its recorded identity.
	var decoded marginReport
	if err := decodeMarginReport(encoded(t, legacy), &decoded); err != nil {
		t.Fatal(err)
	}
	if failed, err := evaluateMargins(&current, []marginReport{decoded}); err != nil || failed {
		t.Fatalf("valid legacy history: failed=%v err=%v", failed, err)
	}
	for _, m := range current.Metrics {
		if m.Samples != 1 {
			t.Fatal("different legacy configuration increased current samples")
		}
	}
	if _, err := evaluateMargins(&decoded, nil); err == nil {
		t.Fatal("legacy job accepted as current evidence")
	}
	if _, err := currentMargins(legacy.Run, legacy.Configs, jobs, core); err == nil {
		t.Fatal("legacy producer accepted in current API observations")
	}
	for name, mutate := range map[string]func(*marginReport){
		"forged-key":       func(r *marginReport) { r.Metrics[0].Config = strings.Repeat("0", 64) },
		"unknown-job":      func(r *marginReport) { r.Configs[6].Run.Job = "other-mac" },
		"missing-compiler": func(r *marginReport) { r.Configs[6].Compiler = nil },
		"current-source": func(r *marginReport) {
			for i := range r.Configs {
				r.Configs[i].Source = current.Configs[0].Source
			}
			for i := range r.Metrics {
				m := &r.Metrics[i]
				for _, c := range r.Configs {
					if strings.HasPrefix(m.ID, c.Run.Job+"/"+c.Run.GOOS+"/") {
						m.Config = marginKey(c, m.ID, m.Limit)
					}
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var bad marginReport
			if err := decodeMarginReport(encoded(t, legacy), &bad); err != nil {
				t.Fatal(err)
			}
			mutate(&bad)
			if _, err := evaluateMargins(&current, []marginReport{bad}); err == nil {
				t.Fatal("invalid legacy history accepted")
			}
		})
	}
}

func TestMarginMacHostBinding(t *testing.T) {
	r, _, _ := marginFixture(t, "900")
	c := r.Configs[6]
	if c.Run.Job != "native-routes-macos" || c.Run.GOOS != "darwin" || c.Compiler == nil {
		t.Fatal("Mac fixture must use the actual producer job and compiler")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "s05-routes-darwin-900-2")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	put := func(name string, value any) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), encoded(t, value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("run-identity.json", c.Run)
	put("summary.json", map[string]any{"compiler": c.Compiler})
	if err := bindMarginHost(c, root, r.Run); err != nil {
		t.Fatal(err)
	}
	wrong := c.Run
	wrong.Job = "native-routes"
	put("run-identity.json", wrong)
	if bindMarginHost(c, root, r.Run) == nil {
		t.Fatal("renamed raw host identity accepted")
	}
	put("run-identity.json", c.Run)
	put("summary.json", map[string]any{"compiler": fileDigest{strings.Repeat("0", 64), c.Compiler.Bytes}})
	if bindMarginHost(c, root, r.Run) == nil {
		t.Fatal("wrong Mac compiler accepted")
	}
}

// #260: a past report of another source under a former limit is valid history of another
// configuration; the same source must carry the current limits, a changed limit its own key
// and a positive bounded value, and the current report the current limits exactly.
func TestMarginFormerLimitHistory(t *testing.T) {
	current, _, _ := marginFixture(t, "900")
	former := func(limit int64, rekey, source bool) marginReport {
		past, _, _ := marginFixture(t, "1")
		for i := range past.Configs {
			if source {
				past.Configs[i].Source = strings.Repeat("e", 64)
			}
		}
		for i := range past.Metrics {
			m := &past.Metrics[i]
			if m.ID == "foundation/windows/job" {
				m.Limit = limit
			}
			for _, c := range past.Configs {
				if (rekey || m.ID != "foundation/windows/job") && strings.HasPrefix(m.ID, c.Run.Job+"/"+c.Run.GOOS+"/") {
					m.Config = marginKey(c, m.ID, m.Limit)
				}
			}
		}
		return past
	}
	if failed, err := evaluateMargins(&current, []marginReport{former(600000, true, true)}); err != nil || failed {
		t.Fatalf("former limit rejected: failed=%v err=%v", failed, err)
	}
	for _, m := range current.Metrics {
		if m.Samples != 1 {
			t.Fatalf("%s samples=%d: another source counted", m.ID, m.Samples)
		}
	}
	for name, past := range map[string]marginReport{"same-source": former(600000, true, false), "stale-key": former(600000, false, true),
		"zero": former(0, true, true), "unbounded": former(24*time.Hour.Milliseconds()+1, true, true)} {
		if _, err := evaluateMargins(&current, []marginReport{past}); err == nil {
			t.Fatalf("%s history accepted", name)
		}
	}
	for _, id := range []string{"foundation/windows/job", "foundation/windows/step/frame 상태와 native cancellation 반복 시험 (#65)", "large-api/windows/page-max"} {
		changed, _, _ := marginFixture(t, "900")
		found := false
		for i := range changed.Metrics {
			m := &changed.Metrics[i]
			if m.ID != id {
				continue
			}
			found = true
			m.Limit++
			for _, c := range changed.Configs {
				if strings.HasPrefix(m.ID, c.Run.Job+"/"+c.Run.GOOS+"/") {
					m.Config = marginKey(c, m.ID, m.Limit)
				}
			}
		}
		if _, err := evaluateMargins(&changed, nil); !found || err == nil {
			t.Fatalf("current report accepted under another limit: %s", id)
		}
	}
}
