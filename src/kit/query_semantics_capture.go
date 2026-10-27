package kit

import (
	"encoding/binary"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit/nativewire"
)

type semanticCaptureSpec struct {
	Path        string        `json:"Path"`
	Args        []string      `json:"Args"`
	Dir         string        `json:"Dir"`
	Env         []string      `json:"Env"`
	Stdin       *ReplayMember `json:"Stdin"`
	Interactive bool          `json:"Interactive"`
	StdoutBytes int64         `json:"StdoutBytes"`
	StderrBytes int64         `json:"StderrBytes"`
	Wall        int64         `json:"Wall"`
	Grace       int64         `json:"Grace"`
	Memory      struct {
		Bytes uint64 `json:"Bytes"`
		Hard  bool   `json:"Hard"`
	} `json:"Memory"`
	CgroupParent string `json:"CgroupParent"`
}

type semanticCaptureError struct {
	Type    string  `json:"type"`
	Code    string  `json:"code"`
	Kind    *string `json:"kind"`
	Blocked *bool   `json:"blocked"`
}

type semanticProcessCapture struct {
	Schema  string                `json:"schema"`
	Stage   string                `json:"stage"`
	Spec    *semanticCaptureSpec  `json:"spec"`
	Process *ReplayMember         `json:"process"`
	Stdout  *ReplayMember         `json:"stdout"`
	Stderr  *ReplayMember         `json:"stderr"`
	Error   *semanticCaptureError `json:"error"`
}

func (b *eligibilityBudget) processCapture(doc string, data []byte) (semanticProcessCapture, *Error) {
	var capture semanticProcessCapture
	if e := b.decode(doc, data, &capture, true); e != nil {
		return capture, e
	}
	bad := func(code string) (semanticProcessCapture, *Error) {
		return capture, fail(KindInvalidInput, code, doc, nil)
	}
	if capture.Schema != "tsgk-native-process-capture/r1" {
		return capture, fail(KindUnsupported, "SCHEMA_UNSUPPORTED", doc, nil)
	}
	if capture.Error != nil {
		e := capture.Error
		switch e.Type {
		case "native":
			if !validID(e.Code) || e.Kind == nil || *e.Kind == "" || e.Blocked != nil {
				return bad("SEMANTICS_CAPTURE_ERROR")
			}
		case "runner":
			if !validID(e.Code) || e.Kind != nil || e.Blocked == nil {
				return bad("SEMANTICS_CAPTURE_ERROR")
			}
		case "other":
			if e.Code != "" || e.Kind != nil || e.Blocked != nil {
				return bad("SEMANTICS_CAPTURE_ERROR")
			}
		default:
			return bad("SEMANTICS_CAPTURE_ERROR")
		}
	}
	switch capture.Stage {
	case "VERIFY_REFUSED":
		if capture.Spec != nil || capture.Process != nil || capture.Stdout != nil || capture.Stderr != nil || capture.Error == nil || capture.Error.Type != "native" {
			return bad("SEMANTICS_CAPTURE_STAGE")
		}
	case "RUN_RETURNED":
		if capture.Spec == nil || capture.Process == nil || capture.Error != nil && capture.Error.Type == "native" {
			return bad("SEMANTICS_CAPTURE_STAGE")
		}
	default:
		return bad("SEMANTICS_CAPTURE_STAGE")
	}
	pins := []struct {
		pin  *ReplayMember
		role string
	}{{capture.Process, "process"}, {capture.Stdout, "stdout-frame"}, {capture.Stderr, "stderr"}}
	if capture.Spec != nil {
		pins = append(pins, struct {
			pin  *ReplayMember
			role string
		}{capture.Spec.Stdin, "stdin-frame"})
	}
	if e := b.charge(4, 256, doc+"/pin-index"); e != nil {
		return capture, e
	}
	seen := map[string]bool{}
	for _, value := range pins {
		if value.pin == nil {
			continue
		}
		pin := value.pin
		if e := b.charge(20*uint64(len(pin.Path))+uint64(len(pin.SHA256)+len(pin.Role))+1, 16*uint64(len(pin.Path)), doc+"/pins"); e != nil {
			return capture, e
		}
		if !portable(pin.Path, false) || pin.Role != value.role || !hex64.MatchString(pin.SHA256) || pin.Bytes > MaxDocumentBytes || seen[pin.Path] {
			return bad("SEMANTICS_CAPTURE_PIN")
		}
		seen[pin.Path] = true
	}
	return capture, nil
}

// Return the original body only after the complete frame length has been checked.
func (b *eligibilityBudget) semanticFrame(doc string, frame []byte) ([]byte, *Error) {
	if e := b.charge(1, 0, doc); e != nil {
		return nil, e
	}
	if len(frame) < 4 || uint64(binary.BigEndian.Uint32(frame)) != uint64(len(frame)-4) {
		return nil, fail(KindInvalidInput, "SEMANTICS_FRAME_LENGTH", doc, nil)
	}
	// The small offline file cap includes the header, unlike the native payload cap.
	if uint64(len(frame)) > ReplayOperations()["evidence-replay"].FileBytes {
		return nil, fail(KindResourceLimit, "DOCUMENT_BYTES_LIMIT", doc, nil)
	}
	return frame[4:], nil
}

func (b *eligibilityBudget) nativeRequestFrame(doc string, frame []byte) (semanticNativeRequest, *Error) {
	payload, e := b.semanticFrame(doc, frame)
	if e != nil {
		return semanticNativeRequest{}, e
	}
	request, e := b.nativeRequest(doc, payload)
	if e != nil {
		return request, e
	}
	if request.Protocol != nativewire.ProtocolR4 {
		return request, fail(KindUnsupported, "SEMANTICS_REQUEST_REVISION", doc, nil)
	}
	return request, nil
}
