package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	OraclePagedSemanticsManifestSchema = "tsgk-oracle-manifest/r6"
	semanticPagedContext               = "full-input-utf8-paged-parser/r1"
)

func semanticManifestSchema(schema string) bool {
	return schema == OracleSemanticsManifestSchema || schema == OraclePagedSemanticsManifestSchema
}

func semanticContextManifest(context string) string {
	if context == semanticPagedContext {
		return OraclePagedSemanticsManifestSchema
	}
	return OracleSemanticsManifestSchema
}

func semanticReducer(id string) bool { return id == "oracle-set-r4" || id == "oracle-set-r5" }

type semanticParserDescriptor struct {
	Schema  string `json:"schema"`
	Owner   string `json:"owner"`
	Logical struct {
		ClosureIndex uint64 `json:"closure_index"`
		Role         string `json:"role"`
		Path         string `json:"path"`
		Bytes        uint64 `json:"bytes"`
		SHA256       string `json:"sha256"`
	} `json:"logical"`
	Parts []struct {
		Index  uint64 `json:"index"`
		Offset uint64 `json:"offset"`
		Path   string `json:"path"`
		Bytes  uint64 `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"parts"`
}

// Parts come from the same verified snapshot; the logical file is never reassembled.
func (b *eligibilityBudget) semanticPagedParser(data []byte, input semanticBuildInput, index int, part func(ReplayMember) ([]byte, bool, *Error)) (bool, *Error) {
	var d semanticParserDescriptor
	if e := b.decode("parser-descriptor", data, &d, true); e != nil {
		return false, e
	}
	l := d.Logical
	if d.Schema != "tsgk-query-parser-pages/r1" || d.Owner != "D" || input.Role != "grammar-parser" || input.Bytes <= MaxDocumentBytes || input.Bytes > BuildFileBytes ||
		l.ClosureIndex != uint64(index) || l.Role != input.Role || l.Path != input.Path || l.Bytes != input.Bytes || l.SHA256 != input.SHA256 || !hex64.MatchString(l.SHA256) ||
		uint64(len(d.Parts)) != (input.Bytes+MaxDocumentBytes-1)/MaxDocumentBytes {
		return false, nil
	}
	h := sha256.New()
	for i, p := range d.Parts {
		offset := uint64(i) * MaxDocumentBytes
		size := min(uint64(MaxDocumentBytes), input.Bytes-offset)
		if p.Index != uint64(i) || p.Offset != offset || p.Bytes != size || p.Path != fmt.Sprintf("capture/build-input-%05d.part-%05d.bin", index, i) || !hex64.MatchString(p.SHA256) {
			return false, nil
		}
		bytes, ok, e := part(ReplayMember{Path: p.Path, Role: "grammar-paged-part", Bytes: p.Bytes, SHA256: p.SHA256})
		if e != nil || !ok {
			return false, e
		}
		if e := b.charge(1, 2*(p.Bytes+1), "parser-part-hash"); e != nil {
			return false, e
		}
		if uint64(len(bytes)) != p.Bytes || digestHex(bytes) != p.SHA256 {
			return false, nil
		}
		_, _ = h.Write(bytes)
	}
	return hex.EncodeToString(h.Sum(nil)) == input.SHA256, nil
}
