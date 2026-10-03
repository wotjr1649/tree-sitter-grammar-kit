package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ReproduceSchema is the strict reproduction profile accepted by this build. Decoding is
// offline; executing the profile belongs to the CLI's runner, never to this package.
const ReproduceSchema = "tsgk-reproduce/r1"

// Generation modes. JS regenerates from grammar.js and its declared JS closure; JSON
// regenerates from src/grammar.json and can never prove the JS closure.
const (
	ModeJS   = "js"
	ModeJSON = "json"
)

// Reference states of a registered output.
const (
	ReferencePresent = "PRESENT"
	ReferenceAbsent  = "ABSENT"
)

// Ceilings of the adopted generator operation. A profile may narrow but never raise them;
// only the postgresql-sql route has the 6 GiB generation memory exception.
const (
	GenWallSeconds   = 300
	GenOutputBytes   = 8388608
	GenStorageBytes  = 536870912
	GenMemoryBytes   = 4294967296
	GenPGMemoryBytes = 6442450944
	GenInputFiles    = 64
	GenInputBytes    = 67108864
	GenFileBytes     = 16777216
)

var inputRoles = map[string]bool{"grammar_js": true, "js_helper": true, "lock": true, "dependency": true, "grammar_json": true, "scanner": true, "header": true, "metadata": true}

// ToolIdentity pins one executable by content; the path is supplied at run time.
type ToolIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Bytes   uint64 `json:"bytes"`
}

// ReproduceInput is one file of the immutable declared source snapshot.
type ReproduceInput struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
	Bytes  uint64 `json:"bytes"`
}

// ReproduceOutput is one registered generated output, relative to the generator output
// directory, with its reference artifact or an explicit ABSENT reference.
type ReproduceOutput struct {
	Path      string `json:"path"`
	Reference string `json:"reference"`
	SHA256    string `json:"sha256,omitempty"`
	Bytes     uint64 `json:"bytes,omitempty"`
}

// ReproduceLimits are the finite generator bounds of one profile.
type ReproduceLimits struct {
	WallSeconds  uint64 `json:"wall_seconds"`
	OutputBytes  uint64 `json:"output_bytes"`
	StorageBytes uint64 `json:"storage_bytes"`
	MemoryBytes  uint64 `json:"memory_bytes"`
	InputFiles   uint64 `json:"input_files"`
	InputBytes   uint64 `json:"input_bytes"`
	FileBytes    uint64 `json:"file_bytes"`
}

// ReproduceProfile is a decoded tsgk-reproduce/r1 document.
type ReproduceProfile struct {
	SHA256    string            `json:"sha256"`
	ID        string            `json:"id"`
	Route     string            `json:"route"`
	Mode      string            `json:"mode"`
	Generator ToolIdentity      `json:"generator"`
	JSRuntime *ToolIdentity     `json:"js_runtime,omitempty"`
	ABI       uint64            `json:"abi"`
	Optimize  bool              `json:"optimize"`
	Grammar   string            `json:"grammar"`
	Inputs    []ReproduceInput  `json:"inputs"`
	Outputs   []ReproduceOutput `json:"outputs"`
	Limits    ReproduceLimits   `json:"limits"`
}

// ParseReproduceProfile strictly decodes a reproduction profile. It rejects unknown or
// duplicate members, raised limits, an unsupported ABI, a JS runtime in JSON mode (or its
// absence in JS mode) and an entry grammar outside the declared snapshot.
func ParseReproduceProfile(data []byte) (ReproduceProfile, error) {
	p, e := parseReproduce(data)
	if e != nil {
		return ReproduceProfile{}, e
	}
	return p, nil
}

func parseReproduce(data []byte) (ReproduceProfile, *Error) {
	var p ReproduceProfile
	t := typed{doc: "reproduce"}
	v, e := decodeStrict(t.doc, data, MaxDocumentBytes)
	if e != nil {
		return p, e
	}
	m, e := t.object(v, []string{"schema", "id", "route", "mode", "generator", "abi", "optimize", "grammar", "inputs", "outputs", "limits"}, "js_runtime")
	if e != nil {
		return p, e
	}
	if s, e := t.str(m["schema"]); e != nil {
		return p, e
	} else if s != ReproduceSchema {
		return p, t.bad("SCHEMA_UNSUPPORTED", m["schema"])
	}
	sum := sha256.Sum256(data)
	p.SHA256 = hex.EncodeToString(sum[:])
	for _, f := range []struct {
		dst  *string
		name string
	}{{&p.ID, "id"}, {&p.Route, "route"}} {
		s, e := t.str(m[f.name])
		if e != nil {
			return p, e
		}
		if !validID(s) {
			return p, t.bad(strings.ToUpper(f.name)+"_INVALID", m[f.name])
		}
		*f.dst = s
	}
	if p.Mode, e = t.str(m["mode"]); e != nil {
		return p, e
	} else if p.Mode != ModeJS && p.Mode != ModeJSON {
		return p, t.bad("MODE_INVALID", m["mode"])
	}
	if p.Generator, e = parseTool(t, m["generator"]); e != nil {
		return p, e
	}
	if x := m["js_runtime"]; x != nil {
		if p.Mode != ModeJS {
			return p, t.bad("JS_RUNTIME_IN_JSON_MODE", x)
		}
		tool, e := parseTool(t, x)
		if e != nil {
			return p, e
		}
		p.JSRuntime = &tool
	} else if p.Mode == ModeJS {
		return p, t.bad("JS_RUNTIME_REQUIRED", m["mode"])
	}
	if p.ABI, e = t.uint(m["abi"]); e != nil {
		return p, e
	} else if p.ABI != 14 && p.ABI != 15 {
		return p, t.bad("ABI_UNSUPPORTED", m["abi"])
	}
	if p.Optimize, e = t.boolean(m["optimize"]); e != nil {
		return p, e
	}
	if p.Limits, e = parseGenLimits(t, m["limits"], p.Route); e != nil {
		return p, e
	}
	if p.Inputs, e = parseInputs(t, m["inputs"], p.Limits); e != nil {
		return p, e
	}
	if p.Grammar, e = t.str(m["grammar"]); e != nil {
		return p, e
	}
	want := map[string]string{ModeJS: "grammar_js", ModeJSON: "grammar_json"}[p.Mode]
	found := false
	for _, in := range p.Inputs {
		if in.Path == p.Grammar {
			found = in.Role == want
		}
	}
	if !found {
		return p, t.bad("GRAMMAR_ENTRY_INVALID", m["grammar"])
	}
	if p.Outputs, e = parseOutputs(t, m["outputs"]); e != nil {
		return p, e
	}
	return p, nil
}

func parseTool(t typed, v *jv) (ToolIdentity, *Error) {
	var tool ToolIdentity
	m, e := t.object(v, []string{"name", "version", "sha256", "bytes"})
	if e != nil {
		return tool, e
	}
	if tool.Name, e = t.str(m["name"]); e != nil {
		return tool, e
	} else if !validID(tool.Name) {
		return tool, t.bad("TOOL_NAME_INVALID", m["name"])
	}
	if tool.Version, e = t.str(m["version"]); e != nil {
		return tool, e
	} else if !validID(tool.Version) {
		return tool, t.bad("TOOL_VERSION_INVALID", m["version"])
	}
	if tool.SHA256, e = hexDigest(t, m["sha256"]); e != nil {
		return tool, e
	}
	if tool.Bytes, e = t.uint(m["bytes"]); e != nil {
		return tool, e
	} else if tool.Bytes == 0 {
		return tool, t.bad("TOOL_BYTES_INVALID", m["bytes"])
	}
	return tool, nil
}

func hexDigest(t typed, v *jv) (string, *Error) {
	s, e := t.str(v)
	if e != nil {
		return "", e
	}
	if len(s) != 64 || strings.Trim(s, "0123456789abcdef") != "" {
		return "", t.bad("SHA256_INVALID", v)
	}
	return s, nil
}

func parseGenLimits(t typed, v *jv, route string) (ReproduceLimits, *Error) {
	var l ReproduceLimits
	names := []string{"wall_seconds", "output_bytes", "storage_bytes", "memory_bytes", "input_files", "input_bytes", "file_bytes"}
	m, e := t.object(v, names)
	if e != nil {
		return l, e
	}
	memCeil := uint64(GenMemoryBytes)
	if route == "postgresql-sql" {
		memCeil = GenPGMemoryBytes
	}
	dst := []*uint64{&l.WallSeconds, &l.OutputBytes, &l.StorageBytes, &l.MemoryBytes, &l.InputFiles, &l.InputBytes, &l.FileBytes}
	ceil := []uint64{GenWallSeconds, GenOutputBytes, GenStorageBytes, memCeil, GenInputFiles, GenInputBytes, GenFileBytes}
	for i, name := range names {
		n, e := t.uint(m[name])
		if e != nil {
			return l, e
		}
		if n == 0 || n > ceil[i] {
			return l, t.bad("PROFILE_LIMIT_INVALID", m[name])
		}
		*dst[i] = n
	}
	return l, nil
}

func parseInputs(t typed, v *jv, l ReproduceLimits) ([]ReproduceInput, *Error) {
	list, e := t.array(v)
	if e != nil {
		return nil, e
	}
	if len(list) == 0 {
		return nil, t.bad("EMPTY_SELECTION", v)
	}
	if uint64(len(list)) > l.InputFiles {
		return nil, t.bad("INPUT_FILES_LIMIT", v)
	}
	var out []ReproduceInput
	var paths []*jv
	var total uint64
	for _, item := range list {
		f, e := t.object(item, []string{"path", "role", "sha256", "bytes"})
		if e != nil {
			return nil, e
		}
		var in ReproduceInput
		if in.Path, e = t.str(f["path"]); e != nil {
			return nil, e
		} else if !portable(in.Path, false) {
			return nil, t.bad("SELECTION_INVALID", f["path"])
		}
		if in.Role, e = t.str(f["role"]); e != nil {
			return nil, e
		} else if !inputRoles[in.Role] {
			return nil, t.bad("ROLE_INVALID", f["role"])
		}
		if in.SHA256, e = hexDigest(t, f["sha256"]); e != nil {
			return nil, e
		}
		if in.Bytes, e = t.uint(f["bytes"]); e != nil {
			return nil, e
		}
		if in.Bytes > l.FileBytes {
			return nil, t.bad("FILE_BYTES_LIMIT", f["bytes"])
		}
		if total += in.Bytes; total > l.InputBytes {
			return nil, t.bad("INPUT_BYTES_LIMIT", f["bytes"])
		}
		out = append(out, in)
		paths = append(paths, f["path"])
	}
	if e := sortedUnique(t, "SELECTION", paths); e != nil {
		return nil, e
	}
	return out, nil
}

func parseOutputs(t typed, v *jv) ([]ReproduceOutput, *Error) {
	list, e := t.array(v)
	if e != nil {
		return nil, e
	}
	if len(list) == 0 {
		return nil, t.bad("OUTPUTS_EMPTY", v)
	}
	var out []ReproduceOutput
	var paths []*jv
	for _, item := range list {
		f, e := t.object(item, []string{"path", "reference"}, "sha256", "bytes")
		if e != nil {
			return nil, e
		}
		var o ReproduceOutput
		if o.Path, e = t.str(f["path"]); e != nil {
			return nil, e
		} else if !portable(o.Path, false) {
			return nil, t.bad("OUTPUT_PATH_INVALID", f["path"])
		}
		if o.Reference, e = t.str(f["reference"]); e != nil {
			return nil, e
		}
		switch o.Reference {
		case ReferencePresent:
			if f["sha256"] == nil || f["bytes"] == nil {
				return nil, t.bad("REFERENCE_IDENTITY_REQUIRED", f["reference"])
			}
			if o.SHA256, e = hexDigest(t, f["sha256"]); e != nil {
				return nil, e
			}
			if o.Bytes, e = t.uint(f["bytes"]); e != nil {
				return nil, e
			}
		case ReferenceAbsent:
			if f["sha256"] != nil || f["bytes"] != nil {
				return nil, t.bad("REFERENCE_IDENTITY_FORBIDDEN", f["reference"])
			}
		default:
			return nil, t.bad("REFERENCE_INVALID", f["reference"])
		}
		out = append(out, o)
		paths = append(paths, f["path"])
	}
	if e := sortedUnique(t, "OUTPUT", paths); e != nil {
		return nil, e
	}
	return out, nil
}
