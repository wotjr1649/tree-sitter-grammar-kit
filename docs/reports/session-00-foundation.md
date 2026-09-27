# Session 00 foundation — pre-merge 보고

이 문서는 구현 범위와 pre-merge 검증을 기록한다. 최종 merge SHA/main CI/Issue close는 PR comment와 local handoff receipt가 소유하며 이 문서만으로 FOUNDATION_READY를 선언하지 않는다.

구현: root Go module, src/internal/foundation의 filesystem·Git 정책 검사와 negative controls, canonical 계약, 세 OS Foundation CI, template. 제품 명령·native oracle·Go adapter는 아직 구현하지 않았다.

로컬 검사 결과와 아직 남은 원격 gate를 구분한다.

| 범위 | 현재 관측 |
|---|---|
| 대상·seed | empty remote에서 README/LICENSE/.gitignore/.gitattributes만 seed; `e8d2d451267915fc48f5893366b99016d5d82ec4` |
| reference | stable v0.1.3 source/manifest/notes hash 확인, development snapshot 별도 분류; [출처](../provenance/upstream-sources.md) |
| foundation 로컬 | Windows amd64, Go 1.27.1, PowerShell 7.6.6에서 gofmt/test/vet/build/diff 검사 PASS. review 수정 후 top-level test 6개, negative 대조 18개 실행 |
| CGO dependency | `CGO_ENABLED=0`의 `go list -deps -test -json ./src/...` 활성 closure 140개 record에 CgoFiles/runtime/cgo 없음. build tag에 숨은 저장소 C import는 별도 AST negative 검사로 거부 |
| source-only | Git/PATH 없이 filesystem 검사 통과, 부모 Git marker가 있어도 checkout 검사 진입 거부 |
| 필수 세 OS CI | [36347276561](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/36347276561) attempt 1, pull_request, head `746c1ad579c90f39f069f190da0bfc41a51a904c`, synthetic checkout `49e0097562e568185c7a347da118ca5e07704dbb`에서 세 OS job/step PASS. 아래 수정의 최종 CI는 PR receipt에서 별도로 확인해야 함 |
| 분리 context review | 위 head에 대해 Codex collaboration reviewer가 공개 diff/문서만 읽은 STATIC_REVIEW 수행. 직접 검사 실행·정식 GitHub approval은 아님. 아래 처분의 후속 확인은 PR comment에 기록 |
| merge/post-merge | NOT_RUN |

후속 추적은 [roadmap](../roadmap.md), 검사 계약은 [validation](../validation/validation.md)에 있다. local prompt/hash와 raw evidence는 Git에서 제외하며 공개 acceptance는 canonical 문서와 Issue에서 제공한다.

최초 local test는 설명 문서의 code-span 링크 예시를 실제 경로로 오인해 실패했다. 링크 검사에서 code span/fence를 제외하고 유효 예시와 실제 broken link 대조를 함께 재검증했다. 최초 실패 로그는 local evidence에 보존한다. 필수 파일·Git 정책·negative 검사를 완화하지 않았다.

## Review 처분

| ID | finding와 처분 | 재검증 |
|---|---|---|
| R1 | CGO-disabled list의 증명 범위를 활성 closure로 명시. 모든 설정의 dependency source에 cgo가 없다고 주장하지 않으며 저장소의 숨은 C import는 build-tag 무관 AST 검사로 거부 | hidden-cgo negative와 실제 dependency JSON PASS |
| R2 | required 파일을 읽기 전 link/special entry 거부, index의 mode 120000 등 비정상 mode 거부. 동시 적대적 변경 방어 claim은 없음 | file/parent link와 index symlink-mode negative PASS |
| R3 | 실제 CheckGit이 쓰는 index 판정에 tracked local path와 go.work 변이를 추가 | 여섯 local-path negative PASS |
| R4 | event github.sha와 checkout SHA 일치 assertion, PR head와 checkout identity를 별도 출력 | 로컬 workflow 정적 검토; 수정된 workflow의 hosted 결과는 최종 receipt |
| R5 | stale NOT_RUN CI/review 표를 실제 관측으로 갱신 | exact 기존 run/head/합성 merge 대조 |
| R6 | 보호 설정 부재만으로 merge를 막아야 한다는 제안은 채택하지 않음. 현재 세션 계약은 required approval이 없는 경우 명시적 merge 권한과 engineering gate를 허용하며 보호 설정 변경은 승인 밖 | protected=false, rulesets=[], classic protection 404 관측. 최종 head의 review/세 OS gate와 실제 규칙은 계속 적용 |

R1~R6에 대한 reviewer의 수정 확인·최종 head/CI 및 실제 merge 여부는 [PR #11](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/11)의 receipt를 읽는다. 이 pre-merge 문서의 과거 CI 성공을 후속 수정의 성공으로 옮기지 않는다.

미실행: 제품 inspect/identity/verify/schema, generation/native/incremental/replay/parity, race diagnostic, go-treesitter adapter, tag/Release/package. GitHub required approval과 STATIC_REVIEW를 동일시하지 않는다.
