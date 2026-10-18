# 공개 offline Go API — 설계 r2 (S01 구현)

이 문서는 S01에서 구현한 첫 공개 계약이다. root module의 `github.com/wotjr1649/tree-sitter-grammar-kit/src/kit`이 entry이고, CLI `src/cmd/tsgk`는 같은 operation을 호출하며 API가 CLI를 실행하지 않는다. r1 초안 대비 r2는 kit 설정 wall, encoding 선언, identity 한정 대용량 예외, 비공개 corpus 진입점과 결과의 policy 공개를 더했다. 고급 native/replay API 전체 공개는 약속하지 않는다.

## #197 logger 관측의 offline validator

`src/kit/reuseobs`는 native host와 offline replay가 공유하는 실험적 pure Go validator다. `Observation`, `Counts`, `Binding`, `Decode`와 `Judge` 및 고정 schema/policy/runtime·수집 상한, 기록된 관측을 읽을 runtime manifest를 판정하는 `VerifiedRuntimeManifest`를 제공한다. `Decode`는 호출자가 독립적으로 검증한 입력·build·run·history·role의 `Binding`과 versioned 관측을 대조하며, `Judge`는 유효한 수집과 actual old-tree 전달의 별도 축만 판정한다. event 수로 공개 node 재사용·구문 적합성·성능을 판정하지 않는다. 자체 구조 검증은 실행의 인증을 대신하지 않으며 연결되지 않은 관측을 `Judge`에 제공하는 것만으로 workload 결속을 주장할 수 없다. package는 파일·process·network를 사용하지 않는다. native 실행·logger 설치는 계속 CLI의 별도 process 경계 안에 있고 `TestOfflineClosure`의 기존 금지는 유지한다. 구체적인 값·상태는 [native protocol](tree-and-adapter-protocol.md), 공개 replay 선택자는 [identity/evidence](identity-and-evidence.md)가 소유한다.

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

## T-SQL의 한정된 문맥 검사 (#141)

`CheckTSQLContext(nodes []TreeNode, source []byte, encoding string) ([]TSQLDiagnostic, error)`는 caller가 source·grammar identity에 묶은 완전한 T-SQL CST와 원래 bytes를 받는다. source나 파일 이름을 출력하지 않으며 process·filesystem·network 효과가 없다. `TSQLDiagnostic`은 `Code`, preorder `Node`, 원래 byte/point `Range Span`을 반환한다. 입력을 변경·보관하지 않고 결과는 caller가 소유한다.

검사 범위는 SELECT CTE 뒤 table source 부재(`TSQL_CTE_SELECT_WITHOUT_TABLE_SOURCE`), scalar predicate(`TSQL_SCALAR_PREDICATE`), table hint 이름(`TSQL_TABLE_HINT_NAME`)과 locking/access hint 충돌(`TSQL_TABLE_HINT_CONFLICT`), single-part COUNT/COUNT_BIG의 인자 수(`TSQL_COUNT_ARITY`), scalar function의 값 없는 RETURN(`TSQL_SCALAR_FUNCTION_RETURN_VALUE`)이다. FORCESEEK의 index0은 `TSQL_TABLE_HINT_ARGUMENT`로 진단하고 parameterized FORCESEEK와 INDEX의 병용도 충돌에 포함한다. unqualified REGEXP_LIKE predicate, simple CASE의 WHEN 값, qualified UDF, TVF·procedure·batch의 bare RETURN, 중복 및 동의어 hint는 구분한다. CTE 이름의 binding·collation을 추정하지 않고 SELECT가 table source를 전혀 갖지 않는 관측만 진단한다. hint 이름은 SQL Server 2012~2025의 문서화된 구문 합집합을 대상으로 한다.

DML 쓰기 대상의 NOLOCK/READUNCOMMITTED는 `TSQL_DML_TARGET_HINT`로 진단하며 읽기용 FROM relation과 query hint에는 적용하지 않는다. GRANT/DENY/REVOKE의 column 목록은 SELECT·UPDATE·REFERENCES·UNMASK에만 허용하고 나머지는 `TSQL_COLUMN_PERMISSION`로 진단한다. per-permission 목록과 securable 뒤 전체 목록을 구분한다. 직접 ORDER BY가 없고 base_window도 없는 ROWS/RANGE는 `TSQL_WINDOW_FRAME_ORDER`, RANGE numeric offset 및 단독 FOLLOWING·역방향 UNBOUNDED 경계는 `TSQL_WINDOW_FRAME_BOUND`다. named window의 binding·상속과 numeric frame 크기·함수별 frame 가능 여부는 추정하지 않는다. CREATE/ALTER의 DDL trigger 이름에 qualifier를 붙이면 `TSQL_DDL_TRIGGER_NAME`, DDL의 OWNER는 `TSQL_DDL_TRIGGER_OWNER`, 고정 DML/DDL event 및 DATABASE의 DDL_SERVER_LEVEL_EVENTS·CREATE_LOGIN은 `TSQL_TRIGGER_EVENT_SCOPE`로 진단한다. 전체 DDL event catalog·permission/login/user 존재는 검사하지 않는다.

EXEC의 positional/named/implicit module 인자와 pass-through 명령의 OUT/OUTPUT 값이 local variable이 아니면 `TSQL_EXEC_OUTPUT_VARIABLE`로 진단한다. 상수·DEFAULT·bare/qualified 이름·@@global 값은 반환 변수가 될 수 없다. OUTPUT 없는 값, module formal OUTPUT과 동적 SQL string은 구분한다. 변수 존재·scope·type 및 procedure의 OUTPUT 선언·catalog는 검사하지 않는다. 진단 범위는 OUTPUT 직전 값의 원래 범위다.

`ValidateTree`와 native source encoding 검증을 먼저 수행하며, 구조·범위·encoding 오류는 기존 `*kit.Error`를 반환한다. UTF-16 node의 홀수 byte 경계는 `TREE_ENCODING_BOUNDARY`, ERROR·missing·has-error CST는 `TSQL_TREE_HAS_ERROR`로 거부한다. source decoding은 UTF-8·UTF-16LE/BE·고정 CP949 table을 재사용한다. 이름 token 복사는 512 bytes 안으로 한정한다. context 결과는 ERROR tree에 대한 성공 판정을 대신하지 않는다. WITH·bare hint와 OPTION(TABLE HINT ...)의 같은 table_hint_item을 검사하며 일반 query hint를 table hint로 취급하지 않는다. 숫자 hint 인자는0..2147483647이고 FORCESEEK의 index0은 진단한다.

빈 diagnostics는 이 검사 범위에서 위반을 관측하지 않았다는 뜻이다. 전체 SQL Server의 compile·execution 성공이나 모든 T-SQL의 무결함을 뜻하지 않는다. catalog/name binding, type resolution, collation/configuration, version compatibility, 계획·권한·runtime 검증은 별도의 엔진 근거가 필요하다. 기존 oracle/API claim 및 parser ERROR의 의미는 보존한다.

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
func ObserveServiceHostWithContext(enc string, src []byte, context *SvcContext) SvcObservation
func (o SvcObservation) Syntax(inlineHasError *bool) (hasError, known bool)
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
func DeriveDynamicSQLChecked(route, enc string, caps []Capture, src []byte) (DynamicSQLFacts, error)
func CaptureText(src []byte, c Capture) (string, error)
```

모두 offline이며 process·network를 쓰지 않는다. `VerifyOracleSet`만 호출자가 준 `fs.FS`를 읽는다. `Capture`는 runtime이 돌려준 순서의 capture 하나다(`match`, `pattern`, `capture`, `name`, preorder `node`, `type`, node flag 다섯 개, byte·point 범위). `CompareCaptures`는 하나뿐인 capture stream 비교 함수(`tsgk-capture-compare/r1`)다. stream 순서대로 모든 필드를 정렬·중복 제거 없이 비교하고 첫 차이(`index`, `field`, `left`, `right`; 한쪽이 접두면 `capture_count`)를 낸다. `ParseOracleProfile`은 [CLI 계약](cli-and-profile.md) `S06 구현`의 `tsgk-oracle/r2`(r1도 읽음)를 S05와 같은 strict decoder로 해석한다. `VerifyOracleSet`은 [tree/protocol](tree-and-adapter-protocol.md) `S06 구현`의 기록 set 완결성(마지막 `complete` member, member bytes·sha256, 목록 밖 파일, record 수, record의 입력·step 0 결속)을 검사하고 finding을 모아 `valid`와 manifest의 실행 상태·판정을 낸다(`valid`는 완결·무손상만 뜻한다). 고치거나 추정하지 않는다. `VerifyOracleSet`은 명시 source witness manifest r2도 검사한다. 모든 사례의 source member(32 MiB 상한), 입력·record 교차 결속과 빈 파일/누락을 검증하며 기존 replay/qualification decoder는 r2를 거부한다. `OracleSourceManifestSchema`와 `MaxOracleSourceBytes`가 이 revision과 member 상한이다. Source 무결성은 query 의미 평가나 reference 자격의 증명이 아니다. `ParseFactPack`은 `tsgk-fact-query-pack/r1`을 해석하고 파일 bytes의 sha256을 identity로 돌려준다. `DeclarationQuery`는 선언 query text를 만들며 pack의 text가 그 결과와 같다(`TestFactQueryPack`). `DeriveDeclarations`와 `DeriveDynamicSQL`은 pack query의 capture(동적 SQL은 원본 bytes와 판별 encoding도)에서 같은 절의 규칙으로 사실을 도출한다. 소비자가 같은 pack과 기록으로 사실을 다시 만들 수 있게 공개한다. `CaptureText`는 capture의 원본 text를 돌려주고 UTF-8이 아니면 오류다. `NativeOperations`에는 `native-query`, `native-query-large`(windows/amd64), `real-world-source-r3`(windows/amd64, 8 GiB)가 더해졌고 `NativeOperation`에는 `platforms`·`matches`·`captures`·`query_ms`가 더해졌다. driver build·실행·기록 set 발행은 공개 API가 아니며 CLI `oracle record`가 내부 `src/internal/native`로 한다. 공개 closure에 runner·`os/exec`·network가 없음은 `TestOfflineClosure`가 계속 확인한다.

`DeriveDynamicSQLChecked`는 모든 capture의 `0 ≤ start_byte ≤ end_byte ≤ len(src)`를 먼저 검증하고 위반하면 `INVALID_INPUT/CAPTURE_RANGE_INVALID`와 무효 zero value를 반환한다. 정상 입력의 결과는 기존 함수와 같고 원문 범위를 유지한다. 기존 `DeriveDynamicSQL` 시그니처는 유지하며 같은 검증 실패에서 panic 대신 zero value(`Mapping == ""`, nil 목록)를 반환한다. 정상적인 빈 결과는 `Mapping == "dynamic-sql-r1"`이므로 구분된다. 오류 원인이 필요한 소비자는 checked 함수를 사용한다. native oracle은 checked 오류를 `BLOCKED`로 처리하며 사실이나 성공 claim을 만들지 않는다.

## 저장 query witness API (#216)

공용 data-only 지원 package는 `src/kit/querypolicy`(고정 predicate metadata·support·filter)와 `src/kit/querywire`(native query wire validation)다. 기존 native caller와 이 API는 각각 같은 구현을 사용한다. `FilterChecked`는 checkpoint 오류를 별도로 반환하며 중도 취소 결과를 정상 빈 capture로 바꾸지 않는다. 두 package 모두 실행 capability를 갖지 않으며 `kit`을 경유하지 않고 호출할 경우에도 source·문서 크기·full CST의 선행 validation 책임은 해당 함수 주석을 따른다.

```go
func VerifyQueryWitness(ctx context.Context, req QueryWitnessRequest) (QueryWitnessResult, error)
```

`QueryWitnessRequest`는 `Registration`, 초기 `Source`, native r2 원 응답 `Raw`, 저장된 case `Projection` bytes만 받는다. 파일·process·network를 사용하지 않으며 caller deadline이 필요하다. `tsgk-query-witness-registration/r1`은 caller가 evidence 밖에서 독립적으로 선택하는 신뢰 입력이다. registration 파일을 evidence 안에 저장하더라도 그 파일의 hash와 선택 근거는 evidence에서 유도해서는 안 된다. 자체 hash는 독립성이나 진위를 증명하지 않는다.

registration은 `schema`, 고정 `policy: tsgk-predicates/r1`, `case`, `route`, `platform`, `role`, `encoding`, 초기 `input`, `edits`, `queries`, `required`를 요구한다. 각 query는 정확한 `id`·`source`와 독립 compiled `metadata`(`patterns`, `capture_names`, `predicates`)를 결속한다. `QueryPredicate`·`QueryPredicateStep`은 공유 data-only 정책의 공개 metadata type이다. `required`의 각 tuple은 `query`, `step`, `tree_role`(`incremental` 또는 edit 이후 `fresh`), 전체 raw `structural` capture와 평가 후 `expected` capture를 등록한다. 탈락한 match도 structural anchor에 포함한다. 순서와 match/capture/node 번호 및 모든 capture 필드를 보존하고 정렬·중복 제거·사후 번호 보정을 하지 않는다.

anchor의 `null`은 근거 부재이고 `[]`는 완료된 빈 기대값이다. 공개 tuple의 `MarshalJSON`은 두 JSON API에서 이 구분을 유지한다. `metadata: null`이나 필수 tuple 0개도 PASS 근거가 아니다. 필수 tuple의 분모는 결과를 읽기 전에 독립 등록으로 정하며, 실제 결과의 개수로 축소하지 않는다. 이 API는 caller가 선택한 분모를 검사하며 전체 지원 범위의 분모를 자동 생성하지 않는다.

검증은 초기 source의 hash·길이와 실제 edit history를 재적용하고, 각 step의 source·edit·point 결속, full raw CST의 type·field table·preorder node·flag·범위·point·digest, capture의 실제 node 연결, 독립 compiled metadata, 전체 structural stream, 공유 predicate 정책의 평가 결과, 독립 expected stream 및 저장 projection을 대조한다. 전체 stream 동등 비교는 완료된 query에만 적용한다. 일관된 partial·unsupported·missing 근거는 BLOCKED이며, 확인 가능한 변조·모순은 다른 근거 부재보다 우선하는 FAIL이다. source/raw가 없어도 제공된 document의 shape·case·query identity 등 독립적으로 검사 가능한 결속은 검사한다.

제공된 full raw/projection CST는 `required`에 선택되지 않은 step·tree role도 검사한다. `required`는 capture 의무의 분모이며 저장된 CST의 무결성 검사를 생략하는 선택자가 아니다. Full tree의 해석·raw query node 연결은 tree마다 한 번 수행해 재사용한다. Source와 decode 가능한 raw가 있는 모든 query의 predicate 평가도 필수 tuple 선택과 무관하게 재계산해 projection과 대조하며, 필수 anchor 검사에는 같은 계산을 재사용한다. Summary·record raw도 capture byte 범위로 평가하지만 full CST 부재는 계속 BLOCKED이며 검증 완료 tuple로 세지 않는다. Raw가 없어도 완료 projection의 누락·비완료 tree와 native query의 미실행·자원 제한·실패 모순은 FAIL이다. Projection capture의 node index·type·flag·byte·point는 제공된 full CST와 연결한다. Full CST가 없어도 capture의 pattern/name/index/node 범위는 보관된 metadata와 대조하고, source 유무와 무관하게 등록 step 크기 안의 start <= end byte 범위를 요구한다. Source가 있으면 summary·record capture의 byte/point도 재계산한다. INVALID_QUERY는 등록 query의 byte offset·LF point와 여섯 오류 class를 확인한다. 독립 metadata 대조는 compiled metadata를 보존하는 완료·자원 제한·미지원·CAPTURE_NODE_UNMAPPED·host predicate 실패에 적용하며, INVALID_QUERY·NOT_RUN의 빈 metadata와 혼동하지 않는다. Unfiltered STRUCTURAL projection은 raw와 같은 match 개수·최초 등장 순서·같은 match의 pattern을 검사하며 정상 interleaving·중복을 보존한다. EVALUATED projection은 탈락한 match의 번호 공백을 허용하되 생존 match의 최초 등장 순서와 match별 단일 pattern을 공유 FilteredMatchAccounting으로 검사한다. Projection의 INVALID_QUERY·NOT_RUN은 timing·count 0과 present-empty metadata를 요구한다. Raw INVALID_QUERY도 timing·count 0과 비어 있는 capture name/predicate/type 목록을 요구하되 기존 wire의 legacy null 목록은 유지한다. 일반 wire decoder의 허용 범위는 변경하지 않는다. Capability 거부 예외 밖의 raw/projection step 수 차이는 FAIL이다. 초기 입력 한도와 각 edit의 범위·old/new payload 길이·중간 크기 한도는 source가 없어도 등록 형식 검사에서 확인한다. UTF-16 초기 bytes와 모든 edit offset은 짝수여야 하며 old/new payload도 해당 UTF-16 규칙을 만족해야 한다. Projection의 `encoding_source`는 legacy 생략만 허용하며, 있으면 `DECLARATION`이어야 한다. Source가 없어도 raw/projection의 편집 step hash·길이·edit point를 직접 대조하며, raw가 없어도 native 계약 밖의 실패 query code는 FAIL이다. Raw 없는 CANCELLED projection도 code CANCELLED를 요구한다. Predicate가 있는 RESOURCE_LIMIT projection은 지원 가능한 metadata에만 허용하며 미지원 predicate는 producer의 support 우선 순서에 따라 UNSUPPORTED여야 한다. Source가 있는 raw summary/record capture point도 projection 없이 재계산한다.

`steps_completed`는 보관된 step 수가 아니라 tree·query가 모두 완료된 step 수와 대조한다. 초기 fresh는 거부하고 완료된 edit에는 fresh를 요구한다. Source가 없어도 등록된 old/new payload의 LF·byte column 변화와 edit 시작 point를 결합해 끝 point를 검사하며, 빈 span은 시작·끝 point가 같아야 한다. 비완료 raw의 마지막 보관 step에는 부모 상태·code와 일치하는 실제 비완료 tree/query가 있어야 한다. 비완료 tree는 RESOURCE_LIMIT/PARSE_TIME_LIMIT·NODE_LIMIT·DEPTH_LIMIT 또는 FAILED/PARSE_NULL·DESCENDANT_COUNT_MISMATCH만 허용한다. 해당 raw/projection 요약은 descendant_count 0·has_error false·빈 digest이고, raw type/field/node table도 없어야 한다. Steps 없는 OUTPUT_LIMIT·ALLOCATION_LIMIT은 RESOURCE_LIMIT에, LANGUAGE_INCOMPATIBLE은 FAILED에만 대응한다. Steps 없는 QUERY_TIME_LIMIT은 query가 있는 r2 RESOURCE_LIMIT에만 허용한다. Raw/projection의 case 상태·code 모순은 FAIL이다. Raw가 없어도 steps가 있으면 마지막 보관 tree/query의 실제 미완료 사유와 부모 상태·code를 대조한다. 미지원 partial query가 가린 정확한 code는 네 native query limit 중 하나여야 하며, host predicate 실패만으로 부모를 native 실패로 만들 수 없다. 완료 raw에 대응하는 steps 없는 NOT_RUN projection의 QUERY_CAPABILITY_MISSING·API_CAPABILITY_MISSING은 관측 해석 전 capability 거부 경로로 BLOCKED를 유지한다. 이 API에는 producer capability 근거가 없으므로 거부 사유를 입증하거나 tuple 완료로 세지 않는다. 실제 protocol이 생략하는 실패 tree의 CST 필드·summary의 full 배열·비 full projection의 tree만 생략을 허용하며, 제공된 필드의 형식 검사는 유지한다.

결과는 E0에 `result_schema: tsgk-query-witness-result/r1`, 정확한 registration bytes의 `registration_sha256`, `required`, `verified`, `evaluated`를 더한다. `verified`는 모든 해당 tuple 검사를 통과한 수, `evaluated`는 그중 predicate를 실제 평가한 수다. 일부 tuple이 실패해도 다른 tuple의 완료 수는 보존한다. PASS는 등록된 저장 capture 검증만 뜻한다. producer 실행, 임의 입력의 parser 완전성, stock CLI parity, 계측 reference eligibility 또는 전체 `query_semantics` qualification을 증명하지 않는다. source/CST/reference를 kit의 결과로 채워 독립 기대값을 만드는 것도 허용하지 않는다.

신뢰 registration의 형식 오류는 `KindInvalidInput`, 저장 raw/projection의 형식·의미 불일치는 완료된 FAIL이다. caller 취소는 CANCELLED, 120초 wall 및 크기·값 개수 한도 초과는 RESOURCE_LIMIT이다. 각 JSON 문서는 16 MiB·depth32·2,000,000 values, 초기 source는 32 MiB, edit는4개, query는16개·각64 KiB, required tuple은1,000개, finding은최대1,000개다. 긴 shape/node/predicate scan에서 취소와 wall을 확인한다. CLI replay 경로에는 기존 16 MiB member 한도가 추가로 적용된다. core의 CGO·process·network 금지는 유지한다.

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

## 버전과 소비자 채택 경계

문서의 설계 revision, Go module의 version/commit, JSON schema·protocol revision, grammar/runtime upstream version은 서로 다른 identity다. `tsgk-manifest/r2` 같은 데이터 형식 이름으로 소비자의 grammar version이나 module 호환성을 추론하지 않는다. 소비자는 사용할 module commit/version과 읽는 schema를 고정하고, 갱신 후보를 별도 입력으로 비교한다. 문서나 검증 보고서의 추가가 새 release를 발행한 것은 아니다.

검증된 offline core와 experimental 연산의 상태는 [platform 계약](platform-support.md)이 소유한다. 이 문서에 함수가 공개되어 있거나 등록 qualification이 SUPPORTED여도 experimental 연산의 안정 호환성을 약속하지 않는다. 내부 native build/runner 타입은 공개 소비 계약이 아니며, import 경계를 우회한 도입을 외부 API 검증으로 세지 않는다.

실제 소비자 pilot의 통합 상태와 byte identity, 구문 적합성, C oracle agreement는 별도 축이다. [#193 평가](../reports/issue-193-evaluation.md)의 `go-treesitter` pilot은 로컬 CLI 도입이며 소비자 main/CI 채택 또는 runtime baseline 교체가 아니다. 일반 external consumer 시험의 PASS를 실제 제품 채택 수로 세지 않는다.

## 외부 소비자 검증

`src/testdata/consumer/`에 source와 `go.mod.tmpl` 데이터를 두고, 실제 module은 checkout 밖 임시 디렉터리에 생성한다. 시험(`src/cmd/tsgk` 의 `TestExternalConsumerAndCLI`, schema는 `TestExternalConsumerSchema`)은 `GOWORK=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`, `GOPROXY=off`에서 공개 import만 사용해 build한다. internal/native/consumer 타입을 import하지 않는다. 같은 fixture에서 CLI 결과와 API 결과의 JSON bytes가 같고, 경로 탈출 selection에서 API 오류 code와 CLI 오류 code·exit가 같은지 확인한다. 두 실행 파일은 `PATH`를 비운 환경에서 실행한다.

S01의 local replace는 초기 소비 경계만 검증한다. S08 `TestModuleProxyConsumer`는 checkout의 추적 파일로 module zip(`github.com/wotjr1649/tree-sitter-grammar-kit@v0.0.0-20261004000000-000000000000`)을 만들고 파일 module proxy(`GOPROXY=file://…`, 고정 `golang.org/x/sys`는 module cache의 download 파일)로만 별도 module이 그 version을 require해 build한다. replace·checkout 경로·network·tag가 없다. module zip에 실행 파일·native library·object 파일이 있으면 실패한다. 그 module의 `kit.Qualify` 결과가 `PATH`를 비운 환경에서 CLI `qualify`와 같은 bytes인지 확인한다. publication은 수행하지 않는다. CLI만으로 하는 grammar 갱신 흐름(기준 identity → 후보 verify FAIL과 변경 파일 → node schema diff의 검토 항목, 입력 불변·자동 채택 없음)은 `TestGrammarUpdateWorkflow`가 확인한다.

## SVC r2 offline 관측

`SvcFormat`은 `SVC-SERVICEHOST-r2`, `SvcLegacyFormat`은 `SVC-SERVICEHOST-r1`, `SvcCompositeSchema`는 `tsgk-svc-composite/r2`다. observer 입력은 `SourceEncodingValid`를 만족해야 한다. `SvcContext{DefaultLanguage,LanguageSource,CodeBehind []SvcReference}`는 caller가 검증해 준 문맥이며 `SvcReference{Name,Case}`는 불활성 문자열을 등록 case에 연결한다. API는 configuration·CodeBehind 파일·assembly를 읽거나 실행하지 않는다. profile decoder가 문맥의 상한·pair·고유 이름·등록 ID를 검사하며 직접 Go caller도 같은 사전조건을 유지해야 한다. `IncrementalCase.SvcSource`는 profile input identity와 결속한 원본 재관측 bytes다.

`SvcObservation.AdditionalDirectives`, `SvcDirective.DiagnosticSpans`는 복수 directive와 오류 위치를 원본 좌표로 보존한다. `Syntax(nil)`은 directive 오류만으로 ERROR를 알 수 있지만 필요한 inline tree 없이 NO_ERROR를 증명하지 않는다. bool 포인터를 주는 caller는 해당 원본 included range의 실제 parse 결과를 제공해야 한다. 시험 PASS와 source 유효성을 구분하며 코드의 실행 성공을 판단하지 않는다.

`SvcObservation.HasDirectivePrefix(encoding, source)`는 같은 source의 관측에서 ServiceHost 앞이 공백/BOM뿐인지 판정한다. UTF-8·UTF-16·CP949를 같은 decoder로 처리한다. 일반 C# 문자열의 directive marker와 host source를 구별하는 참조 guard이며 C# 구문 검증은 별도 parser가 수행한다. 별도로 CodeBehind의 같은 case 자기 참조는 prefix와 무관하게 거부한다.

`VerifyOracleSet`의 실제 파일 읽기는 manifest 16 MiB, record/raw member 64 MiB, source member 32 MiB 상한으로 제한한다. Member는 선언한 bytes+1까지만 읽으며, 거짓 크기 선언으로 큰 실제 파일을 전부 메모리에 적재하지 않는다. 초과는 MANIFEST_BYTES_LIMIT·MEMBER_BYTES_LIMIT·SOURCE_BYTES_LIMIT finding이며 열린 파일을 닫는다. Source 무결성 검사도 이 읽기 경계를 공유한다.

