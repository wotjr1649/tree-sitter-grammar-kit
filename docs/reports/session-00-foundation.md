# Session 00 foundation — pre-merge 보고

이 문서는 구현 범위와 pre-merge 검증을 기록한다. 최종 merge SHA/main CI/Issue close는 PR comment와 local handoff receipt가 소유하며 이 문서만으로 FOUNDATION_READY를 선언하지 않는다.

구현: root Go module, src/internal/foundation의 filesystem·Git 정책 검사와 negative controls, canonical 계약, 세 OS Foundation CI, template. 제품 명령·native oracle·Go adapter는 아직 구현하지 않았다.

로컬 검사 결과와 아직 남은 원격 gate를 구분한다.

| 범위 | 현재 관측 |
|---|---|
| 대상·seed | empty remote에서 README/LICENSE/.gitignore/.gitattributes만 seed; `e8d2d451267915fc48f5893366b99016d5d82ec4` |
| reference | stable v0.1.3 source/manifest/notes hash 확인, development snapshot 별도 분류; [출처](../provenance/upstream-sources.md) |
| foundation 로컬 | Windows amd64, Go 1.27.1, PowerShell 7.6.6에서 gofmt/test/vet/build/diff 검사 PASS. top-level test 5개, negative 대조 8개 실행 |
| CGO dependency | `go list -deps -test -json ./src/...`의 140개 record에서 CgoFiles/runtime/cgo 없음 |
| source-only | Git/PATH 없이 filesystem 검사 통과, 부모 Git marker가 있어도 checkout 검사 진입 거부 |
| 필수 세 OS CI | NOT_RUN |
| 분리 context review | NOT_RUN |
| merge/post-merge | NOT_RUN |

후속 추적은 [roadmap](../roadmap.md), 검사 계약은 [validation](../validation/validation.md)에 있다. local prompt/hash와 raw evidence는 Git에서 제외하며 공개 acceptance는 canonical 문서와 Issue에서 제공한다.

최초 local test는 설명 문서의 code-span 링크 예시를 실제 경로로 오인해 실패했다. 링크 검사에서 code span/fence를 제외하고 유효 예시와 실제 broken link 대조를 함께 재검증했다. 최초 실패 로그는 local evidence에 보존한다. 필수 파일·Git 정책·negative 검사를 완화하지 않았다.

미실행: 제품 inspect/identity/verify/schema, generation/native/incremental/replay/parity, race diagnostic, go-treesitter adapter, tag/Release/package. GitHub required approval과 STATIC_REVIEW를 동일시하지 않는다.
