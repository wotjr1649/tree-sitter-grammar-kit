package kit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The grounds of G1 (#233): the places in the pinned grammar text or the specification that support a correction or a row
// of an audit. The offline check (g1CheckGround) sees the form of a ground; the check against the documents themselves
// (CheckG1GroundList) needs the documents and is env-gated. The ground pins (csharp_g1_ground_pins.go) close the gap
// between the two: each audit row has the SHA-256 of its normalized grounds in a tracked table, so a quote that was
// invented, copied from another row or edited fails without the documents.

const (
	// where a ground is written
	G1DocGrammar = "grammar.js"
	G1DocSpec    = "spec"

	// the documents the grounds quote, by SHA-256: the grammar.js of the commit the kind map pins and the files of
	// the specification (dotnet/csharpstandard at the commit of g1gSpecPre) the grounds name
	g1gGrammarSHA = "63d357e04fdbe05f424463f365fb595f8f05866711cc279f85a7288638e753bd"
	g1gLexSHA     = "332d112957907adc721cf549ccaa05af85e582f5342aa6c4edfbbdba933060d2"
	g1gExprSHA    = "bcc4c11c6d25aae3584d038aa7272adae5e26820f62d0120424f2c180fede64c"
	g1gTypesSHA   = "294308e75f99d6416c41ee65eea655fb0a3bc1bd14d32dcf134946b0ad049bf4"
	g1gClassesSHA = "0d6ac924504cdf5e4bf6974639a5853fa0c77ce00dd09c833b7607477864623f"
	g1gUnsafeSHA  = "ba0202020568b066e46b30bdb5146b1c29676d6a0b3a1c286cfad8ab5d7d4a91"

	g1gMinQuote = 40  // characters of a quote, whitespace runs counted as one
	g1gMaxGap   = 200 // lines between the heading and the quote
)

// g1gSpecSHA is the SHA-256 of each specification file a ground may name.
var g1gSpecSHA = map[string]string{"lexical-structure.md": g1gLexSHA, "expressions.md": g1gExprSHA, "types.md": g1gTypesSHA, "classes.md": g1gClassesSHA, "unsafe-code.md": g1gUnsafeSHA}

// G1SpecFiles are the specification files a ground may name.
var G1SpecFiles = []string{"lexical-structure.md", "expressions.md", "types.md", "classes.md", "unsafe-code.md"}

var g1gSpecAt = regexp.MustCompile(`standard/([a-z-]+\.md) ([0-9]+(?:\.[0-9]+)*) `)

// G1Ground is one place in the pinned grammar text or the specification that supports a correction.
// Quote is a verbatim text of that place (whitespace runs count as one space). The ground is checkable without the
// document: DocSHA256 names the document (the pinned grammar.js, or the pinned specification file), Heading is the
// heading line of the section At names (a markdown heading of the specification, "<rule>:" of the grammar) and Gap is
// the number of lines from that heading line to the first line of the quote. The offline check requires a quote of at
// least g1gMinQuote characters, the document SHA-256 pinned below and a heading that carries the section of At within
// g1gMaxGap lines; the check against the document itself (CheckG1Grounds) needs the documents and is a dev tool step.
type G1Ground struct {
	Doc       string `json:"doc"`
	At        string `json:"at"`
	Quote     string `json:"quote"`
	DocSHA256 string `json:"doc_sha256"`
	Heading   string `json:"heading"`
	Gap       int    `json:"gap"`
}

// g1CheckGround is the offline check of one ground: a quote of at least g1gMinQuote characters; the SHA-256 of the
// document it quotes, which is the pinned one (the grammar.js of the kind map's commit, or the specification file the
// ground names); and a heading, within g1gMaxGap lines of the quote, that carries the section the ground names (the
// rule name of a grammar ground, the section number of a specification ground).
func g1CheckGround(g G1Ground) bool {
	if g.At == "" || utf8.RuneCountInString(G1CorrectionNorm(g.Quote)) < g1gMinQuote || g.Gap < 0 || g.Gap > g1gMaxGap || strings.TrimSpace(g.Heading) != g.Heading || g.Heading == "" {
		return false
	}
	switch g.Doc {
	case G1DocGrammar:
		return g.DocSHA256 == g1gGrammarSHA && g.Heading == g.At+":"
	case G1DocSpec:
		m := g1gSpecAt.FindStringSubmatch(g.At)
		if m == nil || g1gSpecSHA[m[1]] == "" || g.DocSHA256 != g1gSpecSHA[m[1]] {
			return false
		}
		return regexp.MustCompile(`^#+ ` + regexp.QuoteMeta(m[2]) + ` `).MatchString(g.Heading)
	}
	return false
}

// G1CorrectionNorm collapses whitespace runs, the form a ground's quote is compared in.
func G1CorrectionNorm(s string) string { return strings.Join(strings.Fields(s), " ") }

// G1GroundPin is the SHA-256 of the grounds of one audit row, in order, each in its normalized form (the document, the
// place, the quote with whitespace runs collapsed, the document SHA-256, the heading and the gap). A change to any of
// them, a quote that was never in a document, and the grounds of another row all give another pin.
func G1GroundPin(gs []G1Ground) string {
	var b strings.Builder
	for _, g := range gs {
		b.WriteString(strings.Join([]string{g.Doc, g.At, G1CorrectionNorm(g.Quote), g.DocSHA256, g.Heading, strconv.Itoa(g.Gap)}, "\x1f"))
		b.WriteByte('\x1e')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// CheckG1GroundList checks the grounds of one item against the documents, as CheckG1Grounds does for a correction.
func CheckG1GroundList(item string, grounds []G1Ground, grammar string, spec map[string]string) error {
	for _, g := range grounds {
		doc := grammar
		if g.Doc == G1DocSpec {
			doc = ""
			for name, text := range spec {
				if strings.Contains(g.At, "standard/"+name+" ") {
					doc = text
				}
			}
		}
		bad := func(why string) error {
			return fmt.Errorf("%s: the %s ground %q: %s", item, g.Doc, g.At, why)
		}
		if sum := sha256.Sum256([]byte(doc)); hex.EncodeToString(sum[:]) != g.DocSHA256 {
			return bad("the document is not the pinned one")
		}
		if !g1gHeadingGap(doc, g) {
			return bad("the quote is not exactly Gap lines after the heading line")
		}
	}
	return nil
}

// g1gHeadingGap reports whether some place of doc holds the quote, starting on a line that is Gap lines after a line
// that begins with the heading (the quote may start in the middle of that line).
func g1gHeadingGap(doc string, g G1Ground) bool {
	lines := strings.Split(strings.ReplaceAll(doc, "\r\n", "\n"), "\n")
	norm := make([]string, len(lines))
	for i, l := range lines {
		norm[i] = G1CorrectionNorm(l)
	}
	quote := G1CorrectionNorm(g.Quote)
	for h, l := range norm {
		if !strings.HasPrefix(l, g.Heading) || h+g.Gap >= len(norm) {
			continue
		}
		// the quote starts on the line h+Gap: that line and the following ones, joined, hold it from a place in that line
		first := len(norm[h+g.Gap])
		var text string
		for _, l := range norm[h+g.Gap:] {
			if l == "" { // a blank line is whitespace: the quote may run over it
				continue
			}
			if text += l + " "; len(text) > first+len(quote) {
				break
			}
		}
		if i := strings.Index(text, quote); i >= 0 && i < first {
			return true
		}
	}
	return false
}

// G1GroundPinSource writes a pin table as the Go source of csharp_g1_ground_pins.go (a dev tool step: the tools print the
// tables of the tracked assets with -pins, and a change of a ground is reviewed as a change of the table).
func G1GroundPinSource(name string, pins map[string]string) string {
	keys := make([]string, 0, len(pins))
	for k := range pins {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	b.WriteString("var " + name + " = map[string]string{\n")
	for _, k := range keys {
		b.WriteString("\t" + strconv.Quote(k) + ": " + strconv.Quote(pins[k]) + ",\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// G1Audit76GroundPins are the pins of the rows of a position audit: one per row and one per node of a row ("<id>#node").
func G1Audit76GroundPins(rows []G1Audit76Row) map[string]string {
	pins := map[string]string{}
	for _, r := range rows {
		pins[r.ID] = G1GroundPin(r.Grounds)
		if r.Node != nil {
			pins[r.ID+"#node"] = G1GroundPin(r.Node.Grounds)
		}
	}
	return pins
}

// G1TypeAudit76GroundPins are the pins of the rows of a type audit.
func G1TypeAudit76GroundPins(rows []G1TypeAudit76Row) map[string]string {
	pins := map[string]string{}
	for _, r := range rows {
		pins[r.ID] = G1GroundPin(r.Grounds)
	}
	return pins
}
