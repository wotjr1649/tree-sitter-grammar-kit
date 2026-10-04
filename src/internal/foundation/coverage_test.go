package foundation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

// The production alternatives registry is the mechanical boundary of "every production
// alternative of the cited sections" (language-feature-disposition.md): coverage rule r2
// covers a row's P only when the row is COMPLETE and each alternative has a case.
const (
	alternativesPath = "src/contracts/feature-alternatives.json"
	alternativesRev  = "tsgk-feature-alternatives/r1"
	samplesNotice    = "src/testdata/native/samples/NOTICE.md"
)

type featureAlternative struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"` // production | fact
	VariantOf  *string `json:"variant_of"`
	Fact       string  `json:"fact"`
	Production string  `json:"production"`
	Ref        string  `json:"ref"`
}

type featureAlternativesRow struct {
	Status       string               `json:"status"` // COMPLETE | PENDING
	Alternatives []featureAlternative `json:"alternatives"`
}

type featureAlternatives struct {
	Schema string                                       `json:"schema"`
	Routes map[string]map[string]featureAlternativesRow `json:"routes"`
}

var altSuffix = regexp.MustCompile(`^\.a[0-9]{2,}$`)

func decodeAlternatives(data []byte) (featureAlternatives, error) {
	var a featureAlternatives
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	err := d.Decode(&a)
	return a, err
}

// checkAlternatives validates the registry against the disposition's REQ rows (route →
// row → kinds with "#order", as requirementRows returns them).
func checkAlternatives(a featureAlternatives, reqs map[string]map[string][]string) error {
	if a.Schema != alternativesRev {
		return fmt.Errorf("schema %q", a.Schema)
	}
	ids := map[string]bool{}
	var variants []featureAlternative
	for _, route := range sortedRoutes(a.Routes) {
		for _, row := range sortedRoutes(a.Routes[route]) {
			if _, ok := reqs[route][row]; !ok || row == "#order" {
				return fmt.Errorf("%s/%s is not a REQ row of the disposition", route, row)
			}
			r := a.Routes[route][row]
			switch {
			case r.Status != kit.AlternativesComplete && r.Status != kit.AlternativesPending:
				return fmt.Errorf("%s: status %q", row, r.Status)
			case r.Alternatives == nil:
				return fmt.Errorf("%s: alternatives missing", row)
			case r.Status == kit.AlternativesComplete && len(r.Alternatives) == 0:
				return fmt.Errorf("%s: COMPLETE row without alternatives", row)
			}
			for _, x := range r.Alternatives {
				if !strings.HasPrefix(x.ID, row) || !altSuffix.MatchString(x.ID[len(row):]) {
					return fmt.Errorf("%s: alternative id %q is not prefixed by its row", row, x.ID)
				}
				if ids[x.ID] {
					return fmt.Errorf("%s: duplicate alternative id %s", row, x.ID)
				}
				ids[x.ID] = true
				if u, err := url.Parse(x.Ref); err != nil || u.Scheme != "https" || u.Host == "" {
					return fmt.Errorf("%s: ref %q is not an https URL", x.ID, x.Ref)
				}
				switch {
				case x.Kind == "production" && strings.TrimSpace(x.Production) == "":
					return fmt.Errorf("%s: production alternative without production", x.ID)
				case x.Kind == "fact" && strings.TrimSpace(x.Fact) == "":
					return fmt.Errorf("%s: fact alternative without fact", x.ID)
				case x.Kind != "production" && x.Kind != "fact":
					return fmt.Errorf("%s: kind %q", x.ID, x.Kind)
				}
				if x.VariantOf != nil {
					variants = append(variants, x)
				}
			}
		}
	}
	for _, x := range variants {
		if *x.VariantOf == x.ID || !ids[*x.VariantOf] {
			return fmt.Errorf("%s: variant_of %q is not another registered alternative", x.ID, *x.VariantOf)
		}
	}
	for route, rows := range reqs {
		for _, row := range rows["#order"] {
			if _, ok := a.Routes[route][row]; !ok {
				return fmt.Errorf("REQ row %s missing from the registry", row)
			}
		}
	}
	return nil
}

func sortedRoutes[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestFeatureAlternatives checks the tracked registry lists exactly the disposition's REQ
// rows with well-formed alternatives, and that each mutation is detected.
func TestFeatureAlternatives(t *testing.T) {
	root := repository(t)
	data := mustRead(t, filepath.Join(root, alternativesPath))
	reqs := requirementRows(t, string(mustRead(t, filepath.Join(root, "docs/validation/language-feature-disposition.md"))))
	reg, err := decodeAlternatives(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkAlternatives(reg, reqs); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeAlternatives(bytes.Replace(data, []byte(`"status"`), []byte(`"covered": true, "status"`), 1)); err == nil {
		t.Fatal("unknown registry field accepted")
	}
	// base for the mutations: one row COMPLETE with a production and a fact variant of it
	base := func() featureAlternatives {
		r, err := decodeAlternatives(data)
		if err != nil {
			t.Fatal(err)
		}
		a01 := "csharp-B01.a01"
		r.Routes["csharp"]["csharp-B01"] = featureAlternativesRow{Status: kit.AlternativesComplete, Alternatives: []featureAlternative{
			{ID: a01, Kind: "production", Production: "identifier : available_identifier | '@' identifier_or_keyword", Ref: "https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure"},
			{ID: "csharp-B01.a02", Kind: "fact", VariantOf: &a01, Fact: "escaped identifier", Ref: "https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure#identifiers"},
		}}
		return r
	}
	if err := checkAlternatives(base(), reqs); err != nil {
		t.Fatalf("control: %v", err)
	}
	row := func(r *featureAlternatives) *featureAlternativesRow {
		x := r.Routes["csharp"]["csharp-B01"]
		return &x
	}
	set := func(r *featureAlternatives, x *featureAlternativesRow) { r.Routes["csharp"]["csharp-B01"] = *x }
	other := "csharp-B01.a09"
	self := "csharp-B01.a02"
	for _, tc := range []struct {
		name, diagnostic string
		mutate           func(*featureAlternatives)
	}{
		{"schema", "schema", func(r *featureAlternatives) { r.Schema = "tsgk-feature-alternatives/r0" }},
		{"missing-row", "missing from the registry", func(r *featureAlternatives) { delete(r.Routes["html"], "html-B02") }},
		{"missing-route", "missing from the registry", func(r *featureAlternatives) { delete(r.Routes, "yaml") }},
		{"non-req-row", "not a REQ row", func(r *featureAlternatives) {
			r.Routes["html"]["html-S01"] = featureAlternativesRow{Status: kit.AlternativesPending, Alternatives: []featureAlternative{}}
		}},
		{"unknown-row", "not a REQ row", func(r *featureAlternatives) {
			r.Routes["csharp"]["csharp-B99"] = featureAlternativesRow{Status: kit.AlternativesPending, Alternatives: []featureAlternative{}}
		}},
		{"row-under-other-route", "not a REQ row", func(r *featureAlternatives) {
			r.Routes["java"]["csharp-B01"] = r.Routes["csharp"]["csharp-B01"]
		}},
		{"status", "status", func(r *featureAlternatives) { x := row(r); x.Status = "DONE"; set(r, x) }},
		{"null-alternatives", "alternatives missing", func(r *featureAlternatives) {
			r.Routes["html"]["html-B01"] = featureAlternativesRow{Status: kit.AlternativesPending}
		}},
		{"complete-empty", "COMPLETE row without", func(r *featureAlternatives) {
			r.Routes["html"]["html-B01"] = featureAlternativesRow{Status: kit.AlternativesComplete, Alternatives: []featureAlternative{}}
		}},
		{"foreign-prefix", "not prefixed by its row", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].ID = "csharp-B02.a01"; set(r, x) }},
		{"bad-suffix", "not prefixed by its row", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].ID = "csharp-B01-1"; set(r, x) }},
		{"duplicate-id", "duplicate alternative id", func(r *featureAlternatives) { x := row(r); x.Alternatives[1].ID = x.Alternatives[0].ID; set(r, x) }},
		{"http-ref", "not an https URL", func(r *featureAlternatives) {
			x := row(r)
			x.Alternatives[0].Ref = "http://example.org/spec"
			set(r, x)
		}},
		{"relative-ref", "not an https URL", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].Ref = "spec.html#ids"; set(r, x) }},
		{"empty-ref", "not an https URL", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].Ref = ""; set(r, x) }},
		{"kind", "kind", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].Kind = "example"; set(r, x) }},
		{"production-empty", "without production", func(r *featureAlternatives) { x := row(r); x.Alternatives[0].Production = " "; set(r, x) }},
		{"fact-empty", "without fact", func(r *featureAlternatives) { x := row(r); x.Alternatives[1].Fact = ""; set(r, x) }},
		{"variant-unknown", "variant_of", func(r *featureAlternatives) { x := row(r); x.Alternatives[1].VariantOf = &other; set(r, x) }},
		{"variant-self", "variant_of", func(r *featureAlternatives) { x := row(r); x.Alternatives[1].VariantOf = &self; set(r, x) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base()
			tc.mutate(&r)
			if err := checkAlternatives(r, reqs); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("mutation not detected as %q: %v", tc.diagnostic, err)
			}
		})
	}
}

// sampleRef is one registered W sample of a case source file.
type sampleRef struct {
	file, id string
	sample   kit.QualSample
}

// checkSampleNotices requires a NOTICE entry for every sample: a "## <repository>@<commit>"
// heading whose section names the sample's SPDX license id.
func checkSampleNotices(samples []sampleRef, notice string) error {
	sections := map[string]string{}
	for _, s := range strings.Split("\n"+notice, "\n## ")[1:] {
		head, body, _ := strings.Cut(s, "\n")
		sections[strings.TrimSpace(head)] = body
	}
	for _, s := range samples {
		body, ok := sections[s.sample.Repository+"@"+s.sample.Commit]
		if !ok {
			return fmt.Errorf("%s %s: no NOTICE entry for %s@%s", s.file, s.id, s.sample.Repository, s.sample.Commit)
		}
		if !strings.Contains(body, s.sample.License) {
			return fmt.Errorf("%s %s: NOTICE entry without license %s", s.file, s.id, s.sample.License)
		}
	}
	return nil
}

// TestSampleNotices checks every case that registers a W sample has its NOTICE entry, with
// controls for a missing entry and a missing license.
func TestSampleNotices(t *testing.T) {
	root := repository(t)
	var samples []sampleRef
	for _, kind := range []string{"routes", "gaps", "n461"} {
		entries, err := os.ReadDir(filepath.Join(root, "src", "testdata", "native", kind))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			p := "src/testdata/native/" + kind + "/" + e.Name()
			var f caseFile
			if err := json.Unmarshal(mustRead(t, filepath.Join(root, filepath.FromSlash(p))), &f); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			for _, c := range f.Cases {
				if c.Sample != nil {
					samples = append(samples, sampleRef{file: p, id: c.ID, sample: *c.Sample})
				}
			}
		}
	}
	notice := string(mustRead(t, filepath.Join(root, filepath.FromSlash(samplesNotice))))
	if err := checkSampleNotices(samples, notice); err != nil {
		t.Fatal(err)
	}
	s := sampleRef{file: "routes/x.json", id: "x-sample", sample: kit.QualSample{Repository: "owner/name", Commit: strings.Repeat("a", 40), Path: "src/a.txt", License: "MIT"}}
	if err := checkSampleNotices([]sampleRef{s}, notice); err == nil || !strings.Contains(err.Error(), "no NOTICE entry") {
		t.Fatalf("sample without NOTICE entry: %v", err)
	}
	entry := notice + "\n## owner/name@" + strings.Repeat("a", 40) + "\n\n`src/a.txt`, MIT\n\nCopyright (c) Owner\n"
	if err := checkSampleNotices([]sampleRef{s}, entry); err != nil {
		t.Fatalf("control: %v", err)
	}
	other := s
	other.sample.Commit = strings.Repeat("b", 40)
	if err := checkSampleNotices([]sampleRef{other}, entry); err == nil {
		t.Fatal("entry of another commit accepted")
	}
	other = s
	other.sample.License = "Apache-2.0"
	if err := checkSampleNotices([]sampleRef{other}, entry); err == nil || !strings.Contains(err.Error(), "without license") {
		t.Fatalf("entry without the license: %v", err)
	}
}
