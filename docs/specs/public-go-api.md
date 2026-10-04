# 공개 offline Go API — 설계 r2 (S01 구현)

이 문서는 S01에서 구현한 첫 공개 계약이다. root module의 `github.com/wotjr1649/tree-sitter-grammar-kit/src/kit`이 entry이고, CLI `src/cmd/tsgk`는 같은 operation을 호출하며 API가 CLI를 실행하지 않는다. r1 초안 대비 r2는 kit 설정 wall, encoding 선언, identity 한정 대용량 예외, 비공개 corpus 진입점과 결과의 policy 공개를 더했다. 고급 native/replay API 전체 공개는 약속하지 않는다.

## S01 함수와 소유권

```go
package kit

func Inspect(ctx context.Context, request InspectRequest) (InventoryResult, error)
func Identity(ctx context.Context, request IdentityRequest) (IdentityResult, error)
func Corpus(ctx context.Context, request CorpusRequest) (CorpusResult, error)
func DefaultLimits() Limits             // offline-inspect 채택값
func DefaultCorpusLimits() CorpusLimits // private-corpus-local S01 채택값

type Limits struct {
    Files, FileBytes, TotalBytes, Depth, OutputBytes uint64
    Wall time.Duration // kit가 설정한 wall
}
type Selection struct {
    Grammar string          // exact "." root sentinel, otherwise a portable relative directory
    Files   []FileSelection // nil: documented bounded discovery; empty non-nil: invalid
}
type FileSelection struct{ Path, Role string }
type EncodingPolicy struct {
    Profile string         // "" 또는 "cp949"
    Files   []FileEncoding // 파일별 선언 "utf-8" | "cp949"
}
type FileEncoding struct{ Path, Encoding string }
type InspectRequest struct {
    Root      string
    Selection Selection
    Limits    Limits
}
type IdentityRequest struct {
    Root             string
    Selection        Selection
    Limits           Limits
    Encoding         EncodingPolicy
    LargeFileProfile string // "" 또는 등록된 identity 한정 예외(pg-large-source-r1, large-parser-source-r1)
}
type CorpusRequest struct {
    Root     string
    Limits   CorpusLimits
    Encoding EncodingPolicy
}
type CorpusLimits struct {
    Files, Records, FileBytes, TotalBytes, Depth, ReportBytes uint64
    Wall time.Duration
}
type InventoryResult struct {
    Report // E0 필드를 같은 JSON 객체에 펼친다
    Policy       Policy
    Grammar      string
    Grammars     []GrammarCandidate
    Entries      []InventoryEntry
    ClosureState string
}
type InventoryEntry struct {
    Path, Role, State, Basis string
    Size *uint64 // nil when not observed
}
type GrammarCandidate struct{ Path, Name, Source string }
type IdentityResult struct {
    Report
    Policy       Policy
    Grammar      string
    ClosureState string
    Manifest     Manifest
    SetSHA256    string
}
type Manifest struct {
    Schema, Algorithm, ModePolicy, EncodingPolicy string
    Files []FileIdentity
}
type FileIdentity struct {
    Path, Role, Mode, ModeProvenance, SHA256 string
    Size     uint64
    Encoding EncodingOutcome
}
type EncodingOutcome struct{ Assessment, Encoding, Source, Code string }
type CorpusResult struct {
    Report
    Policy   Policy
    Summary  CorpusSummary   // 개수와 판정만; 공개 기록에 쓰는 투영
    Records  []CorpusRecord  // 경로를 담으므로 로컬 비추적 보관
    Projects []ProjectRecord // .csproj 선언 포함 관계 관측
}
type Report struct {
    Schema, Command, ExecutionStatus, EvidenceMode, Assessment string
    Identities []IdentityRef
    Findings   []Finding
    Coverage   Coverage
}
type Policy struct { /* operation, discovery, 한도, 대용량 예외, encoding policy, 제외 규칙 */ }
type IdentityRef struct{ Role, Schema, SHA256 string }
type Finding struct{ Code, Severity, Path, Message string }
type Coverage struct{ Requested, Observed, Unsupported []string }
type Error struct { Kind, Code, Path string; Cause error }
func (e *Error) Error() string
func (e *Error) Unwrap() error
```

필드의 문자열 값과 JSON 이름은 [CLI/profile](cli-and-profile.md)과 [E0/identity](identity-and-evidence.md)의 닫힌 값 집합을 사용한다. `Error.Kind`는 `INVALID_INPUT`, `IO`, `CANCELLED`, `RESOURCE_LIMIT`, `UNSUPPORTED`이고 `Code`는 안정적인 기계 값이다(예: `GRAMMAR_INVALID`, `SELECTION_INVALID`, `LINK_OR_SPECIAL_REJECTED`, `HARDLINK_REJECTED`, `ROOT_NOT_LOCAL`, `NO_GRAMMAR_SELECTED`, `EMPTY_SELECTION`, `FILE_COUNT_LIMIT`, `FILE_BYTES_LIMIT`, `TOTAL_BYTES_LIMIT`, `DEPTH_LIMIT`, `RECORD_LIMIT`, `OUTPUT_LIMIT`, `WALL_LIMIT`, `SOURCE_CHANGED`, `READ_FAILED`, `CANCELLED`). `errors.As`로 `*kit.Error`를 구분하고 취소는 `errors.Is(err, context.Canceled/DeadlineExceeded)`도 보존한다. 오류 문자열은 machine identity가 아니다. 성공한 관측(unknown layout, 미해결 closure, encoding `BLOCKED`/`UNRESOLVED`)은 실행 오류가 아니라 결과 안의 finding과 필드다. S02의 verify 함수와 추가 result는 아래 `S02 함수와 추가 필드` 절이 고정한다. S03의 schema 함수는 아래 `S03 함수와 추가 필드` 절이 고정한다.

Root는 절대화한 뒤 한 번 `EvalSymlinks`로 해석하고 root 자체는 따라가서 디렉터리인지 확인한다(macOS `/var` → `/private/var`, root를 가리키는 symlink·Windows junction 같은 alias를 의도적으로 허용). 존재하지 않거나 디렉터리가 아닌 root는 `INVALID_INPUT`이고, Windows UNC share와 `\\?\`·`\\.\` device namespace root는 network·device 접근을 피하려고 `ROOT_NOT_LOCAL`로 거부하며, 입력 문자열과 해석된 경로를 모두 검사한다. root 아래 항목은 따라가지 않는다. caller가 명시한 Root와 Selection만 읽는다. 부모 저장소 탐색·Git·Node·shell·compiler·target JS·network·plugin·stdout/stderr·os.Exit·chdir·process 환경 변경·파일 생성은 API 효과에 포함되지 않는다. 제품 dependency closure(`src/kit`, `src/cmd/tsgk`)에는 `os/exec`, `net`, `plugin`이 없다. archive는 S02의 `Verify`가 `zip-r1`로만 읽는다. `parser.c` 부재는 inspect/identity 자체의 실패 조건이 아니다.

`Selection.Grammar`의 전체 값 `"."`은 이미 확인한 Root 자체를 선택하는 sentinel이다. 빈 값은 오류다. source 등록부의 `grammar_subdirectory`에도 같은 규칙을 적용한다. sentinel은 파일/member 경로나 내부 segment 허용 규칙이 아니다. `./x`, `x/.`, `x/../y`, 절대·drive·UNC·역슬래시 경로는 거부하고 다른 값은 [portable path 계약](trust-and-execution.md)을 따른다. `Selection.Files`의 role은 grammar/generated/scanner/query/corpus/metadata 중 하나이고 중복 path는 오류다.

모든 Limits 필드는 양수여야 하며 0은 invalid input이다. `nil` context와 deadline 없는 context는 거부한다. `Wall`은 kit가 설정한 한도이며 그 초과는 `RESOURCE_LIMIT`(`WALL_LIMIT`)다. caller context의 취소나 deadline 도달만 `CANCELLED`다. 둘이 함께 관측되면 caller 취소를 먼저 보고한다. 각 파일 열기 전, 1 MiB bounded read 사이, 디렉터리 listing batch 사이에 취소를 확인한다. source read 자체가 host I/O에서 멈추는 경우 hard 실시간 취소를 보장하지 않는다. 무한·무제한 fallback은 없다. CLI의 기본 유한 값은 CLI owner가 소유하며 직접 API 호출도 동일 guard를 거친다.

caller는 호출 중 request의 slice와 source snapshot을 변경하지 않는다. 열린 파일의 크기·수정 시각·파일 identity가 열기 전 관측과 다르거나 읽은 bytes가 크기와 다르면 `IO/SOURCE_CHANGED`다. 이것은 관측 가능한 변경의 검출이며 적대적 동시 교체를 막는 sandbox가 아니다. API는 caller 입력을 변경하거나 반환 후 보관하지 않으며 결과는 호출별로 새로 할당해 caller가 소유한다. iterator나 close할 native 자원은 반환하지 않는다. 서로 독립적인 불변 root에 대한 concurrent 호출을 지원하며 S01 시험이 8개 root의 동시 호출 결과를 순차 결과와 대조한다. shared root를 외부 process가 바꾸는 상황을 안전한 snapshot으로 보장하지 않는다.

실패 시 오류와 함께 반환된 report는 `COMPLETED`/`PASS`가 될 수 없다. 실패 결과는 E0 축·policy·실패 finding만 담고 entries/manifest/records는 비운다. `IdentityResult.SetSHA256`은 완전한 선택 집합을 읽지 못하면 비어 있다. 결과의 JSON encoding(+CLI 줄바꿈 1 byte)이 `OutputBytes`(corpus는 `ReportBytes`)를 넘으면 `RESOURCE_LIMIT`이다. CLI의 파일 publication은 API 반환 후 별도 no-clobber 단계이며 publication 실패도 CLI 실패다.

## S02 함수와 추가 필드 — 설계 r3

S02는 r2에 아래를 더한다. 기존 S01 함수의 의미와 결과는 profile을 주지 않으면 바뀌지 않는다.

```go
func Verify(ctx context.Context, request VerifyRequest) (VerifyResult, error)
func DefaultArchiveLimits() ArchiveLimits // entries 10000, archive bytes 268435456, nesting depth 2

const (
    ProfileSchema    = "tsgk-profile/r1"
    ExpectedSchema   = "tsgk-expected/r1"
    ArchiveProfile   = "zip-r1"
    MaxDocumentBytes = 16777216 // profile·expected 문서 상한, subject 한도와 별개
    AssessPass, AssessFail = "PASS", "FAIL"
    SubjectDirectory, SubjectArchive = "DIRECTORY", "ARCHIVE"
    ScopeKnownPaths, ScopeListed, ScopeArchiveMembers = "known-paths-r1", "listed-r1", "archive-members-r1"
)

type IdentityRequest struct { /* r2 필드 */ Profile []byte } // nil 또는 strict tsgk-profile/r1 원문
type CorpusRequest struct { /* r2 필드 */ Profile []byte }   // encoding·더 낮은 한도만
type ArchiveLimits struct{ Entries, Bytes, Depth uint64 }      // 모두 양수
type VerifyRequest struct {
    Root, Archive    string   // 정확히 하나
    ArchiveRoot      string   // "" 또는 portable member 디렉터리
    Nested           []string // 명시 선택한 중첩 ZIP member
    Selection        Selection // 디렉터리: Grammar(+Files); archive: Grammar는 "" 이어야 한다
    Expected         []byte   // strict tsgk-expected/r1 원문, 필수
    Profile          []byte
    Limits           Limits
    ArchiveLimits    ArchiveLimits // archive subject에서만 사용
    Encoding         EncodingPolicy
    LargeFileProfile string
}
type VerifyResult struct {
    Report
    Policy          Policy // archive subject면 ArchiveProfile·ArchiveEntries·ArchiveBytes·ArchiveDepth 포함
    Subject, Scope, Grammar, ArchiveRoot string
    Nested          []string
    Expected        ExpectedRef // Provenance, SetSHA256, FileCount, DocumentSHA256
    Actual          Manifest
    ActualSetSHA256 string
    Differences     []Difference // Code, Path, Expected *FileIdentity, Actual *FileIdentity
    Excluded        uint64
}
```

profile과 expected는 원문 bytes로 받아 CLI와 같은 strict decoder를 거친다. 직접 API 호출도 같은 path·JSON·크기 규칙과 같은 `Error.Code`를 받는다(외부 consumer 시험이 디렉터리와 archive verify의 CLI·API JSON bytes와 오류 code를 대조한다). 문서는 파일 경로가 아니라 bytes이므로 trust 문서가 subject 안에 있는지는 caller 책임이다(CLI는 디렉터리 subject에서 이를 거부한다). verify의 완료된 비교는 `error == nil`이며 차이는 `Assessment == FAIL`과 `Differences`로 나타난다. 잘못된 입력·지원하지 않는 기능·한도·취소·I/O는 S01과 같은 `*Error` 종류로 구분한다. S02 code 예: `SUBJECT_INVALID`, `EXPECTED_REQUIRED`, `DOCUMENT_BYTES_LIMIT`, `JSON_*`, `SCHEMA_UNSUPPORTED`, `PROFILE_LIMIT_ABOVE_OPERATION`, `PROFILE_LIMIT_NOT_APPLICABLE`, `SELECTION_SOURCE_CONFLICT`, `ENCODING_SOURCE_CONFLICT`, `ENCODING_POLICY_MISMATCH`, `EXPECTED_SET_MISMATCH`, `ARCHIVE_PATH_*`, `ZIP_*`, `ARCHIVE_ENTRY_LIMIT`, `ARCHIVE_BYTES_LIMIT`, `ARCHIVE_DEPTH_LIMIT`, `NESTED_MEMBER_NOT_FOUND`. 값 집합과 판정은 [CLI/profile](cli-and-profile.md)과 [trust](trust-and-execution.md)가 소유한다.

Verify의 효과는 Root(또는 Archive 파일 하나)와 caller가 준 bytes를 읽는 것뿐이다. archive를 풀거나 파일을 만들지 않는다. 취소 확인은 S01 지점에 더해 central directory entry마다, archive 원본 hash와 member 해제의 1 MiB 읽기 사이에 한다. 결과는 호출별로 새로 할당되며 request의 slice를 보관하지 않는다.

## S03 함수와 추가 필드 — 설계 r4

S03은 r3에 아래를 더한다. 기존 함수와 결과는 바뀌지 않는다.

```go
func SchemaCheck(ctx context.Context, request SchemaCheckRequest) (SchemaCheckResult, error)
func SchemaDiff(ctx context.Context, request SchemaDiffRequest) (SchemaDiffResult, error)
func DefaultSchemaLimits() SchemaLimits // document 16777216, records 200000, output 16777216, wall 120s

const (
    SchemaFormat     = "node-types-r1"
    SchemaComparator = "node-types-diff-r1"
    SchemaPolicyName = "tsgk-schema-policy/r1"
    RiskAddition, RiskRemoval, RiskIdentity, RiskClassification = "ADDITION", "REMOVAL", "IDENTITY", "CLASSIFICATION"
    RiskNarrowed, RiskWidened = "CARDINALITY_NARROWED", "CARDINALITY_WIDENED"
)

type SchemaLimits struct {
    DocumentBytes, Records, OutputBytes uint64 // 모두 양수
    Wall time.Duration
}
type SchemaInput struct {
    Name string // caller 표시 이름; kit는 열지 않는다
    Data []byte // node-types.json 원문 bytes
}
type SchemaCheckRequest struct { Input SchemaInput; Limits SchemaLimits }
type SchemaDiffRequest struct { Baseline, Candidate SchemaInput; Limits SchemaLimits }
type SchemaCheckResult struct {
    Report
    Policy SchemaPolicy
    Input  SchemaSummary // JSON "input"; E0 "schema" stays the report revision
}
type SchemaDiffResult struct {
    Report
    Policy              SchemaPolicy
    Baseline, Candidate SchemaSummary
    Differences         []SchemaDifference
}
type SchemaPolicy struct {
    Operation, Format, Comparator string // "schema-check"|"schema-diff"; comparator는 diff만
    DocumentBytes, Records, OutputBytes uint64
    WallMillis int64
}
type SchemaSummary struct {
    Role, Name, SHA256 string // role: "input"|"baseline"|"candidate"
    Bytes  uint64
    Counts *SchemaCounts // 유효한 schema일 때만, 그 밖은 nil
}
type SchemaCounts struct {
    Nodes, Named, Anonymous, Supertypes, Fields, References uint64
    Roots []NodeRef
}
type NodeRef struct{ Type string; Named bool }
type SchemaDifference struct {
    Code, Risk    string
    Node          NodeRef
    Field         string   // field 범위 차이만
    Member        *NodeRef // 허용 type·subtype 원소 차이만
    Before, After json.RawMessage // canonical 값, 없는 쪽은 null
    BaselinePath, CandidatePath string // 원본 JSON pointer, 없는 쪽은 ""
}
```

판정·code·순서·한도는 [정적 node-types 계약](tree-and-adapter-protocol.md)이 소유한다. 실행되는 공개 예시는 `src/kit/example_test.go`의 `ExampleSchemaDiff`다. 입력은 파일 경로가 아니라 caller가 가진 bytes이며 API는 파일·process·network를 쓰지 않고 `parser.c`가 필요 없다. 호출 중 caller는 `Data`를 바꾸지 않고, API는 반환 후 이를 보관하지 않는다. 결과는 호출별로 새로 할당된다. 서로 다른 입력에 대한 동시 호출은 공유 상태가 없다. 형식 위반은 `SchemaCheck`의 오류가 아니라 `error == nil`인 결과의 `FAIL`/`BLOCKED`다. `SchemaDiff`는 잘못된 입력을 `*Error`(`INVALID_INPUT`/`SCHEMA_INVALID`, `UNSUPPORTED`/`SCHEMA_KEY_UNSUPPORTED`)로 돌려주고 그 입력의 finding을 결과에 남긴다. 한도·취소·deadline 없는 context·잘못된 limits(`LIMITS_INVALID`)는 S01과 같은 `*Error`다. 오류와 함께 반환된 결과는 `COMPLETED`/`PASS`가 아니고 `Counts`는 nil, `Differences`는 비어 있다.

## S04 함수 — reproduce profile 해석

```go
func ParseReproduceProfile(data []byte) (ReproduceProfile, error)
```

`tsgk-reproduce/r1` 문서를 S02와 같은 strict decoder로 해석해 `ReproduceProfile`(`SHA256`, `ID`, `Route`, `Mode`, `Generator`, `JSRuntime`, `ABI`, `Optimize`, `Grammar`, `Inputs`, `Outputs`, `Limits`)을 돌려준다. 오류는 `*Error`(`INVALID_INPUT`, code는 [CLI 계약](cli-and-profile.md)의 `S04 구현` 절)다. 상수 `ModeJS`, `ModeJSON`, `ReferencePresent`, `ReferenceAbsent`와 generator 연산 상한 `Gen*`를 공개한다. 이 함수는 파일·process·network를 쓰지 않는다. 생성기 실행은 공개 API가 아니며 CLI `reproduce`가 내부 runner로 한다. 공개 package의 dependency closure에는 runner, `os/exec`, `golang.org/x/sys`, network package가 없다(`TestOfflineClosure`).

## S05 함수 — tree 비교, edit 검사, cp949, incremental profile

```go
func CompareTrees(a, b []TreeNode) *TreeDifference
func TreeDigest(nodes []TreeNode) string
func ValidateTree(nodes []TreeNode, inputBytes uint64) error
func ApplyEdits(enc string, source []byte, edits []Edit, maxBytes uint64) ([][]byte, []EditPoints, error)
func PointAt(enc string, s []byte, off int) Point
func SourceEncodingValid(enc string, s []byte) bool
func CP949Pair(lead, trail byte) bool
func CP949Rune(lead, trail byte) (rune, bool)
func ValidLanguageSymbol(s string) bool
func ObserveServiceHost(enc string, src []byte) SvcObservation
func NativeOperations() map[string]NativeOperation
func ParseIncrementalProfile(data []byte) (IncrementalProfile, error)
```

모두 offline이며 파일·process·network를 쓰지 않는다. `CompareTrees`는 S05의 하나뿐인 의미 tree 비교 함수로, 공개 `TreeNode`(`tsgk-tree/r1` node) 12개 필드를 preorder 순서대로 정렬·중복 제거 없이 비교하고 첫 차이(`node`, `field`, `left`, `right`; 한쪽이 접두면 `node_count`)를 낸다. `TreeDigest`는 `tsgk-tree-digest/r1`, `ValidateTree`는 완료 tree 구조 규칙(root 하나, 연속 preorder ancestry, 입력 안 범위)을 검사하며 같은 범위의 형제·부모-자식 중복과 zero-width node를 허용한다. `ApplyEdits`는 [tree/protocol](tree-and-adapter-protocol.md)의 edit·encoding 경계 규칙을 적용해 중간 source와 native edit point를 돌려주고, 위반은 `*Error`(`INVALID_INPUT`, code는 같은 절)로 첫 parse 전에 거부한다. cp949 함수는 내장한 WHATWG `index-euc-kr`(identifier·sha256은 `EUCKRIdentifier`·`EUCKRIndexSHA256`, 고지 `src/kit/data/NOTICE-index-euc-kr.md`)를 쓰며, S01 encoding 판정도 같은 표로 AMBIGUOUS(`AMBIGUOUS_ENCODING`)와 표 밖 쌍(`CP949_UNMAPPED`, 선언이면 `DECLARED_ENCODING_INVALID`)을 완성한다. `ParseIncrementalProfile`은 [CLI 계약](cli-and-profile.md) `S05 구현`의 profile `tsgk-incremental/r2`(r1도 읽음)를 해석하고(`IncrementalProfile.Schema`는 선언된 revision, `StepExpectation.Anchors`는 `ExpectAnchor{Type, StartByte, EndByte}` 목록, `MaxExpectAnchors`는 64, `IncrementalSchema`는 `tsgk-incremental/r2`), `NativeOperations`는 그 연산 상한을 돌려준다. driver build·실행은 공개 API가 아니며 CLI `incremental`이 내부 `src/internal/native`와 runner로 한다(`TestOfflineClosure`가 공개 closure에 runner·`os/exec`·network가 없음을 확인).

## S06 함수 — capture 비교, oracle profile, 기록 set 검증, 사실 query 세트

```go
func CompareCaptures(a, b []Capture) *CaptureDifference
func ParseOracleProfile(data []byte) (OracleProfile, error)
func VerifyOracleSet(fsys fs.FS) OracleSetReport
func ParseFactPack(data []byte) (FactPack, error)
func DeclarationQuery(items []NativeDeclaration) string
func DeriveDeclarations(items []NativeDeclaration, caps []Capture) []DeclarationItem
func DeriveDynamicSQL(route, enc string, caps []Capture, src []byte) DynamicSQLFacts
func CaptureText(src []byte, c Capture) (string, error)
```

모두 offline이며 process·network를 쓰지 않는다. `VerifyOracleSet`만 호출자가 준 `fs.FS`를 읽는다. `Capture`는 runtime이 돌려준 순서의 capture 하나다(`match`, `pattern`, `capture`, `name`, preorder `node`, `type`, node flag 다섯 개, byte·point 범위). `CompareCaptures`는 하나뿐인 capture stream 비교 함수(`tsgk-capture-compare/r1`)다. stream 순서대로 모든 필드를 정렬·중복 제거 없이 비교하고 첫 차이(`index`, `field`, `left`, `right`; 한쪽이 접두면 `capture_count`)를 낸다. `ParseOracleProfile`은 [CLI 계약](cli-and-profile.md) `S06 구현`의 `tsgk-oracle/r2`(r1도 읽음)를 S05와 같은 strict decoder로 해석한다. `VerifyOracleSet`은 [tree/protocol](tree-and-adapter-protocol.md) `S06 구현`의 기록 set 완결성(마지막 `complete` member, member bytes·sha256, 목록 밖 파일, record 수, record의 입력·step 0 결속)을 검사하고 finding을 모아 `valid`와 manifest의 실행 상태·판정을 낸다(`valid`는 완결·무손상만 뜻한다). 고치거나 추정하지 않는다. `ParseFactPack`은 `tsgk-fact-query-pack/r1`을 해석하고 파일 bytes의 sha256을 identity로 돌려준다. `DeclarationQuery`는 선언 query text를 만들며 pack의 text가 그 결과와 같다(`TestFactQueryPack`). `DeriveDeclarations`와 `DeriveDynamicSQL`은 pack query의 capture(동적 SQL은 원본 bytes와 판별 encoding도)에서 같은 절의 규칙으로 사실을 도출한다. 소비자가 같은 pack과 기록으로 사실을 다시 만들 수 있게 공개한다. `CaptureText`는 capture의 원본 text를 돌려주고 UTF-8이 아니면 오류다. `NativeOperations`에는 `native-query`, `native-query-large`(windows/amd64), `real-world-source-r3`(windows/amd64, 8 GiB)가 더해졌고 `NativeOperation`에는 `platforms`·`matches`·`captures`·`query_ms`가 더해졌다. driver build·실행·기록 set 발행은 공개 API가 아니며 CLI `oracle record`가 내부 `src/internal/native`로 한다. 공개 closure에 runner·`os/exec`·network가 없음은 `TestOfflineClosure`가 계속 확인한다.

## S07 함수 — replay, reducer 등록부, 수치 비교, evidence graph

```go
func Replay(ctx context.Context, req ReplayRequest) (ReplayResult, error)
func ParseReplayProfile(data []byte) (ReplayProfile, error)
func ReplayOperations() map[string]ReplayLimits
func Reducers() []ReducerInfo
func CompareGates(recorded, recomputed []byte) (GateComparison, error)
func VerifyEvidence(ctx context.Context, req EvidenceRequest) (EvidenceResult, error)
```

모두 offline이며 process·network를 쓰지 않는다. `Replay`와 `VerifyEvidence`는 호출자가 준 root만 S01 guard로 읽고, `ctx`의 deadline이 필요하다(없으면 `DEADLINE_REQUIRED`). 연산 wall에 닿으면 `RESOURCE_LIMIT`, 호출자 취소·deadline은 `CANCELLED`다. 실패는 `*Error`와 함께 실패 report를 돌려준다. 등록되지 않은 reducer·schema·연산은 `KindUnsupported`이고, 손상되었거나 불완전한 evidence는 오류가 아니라 `evidence_valid: false`와 `FAIL` 결과다. `ReplayResult`는 E0 `Report`에 `subject`, `recorded`, `recomputed`, `evidence_valid`, `observed_identities`, `gates`, `consumption`, `members`, `bytes_read`, 한국어 `explanation`을 더한다. `ReplayOperations`는 `evidence-replay`와 `private-corpus-replay`의 한도를, `Reducers`는 등록 reducer(다시 계산하는 gate와 기록으로 남는 것)를 돌려준다. `CompareGates`는 BrightScript `S07-REPLAY-2ULP-r1` 비교이며 그 workload에만 쓴다. 차이는 오류 text가 보관된 verifier의 code로 시작한다. `VerifyEvidence`는 `tsgk-evidence/r1` graph를 `tsgk-evidence-policy/r1`로 검사하고 E0에 `nodes`, `files`, `modes`, `explanation`을 더한 `EvidenceResult`를 돌려준다. 계약은 [identity/evidence](identity-and-evidence.md) `S07 구현`과 [CLI/profile](cli-and-profile.md) `S07 구현`이다. 같은 입력의 두 호출은 같은 결과이며 동시 호출은 서로 상태를 공유하지 않는다.

## S08 함수 — qualification

```go
func Qualify(ctx context.Context, req QualifyRequest) (QualificationResult, error)
func ParseQualificationInventory(data []byte) (QualificationInventory, error)
func QualificationLimits() ReplayLimits
```

offline이며 process·network를 쓰지 않는다. `Qualify`는 `QualifyRequest{Inventory, Candidate, Hosts}`의 host 디렉터리만 S01 guard로 읽고 `ctx` deadline이 필요하다. 연산 wall·한도는 `RESOURCE_LIMIT`, 호출자 취소는 `CANCELLED`, 잘못된 inventory·후보는 `KindInvalidInput`이며 실패 report를 함께 돌려준다. 손상된·섞인·자격 없는 근거는 오류가 아니라 칸의 FAIL과 finding이다. 다만 host 디렉터리가 없거나 그 안에 link·special 파일이 있으면 S01 guard가 호출 전체를 `KindInvalidInput`으로 끝낸다. host당 record 수가 한도를 넘으면 `RECORDS_LIMIT`(`RESOURCE_LIMIT`)다. `ParseQualificationInventory`는 `tsgk-qualification-inventory/r3`을 strict decode하고 칸 수·identity 중복·coverage 규칙(`tsgk-coverage-rule/r2`)·production 대안·error node·W sample 등록·기대값 anchor 범위를 검사한다. `QualificationLimits`는 `qualification` 연산 한도다. 계약은 [identity/evidence](identity-and-evidence.md) `S08 구현`과 [CLI/profile](cli-and-profile.md) `S08 구현`이다. CLI는 같은 함수를 부르며 같은 bytes를 낸다(`TestQualifyCLI`, `TestModuleProxyConsumer`).

## 외부 소비자 검증

`src/testdata/consumer/`에 source와 `go.mod.tmpl` 데이터를 두고, 실제 module은 checkout 밖 임시 디렉터리에 생성한다. 시험(`src/cmd/tsgk` 의 `TestExternalConsumerAndCLI`, schema는 `TestExternalConsumerSchema`)은 `GOWORK=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`, `GOPROXY=off`에서 공개 import만 사용해 build한다. internal/native/consumer 타입을 import하지 않는다. 같은 fixture에서 CLI 결과와 API 결과의 JSON bytes가 같고, 경로 탈출 selection에서 API 오류 code와 CLI 오류 code·exit가 같은지 확인한다. 두 실행 파일은 `PATH`를 비운 환경에서 실행한다.

S01의 local replace는 초기 소비 경계만 검증한다. S08 `TestModuleProxyConsumer`는 checkout의 추적 파일로 module zip(`github.com/wotjr1649/tree-sitter-grammar-kit@v0.0.0-20261004000000-000000000000`)을 만들고 파일 module proxy(`GOPROXY=file://…`, 고정 `golang.org/x/sys`는 module cache의 download 파일)로만 별도 module이 그 version을 require해 build한다. replace·checkout 경로·network·tag가 없다. module zip에 실행 파일·native library·object 파일이 있으면 실패한다. 그 module의 `kit.Qualify` 결과가 `PATH`를 비운 환경에서 CLI `qualify`와 같은 bytes인지 확인한다. publication은 수행하지 않는다. CLI만으로 하는 grammar 갱신 흐름(기준 identity → 후보 verify FAIL과 변경 파일 → node schema diff의 검토 항목, 입력 불변·자동 채택 없음)은 `TestGrammarUpdateWorkflow`가 확인한다.
