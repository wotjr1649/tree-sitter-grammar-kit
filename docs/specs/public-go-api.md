# 공개 offline Go API — 설계 r1

이 문서는 S01에서 구현할 첫 공개 계약이다. 현재 package와 함수는 구현되지 않았다. root module의 `github.com/wotjr1649/tree-sitter-grammar-kit/src/kit`을 entry로 채택한다. CLI는 같은 operation을 호출하며 API가 CLI를 실행하지 않는다. 고급 native/replay API 전체 공개는 약속하지 않는다.

## S01 함수와 소유권

```go
package kit

func Inspect(ctx context.Context, request InspectRequest) (InventoryResult, error)
func Identity(ctx context.Context, request IdentityRequest) (IdentityResult, error)

type Limits struct {
    Files, FileBytes, TotalBytes, Depth, OutputBytes uint64
}
type Selection struct {
    Grammar string // explicit portable path inside Root; "." selects root grammar
    Files []FileSelection // nil: documented bounded discovery; empty non-nil: invalid
}
type FileSelection struct {
    Path string
    Role string
}
type InspectRequest struct {
    Root string
    Selection Selection
    Limits Limits
}
type IdentityRequest struct {
    Root string
    Selection Selection
    Limits Limits
}
type InventoryResult struct {
    Report Report
    Entries []InventoryEntry
    ClosureState string
}
type InventoryEntry struct {
    Path, Role, State string
    Size *uint64 // nil when not observed
}
type IdentityResult struct {
    Report Report
    Manifest Manifest
    SetSHA256 string
}
type Manifest struct {
    Schema, Algorithm, ModePolicy string
    Files []FileIdentity
}
type FileIdentity struct {
    Path, Role, Mode, ModeProvenance, SHA256 string
    Size uint64
}
type Report struct {
    Schema, Command, ExecutionStatus, EvidenceMode, Assessment string
    Identities []IdentityRef
    Findings []Finding
    Coverage Coverage
}
type IdentityRef struct { Role, Schema, SHA256 string }
type Finding struct { Code, Severity, Path, Message string }
type Coverage struct { Requested, Observed, Unsupported []string }
type Error struct { Kind, Code, Path string; Cause error }
func (e *Error) Error() string
func (e *Error) Unwrap() error
```

필드의 문자열 값은 [CLI/profile](cli-and-profile.md)과 [E0/identity](identity-and-evidence.md)의 닫힌 값 집합을 사용한다. `Error.Kind`는 `INVALID_INPUT`, `IO`, `CANCELLED`, `RESOURCE_LIMIT`, `UNSUPPORTED`다. `errors.As`로 `*kit.Error`를 구분하고 취소는 `errors.Is(err, context.Canceled/DeadlineExceeded)`도 보존한다. 오류 문자열은 machine identity가 아니다. 성공한 관측이나 유효한 비교의 불일치는 실행 오류와 구분한다. S02/S03의 verify/schema 함수와 추가 result는 해당 구현 전에 같은 owner에서 별도 revision으로 고정한다.

caller가 명시한 Root와 Selection만 읽는다. 부모 저장소 탐색·Git·Node·shell·compiler·target JS·network·plugin·stdout/stderr·os.Exit·chdir·process 환경 변경·파일 생성은 API 효과에 포함되지 않는다. archive 확장은 S02의 명시된 입력 계약 이전에 지원하지 않는다. `parser.c` 부재는 inspect/identity 자체의 실패 조건이 아니다.

모든 Limits 필드는 양수여야 하며 0은 invalid input이다. `nil` context와 deadline 없는 context는 거부한다. deadline과 size/count/depth/output 상한을 함께 적용하며 각 파일 열기 전, bounded read 사이, record 생성 전에 취소를 확인한다. source read 자체가 host I/O에서 멈추는 경우 hard 실시간 취소를 보장하지 않는다. 무한·무제한 fallback은 없다. CLI의 기본 유한 값은 CLI owner가 소유하며 직접 API 호출도 동일 guard를 거친다.

caller는 호출 중 request의 slice와 source snapshot을 변경하지 않는다. API는 caller 입력을 변경하거나 반환 후 보관하지 않으며 결과는 호출별로 새로 할당해 caller가 소유한다. iterator나 close할 native 자원은 반환하지 않는다. 서로 독립적인 불변 root에 대한 concurrent 호출을 지원하는 계약이며 S01에서 실제 동시성 시험으로 검증한다. shared root를 외부 process가 바꾸는 상황을 안전한 snapshot으로 보장하지 않는다.

실패 시 오류와 함께 반환된 report는 `COMPLETED`/`PASS`가 될 수 없다. partial 관측은 한도 내에서 유용한 것만 반환하며 `IdentityResult.SetSHA256`은 완전한 선택 집합을 읽지 못하면 비어 있어야 한다. 결과가 한도 안에 담기지 않으면 `RESOURCE_LIMIT`이고 출력 한도를 넘어 전체 raw를 확보했다고 주장하지 않는다. CLI의 파일 publication은 API 반환 후 별도 no-clobber 단계이며 publication 실패도 CLI 실패다.

## 외부 소비자 검증

S01은 `src/testdata/consumer/`에 source와 `go.mod.tmpl` 데이터를 두고, 실제 module은 checkout 밖 임시 디렉터리에 생성한다. test는 `GOWORK=off`, `GOTOOLCHAIN=local`, `CGO_ENABLED=0`, `GOPROXY=off`에서 공개 import만 사용하고 internal/native/consumer 타입을 import하지 않는다. CLI/API 결과·finding·오류를 동일 fixture로 비교한다. API를 직접 불러 CLI guard를 건너뛰는 negative case도 검사한다.

S01의 local replace는 초기 소비 경계만 검증한다. S08은 준비된 source-export/versioned local module proxy를 사용해 developer checkout 경로와 unpublished tag에 의존하지 않는 배포 형식도 검증한다. publication은 수행하지 않는다. 이 PREPARE에서 구현 전 API에 대한 consumer 성공을 만들지 않는다.
