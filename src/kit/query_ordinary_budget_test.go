package kit

import "testing"

func TestOrdinaryReferenceBudgetIncludesReceiptAndBothDecodes(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		source, density, old, total uint64
	}{
		{"postgresql-nominal", 4547, 8, 10809604, 30249029},
		{"tsql-nominal", 24678, 8, 58141956, 177534284},
		{"tsql-dense", 24678, 2, 232269572, 708656264},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := ordinaryReferenceEstimate(tc.source, tc.source, tc.density)
			if e.PreviousModelLogical != tc.old || e.TotalLogical != tc.total || e.TotalLogical != e.ReceiptLogical+e.DoubleRowDecodeLogical+e.ReserveReferenceLogical {
				t.Fatalf("reference cost: %+v", e)
			}
			longer := ordinaryReferenceEstimate(tc.source, 2*tc.source, tc.density)
			if longer.TotalLogical-e.TotalLogical != longer.ReserveReferenceLogical-e.ReserveReferenceLogical || longer.TotalLogical <= e.TotalLogical {
				t.Fatal("full-history text was omitted")
			}
		})
	}
	if got := int64(1<<30) - 1017472466 - 141748 - int64(ordinaryFixedDocumentDecodeLogical); got != -297750339 {
		t.Fatalf("PostgreSQL fixed-document omission: %d", got)
	}
	if got := int64(1<<30) - 1061314324 - 202898 - int64(ordinaryFixedDocumentDecodeLogical); got != -341653347 {
		t.Fatalf("TSQL fixed-document omission: %d", got)
	}
}
