package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"unicode/utf8"
)

// This deliberately supports RFC JSON only, not Tree-sitter recovery or extensions.
type jsonCapture struct {
	Name      string
	Type      string
	Text      string
	StartByte int
	EndByte   int
	Pattern   int
	Match     int
	Capture   int
}

func jsonExpect(source []byte) ([]jsonCapture, error) {
	if len(source) > maxSource || !utf8.Valid(source) || !json.Valid(source) {
		return nil, fmt.Errorf("independent rule requires valid UTF-8 RFC JSON <=65536 bytes")
	}
	d := json.NewDecoder(bytes.NewReader(source))
	d.UseNumber()
	counts := [3]int{}
	out := []jsonCapture{}
	var value func(int) (byte, int, int, error)
	value = func(depth int) (byte, int, int, error) {
		if depth > 64 {
			return 0, 0, 0, fmt.Errorf("independent JSON rule depth exceeds 64")
		}
		start := int(d.InputOffset())
		for start < len(source) && bytes.ContainsRune([]byte(" \t\r\n,:"), rune(source[start])) {
			start++
		}
		token, err := d.Token()
		if err != nil {
			return 0, 0, 0, err
		}
		kind := source[start]
		if delim, ok := token.(json.Delim); ok {
			for d.More() {
				if delim == '{' {
					_, ks, ke, err := value(depth + 1)
					if err != nil {
						return 0, 0, 0, err
					}
					// Reserve the parent's match before walking nested object members.
					at := int(d.InputOffset())
					for at < len(source) && bytes.ContainsRune([]byte(" \t\r\n:"), rune(source[at])) {
						at++
					}
					pattern, name := -1, ""
					switch source[at] {
					case '[':
						pattern, name = 0, "array_key"
					case '{':
						pattern, name = 1, "object_key"
					case '"':
						pattern, name = 2, "key"
					}
					match := 0
					if pattern >= 0 {
						match = counts[pattern]
						counts[pattern]++
					}
					vk, vs, ve, err := value(depth + 1)
					if err != nil {
						return 0, 0, 0, err
					}
					if pattern >= 0 {
						out = append(out, jsonCapture{name, "string", string(source[ks:ke]), ks, ke, pattern, match, 0})
						if vk == '"' {
							out = append(out, jsonCapture{"value", "string", string(source[vs:ve]), vs, ve, pattern, match, 1})
						}
					}
				} else {
					if _, _, _, err := value(depth + 1); err != nil {
						return 0, 0, 0, err
					}
				}
			}
			if _, err = d.Token(); err != nil {
				return 0, 0, 0, err
			}
		}
		return kind, start, int(d.InputOffset()), nil
	}
	if _, _, _, err := value(0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("JSON trailing input: %v", err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartByte < out[j].StartByte })
	// Like the runtime cursor and the reference, number matches query-wide in first-observed capture order,
	// and index captures by the fixed query's capture-name declaration order.
	global := map[[2]int]int{}
	for i := range out {
		key := [2]int{out[i].Pattern, out[i].Match}
		if _, ok := global[key]; !ok {
			global[key] = len(global)
		}
		out[i].Match = global[key]
		out[i].Capture = slices.Index(jsonQueryCaptureNames, out[i].Name)
	}
	return out, nil
}

// jsonQueryCaptureNames is the capture-name order of the fixed JSON route query.
var jsonQueryCaptureNames = []string{"array_key", "object_key", "key", "value"}
