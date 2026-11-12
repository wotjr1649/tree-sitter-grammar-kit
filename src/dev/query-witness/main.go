// Source-only P1 export. This command never runs a grammar or consumes query results.
package main

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/queryregistry"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const (
	maxFile   = 16 << 20
	maxSource = 65536
	maxRules  = 256 << 10
	rulesPath = "src/contracts/query-expectation-rules-r1.json"
)

type origin struct {
	File     queryregistry.Pin `json:"file"`
	Selector string            `json:"selector"`
	Case     string            `json:"case"`
}

type sourceCase struct {
	ID     string        `json:"id"`
	Source string        `json:"source_utf8"`
	Edits  []replacement `json:"edits"`
}

type replacement struct {
	Find    string `json:"find"`
	Replace string `json:"replace"`
}

type fixture struct {
	Cases []sourceCase `json:"cases"`
}

type queryFile struct {
	Queries []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	} `json:"queries"`
	Cases []struct {
		ID         string `json:"id"`
		SourceCase string `json:"source_case"`
	} `json:"cases"`
}

type recovered struct {
	data    []byte
	edits   []replacement
	origins []origin
}

type version struct {
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
	Source []byte `json:"source_base64"`
}

type witnessCase struct {
	ID       string           `json:"id"`
	Selector string           `json:"selector"`
	Origins  []origin         `json:"origins"`
	Steps    []string         `json:"step_source_sha256"`
	Points   []kit.EditPoints `json:"edit_points"`
}

type witnessRoute struct {
	Route    string            `json:"route"`
	Set      string            `json:"set"`
	Encoding string            `json:"encoding"`
	Context  string            `json:"context"`
	Grammar  []kit.NativeInput `json:"grammar"`
	Queries  []kit.QualQuery   `json:"queries"`
	Versions []version         `json:"source_versions"`
	Cases    []witnessCase     `json:"cases"`
}

type counts struct {
	Cases         int    `json:"cases"`
	Embedded      int    `json:"embedded"`
	Recovered     int    `json:"recovered"`
	Edits         int    `json:"edits"`
	Steps         int    `json:"steps"`
	TreeRoles     int    `json:"tree_roles"`
	Queries       int    `json:"queries"`
	InitialUnique int    `json:"initial_unique"`
	Versions      int    `json:"source_versions"`
	Bundles       int    `json:"query_source_bundles"`
	InitialBytes  uint64 `json:"initial_bytes"`
	StepBytes     uint64 `json:"step_bytes"`
	UniqueBytes   uint64 `json:"unique_version_bytes"`
}

type export struct {
	Schema        string            `json:"schema"`
	Registry      queryregistry.Pin `json:"registry"`
	Qualification queryregistry.Pin `json:"qualification_inventory"`
	Slots         queryregistry.Pin `json:"query_slot_inventory"`
	State         string            `json:"state"`
	Counts        counts            `json:"counts"`
	Routes        []witnessRoute    `json:"routes"`
}

type inputs struct {
	root     string
	files    map[string][]byte
	inv      kit.QualificationInventory
	reg      queryregistry.Registry
	registry []byte
}

func (in *inputs) read(path string) ([]byte, error) {
	if data, ok := in.files[path]; ok {
		return data, nil
	}
	// All paths are fixed below or selected from the immutable qualification input.
	if filepath.IsAbs(path) || strings.Contains(path, "\\") || strings.Contains(path, ":") || strings.Contains(path, "..") || !strings.HasPrefix(path, "src/") {
		return nil, fmt.Errorf("invalid repository input path %q", path)
	}
	f, err := os.Open(filepath.Join(in.root, filepath.FromSlash(path)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFile {
		return nil, fmt.Errorf("input exceeds 16 MiB: %s", path)
	}
	in.files[path] = data
	return data, nil
}

func (in *inputs) decode(path string, out any) error {
	data, err := in.read(path)
	if err != nil {
		return err
	}
	if err := jsonv2.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func load(root string) (*inputs, error) {
	in := &inputs{root: root, files: map[string][]byte{}}
	q, err := in.read(queryregistry.QualificationPin.Path)
	if err != nil {
		return nil, err
	}
	s, err := in.read(queryregistry.SlotPin.Path)
	if err != nil {
		return nil, err
	}
	in.inv, _, err = queryregistry.Sources(q, s)
	if err != nil {
		return nil, err
	}
	in.registry, err = in.read(queryregistry.Path)
	if err != nil {
		return nil, err
	}
	basis := map[string][]byte{}
	for _, path := range []string{"src/dev/query-reference/check.py", "src/internal/native/query_policy_test.go"} {
		basis[path], err = in.read(path)
		if err != nil {
			return nil, err
		}
	}
	if _, err = queryregistry.Validate(in.registry, queryregistry.Digest(in.registry), q, s, basis); err != nil {
		return nil, err
	}
	in.reg, err = queryregistry.Parse(in.registry)
	return in, err
}

func (in *inputs) sources(route string) (map[string]recovered, error) {
	out := map[string]recovered{}
	for _, kind := range []string{"routes", "gaps", "n461"} {
		path := "src/testdata/native/" + kind + "/" + route + ".json"
		data, err := in.read(path)
		if os.IsNotExist(err) && kind != "routes" {
			continue
		}
		if err != nil {
			return nil, err
		}
		var f fixture
		if err = jsonv2.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		for i, c := range f.Cases {
			if _, exists := out[c.ID]; exists {
				return nil, fmt.Errorf("duplicate source case %s/%s", route, c.ID)
			}
			out[c.ID] = recovered{[]byte(c.Source), c.Edits, []origin{{queryregistry.FilePin(path, data), fmt.Sprintf("#/cases/%d/source_utf8", i), c.ID}}}
		}
	}
	return out, nil
}

func (in *inputs) recover(route string, c kit.QualCase, sources map[string]recovered, qf queryFile, qpath string) (recovered, error) {
	if c.Source != nil {
		for i, q := range qf.Cases {
			if q.ID != c.ID {
				continue
			}
			s, ok := sources[q.SourceCase]
			if !ok || !strings.Contains(s.origins[0].File.Path, "/routes/") || !bytes.Equal(s.data, []byte(*c.Source)) {
				return recovered{}, fmt.Errorf("query source_case mismatch: %s/%s", route, c.ID)
			}
			s.origins = append(append([]origin{}, s.origins...), origin{queryregistry.FilePin(qpath, in.files[qpath]), fmt.Sprintf("#/cases/%d/source_case", i), c.ID})
			return s, nil
		}
		return recovered{}, fmt.Errorf("missing query source_case: %s/%s", route, c.ID)
	}
	if s, ok := sources[c.ID]; ok {
		return s, nil
	}
	if c.ID != "dynamic-sql-"+route {
		return recovered{}, fmt.Errorf("missing source case: %s/%s", route, c.ID)
	}
	const path = "src/testdata/native/dynamic-sql/expected.json"
	var dyn struct {
		Files []struct {
			Path   string `json:"path"`
			Route  string `json:"route"`
			SHA256 string `json:"sha256"`
			Bytes  uint64 `json:"bytes"`
		} `json:"files"`
	}
	if err := in.decode(path, &dyn); err != nil {
		return recovered{}, err
	}
	for i, f := range dyn.Files {
		if f.Route != route {
			continue
		}
		p := "src/testdata/native/dynamic-sql/" + f.Path
		data, err := in.read(p)
		if err != nil {
			return recovered{}, err
		}
		if queryregistry.FilePin(p, data) != (queryregistry.Pin{Path: p, Bytes: f.Bytes, SHA256: f.SHA256}) {
			return recovered{}, fmt.Errorf("dynamic source pin mismatch: %s", p)
		}
		return recovered{data, nil, []origin{{queryregistry.FilePin(p, data), "#", c.ID}, {queryregistry.FilePin(path, in.files[path]), fmt.Sprintf("#/files/%d", i), c.ID}}}, nil
	}
	return recovered{}, fmt.Errorf("missing dynamic source: %s", route)
}

func reconstruct(c kit.QualCase, s recovered) ([][]byte, []kit.EditPoints, error) {
	if uint64(len(s.data)) != c.Input.Bytes || queryregistry.Digest(s.data) != c.Input.SHA256 || !utf8.Valid(s.data) {
		return nil, nil, fmt.Errorf("source pin mismatch: %s", c.ID)
	}
	if len(s.edits) != len(c.Edits) {
		return nil, nil, fmt.Errorf("edit count mismatch: %s", c.ID)
	}
	cur := s.data
	// Check the original find/replace recipe independently of the byte-edit API.
	for i, e := range s.edits {
		at := len(cur)
		if e.Find != "" {
			at = bytes.Index(cur, []byte(e.Find))
		}
		if at < 0 {
			return nil, nil, fmt.Errorf("fixture edit target missing: %s/%d", c.ID, i)
		}
		want := kit.Edit{StartByte: uint32(at), OldEndByte: uint32(at + len(e.Find)), NewEndByte: uint32(at + len(e.Replace)), Old: []byte(e.Find), New: []byte(e.Replace)}
		if !reflect.DeepEqual(want, c.Edits[i]) {
			return nil, nil, fmt.Errorf("edit order/recipe mismatch: %s/%d", c.ID, i)
		}
		cur = bytes.Join([][]byte{cur[:at], []byte(e.Replace), cur[at+len(e.Find):]}, nil)
	}
	versions, points, err := kit.ApplyEdits(kit.EncodingUTF8, s.data, c.Edits, maxSource)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", c.ID, err)
	}
	for _, v := range versions {
		if !utf8.Valid(v) {
			return nil, nil, fmt.Errorf("invalid UTF-8 version: %s", c.ID)
		}
	}
	return versions, points, nil
}

func build(in *inputs) (export, error) {
	out := export{Schema: "tsgk-query-source-export/r1", Registry: queryregistry.FilePin(queryregistry.Path, in.registry), Qualification: queryregistry.QualificationPin, Slots: queryregistry.SlotPin, State: "DERIVED_UNREVIEWED"}
	for ri, route := range in.inv.Routes {
		selection := in.reg.Ordinary[ri]
		if selection != queryregistry.Select(route, ri) {
			return out, fmt.Errorf("registry selector mismatch")
		}
		sources, err := in.sources(route.Route)
		if err != nil {
			return out, err
		}
		qpath := "src/testdata/native/queries/" + route.Route + ".json"
		var qf queryFile
		if err = in.decode(qpath, &qf); err != nil {
			return out, err
		}
		r := witnessRoute{Route: route.Route, Set: selection.Set, Encoding: kit.EncodingUTF8, Context: "qualification/r8/full-input", Grammar: route.Workload.Grammar, Queries: route.Workload.Queries}
		seen, initial := map[string][]byte{}, map[string]bool{}
		for ci, c := range route.Workload.Cases {
			s, err := in.recover(route.Route, c, sources, qf, qpath)
			if err != nil {
				return out, err
			}
			versions, points, err := reconstruct(c, s)
			if err != nil {
				return out, err
			}
			wc := witnessCase{ID: c.ID, Selector: fmt.Sprintf("%s/%d", selection.Cases, ci), Origins: s.origins, Steps: []string{}, Points: points}
			initial[c.Input.SHA256] = true
			for _, data := range versions {
				sha := queryregistry.Digest(data)
				wc.Steps = append(wc.Steps, sha)
				out.Counts.StepBytes += uint64(len(data))
				if old, ok := seen[sha]; ok {
					if !bytes.Equal(old, data) {
						return out, fmt.Errorf("source digest collision")
					}
				} else {
					seen[sha] = data
					r.Versions = append(r.Versions, version{sha, uint64(len(data)), data})
					out.Counts.UniqueBytes += uint64(len(data))
				}
			}
			r.Cases = append(r.Cases, wc)
			out.Counts.Cases++
			if c.Source != nil {
				out.Counts.Embedded++
			} else {
				out.Counts.Recovered++
			}
			out.Counts.Edits += len(c.Edits)
			out.Counts.Steps += len(versions)
			out.Counts.TreeRoles += 1 + 2*len(c.Edits)
			out.Counts.InitialBytes += c.Input.Bytes
		}
		out.Counts.Queries += len(r.Queries)
		out.Counts.InitialUnique += len(initial)
		out.Counts.Versions += len(r.Versions)
		out.Counts.Bundles += len(r.Versions) * len(r.Queries)
		out.Routes = append(out.Routes, r)
	}
	return out, nil
}

func encode(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > maxFile {
		return nil, fmt.Errorf("derived output exceeds 16 MiB")
	}
	return append(data, '\n'), nil
}

func run(root string, rules bool) ([]byte, error) {
	in, err := load(root)
	if err != nil {
		return nil, err
	}
	if rules {
		ledger, err := buildRules(in)
		if err != nil {
			return nil, err
		}
		data, err := encode(ledger)
		if err == nil && len(data) > maxRules {
			return nil, fmt.Errorf("rule ledger exceeds 256 KiB")
		}
		return data, err
	}
	out, err := build(in)
	if err != nil {
		return nil, err
	}
	return encode(out)
}

func main() {
	root := flag.String("root", ".", "repository root")
	rules := flag.Bool("rules", false, "emit expectation rule skeleton instead of derived sources")
	flag.Parse()
	data, err := run(*root, *rules)
	if err == nil {
		_, err = os.Stdout.Write(data)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
