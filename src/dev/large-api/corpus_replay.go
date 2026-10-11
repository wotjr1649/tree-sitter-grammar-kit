package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/native"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

type corpusRegistration struct {
	Schema   string     `json:"schema"`
	PlanSHA  string     `json:"plan_sha256"`
	HostSHA  string     `json:"host_executable_sha256"`
	Result   pinnedFile `json:"result"`
	Bindings pinnedFile `json:"bindings"`
}

func outsideCorpusRoot(name, root string) bool {
	abs, err := filepath.Abs(name)
	if err != nil {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return false
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	for {
		info, err := os.Stat(parent)
		if err != nil || os.SameFile(rootInfo, info) {
			return false
		}
		next := filepath.Dir(parent)
		if next == parent {
			return true
		}
		parent = next
	}
}

func readLocalDocument(name string, limit uint64) ([]byte, error) {
	if name == "" || strings.HasPrefix(strings.ReplaceAll(name, `\`, "/"), "//") {
		return nil, errors.New("LOCAL_DOCUMENT_REQUIRED")
	}
	r, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	info, err := r.Lstat(filepath.Base(name))
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) > limit {
		return nil, errors.New("LOCAL_DOCUMENT_LIMIT_OR_TYPE")
	}
	f, err := r.Open(filepath.Base(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = corpusCheckLinks(f); err != nil {
		return nil, err
	}
	// readPinned supplies the same no-follow, actual-size and bounded read semantics.
	b, err := io.ReadAll(io.LimitReader(f, info.Size()+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || int64(len(b)) != info.Size() {
		return nil, errors.New("LOCAL_DOCUMENT_CHANGED")
	}
	return b, nil
}

// replayCorpus uses caller-trusted pins outside the data root. API stream hashes remain
// native observations; full CST digest/shape and coverage are independently recomputed here.
func replayCorpus(evidenceName, sourceName, planName, registrationName, outName string) error {
	start := time.Now()
	resolved, err := filepath.EvalSymlinks(evidenceName)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if !outsideCorpusRoot(registrationName, resolved) || !outsideCorpusRoot(planName, resolved) || !outsideCorpusRoot(outName, resolved) {
		return errors.New("REPLAY_TRUST_OR_OUTPUT_INSIDE_EVIDENCE")
	}
	registrationBytes, err := readLocalDocument(registrationName, 16777216)
	if err != nil {
		return err
	}
	var registration corpusRegistration
	if err = jsonv2.Unmarshal(registrationBytes, &registration, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if registration.Schema != "tsgk-corpus-cst-api-registration/r1" || registration.Result.Path != "result.json" || registration.Bindings.Path != "bindings.jsonl" {
		return errors.New("CORPUS_REPLAY_REGISTRATION_INVALID")
	}
	planBytes, err := readLocalDocument(planName, 16777216)
	if err != nil {
		return err
	}
	if digest(planBytes) != registration.PlanSHA {
		return errors.New("CORPUS_REPLAY_PLAN_IDENTITY_MISMATCH")
	}
	var plan corpusPlan
	if err = jsonv2.Unmarshal(planBytes, &plan, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if plan.Schema != "tsgk-corpus-cst-api-plan/r1" || len(plan.Expected) == 0 || len(plan.Expected) > 26000 {
		return errors.New("CORPUS_REPLAY_PLAN_INVALID")
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := readPinned(root, registration.Result, 16777216)
	if err != nil {
		return err
	}
	var result corpusResult
	if err = jsonv2.Unmarshal(data, &result, jsonv2.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if !result.Complete || result.Error != "" || result.Schema != "tsgk-corpus-cst-api-result/r1" || result.PlanSHA != registration.PlanSHA || result.Candidate != plan.Candidate || result.HostSHA != registration.HostSHA || result.Policy != corpusPolicySHA() || result.StorageLimit != corpusStorageBytes || result.Expected != len(plan.Expected) || result.Completed != result.Expected || result.API+result.SvcNotClaimed != result.Expected || result.BindingsSHA != registration.Bindings.SHA || result.WallMS > 3600000 {
		return errors.New("CORPUS_REPLAY_RESULT_BINDING_OR_COMPLETION_INVALID")
	}
	bindings, err := readPinned(root, registration.Bindings, 134217728)
	if err != nil {
		return err
	}
	if uint64(len(bindings))+uint64(len(data)) > corpusStorageBytes {
		return errors.New("CORPUS_REPLAY_TOTAL_LIMIT")
	}
	expected := map[string]corpusExpected{}
	for _, e := range plan.Expected {
		if expected[e.ID].ID != "" {
			return errors.New("CORPUS_REPLAY_EXPECTED_DUPLICATE")
		}
		expected[e.ID] = e
	}
	producers := map[string]string{}
	for _, p := range result.Producers {
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		producers[digest(b)] = p.Route
	}
	sourceRoot, err := os.OpenRoot(sourceName)
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	seen := map[string]bool{}
	paths := map[string]bool{}
	var active string
	var tree *corpusTree
	var source []byte
	var current corpusExpected
	var pages, api, svc int
	var nodes, checks, totalBytes uint64
	totalBytes = uint64(len(bindings)) + uint64(len(data))
	finish := func() error {
		if active == "" {
			return nil
		}
		if tree.next != current.Nodes || tree.children != current.Nodes-1 || current.HasError == nil || tree.rootError != *current.HasError || hex.EncodeToString(tree.h.Sum(nil)) != current.Digest {
			return errors.New("CORPUS_REPLAY_CST_DIGEST_OR_COVERAGE_INVALID")
		}
		seen[active] = true
		api++
		nodes += tree.next
		checks += tree.checks
		active = ""
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(bindings))
	scanner.Buffer(make([]byte, 65536), 16777216)
	for scanner.Scan() {
		if time.Since(start) > time.Hour {
			return errors.New("CORPUS_REPLAY_WALL_LIMIT")
		}
		var binding corpusBinding
		if err = jsonv2.Unmarshal(scanner.Bytes(), &binding, jsonv2.RejectUnknownMembers(true)); err != nil {
			return err
		}
		e, ok := expected[binding.ID]
		producerRoute := e.Route
		if producerRoute == "svc" {
			producerRoute = "csharp"
		}
		if !ok || binding.Route != e.Route || binding.Input != e.Input || binding.Encoding != e.Encoding || binding.Policy != result.Policy || producers[binding.Producer] != producerRoute || paths[binding.Page.Path] {
			return errors.New("CORPUS_REPLAY_CASE_OR_PRODUCER_BINDING_INVALID")
		}
		paths[binding.Page.Path] = true
		if totalBytes+binding.Page.Bytes > corpusStorageBytes {
			return errors.New("CORPUS_REPLAY_TOTAL_LIMIT")
		}
		pageBytes, err := readPinned(root, binding.Page, 16777220)
		if err != nil {
			return err
		}
		totalBytes += uint64(len(pageBytes))
		if e.Route == "svc" {
			if err = finish(); err != nil {
				return err
			}
			if seen[e.ID] || binding.Start != 0 || binding.End != 0 || binding.Nodes != 0 {
				return errors.New("CORPUS_REPLAY_SVC_DUPLICATE_OR_TREE")
			}
			original, err := readPinned(sourceRoot, pinnedFile{Path: e.Input.Path, SHA: e.Input.SHA256, Bytes: e.Input.Bytes}, 33554432)
			if err != nil {
				return err
			}
			var x struct {
				Case        corpusExpected     `json:"case"`
				Policy      string             `json:"policy"`
				Observation kit.SvcObservation `json:"observation"`
				API         string             `json:"api"`
			}
			if err = jsonv2.Unmarshal(pageBytes, &x, jsonv2.RejectUnknownMembers(true)); err != nil {
				return err
			}
			observed, err := json.Marshal(kit.ObserveServiceHost(e.Encoding, original))
			if err != nil {
				return err
			}
			recorded, err := json.Marshal(x.Observation)
			if err != nil {
				return err
			}
			if x.Case.ID != e.ID || x.Case.Input != e.Input || x.Case.Encoding != e.Encoding || x.Case.Route != e.Route || x.Policy != result.Policy || x.API != "NOT_CLAIMED" || !slices.Equal(observed, recorded) {
				return errors.New("CORPUS_REPLAY_SVC_REOBSERVATION_MISMATCH")
			}
			seen[e.ID] = true
			svc++
			continue
		}
		if active != e.ID {
			if err = finish(); err != nil {
				return err
			}
			if seen[e.ID] {
				return errors.New("CORPUS_REPLAY_CASE_REORDER_OR_DUPLICATE")
			}
			current = e
			active = e.ID
			tree = newCorpusTree()
			source, err = readPinned(sourceRoot, pinnedFile{Path: e.Input.Path, SHA: e.Input.SHA256, Bytes: e.Input.Bytes}, 33554432)
			if err != nil {
				return err
			}
		}
		if !validCorpusProcess(binding.Process, uint64(len(pageBytes))) || binding.Start != tree.next || binding.Nodes != e.Nodes {
			return errors.New("CORPUS_REPLAY_PROCESS_OR_RANGE_INVALID")
		}
		req := native.Request{ID: e.ID, Encoding: e.Encoding, Source: source, Limits: native.LimitsFor(kit.NativeOperations()["native-query-large"])}
		p, err := decodeCorpusPage(pageBytes, req, binding.Start, binding.End, e.Nodes)
		if err != nil {
			return err
		}
		if err = tree.consume(p, e.Input.Bytes); err != nil {
			return err
		}
		pages++
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if err = finish(); err != nil {
		return err
	}
	if len(seen) != len(expected) || pages != result.Pages || api != result.API || svc != result.SvcNotClaimed || nodes != result.Nodes || checks != result.Checks || totalBytes-uint64(len(data)) != result.Bytes {
		return errors.New("CORPUS_REPLAY_COMPLETENESS_OR_SUMMARY_INVALID")
	}
	report := struct {
		Schema          string `json:"schema"`
		RegistrationSHA string `json:"registration_sha256"`
		Expected        int    `json:"expected"`
		API             int    `json:"api_cases"`
		SVC             int    `json:"svc_api_not_claimed"`
		Pages           int    `json:"pages"`
		Nodes           uint64 `json:"nodes"`
		Checks          uint64 `json:"checks"`
		WallMS          int64  `json:"wall_ms"`
		Assessment      string `json:"assessment"`
	}{"tsgk-corpus-cst-api-replay/r1", digest(registrationBytes), len(seen), api, svc, pages, nodes, checks, time.Since(start).Milliseconds(), "PASS"}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = writeExclusive(outName, b); err != nil {
		return err
	}
	fmt.Printf("corpus replay cases=%d api=%d svc_not_claimed=%d nodes=%d pages=%d PASS\n", len(seen), api, svc, nodes, pages)
	return nil
}
