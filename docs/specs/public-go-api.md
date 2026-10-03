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
    LargeFileProfile string // "" 또는 등록된 identity 한정 예외(pg-large-source-r1)
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

필드의 문자열 값과 JSON 이름은 [CLI/profile](cli-and-profile.md)과 [E0/identity](identity-and-evidence.md)의 닫힌 값 집합을 사용한다. `Error.Kind`는 `INVALID_INPUT`, `IO`, `CANCELLED`, `RESOURCE_LIMIT`, `UNSUPPORTED`이고 `Code`는 안정적인 기계 값이다(예: `GRAMMAR_INVALID`, `SELECTION_INVALID`, `LINK_OR_SPECIAL_REJECTED`, `HARDLINK_REJECTED`, `ROOT_NOT_LOCAL`, `NO_GRAMMAR_SELECTED`, `EMPTY_SELECTION`, `FILE_COUNT_LIMIT`, `FILE_BYTES_LIMIT`, `TOTAL_BYTES_LIMIT`, `DEPTH_LIMIT`, `RECORD_LIMIT`, `OUTPUT_LIMIT`, `WALL_LIMIT`, `SOURCE_CHANGED`, `READ_FAILED`, `CANCELLED`). `errors.As`로 `*kit.Error`를 구분하고 취소는 `errors.Is(err, context.Canceled/DeadlineExceeded)`도 보존한다. 오류 문자열은 machine identity가 아니다. 성공한 관측(unknown layout, 미해결 closure, encoding `BLOCKED`/`UNRESOLVED`)은 실행 오류가 아니라 결과 안의 finding과 필드다. S02/S03의 verify/schema 함수와 추가 result는 해당 구현 전에 같은 owner에서 별도 revision으로 고정한다.

Root는 절대화한 뒤 한 번 `EvalSymlinks`로 해석하고 root 자체는 따라가서 디렉터리인지 확인한다(macOS `/var` → `/private/var`, root를 가리키는 symlink·Windows junction 같은 alias를 의도적으로 허용). 존재하지 않거나 디렉터리가 아닌 root는 `INVALID_INPUT`이고, Windows UNC share와 `\\?\`·`\\.\` device namespace root는 network·device 접근을 피하려고 `ROOT_NOT_LOCAL`로 거부한다. root 아래 항목은 따라가지 않는다. caller가 명시한 Root와 Selection만 읽는다. 부모 저장소 탐색·Git·Node·shell·compiler·target JS·network·plugin·stdout/stderr·os.Exit·chdir·process 환경 변경·파일 생성은 API 효과에 포함되지 않는다. 제품 dependency closure(`src/kit`, `src/cmd/tsgk`)에는 `os/exec`, `net`, `plugin`이 없다. archive 확장은 S02의 명시된 입력 계약 이전에 지원하지 않는다. `parser.c` 부재는 inspect/identity 자체의 실패 조건이 아니다.

`Selection.Grammar`의 전체 값 `"."`은 이미 확인한 Root 자체를 선택하는 sentinel이다. 빈 값은 오류다. source 등록부의 `grammar_subdirectory`에도 같은 규칙을 적용한다. sentinel은 파일/member 경로나 내부 segment 허용 규칙이 아니다. `./x`, `x/.`, `x/../y`, 절대·drive·UNC·역슬래시 경로는 거부하고 다른 값은 [portable path 계약](trust-and-execution.md)을 따른다. `Selection.Files`의 role은 grammar/generated/scanner/query/corpus/metadata 중 하나이고 중복 path는 오류다.

모든 Limits 필드는 양수여야 하며 0은 invalid input이다. `nil` context와 deadline 없는 context는 거부한다. `Wall`은 kit가 설정한 한도이며 그 초과는 `RESOURCE_LIMIT`(`WALL_LIMIT`)다. caller context의 취소나 deadline 도달만 `CANCELLED`다. 둘이 함께 관측되면 caller 취소를 먼저 보고한다. 각 파일 열기 전, 1 MiB bounded read 사이, 디렉터리 listing batch 사이에 취소를 확인한다. source read 자체가 host I/O에서 멈추는 경우 hard 실시간 취소를 보장하지 않는다. 무한·무제한 fallback은 없다. CLI의 기본 유한 값은 CLI owner가 소유하며 직접 API 호출도 동일 guard를 거친다.

caller는 호출 중 request의 slice와 source snapshot을 변경하지 않는다. 열린 파일의 크기·수정 시각·파일 identity가 열기 전 관측과 다르거나 읽은 bytes가 크기와 다르면 `IO/SOURCE_CHANGED`다. 이것은 관측 가능한 변경의 검출이며 적대적 동시 교체를 막는 sandbox가 아니다. API는 caller 입력을 변경하거나 반환 후 보관하지 않으며 결과는 호출별로 새로 할당해 caller가 소유한다. iterator나 close할 native 자원은 반환하지 않는다. 서로 독립적인 불변 root에 대한 concurrent 호출을 지원하며 S01 시험이 8개 root의 동시 호출 결과를 순차 결과와 대조한다. shared root를 외부 process가 바꾸는 상황을 안전한 snapshot으로 보장하지 않는다.

실패 시 오류와 함께 반환된 report는 `COMPLETED`/`PASS`가 될 수 없다. 실패 결과는 E0 축·policy·실패 finding만 담고 entries/manifest/records는 비운다. `IdentityResult.SetSHA256`은 완전한 선택 집합을 읽지 못하면 비어 있다. 결과의 JSON encoding(+CLI 줄바꿈 1 byte)이 `OutputBytes`(corpus는 `ReportBytes`)를 넘으면 `RESOURCE_LIMIT`이다. CLI의 파일 publication은 API 반환 후 별도 no-clobber 단계이며 publication 실패도 CLI 실패다.

## 외부 소비자 검증

`src/testdata/consumer/`에 source와 `go.mod.tmpl` 데이터를 두고, 실제 module은 checkout 밖 임시 디렉터리에 생성한다. 시험(`src/cmd/tsgk` 의 `TestExternalConsumerAndCLI`)은 `GOWORK=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`, `GOPROXY=off`에서 공개 import만 사용해 build한다. internal/native/consumer 타입을 import하지 않는다. 같은 fixture에서 CLI 결과와 API 결과의 JSON bytes가 같고, 경로 탈출 selection에서 API 오류 code와 CLI 오류 code·exit가 같은지 확인한다. 두 실행 파일은 `PATH`를 비운 환경에서 실행한다.

S01의 local replace는 초기 소비 경계만 검증한다. S08은 준비된 source-export/versioned local module-proxy를 사용해 developer checkout 경로와 unpublished tag에 의존하지 않는 배포 형식도 검증한다. publication은 수행하지 않는다.
