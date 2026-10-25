package kit

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

type eligibilityBuildIdentity struct {
	Schema               string            `json:"schema"`
	UpstreamCommit       string            `json:"upstream_commit"`
	ArchiveSHA256        string            `json:"archive_sha256"`
	BindingSHA256        string            `json:"binding_sha256"`
	Target               string            `json:"target"`
	BinarySHA256         string            `json:"binary_sha256"`
	LockSHA256           string            `json:"lock_sha256"`
	MetadataSHA256       string            `json:"metadata_sha256"`
	Versions             map[string]string `json:"versions"`
	Inputs               map[string]string `json:"inputs"`
	Tools                map[string]string `json:"tools"`
	ToolPaths            map[string]string `json:"tool_paths"`
	SelectedTreeSHA256   string            `json:"selected_tree_sha256"`
	SourceSnapshotSHA256 string            `json:"source_snapshot_sha256"`
	VendorSnapshotSHA256 string            `json:"vendor_snapshot_sha256"`
	Complete             bool              `json:"complete"`
}

func (s *eligibilitySession) snapshotRows(pin *eligibilityMember) (map[string]eligibilitySnapshotRow, bool, *Error) {
	data, ok, e := s.member(pin)
	if e != nil {
		return nil, false, e
	}
	if !ok {
		return nil, false, nil
	}
	var rows map[string]eligibilitySnapshotRow
	if e := s.b.decode(pin.Member.Path, data, &rows, true); e != nil {
		return nil, false, e
	}
	good := true
	if len(rows) > 30000 {
		return nil, false, fail(KindResourceLimit, "ELIGIBILITY_SNAPSHOT_ROWS", pin.Member.Path, nil)
	}
	var total uint64
	folded := map[string]bool{}
	for path, row := range rows {
		if e := s.b.charge(1, 2*uint64(len(path)), pin.Member.Path); e != nil {
			return nil, false, e
		}
		key := strings.ToLower(path)
		if !portable(path, false) || folded[key] || !hex64.MatchString(row.SHA256) || row.Bytes > 64<<20 || row.Bytes > 1<<30-total {
			s.finding("ELIGIBILITY_SNAPSHOT_INVALID", pin.Member.Path+"/"+path, true)
			good = false
		} else {
			total += row.Bytes
		}
		folded[key] = true
	}
	return rows, good, nil
}

func (s *eligibilitySession) tool(id, cohort, name string, identity eligibilityMember, hashes, paths map[string]string) (bool, *Error) {
	t, ok := s.tools[id]
	if !ok {
		s.finding("ELIGIBILITY_TOOL_MISSING", id, false)
		return false, nil
	}
	_, memberOK, e := s.member(&t.IdentityMember)
	if e != nil {
		return false, e
	}
	if t.CohortID != cohort || t.ToolName != name || t.IdentityMember != identity || t.SubjectSHA256 != hashes[name] || t.PathLiteral != paths[name] || !hex64.MatchString(hashes[name]) || !memberOK {
		s.finding("ELIGIBILITY_TOOL_BINDING", id, true)
		return false, nil
	}
	return s.provenance[t.ProvenanceID].ID != "", nil
}

func (s *eligibilitySession) verifyBuild() *Error {
	defer func() { s.failItem = nil }()
	group := s.reg.Build
	paths := map[string]string{}
	identity := eligibilityBuildIdentity{}
	groupReady := false
	if group != nil {
		paths = group.Paths
		c, ok := s.cohorts[group.CohortID]
		if !ok || c.Purpose != "build" {
			return fail(KindInvalidInput, "ELIGIBILITY_BUILD_COHORT", "build", nil)
		}
		data, ok, e := s.member(&group.Identity)
		if e != nil {
			return e
		}
		if ok {
			if len(data) > 1<<20 {
				return fail(KindResourceLimit, "ELIGIBILITY_IDENTITY_BYTES", group.Identity.Member.Path, nil)
			}
			if e := s.b.decode("build-identity", data, &identity, true); e != nil {
				return e
			}
		}
		source, sourceReady, e := s.snapshotRows(&group.SourceSnapshot)
		if e != nil {
			return e
		}
		vendor, vendorReady, e := s.snapshotRows(&group.VendorSnapshot)
		if e != nil {
			return e
		}
		if len(source)+len(vendor) > 60000 {
			return fail(KindResourceLimit, "ELIGIBILITY_SNAPSHOT_ROWS", "build", nil)
		}
		binary, binaryReady, e := s.member(group.ReferenceBinary)
		if e != nil {
			return e
		}
		_ = binary
		if group.ReferenceBinary == nil {
			s.finding("ELIGIBILITY_BUILD_BINARY_MISSING", "build", false)
		}
		if ok {
			s.buildIdentity = &identity
		}
		groupReady = ok && sourceReady && vendorReady && binaryReady && s.groupPaths(paths, c.Platform, "build")
		if ok {
			expectedTarget := map[string]string{"windows/amd64": "x86_64-pc-windows-gnu", "linux/amd64": "x86_64-unknown-linux-gnu", "darwin/arm64": "aarch64-apple-darwin"}[c.Platform]
			if identity.Schema != instrumentedReferenceBuildSchema || !identity.Complete || identity.UpstreamCommit != s.catalogue.UpstreamCommit || identity.ArchiveSHA256 != s.catalogue.ArchiveSHA256 || identity.BindingSHA256 != s.catalogue.BindingSHA256 || identity.Target != expectedTarget || paths["T"] != expectedTarget || group.ReferenceBinary != nil && identity.BinarySHA256 != group.ReferenceBinary.Member.SHA256 || identity.SourceSnapshotSHA256 != group.SourceSnapshot.Member.SHA256 || identity.VendorSnapshotSHA256 != group.VendorSnapshot.Member.SHA256 || !reflect.DeepEqual(identity.Inputs, s.catalogue.ReferenceInputs) {
				s.finding("ELIGIBILITY_BUILD_IDENTITY", "build", true)
				groupReady = false
			}
			for _, h := range []string{identity.BinarySHA256, identity.LockSHA256, identity.MetadataSHA256, identity.SelectedTreeSHA256, identity.SourceSnapshotSHA256, identity.VendorSnapshotSHA256} {
				if !hex64.MatchString(h) {
					s.finding("ELIGIBILITY_BUILD_DIGEST", "build", true)
					groupReady = false
				}
			}
			tools := []string{"cargo", "rustc", "cc", "ar", "git", "supervisor", "python"}
			if len(identity.Tools) != 7 || len(identity.ToolPaths) != 7 || len(identity.Versions) != 4 {
				s.finding("ELIGIBILITY_BUILD_TOOLS", "build", true)
				groupReady = false
			}
			for _, name := range tools {
				if !hex64.MatchString(identity.Tools[name]) || !eligibilityLiteral(identity.ToolPaths[name]) {
					s.finding("ELIGIBILITY_BUILD_TOOLS", name, true)
					groupReady = false
				}
				matches := 0
				for id, t := range s.tools {
					if t.CohortID == group.CohortID && t.ToolName == name {
						matches++
						if ok, e := s.tool(id, group.CohortID, name, group.Identity, identity.Tools, identity.ToolPaths); e != nil {
							return e
						} else if !ok {
							groupReady = false
						}
					}
				}
				if matches != 1 {
					s.finding("ELIGIBILITY_BUILD_TOOL_COUNT", name, matches > 1)
					groupReady = false
				}
			}
			for _, name := range []string{"rustc", "cargo"} {
				if pinned, e := s.pinnedRelease(identity.Versions[name]); e != nil {
					return e
				} else if !pinned {
					s.finding("ELIGIBILITY_TOOL_VERSION", name, true)
					groupReady = false
				}
			}
			for _, name := range []string{"cc", "python"} {
				if !eligibilityString(identity.Versions[name]) {
					s.finding("ELIGIBILITY_TOOL_VERSION", name, true)
					groupReady = false
				}
			}
			required := map[string]string{"Cargo.lock": identity.LockSHA256, "lib/binding_rust/lib.rs": identity.BindingSHA256, "crates/tsgk-query-reference/Cargo.toml": identity.Inputs["Cargo.toml"], "crates/tsgk-query-reference/main.rs": identity.Inputs["main.rs"]}
			for path, sha := range required {
				row, found := source[path]
				if !found || row.SHA256 != sha {
					s.finding("ELIGIBILITY_SOURCE_RELATION", path, true)
					groupReady = false
				}
			}
			for _, path := range []string{"Cargo.toml", ".cargo/config.toml"} {
				if _, found := source[path]; !found {
					s.finding("ELIGIBILITY_SOURCE_RELATION", path, true)
					groupReady = false
				}
			}
			if len(vendor) == 0 {
				s.finding("ELIGIBILITY_VENDOR_EMPTY", "vendor", true)
				groupReady = false
			}
		}
	}
	for _, rule := range s.catalogue.Builds {
		inv := s.catalogueInvocations["build/"+rule.ID]
		if inv == nil {
			s.build[rule.ID] = "blocked"
			continue
		}
		s.failItem = func() { s.build[rule.ID] = "failed" }
		before := s.failures
		req, requestOK, e := s.request(inv)
		if e != nil {
			return e
		}
		ready := requestOK && s.ready[inv.ID] && groupReady
		if requestOK {
			if group != nil && !s.processEnv(inv, req, paths, identity.ToolPaths, true) {
				ready = false
			}
			inputReady, e := s.input(inv, "source", "NOT_APPLICABLE", nil, "", "", nil)
			if e != nil {
				return e
			}
			if ok, e := s.input(inv, "library", "NOT_APPLICABLE", nil, "", "", nil); e != nil {
				return e
			} else {
				inputReady = inputReady && ok
			}
			allowed := map[string]bool{"source": true, "library": true}
			if rule.ID == "patch-check" || rule.ID == "patch" {
				allowed["patch"] = true
				k := 2
				if rule.ID == "patch-check" {
					k = 3
				}
				path, found := s.expand(rule.Args[k], paths, s.cohorts[inv.CohortID].Platform)
				if found {
					if ok, e := s.input(inv, "patch", "PINNED", eligibilityIndex(k), path, s.catalogue.ReferenceInputs["instrumentation.patch"], nil); e != nil {
						return e
					} else {
						inputReady = inputReady && ok
					}
				} else {
					inputReady = false
				}
			}
			inputReady = s.inputSlots(inv, allowed) && inputReady
			if !inputReady {
				ready = false
			}
			if inv.Executable != nil || inv.ExecutablePath != "" {
				s.finding("ELIGIBILITY_BUILD_DESCRIPTOR", inv.ID, true)
				ready = false
			}
			if !s.fixedRequest(inv, req, rule, paths) {
				ready = false
			}
			if group == nil || inv.CohortID != group.CohortID {
				ready = false
			} else if ok, e := s.tool(inv.ToolID, inv.CohortID, rule.Tool, group.Identity, identity.Tools, identity.ToolPaths); e != nil {
				return e
			} else if !ok {
				ready = false
			}
			if tool, found := s.tools[inv.ToolID]; found && req.Path != tool.PathLiteral {
				s.finding("ELIGIBILITY_TOOL_ARGV", inv.ID, true)
				ready = false
			}
			out, executionReady, e := s.receipt(inv, req, 0, false)
			if e != nil {
				return e
			}
			ready = ready && executionReady
			if out != nil {
				s.outputs[inv.ID] = out
				if !utf8.Valid(out.stdout) || !utf8.Valid(out.stderr) {
					s.finding("ELIGIBILITY_BUILD_UTF8", inv.ID, true)
					ready = false
				}
				if group != nil {
					if rule.ID == "metadata" && digestHex(out.stdout) != identity.MetadataSHA256 {
						s.finding("ELIGIBILITY_METADATA_DIGEST", inv.ID, true)
						ready = false
					}
					if rule.ID == "selected-tree" && inv.Receipt.Member.SHA256 != identity.SelectedTreeSHA256 {
						s.finding("ELIGIBILITY_SELECTED_TREE_DIGEST", inv.ID, true)
						ready = false
					}
					if name := map[string]string{"rustc-version": "rustc", "cargo-version": "cargo", "cc-version": "cc"}[rule.ID]; name != "" && string(out.stdout) != identity.Versions[name] {
						s.finding("ELIGIBILITY_VERSION_RECEIPT", inv.ID, true)
						ready = false
					}
				}
			}
		}
		state := "blocked"
		if s.failures > before {
			state = "failed"
		} else if ready {
			state = "verified"
		}
		s.build[rule.ID] = state
		if e := s.r.check(); e != nil {
			return e
		}
	}
	if inv := s.catalogueInvocations["build/unit"]; inv != nil {
		if out := s.outputs[inv.ID]; out != nil {
			valid, e := s.unitSet(out.stdout)
			if e != nil {
				return e
			}
			for _, id := range s.catalogue.Units {
				if !valid {
					s.units[id] = "failed"
				} else if s.build["unit"] == "verified" {
					s.units[id] = "verified"
				} else {
					s.units[id] = "blocked"
				}
			}
			if !valid {
				s.finding("ELIGIBILITY_RUST_UNIT_SET", inv.ID, true)
				s.build["unit"] = "failed"
			}
		}
	}
	return nil
}

func (s *eligibilitySession) pinnedRelease(text string) (bool, *Error) {
	if e := s.b.charge(uint64(len(text))+1, 8*uint64(len(text)), "pinned-version-scan"); e != nil {
		return false, e
	}
	releases := []string{}
	rows := 0
	for line := range strings.Lines(text) {
		if rows%1024 == 0 {
			if e := s.r.check(); e != nil {
				return false, e
			}
		}
		rows++
		k, v, ok := strings.Cut(strings.TrimSuffix(line, "\n"), ":")
		if ok && strings.TrimSpace(k) == "release" {
			releases = append(releases, strings.TrimSpace(v))
		}
	}
	return slices.Equal(releases, []string{"1.97.1"}), nil
}

func (s *eligibilitySession) unitSet(stdout []byte) (bool, *Error) {
	// Reserve every scan and line visit before processing attacker-controlled output.
	if e := s.b.charge(6*(uint64(len(stdout))+1), 3*uint64(len(stdout)), "rust-unit-scan"); e != nil {
		return false, e
	}
	text := string(stdout)
	valid := strings.Count(text, "running 4 tests\n") == 1 && strings.Count(text, "test result: ok. 4 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out;") == 1
	counts := map[string]int{}
	for _, id := range s.catalogue.Units {
		counts["test tests::"+id+" ... ok"] = 0
	}
	unitLines, summaryLines, rows := 0, 0, 0
	for row := range strings.Lines(text) {
		if rows%1024 == 0 {
			if e := s.r.check(); e != nil {
				return false, e
			}
		}
		rows++
		line := strings.TrimSuffix(row, "\n")
		if strings.HasPrefix(line, "test tests::") {
			unitLines++
		}
		if strings.HasPrefix(line, "test result:") {
			summaryLines++
		}
		if _, known := counts[line]; known {
			counts[line]++
		}
	}
	for _, count := range counts {
		valid = valid && count == 1
	}
	return valid && unitLines == 4 && summaryLines == 1, nil
}
