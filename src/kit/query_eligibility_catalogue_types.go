package kit

type eligibilityCatalogue struct {
	Schema           string                            `json:"schema"`
	UpstreamCommit   string                            `json:"upstream_commit"`
	ArchiveSHA256    string                            `json:"archive_sha256"`
	BindingSHA256    string                            `json:"binding_sha256"`
	ControllerSHA256 map[string]string                 `json:"controller_sha256"`
	ReferenceInputs  map[string]string                 `json:"reference_inputs"`
	GrammarInputs    map[string]eligibilitySnapshotRow `json:"grammar_inputs"`
	Builds           []eligibilityBuildRule            `json:"builds"`
	Units            []string                          `json:"units"`
	Controls         []eligibilityControlRule          `json:"controls"`
	Assertions       []eligibilityAssertionRule        `json:"assertions"`
	Inputs           map[string]eligibilityFixedInput  `json:"inputs"`
	Trees            map[string][]TreeNode             `json:"trees"`
	Captures         map[string]Capture                `json:"captures"`
	Responses        map[string]eligibilityExpected    `json:"responses"`
}

type eligibilitySnapshotRow struct {
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type eligibilityFixedInput struct {
	Bytes      uint64 `json:"bytes"`
	SHA256     string `json:"sha256"`
	Hex        string `json:"hex,omitempty"`
	RepeatByte string `json:"repeat_byte,omitempty"`
}

type eligibilityBuildRule struct {
	ID           string   `json:"id"`
	Tool         string   `json:"tool"`
	Args         []string `json:"args"`
	Dir          string   `json:"dir"`
	WallSeconds  uint64   `json:"wall_seconds"`
	GraceSeconds uint64   `json:"grace_seconds"`
	MemoryBytes  uint64   `json:"memory_bytes"`
	StdoutBytes  uint64   `json:"stdout_bytes"`
	StderrBytes  uint64   `json:"stderr_bytes"`
}

type eligibilityControlRule struct {
	ID                     string   `json:"id"`
	Rule                   string   `json:"rule"`
	Negative               bool     `json:"negative"`
	ExpectedSupervisorExit int      `json:"expected_supervisor_exit"`
	ExecutionStatus        string   `json:"execution_status"`
	ExecutionReason        string   `json:"execution_reason"`
	FailureCode            string   `json:"failure_code"`
	WallSeconds            uint64   `json:"wall_seconds"`
	MemoryBytes            uint64   `json:"memory_bytes"`
	StdoutBytes            uint64   `json:"stdout_bytes"`
	StderrBytes            uint64   `json:"stderr_bytes"`
	Tool                   string   `json:"tool"`
	Dir                    string   `json:"dir"`
	GraceSeconds           uint64   `json:"grace_seconds"`
	Args                   []string `json:"args"`
	QueryIDs               []string `json:"query_ids"`
	Responses              []string `json:"responses"`
}

type eligibilityAssertionRule struct {
	ID                  string   `json:"id"`
	RequiredInvocations []string `json:"required_invocations"`
}

type eligibilityExpected struct {
	Source       string           `json:"source"`
	Query        string           `json:"query"`
	ID           string           `json:"id"`
	Tree         string           `json:"tree"`
	Patterns     uint32           `json:"patterns"`
	CaptureNames []string         `json:"capture_names"`
	Predicates   []QueryPredicate `json:"predicates"`
	Structural   []string         `json:"structural"`
	Evaluated    []string         `json:"evaluated"`
}
