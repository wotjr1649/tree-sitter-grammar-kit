package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/wotjr1649/tree-sitter-grammar-kit/src/kit"
)

const semanticPagedContext = "full-input-utf8-paged-parser/r1"

type semanticCaptureMember struct {
	Owner string `json:"owner"`
	Path  string `json:"path"`
	Role  string `json:"role"`
}
type semanticCapturePlan struct {
	Context string
	Members []semanticCaptureMember
}

// Legacy registrations remain opaque; only an explicit paged context selects the new contract.
func semanticCaptureRegistration(caller context.Context, data []byte) (*semanticCapturePlan, error) {
	ctx, cancel := context.WithTimeout(caller, kit.NativeOperations()["native-query"].RunWall)
	defer cancel()
	check := func() error {
		if caller.Err() != nil {
			return refuse(kit.KindCancelled, "CANCELLED", caller.Err())
		}
		if ctx.Err() != nil {
			return refuse(kit.KindResourceLimit, "WALL_LIMIT", ctx.Err())
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	legacy := &semanticCapturePlan{Context: "full-input-utf8/r2"}
	if len(data) > kit.MaxDocumentBytes {
		return nil, refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
	}
	paged, err := semanticPagedSelected(data, check)
	if err != nil {
		return nil, err
	}
	if !paged {
		return legacy, nil
	}
	return semanticPagedRegistration(data, check)
}

// Extract the header and plan in the same bounded pass that checks the full JSON syntax.
func semanticPagedRegistration(data []byte, check func() error) (*semanticCapturePlan, error) {
	bad := func() error { return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil) }
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	closed := false
	tokens := 0
	read := func() (jsontext.Token, error) {
		t, err := semanticToken(dec, check)
		if err == io.EOF && closed {
			return t, err
		}
		if err != nil {
			var ne *Error
			if errors.As(err, &ne) {
				return t, err
			}
			return t, bad()
		}
		tokens++
		if tokens > 2000000 || dec.StackDepth() > 32 {
			return t, refuse(kit.KindResourceLimit, "SEMANTICS_REGISTRATION_LIMIT", nil)
		}
		return t, nil
	}
	t, err := read()
	if err != nil {
		return nil, err
	}
	if t.Kind() != '{' {
		return nil, bad()
	}
	plan := &semanticCapturePlan{}
	schema := ""
	seen := map[string]bool{}
	manifest := 0
	key := ""
	wantKey := true
	for {
		depth := dec.StackDepth()
		t, err := read()
		if err == io.EOF {
			if !closed {
				return nil, bad()
			}
			break
		}
		if err != nil {
			var ne *Error
			if errors.As(err, &ne) {
				return nil, err
			}
			return nil, bad()
		}
		if closed {
			return nil, bad()
		}
		if depth == 1 && t.Kind() == '}' {
			closed = true
			continue
		}
		if depth != 1 {
			continue
		}
		if wantKey {
			if t.Kind() != '"' {
				return nil, bad()
			}
			key = t.String()
			wantKey = false
			continue
		}
		wantKey = true
		switch key {
		case "schema", "context":
			if t.Kind() != '"' {
				return nil, bad()
			}
			if key == "schema" {
				schema = t.String()
			} else {
				plan.Context = t.String()
			}
		case "planned_members":
			if t.Kind() != '[' {
				return nil, bad()
			}
			for {
				t, err := read()
				if err != nil {
					return nil, err
				}
				if t.Kind() == ']' {
					break
				}
				if t.Kind() != '{' || len(plan.Members) >= 10000 {
					return nil, bad()
				}
				m := semanticCaptureMember{}
				fields := 0
				for {
					k, err := read()
					if err != nil {
						return nil, err
					}
					if k.Kind() == '}' {
						break
					}
					if k.Kind() != '"' {
						return nil, bad()
					}
					name := k.String()
					v, err := read()
					if err != nil {
						return nil, err
					}
					if v.Kind() != '"' {
						return nil, bad()
					}
					switch name {
					case "owner":
						m.Owner = v.String()
					case "path":
						m.Path = v.String()
					case "role":
						m.Role = v.String()
					default:
						return nil, bad()
					}
					fields++
				}
				owner := "D"
				if m.Role == "manifest" || m.Role == "record" || m.Role == "source" || m.Role == "projection" {
					owner = "O"
				}
				folded := strings.ToLower(m.Path)
				if fields != 3 || m.Owner != owner || !semanticPortablePath(m.Path) || !semanticMemberRole(m.Role) || seen[folded] {
					return nil, bad()
				}
				if m.Role == "manifest" {
					if m.Path != "manifest.json" {
						return nil, bad()
					}
					manifest++
				}
				seen[folded] = true
				plan.Members = append(plan.Members, m)
			}
		}
	}
	if schema != "tsgk-query-semantics-registration/r1" || plan.Context != semanticPagedContext || manifest != 1 || len(plan.Members) == 0 {
		return nil, bad()
	}
	return plan, nil
}

// Keep the producer's checks aligned with the offline portable path and identifier contract.
func semanticPortablePath(value string) bool {
	if value == "." || !utf8.ValidString(value) || !fs.ValidPath(value) || strings.ContainsAny(value, `\:`) {
		return false
	}
	for _, seg := range strings.Split(value, "/") {
		if strings.TrimRight(seg, " .") != seg {
			return false
		}
		base, _, _ := strings.Cut(seg, ".")
		base = strings.ToUpper(strings.TrimRight(base, " "))
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return false
		}
		for _, prefix := range []string{"COM", "LPT"} {
			if suffix, ok := strings.CutPrefix(base, prefix); ok && (len(suffix) == 1 && suffix[0] >= '1' && suffix[0] <= '9' || suffix == "¹" || suffix == "²" || suffix == "³") {
				return false
			}
		}
		for _, r := range seg {
			if r < 32 || r == 127 {
				return false
			}
		}
	}
	return true
}
func semanticMemberRole(role string) bool {
	if len(role) == 0 || len(role) > 128 {
		return false
	}
	for _, c := range role {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func (p *semanticCapturePlan) matches(files []captureFile, buildOnly bool) bool {
	want := map[string]string{}
	for _, m := range p.Members {
		if buildOnly && !(m.Role == "driver" || m.Role == "build" || m.Role == "build-manifest" || m.Role == "build-input" || strings.HasPrefix(m.Role, "grammar-")) {
			continue
		}
		if !buildOnly && m.Role == "manifest" && m.Path == "manifest.json" {
			continue
		}
		want[m.Path] = m.Role
	}
	if len(want) != len(files) {
		return false
	}
	for _, f := range files {
		if want[f.pin.Path] != f.pin.Role {
			return false
		}
		delete(want, f.pin.Path)
	}
	return len(want) == 0
}

type semanticParserPages struct {
	Schema  string `json:"schema"`
	Owner   string `json:"owner"`
	Logical struct {
		ClosureIndex int    `json:"closure_index"`
		Role         string `json:"role"`
		Path         string `json:"path"`
		Bytes        int64  `json:"bytes"`
		SHA256       string `json:"sha256"`
	} `json:"logical"`
	Parts []semanticParserPart `json:"parts"`
}
type semanticParserPart struct {
	Index  int    `json:"index"`
	Offset int64  `json:"offset"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func semanticParserFiles(ctx context.Context, root *os.Root, pin ClosureFile, index int) (files []captureFile, resultErr error) {
	if pin.Role != "grammar-parser" || pin.Bytes <= kit.MaxDocumentBytes || pin.Bytes > kit.BuildFileBytes {
		return nil, refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
	}
	f, err := root.Open("grammar/" + pin.Path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close parser capture: %w", err))
		}
	}()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != pin.Bytes {
		return nil, fmt.Errorf("semantic parser identity mismatch")
	}
	d := semanticParserPages{Schema: "tsgk-query-parser-pages/r1", Owner: "D", Parts: []semanticParserPart{}}
	d.Logical.ClosureIndex = index
	d.Logical.Role = pin.Role
	d.Logical.Path = pin.Path
	d.Logical.Bytes = pin.Bytes
	d.Logical.SHA256 = pin.SHA256
	h := sha256.New()
	for offset := int64(0); offset < pin.Bytes; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		size := min(int64(kit.MaxDocumentBytes), pin.Bytes-offset)
		data := make([]byte, int(size))
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, err
		}
		_, _ = h.Write(data)
		i := len(d.Parts)
		name := fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", index, i)
		digest := sha(data)
		d.Parts = append(d.Parts, semanticParserPart{Index: i, Offset: offset, Path: name, Bytes: size, SHA256: digest})
		files = append(files, captureFile{pin: kit.ReplayMember{Path: name, Role: "grammar-paged-part", Bytes: uint64(size), SHA256: digest}, data: data})
		offset += size
	}
	var extra [1]byte
	n, readErr := f.Read(extra[:])
	after, statErr := f.Stat()
	if n != 0 || readErr != io.EOF || statErr != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || hex.EncodeToString(h.Sum(nil)) != pin.SHA256 {
		return nil, fmt.Errorf("semantic parser bytes mismatch")
	}
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	files = append(files, captureFile{pin: kit.ReplayMember{Path: fmt.Sprintf("capture/build-input-%05d.pages.json", index), Role: "grammar-paged-descriptor", Bytes: uint64(len(data)), SHA256: sha(data)}, data: data})
	return files, nil
}

func (p *semanticCapturePlan) parserShape(grammar []kit.NativeInput) error {
	size := uint64(0)
	for _, g := range grammar {
		if g.Role == "parser" && g.Bytes > kit.MaxDocumentBytes {
			if size != 0 {
				return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
			}
			size = g.Bytes
		}
	}
	if size == 0 || size > kit.BuildFileBytes {
		return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
	}
	index := -1
	parts := map[string]bool{}
	for _, m := range p.Members {
		if m.Role == "grammar-paged-descriptor" {
			if index != -1 {
				return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
			}
			if n, err := fmt.Sscanf(m.Path, "capture/build-input-%d.pages.json", &index); err != nil || n != 1 || index < 0 || index >= 10000 || m.Path != fmt.Sprintf("capture/build-input-%05d.pages.json", index) {
				return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
			}
		}
		if m.Role == "grammar-paged-part" {
			parts[m.Path] = true
		}
	}
	count := (size + kit.MaxDocumentBytes - 1) / kit.MaxDocumentBytes
	if index < 0 || uint64(len(parts)) != count {
		return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
	}
	for i := uint64(0); i < count; i++ {
		if !parts[fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", index, i)] {
			return refuse(kit.KindInvalidInput, "SEMANTICS_MEMBER_PLAN", nil)
		}
	}
	return nil
}

// Selection reads only bounded bytes and fixed-size ASCII names, without a decoder depth limit.
func semanticPagedSelected(data []byte, check func() error) (selected bool, resultErr error) {
	defer func() {
		if err := check(); err != nil {
			selected = false
			resultErr = err
		}
	}()
	if len(data) > kit.MaxDocumentBytes {
		return false, refuse(kit.KindResourceLimit, "SEMANTICS_MEMBER_LIMIT", nil)
	}
	pos, next := 0, 0
	checkpoint := func() error {
		if pos < next {
			return nil
		}
		next = pos + 4096
		return check()
	}
	space := func() error {
		for pos < len(data) && (data[pos] == ' ' || data[pos] == '\r' || data[pos] == '\n' || data[pos] == '\t') {
			if err := checkpoint(); err != nil {
				return err
			}
			pos++
		}
		return nil
	}
	quoted := func() (start, end int, err error) {
		start = pos
		if pos >= len(data) || data[pos] != '"' {
			return 0, 0, nil
		}
		pos++
		for pos < len(data) {
			if err := checkpoint(); err != nil {
				return 0, 0, err
			}
			c := data[pos]
			pos++
			if c == '"' {
				return start, pos, nil
			}
			if c == '\\' {
				if pos < len(data) {
					pos++
				}
			}
		}
		return 0, 0, nil
	}
	matches := func(raw []byte, want string) bool {
		if len(raw) > 6*len(want)+2 {
			return false
		}
		var value string
		return jsonv2.Unmarshal(raw, &value) == nil && value == want
	}
	if err := space(); err != nil {
		return false, err
	}
	if pos >= len(data) || data[pos] != '{' {
		return false, nil
	}
	pos++
	for pos < len(data) {
		if err := space(); err != nil {
			return false, err
		}
		if pos >= len(data) || data[pos] == '}' {
			return false, nil
		}
		start, end, err := quoted()
		if err != nil {
			return false, err
		}
		if end == 0 {
			return false, nil
		}
		key := matches(data[start:end], "context")
		if err := space(); err != nil {
			return false, err
		}
		if pos >= len(data) || data[pos] != ':' {
			return false, nil
		}
		pos++
		if err := space(); err != nil {
			return false, err
		}
		start = pos
		depth := 0
		for pos < len(data) {
			if err := checkpoint(); err != nil {
				return false, err
			}
			c := data[pos]
			if depth == 0 && (c == ',' || c == '}') {
				break
			}
			if c == '"' {
				_, end, err := quoted()
				if err != nil {
					return false, err
				}
				if end == 0 {
					return false, nil
				}
				continue
			}
			pos++
			switch c {
			case '{', '[':
				depth++
			case '}', ']':
				if depth > 0 {
					depth--
				}
			}
		}
		if key && matches(bytes.TrimSpace(data[start:pos]), semanticPagedContext) {
			return true, nil
		}
		if pos >= len(data) || data[pos] == '}' {
			return false, nil
		}
		if data[pos] != ',' {
			return false, nil
		}
		pos++
	}
	return false, nil
}

// The last decoder read may observe cancellation while consuming trailing bytes.
func semanticToken(dec *jsontext.Decoder, check func() error) (jsontext.Token, error) {
	if err := check(); err != nil {
		return jsontext.Token{}, err
	}
	t, err := dec.ReadToken()
	if stopped := check(); stopped != nil {
		return jsontext.Token{}, stopped
	}
	return t, err
}
