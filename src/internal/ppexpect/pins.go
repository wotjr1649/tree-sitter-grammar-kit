package ppexpect

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"reflect"
)

// Envelope is a caller-trusted development input, never discovered in an observation.
// Its approval and set use the shared EvidencePins resolver before Read is called.
type Envelope struct {
	ApprovedApprovalSHA256 string `json:"approved_approval_sha256"`
	Approval               []byte `json:"approval_base64"`
	ApprovedPinSetSHA256   string `json:"approved_pin_set_sha256"`
	PinSet                 []byte `json:"pin_set_base64"`
	Tuple                  []byte `json:"tuple_base64"`
}

// ReadEnvelope uses the existing CLI document bound for this trusted test input.
func ReadEnvelope(path string) (Envelope, error) {
	var e Envelope
	f, err := os.Open(path)
	if err != nil {
		return e, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if err != nil {
		return e, err
	}
	if len(b) > 16<<20 {
		return e, fmt.Errorf("Roslyn envelope exceeds document bound")
	}
	err = json.Unmarshal(b, &e, json.RejectUnknownMembers(true))
	if err == nil && (len(e.Tuple) == 0 || len(e.PinSet) == 0 || len(e.Approval) == 0 || e.ApprovedApprovalSHA256 == "" || e.ApprovedPinSetSHA256 == "") {
		err = fmt.Errorf("Roslyn approval envelope incomplete")
	}
	return e, err
}

type Provenance struct {
	SDK                string `json:"sdk"`
	OracleSHA256       string `json:"oracle_sha256"`
	CodeAnalysisSHA256 string `json:"code_analysis_sha256"`
	CSharpSHA256       string `json:"csharp_sha256"`
}

type Tuple struct {
	Schema            string      `json:"schema"`
	Reader            string      `json:"reader"`
	ObservationSHA256 string      `json:"observation_sha256"`
	CasesSHA256       string      `json:"cases_sha256"`
	ObservationSchema string      `json:"observation_schema"`
	Roslyn            string      `json:"roslyn"`
	LanguageVersion   string      `json:"language_version"`
	Scope             string      `json:"scope"`
	Provenance        *Provenance `json:"provenance,omitempty"`
}

type Document struct {
	Schema          string        `json:"schema"`
	CasesSHA256     string        `json:"cases_sha256"`
	Roslyn          string        `json:"roslyn"`
	LanguageVersion string        `json:"language_version"`
	Provenance      *Provenance   `json:"provenance,omitempty"`
	Cases           []Observation `json:"cases"`
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Read binds metadata to an independently approved tuple, after EvidencePins admission.
// legacy-observation supports expectations only; it never proves missing SDK/DLL identity.
func Read(cases, observed, tuple []byte) (d Document, err error) {
	if len(cases) > 1<<20 || len(observed) > 16<<20 || len(tuple) > 16<<20 {
		return d, fmt.Errorf("Roslyn document bound")
	}
	var pin Tuple
	if err = json.Unmarshal(tuple, &pin, json.RejectUnknownMembers(true)); err != nil {
		return d, err
	}
	if pin.Schema != "tsgk-roslyn-pin-tuple/r1" || pin.Reader != "tsgk-roslyn-selection/r1" {
		return d, fmt.Errorf("UNSUPPORTED Roslyn reader")
	}
	if pin.ObservationSHA256 != digest(observed) || pin.CasesSHA256 != digest(cases) {
		return d, fmt.Errorf("Roslyn tuple content mismatch")
	}
	if err = json.Unmarshal(observed, &d, json.RejectUnknownMembers(true)); err != nil {
		return d, err
	}
	if d.Schema != pin.ObservationSchema || d.CasesSHA256 != pin.CasesSHA256 || d.Roslyn == "" || d.Roslyn != pin.Roslyn || d.LanguageVersion != "Latest" || d.LanguageVersion != pin.LanguageVersion {
		return d, fmt.Errorf("Roslyn tuple metadata mismatch")
	}
	switch pin.Scope {
	case "legacy-observation":
		if d.Schema != ObservedSchema || d.Provenance != nil || pin.Provenance != nil {
			return d, fmt.Errorf("legacy Roslyn provenance mismatch")
		}
	case "sdk-dll-provenance":
		if d.Schema != ProvenanceSchema || d.Provenance == nil || !reflect.DeepEqual(d.Provenance, pin.Provenance) {
			return d, fmt.Errorf("BLOCKED Roslyn SDK/DLL provenance missing or mismatched")
		}
		p := d.Provenance
		validHash := func(s string) bool {
			b, err := hex.DecodeString(s)
			return err == nil && len(b) == 32 && hex.EncodeToString(b) == s
		}
		if p.SDK == "" || !validHash(p.OracleSHA256) || !validHash(p.CodeAnalysisSHA256) || !validHash(p.CSharpSHA256) {
			return d, fmt.Errorf("BLOCKED Roslyn SDK/DLL provenance incomplete")
		}
	default:
		return d, fmt.Errorf("UNSUPPORTED Roslyn provenance scope")
	}
	var corpus struct {
		Schema string `json:"schema"`
		Cases  []struct {
			ID       string   `json:"id"`
			Source   []byte   `json:"source"`
			Symbols  []string `json:"symbols"`
			Features []string `json:"features"`
		} `json:"cases"`
	}
	if err = json.Unmarshal(cases, &corpus, json.RejectUnknownMembers(true)); err != nil {
		return d, err
	}
	if corpus.Schema != "tsgk-csharp-pp-cases/r1" || len(d.Cases) == 0 || len(d.Cases) != len(corpus.Cases) {
		return d, fmt.Errorf("Roslyn case coverage mismatch")
	}
	seen := map[string]bool{}
	for i, c := range corpus.Cases {
		if c.ID == "" || seen[c.ID] || d.Cases[i].ID != c.ID {
			return d, fmt.Errorf("Roslyn case identity mismatch")
		}
		seen[c.ID] = true
	}
	return d, nil
}
