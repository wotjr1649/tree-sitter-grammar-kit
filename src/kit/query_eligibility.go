package kit

import (
	"context"
	"encoding/json/jsontext"
)

const (
	QueryReferenceEligibilityApprovalSchema     = "tsgk-query-reference-eligibility-approval/r1"
	QueryReferenceEligibilityRegistrationSchema = "tsgk-query-reference-eligibility-registration/r1"
	QueryReferenceEligibilityResultSchema       = "tsgk-query-reference-eligibility-result/r1"
	queryEligibilityWitnessSchema               = "tsgk-query-witness-registration/r2"
)

// QueryReferenceEligibilityRequest carries caller-trusted approval separately from stored evidence.
// Approval attests provenance; this offline operation does not authenticate its author.
type QueryReferenceEligibilityRequest struct {
	Evidence            EvidenceRequest
	Approval            []byte
	Registration        []byte
	WitnessRegistration []byte
}

// QueryEligibilityCounts partitions a fixed denominator, including incomplete evaluation.
type QueryEligibilityCounts struct {
	Required    uint64 `json:"required"`
	Verified    uint64 `json:"verified"`
	Failed      uint64 `json:"failed"`
	Blocked     uint64 `json:"blocked"`
	Unprocessed uint64 `json:"unprocessed"`
}

// QueryReferenceEligibilityResult is conditional stored consistency, not execution authenticity.
// Complete means evaluation finished, independently of the assessment.
type QueryReferenceEligibilityResult struct {
	Report
	ResultSchema       string                 `json:"result_schema"`
	ApprovalSHA256     string                 `json:"approval_sha256"`
	RegistrationSHA256 string                 `json:"registration_sha256"`
	WitnessSHA256      string                 `json:"witness_sha256"`
	CatalogueSHA256    string                 `json:"catalogue_sha256"`
	GraphSHA256        string                 `json:"graph_sha256"`
	Evidence           *EvidenceResult        `json:"evidence"`
	Build              QueryEligibilityCounts `json:"build"`
	Unit               QueryEligibilityCounts `json:"unit"`
	Control            QueryEligibilityCounts `json:"control"`
	Assertion          QueryEligibilityCounts `json:"assertion"`
	Tuple              QueryEligibilityCounts `json:"tuple"`
	BytesRead          uint64                 `json:"bytes_read"`
	Work               uint64                 `json:"work"`
	LogicalBytes       uint64                 `json:"logical_bytes"`
	Complete           bool                   `json:"complete"`
}

type eligibilityCandidate struct {
	SourceRevision string `json:"source_revision"`
	Platform       string `json:"platform"`
	Run            string `json:"run"`
	Attempt        uint64 `json:"attempt"`
}

type eligibilityCohort struct {
	ID             string `json:"id"`
	Purpose        string `json:"purpose"`
	SourceRevision string `json:"source_revision"`
	Platform       string `json:"platform"`
	Run            string `json:"run"`
	Attempt        uint64 `json:"attempt"`
	ProvenanceID   string `json:"provenance_id"`
}

type eligibilityProvenance struct {
	ID                      string   `json:"id"`
	Kind                    string   `json:"kind"`
	SourceRevision          string   `json:"source_revision"`
	RunnerSHA256            string   `json:"runner_sha256"`
	ControllerSourcesSHA256 string   `json:"controller_sources_sha256"`
	ReviewedContextSHA256   string   `json:"reviewed_context_sha256"`
	ReviewRecordSHA256      string   `json:"review_record_sha256"`
	EvidenceSHA256          string   `json:"evidence_sha256"`
	InvocationIDs           []string `json:"invocation_ids"`
	Repository              string   `json:"repository,omitempty"`
	WorkflowPath            string   `json:"workflow_path,omitempty"`
	WorkflowRevision        string   `json:"workflow_revision,omitempty"`
	RunID                   string   `json:"run_id,omitempty"`
	Attempt                 uint64   `json:"attempt,omitempty"`
	ArtifactID              string   `json:"artifact_id,omitempty"`
	ArtifactSHA256          string   `json:"artifact_sha256,omitempty"`
	SessionID               string   `json:"session_id,omitempty"`
}

type eligibilityApproval struct {
	Schema               string                  `json:"schema"`
	RegistrationSHA256   string                  `json:"registration_sha256"`
	WitnessSHA256        string                  `json:"witness_sha256"`
	EvidencePolicySHA256 string                  `json:"evidence_policy_sha256"`
	CatalogueSHA256      string                  `json:"catalogue_sha256"`
	Candidate            eligibilityCandidate    `json:"candidate"`
	Cohorts              []eligibilityCohort     `json:"cohorts"`
	Provenance           []eligibilityProvenance `json:"provenance"`
}

type eligibilityMember struct {
	NodeID string       `json:"node_id"`
	Member ReplayMember `json:"member"`
}

type eligibilityInput struct {
	Kind         string             `json:"kind"`
	MemberPin    *eligibilityMember `json:"member_pin,omitempty"`
	ContentBasis string             `json:"content_basis,omitempty"`
	ArgvIndex    *uint64            `json:"argv_index,omitempty"`
	PathLiteral  string             `json:"path_literal,omitempty"`
}

type eligibilityTool struct {
	ID             string            `json:"id"`
	CohortID       string            `json:"cohort_id"`
	ToolName       string            `json:"tool_name"`
	PathLiteral    string            `json:"path_literal"`
	SubjectSHA256  string            `json:"subject_sha256"`
	IdentityMember eligibilityMember `json:"identity_member"`
	ProvenanceID   string            `json:"provenance_id"`
}

type eligibilityExit struct {
	Basis        string `json:"basis"`
	Value        int    `json:"value"`
	ProvenanceID string `json:"provenance_id"`
}

type eligibilityInvocation struct {
	ID              string                      `json:"id"`
	Kind            string                      `json:"kind"`
	CohortID        string                      `json:"cohort_id"`
	CatalogueID     string                      `json:"catalogue_id"`
	Node            QueryReferenceNode          `json:"node"`
	Request         *eligibilityMember          `json:"request,omitempty"`
	Receipt         *eligibilityMember          `json:"receipt,omitempty"`
	Response        *eligibilityMember          `json:"response,omitempty"`
	Process         *eligibilityMember          `json:"process,omitempty"`
	ApprovedRequest jsontext.Value              `json:"approved_request"`
	Executable      *eligibilityMember          `json:"executable,omitempty"`
	ExecutablePath  string                      `json:"executable_path,omitempty"`
	ToolID          string                      `json:"tool_id,omitempty"`
	Inputs          map[string]eligibilityInput `json:"inputs"`
	ExitAttestation *eligibilityExit            `json:"exit_attestation,omitempty"`
}

type eligibilityBuild struct {
	CohortID        string             `json:"cohort_id"`
	Paths           map[string]string  `json:"paths"`
	Identity        eligibilityMember  `json:"identity"`
	SourceSnapshot  eligibilityMember  `json:"source_snapshot"`
	VendorSnapshot  eligibilityMember  `json:"vendor_snapshot"`
	ReferenceBinary *eligibilityMember `json:"reference_binary,omitempty"`
}

type eligibilityControl struct {
	CohortID string                       `json:"cohort_id"`
	Paths    map[string]string            `json:"paths"`
	Summary  eligibilityMember            `json:"summary"`
	Library  *eligibilityMember           `json:"library,omitempty"`
	Grammar  map[string]eligibilityMember `json:"grammar"`
}

type eligibilityNative struct {
	InvocationID     string                       `json:"invocation_id"`
	Source           eligibilityMember            `json:"source"`
	BuildManifest    eligibilityMember            `json:"build_manifest"`
	ReferenceLibrary *eligibilityMember           `json:"reference_library,omitempty"`
	Symbol           string                       `json:"symbol"`
	Grammar          map[string]eligibilityMember `json:"grammar"`
}

type eligibilityObservation struct {
	Query           string `json:"query"`
	Step            uint32 `json:"step"`
	TreeRole        string `json:"tree_role"`
	SourceBytes     uint64 `json:"source_bytes"`
	SourceSHA256    string `json:"source_sha256"`
	InvocationID    string `json:"invocation_id"`
	QueryID         string `json:"query_id"`
	ResponseOrdinal uint64 `json:"response_ordinal"`
	ByteOffset      uint64 `json:"byte_offset"`
	ByteLength      uint64 `json:"byte_length"`
	SHA256          string `json:"sha256"`
}

type eligibilityRegistration struct {
	Schema          string                   `json:"schema"`
	Context         string                   `json:"context"`
	CatalogueSHA256 string                   `json:"catalogue_sha256"`
	Graph           ReplayMember             `json:"graph"`
	OwnedMembers    []eligibilityMember      `json:"owned_members"`
	Tools           []eligibilityTool        `json:"tools"`
	Build           *eligibilityBuild        `json:"build,omitempty"`
	Control         *eligibilityControl      `json:"control,omitempty"`
	Native          *eligibilityNative       `json:"native,omitempty"`
	Invocations     []eligibilityInvocation  `json:"invocations"`
	Observations    []eligibilityObservation `json:"observations"`
}

// VerifyQueryReferenceEligibility evaluates the complete fixed closure and witness under
// one finite budget, using bytes from the same guarded evidence verification pass.
func VerifyQueryReferenceEligibility(ctx context.Context, req QueryReferenceEligibilityRequest) (QueryReferenceEligibilityResult, error) {
	return verifyQueryReferenceEligibility(ctx, req)
}

type eligibilityWitness struct {
	QueryWitnessRegistration `json:",inline"`
	Native                   eligibilityNativeExpected `json:"native"`
}

type eligibilityNativeExpected struct {
	Protocol              string                    `json:"protocol"`
	RequestID             string                    `json:"request_id"`
	NodeLimit             uint64                    `json:"node_limit"`
	API                   bool                      `json:"api"`
	Points                []uint32                  `json:"points"`
	RuntimeManifestSHA256 string                    `json:"runtime_manifest_sha256"`
	ProducerSHA256        string                    `json:"producer_sha256"`
	ExecutableSHA256      string                    `json:"executable_sha256"`
	ProducerABI           eligibilityProducerABI    `json:"producer_abi"`
	ObservationRunID      string                    `json:"observation_run_id"`
	HistoryStart          uint64                    `json:"history_start"`
	Process               eligibilityProcessRequest `json:"process"`
}

type eligibilityProducerABI struct {
	LanguageVersion        uint32 `json:"language_version"`
	RuntimeLanguageVersion uint32 `json:"runtime_language_version"`
	RuntimeMinCompatible   uint32 `json:"runtime_min_compatible"`
	SymbolCount            uint32 `json:"symbol_count"`
	FieldCount             uint32 `json:"field_count"`
}
