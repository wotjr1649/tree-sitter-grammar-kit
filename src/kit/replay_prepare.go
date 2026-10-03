package kit

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

// PREPARE native probe evidence (src/dev/prepare-p05 probe): one ledger row per registered
// case and producer, each with the probe's raw stdout. The probe prints one line per tree
// ("original", then for an edit "damaged-incremental", "damaged-fresh", a comparison line,
// "restored-incremental", "restored-fresh", a comparison line) and exits 2 when the original
// has an error, an incremental tree differs from the fresh one, the damaged tree has no
// error, the restored tree has one or differs from the original; 0 otherwise. Its tree
// comparison covers exactly the printed fields, so equal printed trees are equal trees.

type ppEntry struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type ppLedger struct {
	Rows []struct {
		ID        string  `json:"id"`
		Producer  string  `json:"producer"`
		State     string  `json:"state"`
		ExitCode  *int    `json:"exit_code"`
		RawStdout *string `json:"raw_stdout"`
		Case      struct {
			Expected struct {
				Syntax string `json:"syntax"`
			} `json:"expected"`
			Edit *jsontext.Value `json:"edit"`
		} `json:"case"`
	} `json:"rows"`
}

type ppLine struct {
	Stage      string         `json:"stage"`
	HasError   *bool          `json:"has_error"`
	Tree       jsontext.Value `json:"tree"`
	Comparison string         `json:"comparison"`
	Equal      *bool          `json:"equal"`
}

func replayPrepareNative(x *replayEnv) *Error {
	im, e := x.oneMember("inventory")
	if e != nil {
		return e
	}
	lm, e := x.oneMember("ledger")
	if e != nil {
		return e
	}
	if x.prof.Records == nil {
		return fail(KindInvalidInput, "RECORDS_REQUIRED", "replay#/records", nil)
	}
	producer := x.prof.Identities["producer"]
	data, e := x.member(im.Path)
	if e != nil {
		return e
	}
	var inv []ppEntry
	if err := jsonv2.Unmarshal(data, &inv); err != nil {
		return fail(KindInvalidInput, "RECORD_INVALID", im.Path, nil)
	}
	ig := x.gate("inventory")
	entries := map[string]ppEntry{}
	for _, en := range inv {
		if !portable(en.Path, false) || len(en.SHA256) != 64 {
			ig.fail(en.Path, "INVENTORY_ENTRY_INVALID", "inventory 항목의 경로·hash 형식이 틀렸다")
			continue
		}
		if _, dup := entries[en.Path]; dup {
			ig.fail(en.Path, "INVENTORY_DUPLICATE", "inventory에 같은 경로가 두 번 있다")
			continue
		}
		entries[en.Path] = en
		x.covered[en.Path] = true
		// an entry not read here is checked by size only (recorded); an absent one stays
		// recorded too: absent raw is not recomputed, never a pass
		switch info := x.files[en.Path]; {
		case info == nil:
			ig.recorded()
		case uint64(info.Size()) != en.Bytes:
			ig.fail(en.Path, "INVENTORY_MEMBER_MISMATCH", "inventory 파일의 크기가 다르다")
		default:
			ig.recorded()
		}
	}
	if le, ok := entries[lm.Path]; !ok || le.SHA256 != lm.SHA256 || le.Bytes != lm.Bytes {
		x.finding("LEDGER_UNLISTED", lm.Path, "ledger가 inventory에 같은 identity로 없다")
	}
	ldata, e := x.member(lm.Path)
	if e != nil {
		return e
	}
	var led ppLedger
	if err := jsonv2.Unmarshal(ldata, &led); err != nil {
		return fail(KindInvalidInput, "RECORD_INVALID", lm.Path, nil)
	}
	if uint64(len(led.Rows)) > x.lim.Records {
		return fail(KindResourceLimit, "RECORD_COUNT_LIMIT", lm.Path, nil)
	}
	cons := newConsumer(x, x.prof.Records)
	bind, rawg, pv, sy := x.gate("ledger-binding"), x.gate("raw-binding"), x.gate("exit"), x.gate("syntax")
	var outs []caseOutcome
	for i, row := range led.Rows {
		if row.Producer != producer {
			x.cons.Excluded++
			continue
		}
		if !cons.take(row.ID, i) {
			continue
		}
		name := x.name(row.ID, i)
		if !bind.check(row.State == "EXITED" && row.ExitCode != nil && row.RawStdout != nil, name, "ROW_NOT_EXECUTED", "등록 row가 실행되지 않았거나 raw가 없다") {
			outs = append(outs, caseOutcome{status: StatusNotRun, assess: AssessNotAssessed})
			continue
		}
		en, ok := entries[*row.RawStdout]
		if !rawg.check(ok, name, "RAW_UNLISTED", "row의 raw stdout이 inventory에 없다") {
			continue
		}
		if x.files[en.Path] == nil {
			// the raw of this row is not present: recorded, not recomputed
			rawg.recorded()
			x.gate("exit").recorded()
			x.gate("syntax").recorded()
			outs = append(outs, caseOutcome{status: StatusCompleted, assess: AssessUnresolved})
			continue
		}
		raw, e := x.read(en.Path, en.Bytes, en.SHA256)
		if e != nil {
			if e.Kind == KindCancelled || e.Kind == KindIO || e.Kind == KindResourceLimit {
				return e
			}
			rawg.fail(name, e.Code, "row의 raw stdout이 inventory identity와 다르다")
			continue
		}
		rawg.pass()
		lines, ok := probeLines(raw)
		if !pv.check(ok && len(lines) > 0 && lines[0].Stage == "original" && lines[0].HasError != nil, name, "RAW_MALFORMED", "probe 출력 형식이 계약과 다르다") {
			continue
		}
		failed, ok := probeVerdict(lines, row.Case.Edit != nil)
		if !pv.check(ok, name, "RAW_MALFORMED", "probe 출력의 stage 순서나 비교 줄이 계약과 다르다") {
			continue
		}
		want := 0
		if failed {
			want = 2
		}
		pv.check(*row.ExitCode == want, name, "EXIT_MISMATCH", "tree에서 다시 계산한 probe 판정이 기록된 exit와 다르다")
		switch row.Case.Expected.Syntax {
		case "NO_ERROR_OR_MISSING":
			sy.check(!*lines[0].HasError, name, "SYNTAX_MISMATCH", "원본 tree에 오류가 있는데 기대는 오류 없음이다")
		case "ERROR_OR_MISSING_REQUIRED":
			sy.check(*lines[0].HasError, name, "SYNTAX_MISMATCH", "원본 tree에 오류가 없는데 기대는 오류 필수다")
		default:
			sy.recorded()
		}
		// the registered fact checks and edit windows were judged by the PREPARE review
		// helper, which is not ported: recorded, not recomputed
		x.gate("fact-checks").recorded()
		if row.Case.Edit != nil {
			x.gate("edit-recovery").recorded()
		}
		outs = append(outs, caseOutcome{status: StatusCompleted, assess: AssessUnresolved})
	}
	cons.close()
	x.actual["producer"] = producer
	x.actual["inventory"] = im.SHA256
	x.recomp = aggregateCases(outs)
	return nil
}

// probeLines splits the probe stdout into its JSON lines.
func probeLines(raw []byte) ([]ppLine, bool) {
	var out []ppLine
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1<<16), len(raw)+1)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var l ppLine
		if err := jsonv2.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, false
		}
		out = append(out, l)
	}
	return out, sc.Err() == nil
}

// probeVerdict recomputes the probe's failure decision from its printed trees.
func probeVerdict(lines []ppLine, edit bool) (bool, bool) {
	failed := *lines[0].HasError
	if !edit {
		return failed, len(lines) == 1
	}
	if len(lines) != 7 {
		return false, false
	}
	stages := []string{"original", "damaged-incremental", "damaged-fresh", "", "restored-incremental", "restored-fresh", ""}
	for i, s := range stages {
		if lines[i].Stage != s || (s != "" && (lines[i].HasError == nil || len(lines[i].Tree) == 0)) {
			return false, false
		}
	}
	for k, step := range []int{1, 4} {
		inc, fresh, cmp := lines[step], lines[step+1], lines[step+2]
		equal := bytes.Equal(inc.Tree, fresh.Tree)
		if cmp.Comparison != []string{"damaged", "restored"}[k] || cmp.Equal == nil || *cmp.Equal != equal {
			return false, false
		}
		if !equal || (k == 0 && !*inc.HasError) || (k == 1 && *inc.HasError) {
			failed = true
		}
	}
	if !bytes.Equal(lines[4].Tree, lines[0].Tree) {
		failed = true
	}
	return failed, true
}
