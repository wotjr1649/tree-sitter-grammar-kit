package foundation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/internal/ppexpect"
	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func roslynDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func roslynCandidate(t *testing.T, cases, observed []byte, id string) ([]byte, ppexpect.Envelope) {
	t.Helper()
	var doc ppexpect.Document
	if err := json.Unmarshal(observed, &doc); err != nil {
		t.Fatal(err)
	}
	doc.Schema, doc.Roslyn = ppexpect.ProvenanceSchema, "assembly-"+id
	doc.Provenance = &ppexpect.Provenance{SDK: "sdk-" + id, OracleSHA256: strings.Repeat("1", 64), CodeAnalysisSHA256: strings.Repeat("2", 64), CSharpSHA256: strings.Repeat("3", 64)}
	if id == "new" {
		for i := range doc.Cases {
			if doc.Cases[i].ID == "ops-a" {
				changed := false
				for j := range doc.Cases[i].Directives {
					if doc.Cases[i].Directives[j].Kind == "IfDirectiveTrivia" && doc.Cases[i].Directives[j].Taken != nil {
						v := !*doc.Cases[i].Directives[j].Taken
						doc.Cases[i].Directives[j].Taken = &v
						changed = true
						break
					}
				}
				if !changed {
					t.Fatal("ops-a has no observed #if outcome")
				}
			}
		}
	}
	newObservation, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	tuple, err := json.Marshal(ppexpect.Tuple{Schema: "tsgk-roslyn-pin-tuple/r1", Reader: "tsgk-roslyn-selection/r1", ObservationSHA256: roslynDigest(newObservation), CasesSHA256: roslynDigest(cases), ObservationSchema: doc.Schema, Roslyn: doc.Roslyn, LanguageVersion: doc.LanguageVersion, Scope: "sdk-dll-provenance", Provenance: doc.Provenance})
	if err != nil {
		t.Fatal(err)
	}
	materials := []kit.EvidencePinMaterial{
		{Name: "roslyn/tuple.json", Role: "roslyn-tuple", Bytes: uint64(len(tuple)), SHA256: roslynDigest(tuple)},
		{Name: "roslyn/observed.json", Role: "roslyn-observation", Bytes: uint64(len(newObservation)), SHA256: roslynDigest(newObservation)},
		{Name: "roslyn/cases.json", Role: "roslyn-cases", Bytes: uint64(len(cases)), SHA256: roslynDigest(cases)},
	}
	set, err := json.Marshal(map[string]any{"schema": kit.EvidencePinSetSchema, "reader": kit.EvidencePinReader, "materials": materials})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := json.Marshal(map[string]string{"schema": kit.EvidencePinApprovalSchema, "pin_set_sha256": roslynDigest(set), "subject_sha256": roslynDigest(newObservation), "reader": kit.EvidencePinReader, "scope": kit.EvidencePinScope})
	if err != nil {
		t.Fatal(err)
	}
	envelope := ppexpect.Envelope{ApprovedApprovalSHA256: roslynDigest(approval), Approval: approval, ApprovedPinSetSHA256: roslynDigest(set), PinSet: set, Tuple: tuple}
	return newObservation, envelope
}

func roslynInventoryFixture(t *testing.T, dir string, cases, observed []byte, pins ppexpect.Envelope) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"cases.json": cases, "roslyn-observed.json": observed} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(pins)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roslyn-pins.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func roslynSelectProduction(t *testing.T, pins ppexpect.Envelope) {
	t.Helper()
	selection, err := json.Marshal(map[string]string{"schema": kit.EvidencePinSelectionSchema, "pin_set_sha256": pins.ApprovedPinSetSHA256})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := kit.SelectEvidenceProductionPin(ctx, selection, pins.PinSet); err != nil {
		t.Fatal(err)
	}
}

func TestQualificationInventoryRoslynPins(t *testing.T) {
	root := repository(t)
	base := filepath.Join(root, "src/testdata/csharp-pp")
	cases, err := os.ReadFile(filepath.Join(base, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(filepath.Join(base, "roslyn-observed.json"))
	if err != nil {
		t.Fatal(err)
	}
	legacyPins, err := ppexpect.ReadEnvelope(filepath.Join(base, "roslyn-pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	symbol, grammar := "tree_sitter_c_sharp", []kit.NativeInput{{Path: "src/parser.c", Role: "grammar-source", SHA256: strings.Repeat("0", 64), Bytes: 1}}
	t.Setenv("TSGK_PP_INVENTORY_DIR", "")
	old := symbolRole(t, root, symbol, grammar)
	legacyDoc, err := ppexpect.Read(cases, legacy, legacyPins.Tuple)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(old.Reason, legacyDoc.Schema) || !strings.Contains(old.Reason, legacyDoc.Roslyn) {
		t.Fatal("legacy inventory lost its observation provenance")
	}

	for _, id := range []string{"B", "C"} {
		candidate, pins := roslynCandidate(t, cases, legacy, id)
		roslynSelectProduction(t, pins)
		if _, err := ppexpect.Read(cases, candidate, pins.Tuple); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(symbolRole(t, root, symbol, grammar), old) {
			t.Fatalf("production %s changed preserved legacy qualification inventory", id)
		}
	}

	newObservation, newPins := roslynCandidate(t, cases, legacy, "new")
	newDir := filepath.Join(t.TempDir(), "approved-roslyn")
	roslynInventoryFixture(t, newDir, cases, newObservation, newPins)
	t.Setenv("TSGK_PP_INVENTORY_DIR", newDir)
	newRole := symbolRole(t, root, symbol, grammar)
	newDoc, err := ppexpect.Read(cases, newObservation, newPins.Tuple)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(newRole.Reason, newDoc.Schema) || !strings.Contains(newRole.Reason, newDoc.Roslyn) || strings.Contains(newRole.Reason, legacyDoc.Roslyn) {
		t.Fatalf("new inventory lacks approved provenance: %s", newRole.Reason)
	}
	var oldCase, newCase *kit.QualCase
	for i := range old.Workloads {
		for j := range old.Workloads[i].Cases {
			c := &old.Workloads[i].Cases[j]
			if c.ID == "ops-a" {
				oldCase = c
			}
		}
	}
	for i := range newRole.Workloads {
		for j := range newRole.Workloads[i].Cases {
			c := &newRole.Workloads[i].Cases[j]
			if c.ID == "ops-a" {
				newCase = c
			}
		}
	}
	if oldCase == nil || newCase == nil || reflect.DeepEqual(oldCase.PreprocessExpect, newCase.PreprocessExpect) {
		t.Fatal("inventory did not derive changed selection from new observation")
	}
}
