package kit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func semanticBinaryTestDescriptor(kind string, size uint64, block []byte) (semanticBinaryDescriptor, map[string][]byte, string) {
	owner, path, prefix := "D", "capture/driver.bin", "capture/driver"
	if kind == "library" {
		owner, path, prefix = "control-compile", "current/library.bin", "current/library.bin"
	}
	d := semanticBinaryDescriptor{Schema: semanticBinaryPagesSchema, Owner: owner,
		Logical: semanticBinaryLogical{Kind: kind, Path: path, Bytes: size}}
	parts := map[string][]byte{}
	h := sha256.New()
	for i, offset := uint64(0), uint64(0); offset < size; i++ {
		n := min(uint64(MaxDocumentBytes), size-offset)
		data := block[:n]
		name := fmt.Sprintf("%s.part-%05d.bin", prefix, i)
		parts[name] = data
		d.Parts = append(d.Parts, struct {
			Index  uint64 `json:"index"`
			Offset uint64 `json:"offset"`
			Path   string `json:"path"`
			Bytes  uint64 `json:"bytes"`
			SHA256 string `json:"sha256"`
		}{i, offset, name, n, digestHex(data)})
		_, _ = h.Write(data)
		offset += n
	}
	d.Logical.SHA256 = hex.EncodeToString(h.Sum(nil))
	return d, parts, prefix
}

func TestSemanticPagedBinaryValidation(t *testing.T) {
	t.Parallel()
	block := bytes.Repeat([]byte("p"), MaxDocumentBytes)
	for _, kind := range []string{"driver", "library"} {
		t.Run(kind, func(t *testing.T) {
			d, parts, prefix := semanticBinaryTestDescriptor(kind, MaxDocumentBytes+1, block)
			for _, mutation := range []string{"normal", "offset", "order", "drop", "duplicate", "whole-hash", "part-rehashed", "missing", "index", "owner", "truncated", "kind", "path", "schema", "extra-empty-part"} {
				t.Run(mutation, func(t *testing.T) {
					copy := d
					copy.Parts = append(copy.Parts[:0:0], d.Parts...)
					supplied := map[string][]byte{}
					for p, data := range parts {
						supplied[p] = data
					}
					switch mutation {
					case "offset":
						copy.Parts[1].Offset--
					case "order":
						copy.Parts[0], copy.Parts[1] = copy.Parts[1], copy.Parts[0]
					case "drop":
						copy.Parts = copy.Parts[:1]
					case "duplicate":
						copy.Parts[1] = copy.Parts[0]
					case "whole-hash":
						copy.Logical.SHA256 = digestHex([]byte("other"))
					case "part-rehashed":
						supplied[copy.Parts[1].Path] = []byte("q")
						copy.Parts[1].SHA256 = digestHex([]byte("q"))
					case "missing":
						delete(supplied, copy.Parts[1].Path)
					case "index":
						copy.Parts[1].Index++
					case "owner":
						copy.Owner = "other"
					case "truncated":
						supplied[copy.Parts[0].Path] = supplied[copy.Parts[0].Path][:MaxDocumentBytes-1]
					case "kind":
						copy.Logical.Kind = "other"
					case "path":
						copy.Logical.Path = "other.bin"
					case "schema":
						copy.Schema = "tsgk-query-binary-pages/r0"
					case "extra-empty-part":
						extra := copy.Parts[1]
						extra.Index, extra.Offset, extra.Bytes = 2, copy.Logical.Bytes, 0
						extra.Path, extra.SHA256 = fmt.Sprintf("%s.part-00002.bin", prefix), digestHex(nil)
						copy.Parts = append(copy.Parts, extra)
						supplied[extra.Path] = nil
					}
					logical, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, copy), d.Owner, kind, d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
						data, found := supplied[p.Path]
						return data, found, nil
					})
					if e != nil || ok != (mutation == "normal") || ok && logical != d.Logical {
						t.Fatalf("ok=%v logical=%+v err=%v", ok, logical, e)
					}
				})
			}
		})
	}
}

func TestSemanticPagedBinaryBoundaries(t *testing.T) {
	t.Parallel()
	block := bytes.Repeat([]byte("p"), MaxDocumentBytes)
	for _, kind := range []string{"driver", "library"} {
		t.Run(kind, func(t *testing.T) {
			for _, test := range []struct {
				name  string
				size  uint64
				reads int
			}{
				{"16MiB", MaxDocumentBytes, 0},
				{"16MiB-plus1", MaxDocumentBytes + 1, 2},
				{"32MiB", 2 * MaxDocumentBytes, 2},
				{"32MiB-plus1", 2*MaxDocumentBytes + 1, 3},
				{"64MiB", 4 * MaxDocumentBytes, 4},
				{"64MiB-plus1", 4*MaxDocumentBytes + 1, 0},
			} {
				t.Run(test.name, func(t *testing.T) {
					d, parts, prefix := semanticBinaryTestDescriptor(kind, test.size, block)
					reads := 0
					logical, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, d), d.Owner, kind, d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
						reads++
						data, found := parts[p.Path]
						return data, found, nil
					})
					if e != nil || ok != (test.reads > 0) || reads != test.reads || ok && logical.Bytes != test.size {
						t.Fatalf("ok=%v reads=%d want=%d logical=%+v err=%v", ok, reads, test.reads, logical, e)
					}
				})
			}
		})
	}
}

func TestSemanticPagedBinaryCharge(t *testing.T) {
	t.Parallel()
	d, parts, prefix := semanticBinaryTestDescriptor("driver", 2*MaxDocumentBytes+1, bytes.Repeat([]byte("p"), MaxDocumentBytes))
	b := semanticTestBudget(t)
	var charges [][2]uint64
	_, ok, e := b.semanticPagedBinary(eligibilityJSON(t, d), d.Owner, "driver", d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
		charges = append(charges, [2]uint64{b.work, b.logical})
		data, found := parts[p.Path]
		return data, found, nil
	})
	if e != nil || !ok || len(charges) != 3 {
		t.Fatalf("ok=%v charges=%v err=%v", ok, charges, e)
	}
	charges = append(charges, [2]uint64{b.work, b.logical})
	for i, size := range []uint64{MaxDocumentBytes, MaxDocumentBytes, 1} {
		work, logical := charges[i+1][0]-charges[i][0], charges[i+1][1]-charges[i][1]
		if work != 1 || logical != 2*(size+1) {
			t.Fatalf("part %d charge=(%d,%d) want=(1,%d)", i, work, logical, 2*(size+1))
		}
	}
}

func TestSemanticPagedBinaryRoleBinding(t *testing.T) {
	t.Parallel()
	d, parts, prefix := semanticBinaryTestDescriptor("driver", MaxDocumentBytes+1, bytes.Repeat([]byte("p"), MaxDocumentBytes))
	for _, role := range []string{"driver-paged-part", "grammar-paged-part", "library-paged-part"} {
		t.Run(role, func(t *testing.T) {
			pins := map[string]ReplayMember{}
			for _, p := range d.Parts {
				pins[p.Path] = ReplayMember{Path: p.Path, Role: role, Bytes: p.Bytes, SHA256: p.SHA256}
			}
			_, ok, e := semanticTestBudget(t).semanticPagedBinary(eligibilityJSON(t, d), d.Owner, "driver", d.Logical.Path, prefix, func(p ReplayMember) ([]byte, bool, *Error) {
				pin, found := pins[p.Path]
				return parts[p.Path], found && pin.Role == p.Role && pin.Bytes == p.Bytes && pin.SHA256 == p.SHA256, nil
			})
			if e != nil || ok != (role == "driver-paged-part") {
				t.Fatalf("role=%s ok=%v err=%v", role, ok, e)
			}
		})
	}
}
