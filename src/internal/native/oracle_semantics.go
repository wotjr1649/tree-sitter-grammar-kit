package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

func semanticRecording(req OracleRequest, profile kit.OracleProfile) error {
	if req.QuerySemanticsRegistration == nil && profile.Native.Schema != kit.OracleSemanticsSchema {
		return nil
	}
	if req.QuerySemanticsRegistration == nil || len(req.QuerySemanticsRegistration) == 0 || len(req.QuerySemanticsRegistration) > kit.MaxDocumentBytes ||
		profile.Native.Schema != kit.OracleSemanticsSchema || sha(req.QuerySemanticsRegistration) != profile.QuerySemanticsRegistrationSHA256 {
		return refuse(kit.KindInvalidInput, "SEMANTICS_REGISTRATION_BINDING", nil)
	}
	n := profile.Native
	if n.Operation != "native-query" || n.Output != kit.OutputTree || n.Encoding != kit.EncodingUTF8 || n.Format != "" || n.SvcContext != nil || n.Preprocess != nil ||
		len(profile.OracleCases) != 1 || profile.FactPack != nil || req.FactPack != nil || req.ReuseAccepted != nil || req.RunWall != 0 ||
		!slices.Contains([]string{"windows/amd64", "linux/amd64"}, runtime.GOOS+"/"+runtime.GOARCH) {
		return refuse(kit.KindUnsupported, "SEMANTICS_CONTEXT_UNSUPPORTED", nil)
	}
	for _, pin := range n.Grammar {
		if pin.Bytes > kit.MaxDocumentBytes {
			return refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
		}
	}
	c := profile.OracleCases[0]
	if c.Encoding != kit.EncodingUTF8 || len(c.SvcSource) != 0 {
		return refuse(kit.KindUnsupported, "SEMANTICS_CONTEXT_UNSUPPORTED", nil)
	}
	return nil
}

// The local leaf cannot approve itself; the immutable projection keeps the prior verdict.
func semanticLocalCase(cr *CaseResult, registration string, capture *kit.ReplayMember, projection kit.ReplayMember) {
	cr.QuerySemantics = &kit.OracleQuerySemantics{Phase: "PRODUCER_LOCAL", Claim: ClaimBlocked, Code: "EXTERNAL_APPROVAL_PENDING", RegistrationSHA256: registration, Capture: capture, Projection: projection}
	if cr.ExecutionStatus == kit.StatusCompleted && cr.Assessment == kit.AssessPass {
		cr.Assessment, cr.Code = kit.AssessBlocked, cr.QuerySemantics.Code
	}
}

func semanticBuildFiles(ctx context.Context, b *Build) ([]captureFile, error) {
	root, err := os.OpenRoot(b.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var total int64 = b.ExecutableBytes
	for _, pin := range b.Closure {
		if pin.Bytes < 0 || pin.Bytes > kit.MaxDocumentBytes || total > int64(kit.ReplayOperations()["evidence-replay"].TotalBytes)-pin.Bytes {
			return nil, refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
		}
		total += pin.Bytes
	}
	var files []captureFile
	add := func(path, role string, data []byte) {
		files = append(files, captureFile{pin: kit.ReplayMember{Path: path, Role: role, Bytes: uint64(len(data)), SHA256: sha(data)}, data: data})
	}
	read := func(path string, size int64, digest string) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if size < 0 || size > kit.MaxDocumentBytes {
			return nil, refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
		}
		f, err := root.Open(filepath.FromSlash(path))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() || st.Size() != size {
			return nil, fmt.Errorf("semantic build input identity mismatch")
		}
		data, err := io.ReadAll(io.LimitReader(f, size+1))
		if err != nil || int64(len(data)) != size || sha(data) != digest {
			return nil, fmt.Errorf("semantic build input bytes mismatch")
		}
		return data, nil
	}
	data, err := read(filepath.Base(b.Executable), b.ExecutableBytes, b.ExecutableSHA256)
	if err != nil {
		return nil, err
	}
	add("capture/driver.bin", "driver", data)
	for i, pin := range b.Closure {
		path := pin.Path
		if pin.Role == "runtime" {
			path = "runtime/" + path
		} else if strings.HasPrefix(pin.Role, "grammar-") {
			path = "grammar/" + path
		}
		data, err := read(path, pin.Bytes, pin.SHA256)
		if err != nil {
			return nil, err
		}
		role := "build-input"
		if strings.HasPrefix(pin.Role, "grammar-") {
			role = pin.Role
		}
		add(fmt.Sprintf("capture/build-input-%05d.bin", i), role, data)
	}
	data, err = json.Marshal(b)
	if err != nil {
		return nil, err
	}
	add("capture/build.json", "build", data)
	if sha([]byte(b.identityText)) != b.Identity {
		return nil, fmt.Errorf("semantic build identity text unavailable")
	}
	add("capture/build-manifest.txt", "build-manifest", []byte(b.identityText))
	return files, nil
}

func semanticFailure(caller, live context.Context, cause error) (string, string, error) {
	var ne *Error
	errors.As(cause, &ne)
	if caller.Err() != nil {
		if ne != nil && ne.Kind == kit.KindCancelled {
			return kit.StatusCancelled, kit.AssessNotAssessed, ne
		}
		return kit.StatusCancelled, kit.AssessNotAssessed, refuse(kit.KindCancelled, "CANCELLED", caller.Err())
	}
	if live.Err() != nil {
		return kit.StatusResourceLimit, kit.AssessBlocked, refuse(kit.KindResourceLimit, "WALL_LIMIT", cause)
	}
	if ne == nil {
		ne = refuse(kit.KindIO, "SEMANTICS_BUILD_CAPTURE_FAILED", cause)
	}
	switch ne.Kind {
	case kit.KindResourceLimit:
		return kit.StatusResourceLimit, kit.AssessBlocked, ne
	case kit.KindCancelled:
		return kit.StatusCancelled, kit.AssessNotAssessed, ne
	case kit.KindInvalidInput:
		return kit.StatusNotRun, kit.AssessNotAssessed, ne
	case kit.KindUnsupported:
		return kit.StatusNotRun, kit.AssessBlocked, ne
	}
	if ne.Code == "BUILD_RESOURCE_LIMIT" {
		return kit.StatusResourceLimit, kit.AssessBlocked, refuse(kit.KindResourceLimit, ne.Code, ne)
	}
	return kit.StatusFailed, kit.AssessNotAssessed, ne
}
