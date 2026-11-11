package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func verifiedDriverSession(t *testing.T, size int) (*eligibilityFixture, *eligibilitySession, QueryReferenceEligibilityResult) {
	t.Helper()
	f := binaryDriverFixture(t, size)
	req := f.request()
	s, got := verifiedEligibilitySession(t, req)
	return f, s, got
}

func verifiedEligibilitySession(t *testing.T, req QueryReferenceEligibilityRequest) (*eligibilitySession, QueryReferenceEligibilityResult) {
	t.Helper()
	root, err := os.OpenRoot(req.Evidence.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	r, e := startRun(ctxT(t), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(r.cancel)
	// Semantic callers own a 1 GiB ledger before eligibility starts.
	r.eligibility = &eligibilityBudget{r: r}
	scope := &eligibilityScope{r: r, root: root}
	got, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, scope)
	if err != nil || got.Assessment != AssessPass || !got.Complete || scope.session.snapshot.paging == nil {
		t.Fatalf("verified session: %v %+v", err, got)
	}
	return scope.session, got
}

func verifiedDriverSource(t *testing.T, s *eligibilitySession, set bool) (*verifiedPageSource, semanticBinaryDescriptor, []byte) {
	t.Helper()
	pin := *s.invocations[s.reg.Native.InvocationID].Executable
	data, ok, e := s.member(&pin)
	if e != nil || !ok {
		t.Fatalf("descriptor: %v", e)
	}
	var d semanticBinaryDescriptor
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatal(err)
	}
	source, e := s.pageSource(pin, "oracle/", nil)
	if e != nil || source == nil {
		t.Fatalf("source: %v", e)
	}
	if set {
		source.set = map[string]OracleMember{}
		for _, p := range d.Parts {
			source.set[p.Path] = OracleMember{Path: p.Path, Role: "driver-paged-part", Bytes: p.Bytes, SHA256: p.SHA256}
		}
	}
	return source, d, data
}

func noPagingCallback(t *testing.T) func(ReplayMember) ([]byte, bool, *Error) {
	return func(p ReplayMember) ([]byte, bool, *Error) {
		t.Fatalf("verified identity fell back to mutable callback: %s", p.Path)
		return nil, false, nil
	}
}

func TestVerifiedPagingCharge(t *testing.T) {
	f, s, optimized := verifiedDriverSession(t, MaxDocumentBytes+1)
	baseline, err := VerifyQueryReferenceEligibility(ctxT(t), f.request())
	if err != nil || baseline.Assessment != AssessPass {
		t.Fatalf("baseline: %v %+v", err, baseline)
	}
	source, d, data := verifiedDriverSource(t, s, true)
	gross := 2 * (d.Logical.Bytes + uint64(len(d.Parts)))
	// Every new allocation/key reservation is charged in addition to the guarded read.
	overhead := uint64(2048 + 512 + 1024 + 2*(len(data)+1))
	overhead += 1024
	desc := source.descriptor.Member
	overhead += uint64(len(desc.Path) + len(desc.Role) + len(desc.SHA256))
	for _, p := range d.Parts {
		overhead += uint64(512+len("oracle/"+p.Path)) + uint64(256+len("oracle/"+p.Path)+len("driver-paged-part")+len(p.SHA256))
		overhead += uint64(128 + len("oracle/"+p.Path) + len("driver-paged-part") + len(p.SHA256))
	}
	if optimized.LogicalBytes != baseline.LogicalBytes-gross+overhead || optimized.BytesRead != baseline.BytesRead || s.b.logicalCap() != eligibilityLogicalLimit {
		t.Fatalf("core charge: before=%d after=%d gross=%d E=%d reads=%d/%d cap=%d", baseline.LogicalBytes, optimized.LogicalBytes, gross, overhead, baseline.BytesRead, optimized.BytesRead, s.b.logicalCap())
	}
	b := semanticTestBudget(t)
	_, ok, e := b.semanticPagedBinary(data, "D", "driver", d.Logical.Path, "capture/driver", func(p ReplayMember) ([]byte, bool, *Error) {
		return f.files["oracle/"+p.Path], true, nil
	})
	if e != nil || !ok {
		t.Fatalf("set baseline: %v", e)
	}
	before := s.b.logical
	logical, ok, e := s.b.semanticPagedBinaryVerified(data, "D", "driver", d.Logical.Path, "capture/driver", noPagingCallback(t), source)
	after := s.b.logical - before
	if e != nil || !ok || logical != d.Logical || b.logical <= after || b.logical-after <= d.Logical.Bytes+uint64(len(d.Parts)) {
		t.Fatalf("set reuse: %v good=%v before=%d after=%d", e, ok, b.logical, after)
	}
	t.Logf("core logical before=%d after=%d gross=%d E=%d net=%d bytes_read=%d; set hash+decode before=%d after=%d (source creation already charged separately)", baseline.LogicalBytes, optimized.LogicalBytes, gross, overhead, baseline.LogicalBytes-optimized.LogicalBytes, optimized.BytesRead, b.logical, after)
}

func TestVerifiedPagingUnboundFallback(t *testing.T) {
	for _, mutation := range []string{"graph", "node"} {
		t.Run(mutation, func(t *testing.T) {
			f := binaryDriverFixture(t, MaxDocumentBytes+1)
			if mutation == "node" {
				n := f.nodes["native-current"]
				n.Kind = "unknown"
				f.nodes[n.ID] = n
			}
			req := f.request()
			if mutation == "graph" {
				graphPath := filepath.Join(req.Evidence.Root, "evidence.json")
				data, err := os.ReadFile(graphPath)
				if err != nil {
					t.Fatal(err)
				}
				// Keep a valid graph and all members, but violate its independent policy pin.
				if err := os.WriteFile(graphPath, append(data, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			}
			baseline, err := VerifyQueryReferenceEligibility(ctxT(t), req)
			if err != nil || !baseline.Complete || baseline.Assessment != AssessFail {
				t.Fatalf("baseline: %v %+v", err, baseline)
			}
			root, err := os.OpenRoot(req.Evidence.Root)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			r, e := startRun(ctxT(t), time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			defer r.cancel()
			r.eligibility = &eligibilityBudget{r: r}
			scope := &eligibilityScope{r: r, root: root}
			got, err := verifyQueryReferenceEligibilityInRun(ctxT(t), req, scope)
			if err != nil || !got.Complete || got.Assessment != baseline.Assessment || got.Tuple != baseline.Tuple || !reflect.DeepEqual(got.Findings, baseline.Findings) {
				t.Fatalf("unbound graph changed failure classification: %v %+v", err, got)
			}
			if p := scope.session.snapshot.paging; p.finished || len(p.records) != 0 {
				t.Fatal("unbound graph authorized a reusable identity")
			}
		})
	}
}

func TestVerifiedPagingRejects(t *testing.T) {
	for _, mode := range []string{"core", "set"} {
		t.Run(mode, func(t *testing.T) {
			_, s, _ := verifiedDriverSession(t, 2*MaxDocumentBytes+1)
			mutations := []string{"part-hash", "swap", "duplicate", "omit-same-total", "owner", "kind", "index", "role", "prefix", "descriptor-owner"}
			if mode == "set" {
				mutations = append(mutations, "case")
			}
			for _, mutation := range mutations {
				t.Run(mutation, func(t *testing.T) {
					source, d, _ := verifiedDriverSource(t, s, mode == "set")
					owner, kind, prefix := "D", "driver", "capture/driver"
					switch mutation {
					case "part-hash":
						d.Parts[0].SHA256 = strings.Repeat("a", 64)
					case "swap":
						d.Parts[0], d.Parts[1] = d.Parts[1], d.Parts[0]
					case "duplicate":
						d.Parts[1] = d.Parts[0]
					case "omit-same-total":
						d.Parts[2].Bytes += d.Parts[1].Bytes
						d.Parts = append(d.Parts[:1], d.Parts[2])
					case "owner":
						d.Owner = "O"
					case "kind":
						d.Logical.Kind = "library"
					case "index":
						d.Parts[0].Index++
					case "role", "case":
						if mode == "set" {
							m := source.set[d.Parts[0].Path]
							if mutation == "case" {
								m.Case = "borrowed-case"
							} else {
								m.Role = "grammar-paged-part"
							}
							source.set[m.Path] = m
						} else {
							p := "oracle/" + d.Parts[0].Path
							old := s.owned[p]
							wrong := old
							wrong.Member.Role = "grammar-paged-part"
							s.owned[p] = wrong
							defer func() { s.owned[p] = old }()
						}
					case "prefix":
						prefix = "capture/other"
					case "descriptor-owner":
						source.descriptor.NodeID = "native-static"
					}
					_, ok, e := s.b.semanticPagedBinaryVerified(eligibilityJSON(t, d), owner, kind, d.Logical.Path, prefix, func(ReplayMember) ([]byte, bool, *Error) { return nil, false, nil }, source)
					if ok || e != nil && e.Kind != KindInvalidInput {
						t.Fatalf("accepted %s/%s: good=%v error=%v", mode, mutation, ok, e)
					}
				})
			}
		})
	}
}

func TestVerifiedPagingParserBindings(t *testing.T) {
	req, _, _ := semanticBoundFixtureMode(t, 0, true, true)
	s, _ := verifiedEligibilitySession(t, QueryReferenceEligibilityRequest{Evidence: req.Evidence, Approval: req.EligibilityApproval, Registration: req.EligibilityRegistration, WitnessRegistration: req.WitnessRegistration})
	var record verifiedLogical
	found := false
	for _, r := range s.snapshot.paging.records {
		if r.kind == "parser" {
			record, found = r, true
		}
	}
	if !found {
		t.Fatal("missing parser identity")
	}
	for _, mutation := range []string{"normal", "closure-index", "role", "logical-sha", "descriptor-digest", "owner"} {
		t.Run(mutation, func(t *testing.T) {
			input, index := record.input, record.index
			data := []byte(record.data)
			source, e := s.pageSource(record.descriptor, "oracle/", nil)
			if e != nil {
				t.Fatal(e)
			}
			source.set = map[string]OracleMember{}
			for _, p := range record.parts {
				path := strings.TrimPrefix(p.Path, "oracle/")
				source.set[path] = OracleMember{Path: path, Role: p.Role, Bytes: p.Bytes, SHA256: p.SHA256}
			}
			switch mutation {
			case "closure-index":
				var d semanticParserDescriptor
				if err := json.Unmarshal(data, &d); err != nil {
					t.Fatal(err)
				}
				index++
				d.Logical.ClosureIndex++
				data = eligibilityJSON(t, d)
			case "role":
				input.Role = "grammar-scanner"
			case "logical-sha":
				input.SHA256 = strings.Repeat("e", 64)
			case "descriptor-digest":
				source.descriptor.Member.SHA256 = strings.Repeat("e", 64)
			case "owner":
				source.descriptor.NodeID = "native-static"
			}
			ok, e := s.b.semanticPagedParserVerified(data, input, index, noPagingCallback(t), source)
			if e != nil || ok != (mutation == "normal") {
				t.Fatalf("parser %s: good=%v err=%v", mutation, ok, e)
			}
		})
	}
}

func TestVerifiedPagingSessionAndMutation(t *testing.T) {
	f := binaryDriverFixture(t, MaxDocumentBytes+1)
	// A reordered graph retains callback bytes, including the mutable-copy guarantee.
	reorderDriverParts(f)
	s, _ := verifiedEligibilitySession(t, f.request())
	source, d, data := verifiedDriverSource(t, s, true)
	pin := eligibilityMember{NodeID: source.descriptor.NodeID, Member: ReplayMember{Path: "oracle/" + d.Parts[0].Path, Role: "driver-paged-part", Bytes: d.Parts[0].Bytes, SHA256: d.Parts[0].SHA256}}
	borrowed, ok, e := s.member(&pin)
	if e != nil || !ok {
		t.Fatal(e)
	}
	borrowed[0] ^= 1
	f.files[pin.Member.Path][0] ^= 1
	// Mutable callers can change only their own copy, including the pre-read fixture.
	logical, ok, e := s.b.semanticPagedBinaryVerified(data, "D", "driver", d.Logical.Path, "capture/driver", noPagingCallback(t), source)
	if e != nil || !ok || logical != d.Logical {
		t.Fatalf("mutable alias escaped: %v", e)
	}
	p := s.snapshot.paging
	for _, mutation := range []string{"session", "run", "root", "snapshot", "cohort", "candidate", "graph", "policy", "registration", "physical"} {
		t.Run(mutation, func(t *testing.T) {
			copySession := *s
			part := p.snapshot.parts[pin.Member.Path]
			oldRoot, oldSnapshot, oldApproval, oldReg := p.root, s.snapshot, s.approval, s.reg
			oldSHA, oldInfo := s.res.RegistrationSHA256, part.info
			oldRun, oldBudget := s.r, s.b
			inv := s.invocations[s.reg.Native.InvocationID]
			oldCohort := s.cohorts[inv.CohortID]
			defer func() {
				p.root, s.snapshot, s.approval, s.reg = oldRoot, oldSnapshot, oldApproval, oldReg
				s.res.RegistrationSHA256, part.info = oldSHA, oldInfo
				s.r, s.b = oldRun, oldBudget
				s.cohorts[inv.CohortID] = oldCohort
			}()
			caller := s
			switch mutation {
			case "session":
				caller = &copySession
			case "run":
				r, e := startRun(ctxT(t), time.Minute)
				if e != nil {
					t.Fatal(e)
				}
				defer r.cancel()
				s.r, s.b = r, &eligibilityBudget{r: r}
			case "root", "physical":
				root, err := os.OpenRoot(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				if mutation == "root" {
					p.root = root
				} else {
					part.info, err = root.Stat(".")
					if err != nil {
						t.Fatal(err)
					}
				}
			case "snapshot":
				snapshot := *s.snapshot
				s.snapshot = &snapshot
			case "cohort":
				c := oldCohort
				c.Run = "borrowed-cohort"
				s.cohorts[inv.CohortID] = c
			case "candidate":
				s.approval.Candidate.Run = "borrowed-candidate"
			case "graph":
				s.reg.Graph.SHA256 = strings.Repeat("f", 64)
			case "policy":
				s.approval.EvidencePolicySHA256 = strings.Repeat("f", 64)
			case "registration":
				s.res.RegistrationSHA256 = strings.Repeat("f", 64)
			}
			if _, ok, _, e := p.part(caller, pin, true); ok || e == nil {
				t.Fatalf("borrowed %s record: good=%v err=%v", mutation, ok, e)
			}
		})
	}
	// Incomplete eligibility cannot authorize skipping the logical chain.
	p.finished = false
	before := s.b.logical
	_, ok, e = s.b.semanticPagedBinaryVerified(data, "D", "driver", d.Logical.Path, "capture/driver", noPagingCallback(t), source)
	if e != nil || !ok || s.b.logical-before < d.Logical.Bytes+uint64(len(d.Parts)) {
		t.Fatalf("incomplete session skipped chain: good=%v err=%v charge=%d", ok, e, s.b.logical-before)
	}
	t.Run("closed-root", func(t *testing.T) {
		if err := p.root.Close(); err != nil {
			t.Fatal(err)
		}
		if _, e := s.pageSource(source.descriptor, "oracle/", nil); e == nil || e.Code != "SOURCE_CHANGED" {
			t.Fatalf("closed root authorized reuse: %v", e)
		}
	})
	t.Run("expired-run", func(t *testing.T) {
		s.r.cancel()
		if _, ok, _, e := p.part(s, pin, true); ok || e == nil {
			t.Fatalf("expired run authorized reuse: good=%v err=%v", ok, e)
		}
	})
}

// An ownership rejection on the verified path is reported once, not again by the fallback (review F2).
func TestVerifiedPagingRejectedPinReportedOnce(t *testing.T) {
	_, s, _ := verifiedDriverSession(t, 2*MaxDocumentBytes+1)
	source, d, _ := verifiedDriverSource(t, s, false)
	p := "oracle/" + d.Parts[0].Path
	old := s.owned[p]
	wrong := old
	wrong.Member.Role = "grammar-paged-part"
	s.owned[p] = wrong
	defer func() { s.owned[p] = old }()
	before := s.failures
	callback := func(pin ReplayMember) ([]byte, bool, *Error) {
		ok, e := s.memberPin(&eligibilityMember{NodeID: source.descriptor.NodeID, Member: ReplayMember{Path: "oracle/" + pin.Path, Role: pin.Role, Bytes: pin.Bytes, SHA256: pin.SHA256}})
		return nil, ok, e
	}
	_, ok, e := s.b.semanticPagedBinaryVerified(eligibilityJSON(t, d), "D", "driver", d.Logical.Path, "capture/driver", callback, source)
	if ok || e != nil || s.failures-before != 1 {
		t.Fatalf("rejected pin: good=%v err=%v new findings=%d, want exactly 1", ok, e, s.failures-before)
	}
}

// The semantic set treats an invalid owner node as an unavailable member, as before reuse, and
// never appends findings to the finished eligibility session (review F1).
func TestVerifiedPagingSetInvalidNodeIsUnavailable(t *testing.T) {
	_, s, _ := verifiedDriverSession(t, 2*MaxDocumentBytes+1)
	_, d, _ := verifiedDriverSource(t, s, true)
	path := "oracle/" + d.Parts[0].Path
	pin := eligibilityMember{NodeID: s.owners[path], Member: ReplayMember{Path: path, Role: "driver-paged-part", Bytes: d.Parts[0].Bytes, SHA256: d.Parts[0].SHA256}}
	p := s.snapshot.paging
	if _, ok, _, e := p.part(s, pin, false); !ok || e != nil {
		t.Fatalf("control: %v %v", ok, e)
	}
	s.snapshot.invalidNodes[pin.NodeID] = true
	defer delete(s.snapshot.invalidNodes, pin.NodeID)
	before := s.failures
	if _, ok, rejected, e := p.part(s, pin, false); ok || rejected || e != nil || s.failures != before {
		t.Fatalf("invalid node: good=%v rejected=%v err=%v findings=%d", ok, rejected, e, s.failures-before)
	}
}

// A part pin that is structurally valid but carries another real part's hash is rejected by the
// verified layer itself, without falling back to the mutable callback (review F3). Content changed
// on disk after the guarded read is out of scope: verification uses the snapshot's guarded
// identity, exactly like the retained callback path.
func TestVerifiedPagingRejectsAtVerifiedLayer(t *testing.T) {
	_, s, _ := verifiedDriverSession(t, 2*MaxDocumentBytes+1)
	source, d, _ := verifiedDriverSource(t, s, false)
	if d.Parts[1].SHA256 == d.Parts[2].SHA256 {
		t.Fatal("fixture parts 1 and 2 must differ")
	}
	d.Parts[1].SHA256 = d.Parts[2].SHA256
	before := s.failures
	_, ok, e := s.b.semanticPagedBinaryVerified(eligibilityJSON(t, d), "D", "driver", d.Logical.Path, "capture/driver", noPagingCallback(t), source)
	if ok || e != nil || s.failures-before != 1 {
		t.Fatalf("borrowed part hash: good=%v err=%v new findings=%d", ok, e, s.failures-before)
	}
}
