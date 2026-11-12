package kit

// The pinned full inventory and registry are decoded once in every offline pass.
// This is the existing decode model, without proposed K0-K4 structural savings.
const ordinaryFixedDocumentDecodeLogical uint64 = 353877949

// OrdinaryReferenceEstimate is a density estimate, never an admission upper bound.
// Node/capture strings and additional native/API/fact/shape reservations remain unknown.
type OrdinaryReferenceEstimate struct {
	Density                 uint64 `json:"source_bytes_per_node"`
	SourceBytes             uint64 `json:"source_bytes"`
	VersionsTextBytes       uint64 `json:"versions_text_bytes"`
	RowBytes                uint64 `json:"row_bytes"`
	ReceiptBytes            uint64 `json:"receipt_bytes"`
	ReceiptLogical          uint64 `json:"receipt_logical"`
	DoubleRowDecodeLogical  uint64 `json:"double_row_decode_logical"`
	ReserveReferenceLogical uint64 `json:"reserve_reference_logical"`
	TotalLogical            uint64 `json:"total_logical"`
	PreviousModelLogical    uint64 `json:"previous_model_logical"`
}

// Inputs come from bounded source histories; callers use fixed densities 8 and 2.
func ordinaryReferenceEstimate(sourceBytes, versionsText, density uint64) OrdinaryReferenceEstimate {
	n := max(uint64(1), (sourceBytes+density-1)/density)
	c := (n + 7) / 8
	row := 1024 + 192*n + 256*c
	values := 20*n + 16*c + 32
	receipt := (4*(row+1)+2)/3 + 2048
	// Receipt: guarded retain, decodeValue, envelope values, and base64 decoding.
	wrap := 4*(receipt+1) + 39*receipt + 32768 + 448*64 + 4*receipt
	// decodeReferenceResponseBound decodes both the head and full row.
	decode := 2 * (39*row + 32768 + 448*values)
	// Structural queries still reserve text for every capture.
	reserve := 2 * (versionsText + 1) * (1 + c)
	previous := 4*(row+1) + 36*row + 32768 + 448*values
	return OrdinaryReferenceEstimate{density, sourceBytes, versionsText, row, receipt, wrap, decode, reserve, wrap + decode + reserve, previous}
}
