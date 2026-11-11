# #216 Web16 정의 분류

Windows에서 JavaScript4·JSX3·TypeScript7·TSX2의 새 native/reference 관측, graph별 외부 caller approval, 같은 guarded snapshot의 정의 projection과 별도 공개 API parity를 확인했다. 이번 16개 정의는 조건부 `STRUCTURAL_ONLY`다. 원장의 compiled metadata·semantic 평가 상태와 전체 #216/#195 완료 판정은 유지한다.

구현의 기준은 PR310 merge `150621791da9ade381c034c5d0f3bbaa7f8033e9`다. 실제 관측과 guarded reader는 모두 `fc0a7433fb2b5fd27cf6e41f1d166b1a23c66fa6`, tree `4f623c8032a9d80e3adf1fe18b8c220d6639bc91`에 고정했다. 이 보고서를 갱신하는 후속 commit이나 PR/merge revision을 당시 실행 source로 표시하지 않는다.

## 관측 범위

| Profile | 정의 | Patterns | Query별 capture-name 항목 | ABI symbol/field | Guarded tuple |
|---|---:|---:|---:|---:|---|
| `javascript4-empty` | 4 | 17 | 20 | 291/36 | required4 / verified4 |
| `jsx3-empty` | 3 | 7 | 7 | 291/36 | required3 / verified3 |
| `typescript7-empty` | 7 | 20 | 20 | 388/40 | required7 / verified7 |
| `tsx2-empty` | 2 | 7 | 7 | 408/43 | required2 / verified2 |

합계는16개 정의·51 patterns·54개의 query별 capture-name 항목·전역 고유 이름48개다. 각 tuple의 failed·blocked·unprocessed는0이다. 네 profile은 빈 UTF-8 source, 편집 없음, API=false, 단일 `native-query` frame이다. Native/reference는 사전등록한 명시적 빈 predicate 목록과 빈 expected/structural tuple에 일치했다.

공개 CST는 named `program` root 하나이며 parent=-1·field=null·byte span0..0·양쪽 point(0,0)·extra/error/missing=false다. 공개 child 수0과 runtime 내부의 숨겨진 EOF subtree를 구분한다. 실제 ABI language/runtime/min-compatible은15/15/13이다. 모든 query pattern이 `program` 이외의 시작 node를 요구하는 빈 입력 관측이므로, 비어 있지 않은 원래 workload의 query 의미론을 검증한 결과로 세지 않는다.

JavaScript/JSX는 grammar와 `tree_sitter_javascript` symbol이 같아도 route·query·anchor·run·graph·승인은 각각 구분했다. TypeScript/TSX의 중첩 parser/scanner 경로와 `common/scanner.h`를 정확한 역할·경로·bytes·SHA로 결속했다. 기존 Small17 closed5/17, CJR closed3/13, Go 및 C#/CPP paging 경계와 resource 한도는 유지했다.

## 실행과 guarded 검증

Source724개·40,973,710 bytes, 선택 Go/SDK/module 입력12,604개의 전후 pin을 확인했다. 새 Windows 수집은 기존 바이너리·도구·controller·preregistration과 profile별 유한 예산을 고정하고 하나씩 수행했다. 실제 capture·조립·guarded 실행의 원본 tool 종료, HARD4GiB Job Object receipt의 검증된 cleanup·잔류0과 lease 반환을 회수했다. 실패·미실행 초안과 부분 출력도 보존했다.

기존 static build9/control53의 source `9eba39be83174829ef81899f79b13aaccb77aa69`와 새 current native/reference의 source `fc0a7433`을 분리했다. 새 graph 네 개에 독립 provenance 검토를 거쳐 graph 밖 caller 승인을 각각 만들었다. 기존 build/control 근거를 새로 실행한 것처럼 세거나 assembler가 자기 결과를 승인하지 않았다.

Guarded 단계는 profile마다 `TestQueryClassificationWeb16Local`의 `definitions` stage를 한 번 호출했다. 같은 live snapshot의 eligibility·독립 full CST·metadata·query별 reference invocation·request/frame·compiler/library·stderr owner를 확인하고, 별도 공개 API 결과와 대조한 뒤 정의 projection을 저장했다. 이 단계는 새 compiler/native/reference 관측을 생성하지 않았다.

| Profile | Guarded outer wall | Outer peak bytes | 반환된 child leases |
|---|---:|---:|---:|
| JavaScript4 | 130,008ms | 320,053,248 | 1,186/1,186 |
| JSX3 | 11,256ms | 381,587,456 | 1,176/1,176 |
| TypeScript7 | 14,590ms | 403,951,616 | 1,218/1,218 |
| TSX2 | 14,896ms | 406,839,296 | 1,168/1,168 |

각 parent lease도8/8 반환됐다. Outer는 전체 child와 자손에800초·HARD4GiB, 내부 kit는300초·HARD4GiB, Go test는270초 한도였다. 실제 kit wall은 각각690/479/524/486ms였다. 나머지 outer 시간에는 전후 source·선택 SDK·case·graph 확인이 포함된다. Trusted metadata coordinator는 Job 밖에서 유한900초 예산을 사용했다.

최종 결과 correctness와 security/provenance 검토는 각각 B0/M0/Minor0이었다. 검토는 저장 결과·승인·요청·receipt·입력 및 source/SDK snapshot을 읽기 전용으로 대조했으며, reviewer가 Go나 API parity를 다시 실행한 것은 아니다. 원본 tool terminal과 caller lease 반환은 Root가 직접 회수했고 독립 reviewer는 보존 closure를 검토했다.

## 보존 근거

네 정의 결과의 SHA-256은 다음과 같다.

| Profile | Definition result SHA-256 |
|---|---|
| JavaScript4 | `d3daa6ee982756d3d104d1044764359ab379c237ab4ea8e79bc9a4733bf7726a` |
| JSX3 | `9a98682bfaee0bf41cdd28d7d0afa80b6209a10430ba6ce8d365798521b4738a` |
| TypeScript7 | `9db9c54414d03945e1a47908157cdcfda90bc293675e6236b032f339e9eeaaf1` |
| TSX2 | `6c35f5a133b14b265a06f91d97a3ce87ac9db8cf93de77c82d7158846634cdc9` |

| 로컬 보존 근거 | SHA-256 |
|---|---|
| Windows capture cohort | `0461496b9c9418aacb2f6bb712490d65fc848ca879eb1f9be49ebeba0b242e65` |
| Assembly adoption cohort | `f11ce41100f91c349202be3f480e41806fd14cceea8a9df09ad6d4d47b6da63e` |
| Caller approval decision | `6dab7c1c18dc4a3fb9f6618339840a35e7f180b84d2994082c0d4dad565920bf` |
| Caller approval manifest | `90ab402792816abf3aad2b2f679cb578d088120648d1ab0415829e0a157ee900` |
| Guarded originals cohort | `89999b4c25cb6f37950904960cc24487729ae7bb7817e132a4bc7809fec8e186` |
| Guarded final adoption | `39dd52285be506235faaf0a5a1a936b15149b2da8bb7939f2047fe194003dfc1` |
| Go focused validation adoption | `d04171ca45290573ab89b531209c5804da7d04bf8d060caeeb4af881984a706e` |

원본 graph·approval·결과·receipt와 controller는 미추적 로컬 증거다. 이 추적 보고서나 hosted CI만으로 당시 관측을 독립 재구성·재승인할 수 없다. 파일 pin과 receipt는 서명이나 vendor/publisher 인증이 아니며, 단일 Root가 관리하는 기존 Windows·도구·선택 입력의 cooperative host 범위다.

## 검사와 남은 범위

Frozen source의 focused 검사는 native18·kit33·foundation49, 최상위100 PASS다. Windows case-insensitive filesystem의 `case-only-link` 하위 반례1개는 사유를 확인한 SKIP이며 상위 시험은 PASS다. `CGO_ENABLED=0 GOEXPERIMENT=jsonv2`의 `go vet -p=1 ./src/...` 및 CLI build도 통과했다. 전체 module Go test suite는 로컬에서 실행하지 않았으며 PR 필수 CI와 공식 artifact·실제 main 검증은 별도 통합 gate다.

기존 ordinary qualification workload는 API=true이고 이번 빈 anchor는 API=false다. 네 ordinary workload의268 cases·112 edits·380 case steps·492 tree roles·2,039 potential query/tree-role 좌표는 이번16개 정의의 실행 분모가 아니다. 각 정의는 `STRUCTURAL_ONLY`지만 원장의 `STRUCTURAL_CANDIDATE`·compiled metadata `UNVERIFIED`·assessment `NOT_ASSESSED`·semantic_required null·semantic_evaluated false를 유지한다.

기존 [Small17까지의 역사적52/130](issue-216-small17-classification.md)에 이번16개를 중복 없이 더한 **역사적 조건부 분류 누계는68/130**이다. 서로 다른 source/reader/profile의 합계이며 단일 source의68개 검증이나 전체 완료가 아니다. 나머지62개 정의와 전체 ordinary per-case·semantic 평가, Windows/Linux 후속 검증, SVC69·large4·replay·inventory·fan-in 및 #216/#195 완료는 남아 있다.

새 Linux/macOS 수동 관측은 수행하지 않았다. 당시에는 기존 mandatory baseline을 유지하고 #303 macOS race timeout을 비차단·release 후 분석 대상으로 두었으며 최종 release workflow skip은 미구현이었다. 이 처분은 이후 [2026-11-06 #303 범위 재정의](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/303#issuecomment-6682907056)로 대체됐고 skip 요청은 철회됐다. 현재 macOS CI는 #332로 일시 제외됐으며, #303은 Windows foundation `src/kit`의 자연 표본 판정을 추적한다. 새 공개 API·CLI·schema·dependency나 release 발행은 이번 단위에 포함하지 않는다.
