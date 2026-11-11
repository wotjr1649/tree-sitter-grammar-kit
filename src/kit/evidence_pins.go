package kit

import (
	"context"
	"strings"
)

const (
	EvidencePinSetSchema       = "tsgk-evidence-pin-set/r1"
	EvidencePinApprovalSchema  = "tsgk-evidence-pin-approval/r1"
	EvidencePinSelectionSchema = "tsgk-evidence-pin-selection/r1"
	EvidencePinResultSchema    = "tsgk-evidence-pin-result/r1"
	EvidencePinReader          = "tsgk-evidence-materials/r1"
	EvidencePinScope           = "historical-materials"
)

// EvidencePinMaterial identifies original bytes, not an executable or a verification rule.
type EvidencePinMaterial struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type evidencePinSet struct {
	Schema    string                `json:"schema"`
	Reader    string                `json:"reader"`
	Materials []EvidencePinMaterial `json:"materials"`
}

type evidencePinApproval struct {
	Schema        string `json:"schema"`
	PinSetSHA256  string `json:"pin_set_sha256"`
	SubjectSHA256 string `json:"subject_sha256"`
	Reader        string `json:"reader"`
	Scope         string `json:"scope"`
}

// EvidencePinsRequest requires independently approved raw digests outside the evidence.
// Materials are supplied bytes; names are never opened as host paths. The caller must
// keep all input slices and maps unchanged until the call returns.
type EvidencePinsRequest struct {
	Approval               []byte
	ApprovedApprovalSHA256 string
	PinSet                 []byte
	ApprovedPinSetSHA256   string
	Subject                []byte
	Materials              map[string][]byte
}

// EvidencePinsResult attests material binding only, not eligibility or execution.
type EvidencePinsResult struct {
	Report
	ResultSchema   string                `json:"result_schema"`
	Reader         string                `json:"reader"`
	Scope          string                `json:"scope"`
	ApprovalSHA256 string                `json:"approval_sha256"`
	PinSetSHA256   string                `json:"pin_set_sha256"`
	SubjectSHA256  string                `json:"subject_sha256"`
	Materials      []EvidencePinMaterial `json:"materials"`
	Required       uint64                `json:"required"`
	Verified       uint64                `json:"verified"`
	BytesRead      uint64                `json:"bytes_read"`
	Work           uint64                `json:"work"`
	LogicalBytes   uint64                `json:"logical_bytes"`
	Complete       bool                  `json:"complete"`
}

func (b *eligibilityBudget) pinBytes(name string, data []byte, limit uint64) *Error {
	if uint64(len(data)) > limit {
		return fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", name, nil)
	}
	if e := b.r.chargeRead(uint64(len(data)), ReplayOperations()["evidence-replay"].TotalBytes, name); e != nil {
		return e
	}
	return b.charge(1, uint64(len(data)), name)
}

func (b *eligibilityBudget) pinDescriptors(set evidencePinSet) (map[string]bool, *Error) {
	if set.Schema != EvidencePinSetSchema || set.Reader != EvidencePinReader {
		return nil, fail(KindUnsupported, "PIN_READER_UNSUPPORTED", "pin-set", nil)
	}
	if len(set.Materials) == 0 {
		return nil, fail(KindInvalidInput, "PIN_MATERIALS_EMPTY", "pin-set", nil)
	}
	if uint64(len(set.Materials)) > ReplayOperations()["evidence-replay"].Files {
		return nil, fail(KindResourceLimit, "FILE_COUNT_LIMIT", "pin-set", nil)
	}
	if e := b.charge(uint64(len(set.Materials)), 256*uint64(len(set.Materials)), "pin-index"); e != nil {
		return nil, e
	}
	seen, expected := map[string]bool{}, map[string]bool{}
	for _, pin := range set.Materials {
		if e := b.charge(1, uint64(len(pin.Name)+len(pin.Role)), "pin-descriptor"); e != nil {
			return nil, e
		}
		key := strings.ToLower(pin.Name)
		if len(pin.Name) > 1024 || !portable(pin.Name, false) || seen[key] || !hex64.MatchString(pin.SHA256) {
			return nil, fail(KindInvalidInput, "PIN_MATERIAL_DESCRIPTOR", "pin-set", nil)
		}
		seen[key], expected[pin.Name] = true, true
		if pin.Bytes > ReplayOperations()["evidence-replay"].FileBytes {
			return nil, fail(KindResourceLimit, "FILE_BYTES_LIMIT", pin.Name, nil)
		}
		switch pin.Role {
		case "reference-catalogue", "reference-input", "reference-tuple", "runtime-tuple", "runtime-manifest", "runtime-patch", "runtime-readers", "driver", "cp949", "shim-template", "roslyn-observation", "roslyn-cases":
		default:
			return nil, fail(KindUnsupported, "PIN_ROLE_UNSUPPORTED", pin.Name, nil)
		}
	}
	return expected, nil
}

// SelectEvidenceProductionPin selects exactly one candidate before production starts.
// Selection is not approval; historical resolution never consults this pointer.
func SelectEvidenceProductionPin(ctx context.Context, selection, pinSet []byte) (string, error) {
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		return "", e
	}
	defer r.cancel()
	b := &eligibilityBudget{r: r}
	if e := b.pinBytes("pin-selection", selection, MaxDocumentBytes); e != nil {
		return "", e
	}
	var pointer struct {
		Schema       string `json:"schema"`
		PinSetSHA256 string `json:"pin_set_sha256"`
	}
	if e := b.decode("pin-selection", selection, &pointer, true); e != nil {
		return "", e
	}
	if pointer.Schema != EvidencePinSelectionSchema {
		return "", fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "pin-selection", nil)
	}
	if !hex64.MatchString(pointer.PinSetSHA256) {
		return "", fail(KindInvalidInput, "PIN_SELECTION_DIGEST", "pin-selection", nil)
	}
	if len(pinSet) == 0 {
		return "", fail(KindInvalidInput, "PIN_SELECTION_MISSING", "pin-set", nil)
	}
	if e := b.pinBytes("pin-set", pinSet, MaxDocumentBytes); e != nil {
		return "", e
	}
	if e := b.charge(1, uint64(len(pinSet)), "pin-selection-digest"); e != nil {
		return "", e
	}
	if digestHex(pinSet) != pointer.PinSetSHA256 {
		return "", fail(KindInvalidInput, "PIN_SELECTION_MISMATCH", "pin-set", nil)
	}
	var set evidencePinSet
	if e := b.decode("pin-set", pinSet, &set, true); e != nil {
		return "", e
	}
	if _, e := b.pinDescriptors(set); e != nil {
		return "", e
	}
	return pointer.PinSetSHA256, nil
}

// ResolveEvidencePins binds selected historical bytes to caller-approved digests and
// one original subject. It performs no catalogue, runtime, Roslyn or replay evaluation.
func ResolveEvidencePins(ctx context.Context, req EvidencePinsRequest) (res EvidencePinsResult, resultErr error) {
	r, e := startRun(ctx, ReplayOperations()["evidence-replay"].Wall)
	if e != nil {
		res = EvidencePinsResult{Report: newReport("evidence pins"), ResultSchema: EvidencePinResultSchema,
			Reader: EvidencePinReader, Scope: EvidencePinScope, Materials: []EvidencePinMaterial{}}
		failReport(&res.Report, e)
		return res, e
	}
	defer r.cancel()
	return resolveEvidencePins(req, &eligibilityBudget{r: r})
}

func resolveEvidencePins(req EvidencePinsRequest, b *eligibilityBudget) (res EvidencePinsResult, resultErr error) {
	res = EvidencePinsResult{Report: newReport("evidence pins"), ResultSchema: EvidencePinResultSchema,
		Reader: EvidencePinReader, Scope: EvidencePinScope, Materials: []EvidencePinMaterial{}}
	r := b.r
	read, work, logical := r.totalRead, b.work, b.logical
	res.EvidenceMode, res.Assessment = ModeReplayedRaw, AssessPass
	defer func() {
		res.BytesRead, res.Work, res.LogicalBytes = r.totalRead-read, b.work-work, b.logical-logical
		if resultErr != nil {
			known := res.Assessment == AssessFail
			failReport(&res.Report, asError(resultErr))
			if known {
				res.Assessment = AssessFail
			}
			res.Complete = false
		}
	}()
	finding := func(code, at string, bad bool) {
		severity := "warning"
		if bad {
			res.Assessment, severity = AssessFail, "error"
		} else if res.Assessment != AssessFail {
			res.Assessment = AssessBlocked
		}
		if len(res.Findings) < 1000 {
			res.Findings = append(res.Findings, Finding{Code: code, Path: at, Severity: severity, Message: "승인된 역사 자료 결속이 없거나 원문과 모순된다"})
		}
	}
	if req.ApprovedApprovalSHA256 == "" || req.ApprovedPinSetSHA256 == "" || len(req.Approval) == 0 || len(req.PinSet) == 0 {
		finding("PIN_APPROVAL_MISSING", "approval", false)
		return res, nil
	}
	if !hex64.MatchString(req.ApprovedApprovalSHA256) || !hex64.MatchString(req.ApprovedPinSetSHA256) {
		return res, fail(KindInvalidInput, "PIN_APPROVAL_DIGEST", "approval", nil)
	}
	for _, doc := range []struct {
		name string
		data []byte
	}{{"pin-approval", req.Approval}, {"pin-set", req.PinSet}, {"pin-subject", req.Subject}} {
		if e := b.pinBytes(doc.name, doc.data, MaxDocumentBytes); e != nil {
			return res, e
		}
	}
	if e := b.charge(3, uint64(len(req.Approval)+len(req.PinSet)+len(req.Subject)), "pin-digests"); e != nil {
		return res, e
	}
	res.ApprovalSHA256, res.PinSetSHA256, res.SubjectSHA256 = digestHex(req.Approval), digestHex(req.PinSet), digestHex(req.Subject)
	if res.ApprovalSHA256 != req.ApprovedApprovalSHA256 || res.PinSetSHA256 != req.ApprovedPinSetSHA256 {
		finding("PIN_APPROVAL_MISMATCH", "approval", true)
		return res, nil
	}
	var approval evidencePinApproval
	var set evidencePinSet
	if e := b.decode("pin-approval", req.Approval, &approval, true); e != nil {
		return res, e
	}
	if e := b.decode("pin-set", req.PinSet, &set, true); e != nil {
		return res, e
	}
	if approval.Schema != EvidencePinApprovalSchema || set.Schema != EvidencePinSetSchema {
		return res, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", "pins", nil)
	}
	if approval.Reader != EvidencePinReader || set.Reader != EvidencePinReader || approval.Scope != EvidencePinScope {
		return res, fail(KindUnsupported, "PIN_READER_UNSUPPORTED", "pins", nil)
	}
	if !hex64.MatchString(approval.SubjectSHA256) || !hex64.MatchString(approval.PinSetSHA256) {
		return res, fail(KindInvalidInput, "PIN_APPROVAL_DIGEST", "pin-approval", nil)
	}
	if approval.PinSetSHA256 != res.PinSetSHA256 || approval.SubjectSHA256 != res.SubjectSHA256 {
		finding("PIN_SUBJECT_MISMATCH", "pin-approval", true)
		return res, nil
	}
	if uint64(len(req.Materials)) > ReplayOperations()["evidence-replay"].Files {
		return res, fail(KindResourceLimit, "FILE_COUNT_LIMIT", "pins", nil)
	}
	expected, e := b.pinDescriptors(set)
	if e != nil {
		return res, e
	}
	res.Required = uint64(len(set.Materials))
	if e := b.charge(uint64(len(req.Materials)), 128*uint64(len(req.Materials)), "pin-input-index"); e != nil {
		return res, e
	}
	for name := range req.Materials {
		if e := b.charge(1, uint64(len(name)), "pin-input-name"); e != nil {
			return res, e
		}
		if !expected[name] {
			return res, fail(KindInvalidInput, "PIN_MATERIAL_UNUSED", "materials", nil)
		}
	}
	for _, pin := range set.Materials {
		if e := b.charge(1, 0, "pin-material"); e != nil {
			return res, e
		}
		data, ok := req.Materials[pin.Name]
		if !ok {
			finding("PIN_MATERIAL_MISSING", pin.Name, false)
			continue
		}
		if e := b.pinBytes(pin.Name, data, ReplayOperations()["evidence-replay"].FileBytes); e != nil {
			return res, e
		}
		if e := b.charge(1, uint64(len(data)), "pin-material-digest"); e != nil {
			return res, e
		}
		if uint64(len(data)) != pin.Bytes || digestHex(data) != pin.SHA256 {
			finding("PIN_MATERIAL_MISMATCH", pin.Name, true)
			continue
		}
		res.Verified++
		res.Materials = append(res.Materials, pin)
	}
	res.Complete = true
	return res, nil
}
