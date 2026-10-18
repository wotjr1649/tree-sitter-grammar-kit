package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
