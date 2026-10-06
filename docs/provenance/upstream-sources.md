# 고정 출처와 관측 한계

관측: 2026-09-27 19:55 UTC. source는 데이터로만 읽었으며 외부 grammar.js/lifecycle/native/Python을 실행하지 않았다. 선택한 안정 baseline은 이 Session 중 자동 갱신하지 않는다.

## Grammar와 consumer

| 자료 | 고정 identity | 역할·관측 한계 |
|---|---|---|
| [BrightScript v0.1.3](https://github.com/wotjr1649/tree-sitter-brightscript/releases/tag/v0.1.3) | tag object `e4edb54fc4c5a7445a0752d7af0805556aeaab37`, peeled commit `b9eab178472c9a43914bd86eb9a14b8a16a9464e` | 공개 non-draft/non-prerelease, source-only 안정 baseline. MIT LICENSE 관측. source ZIP 내려받아 hash 확인; native 시험은 kit에서 미실행 |
| [BrightScript v0.1.2](https://github.com/wotjr1649/tree-sitter-brightscript/releases/tag/v0.1.2) | tag object `df0f589bd2cd50592c0d049964f780c00c5ca2c9`, commit `6724e03dd9b998b58b4e3525d60dc1e6ce3925e3` | historical replay만. verification ZIP의 API digest `da1a3f932630b09410e82931a22e8dfb9533ebbd57614bef4d0693f1ee42f584`; 이번 다운로드·replay는 NOT_RUN |
| [BrightScript 개발 snapshot](https://github.com/wotjr1649/tree-sitter-brightscript/tree/bb35ac385c74a3eea26fd8a2712a0f8f4835f376) | `session/10-v014-native-parity`, commit `bb35ac385c74a3eea26fd8a2712a0f8f4835f376` | DESIGN_REFERENCE_ONLY. 새 native preflight/qualification, POSIX supervision·memory/parity 설계 참고. release baseline 아님 |
| [go-treesitter oracle](https://github.com/wotjr1649/go-treesitter/tree/2494b35499a3b87281e6837de0f9820a28a6f7ec/tools/native-oracle) | 공개 commit `2494b35499a3b87281e6837de0f9820a28a6f7ec`; driver blob `5fdebabb43b983423023e1d430bc3f80a3045ead` | README와 driver 출력 필드 정적 조사. MIT metadata 관측. core dependency/호환성 검증 아님 |
| [Cooklang audit](https://github.com/addcninblue/tree-sitter-cooklang/tree/4ebe237c1cf64cf3826fc249e9ec0988fe07e58e) | commit `4ebe237c1cf64cf3826fc249e9ec0988fe07e58e` | package.json의 legacy tree-sitter metadata와 peerDependencies/tree_sitter 키 불일치 관측. package의 ISC 표기와 API license=null은 법적 확정 근거가 아니다. vendoring/실행 안 함 |

v0.1.3의 선택 자산 `tree-sitter-brightscript-v0.1.3-source.zip`: 3106108 bytes, SHA-256 `d5d98543439afbf2fbce8e3e9c234d433c1d908fddfb5e2c9ca81e7d7f96f4c7`.
[release manifest](https://github.com/wotjr1649/tree-sitter-brightscript/releases/download/v0.1.3/release-manifest.json) SHA-256 `458783f418508830a03af3bcfd74689a255b95962f2fda1eff71c6afaeffb02e`, release notes SHA-256 `842742b9a93b1e2584b13e9fbb0cb0c76db207774d08aac6b3d89f22819273f4`도 다운로드 bytes와 대조했다. 태그와 manifest의 commit이 일치한다. ZIP 내부 README/보고서는 출하 전 snapshot을 유지하므로 공개 상태는 Release/manifest/출하 CI와 구분한다.

[main CI 36337813324](https://github.com/wotjr1649/tree-sitter-brightscript/actions/runs/36337813324), [tag CI 36337969508](https://github.com/wotjr1649/tree-sitter-brightscript/actions/runs/36337969508)는 `.github/workflows/ci.yml`, attempt 1, push, 위 peeled commit이며 API에서 세 OS job/step success를 확인했다. 전체 raw 재실행은 하지 않았다. Windows 17-gate 및 W12 231-input 결과는 원 report/manifest의 주장으로 읽었고 kit 검증으로 전이하지 않는다. Go/V9·device/V8 결과는 주장하지 않는다.

stable → 개발 snapshot compare API는 12 commits와 변경 20개를 보고했다. grammar/scanner/query/node schema에는 변경이 없고 이번 차이는 문서·CI·qualification harness·native evidence·resource policy에 있다. v0.1.2 → v0.1.3은 원 report상 parser version metadata 변경과 macOS path/RSS 수정이며 grammar 동작 유지와 harness identity 동일성을 구분한다. 개발 commit의 common/preflight CI는 성공했지만 [native qualification 36345241683](https://github.com/wotjr1649/tree-sitter-brightscript/actions/runs/36345241683)은 attempt 1 failure다. 구현 완료나 full cross-platform qualification 기준으로 채택하지 않는다.

## 개발 toolchain과 공식 계약

| 항목 | 채택 identity·이유 |
|---|---|
| Go | 로컬 `go1.27.1 windows/amd64`; [공식 배포 목록](https://go.dev/dl/?mode=json)의 stable 버전 및 세 OS archive 존재 확인. root go.mod와 CI는 1.27.1 고정 |
| checkout | [v7.0.1](https://github.com/actions/checkout/releases/tag/v7.0.1), commit `3d3c42e5aac5ba805825da76410c181273ba90b1`; action.yml의 node24, credential persistence 입력 확인 |
| setup-go | [v7.0.0](https://github.com/actions/setup-go/releases/tag/v7.0.0), commit `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`; action.yml의 version/cache/arch 입력과 node24 확인 |
| hosted runner | [공식 표](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)로 `windows-2025` x64, `ubuntu-24.04` x64, `macos-15` arm64 선택. public 저장소 standard runner 범위. 실제 image/arch는 CI에서도 검사 |
| Tree-sitter | BrightScript manifest의 CLI 0.27.0/ABI 15는 reference identity. kit tool은 설치하지 않음. [generate](https://tree-sitter.github.io/tree-sitter/cli/generate.html), [native parse](https://tree-sitter.github.io/tree-sitter/creating-parsers/1-getting-started.html), [scanner](https://tree-sitter.github.io/tree-sitter/creating-parsers/4-external-scanners.html)는 설계 참고이며 pinned CLI의 모든 기능을 보증하지 않음 |
| Go API | [internal layout](https://go.dev/doc/modules/layout), [CGO_ENABLED](https://pkg.go.dev/cmd/cgo), [encoding/json](https://pkg.go.dev/encoding/json)을 확인. mutable 문서는 구현 시 해당 버전으로 다시 대조 |

Actions의 공식 release·manifest·입력과 실행 효과를 검토했으며 전 의존 코드의 보안 감사는 수행하지 않았다. workflow는 read-only token, credential persistence off, exact SHA, bounded timeout으로 사용 범위를 제한한다. 다운로드/Actions 준비는 CI 개발환경 단계이며 제품 offline claim과 별개다.

## Campaign 01 PREPARE의 추가 관측

위 S00의 v0.1.3/v0.1.2/development와 실패 이력은 원래 identity로 보존한다. 2026-09-29 준비에서는 이미 보관된 [BrightScript v0.1.4](https://github.com/wotjr1649/tree-sitter-brightscript/releases/tag/v0.1.4)를 maintained reference 후보로 연결했다. tag object `33898125ef163487407a60d17608efad32e6a872`, release commit `e47cf8072538d1360256da975716ad552418c286`, tree `dc9e6f4d158926234b77f69f143f799d7d586a2f`다. source ZIP은 6,612,139 bytes, SHA-256 `965d57cb5f71f5307ea83276701e738ce3f14dc8ab0512eeac136757332e4e39`이며 기존 보관 bytes를 다시 확인한다. 이전 qualification/report는 kit의 NEW_RUN으로 승계하지 않는다.

2026-10-02 PREPARE #20에서 csharp·typescript·tsx·swift·postgresql-sql·tsql의 patched 후보를 채택했다. 각 route의 고정 upstream commit, 저장소 안의 patch subject와 결과 hash, native 근거는 등록부의 `adoption`에 있다. T-SQL upstream은 `meloncholera/tree-sitter-mssql@8620fbcfca9438e1ff7104835bcb8305aba3bdfa`(MIT)로 바뀌었고 이전 Crary identity는 `superseded_candidate`에 남는다. 상세는 [6-route 채택 절](../validation/source-feature-feasibility.md#prepare-06-6-route-최종-채택-2026-10-02).

[26-route 후보 등록부](../../src/contracts/language-sources.json)는 기준일의 immutable commit, grammar subdirectory, scanner/shared path, generated artifact 존재와 license metadata를 기록한다. 실제 metadata/source 실행 검증 상태는 분리한다. 후보 SHA는 완전한 JS dependency closure나 legacy~stable feature 지원 증거가 아니며, 실제 dependent operation 전에 필요한 모든 bytes·lock/helper/scanner/query를 검토하고 source manifest로 결속한다.

PREPARE owned fixture는 기존 Tree-sitter CLI 0.27.0(ABI15), UCRT64 GCC16.2.0, 별도 고정 runtime commit `659cda7c7f86ebe31cc825dc5da59e9add172dc7`(header 허용 ABI13~15)을 사용한다. 이 runtime commit을 v0.27.0 release라고 부르지 않는다. CLI v0.27.0 tag object는 `3e719425fc48f5b4cdb25c580e44023882f5e2a7`, peeled commit은 `6070dbfefd326bd735e5683eb128cc1b57dad0c0`이며 runtime 후보와 unicode/header 관련 변경이 있다. 각 identity와 실제 build/parse 결과를 따로 기록한다. 원본 checkout은 변경하지 않고 승인한 새 scratch의 복사본에서 검사한다.

언어별 SDK·DB server·전역 tool·새 Go dependency는 추가하지 않는다. maintainer README의 지원 문구와 release 여부는 출처 관측이며 full conformance나 법적 license 검토로 확대하지 않는다. [준비 보고서](../reports/campaign-01-2026-09-29-preparation.md)가 실제 미해결 입력과 준비 상태를 연결한다.

## Native runtime의 field 조회 patch (#105)

#105에서는 기반 commit `659cda7c7f86ebe31cc825dc5da59e9add172dc7`에 [MIT fork의 수정 commit](https://github.com/mgsloan/tree-sitter/commit/18302989e05fcc8da7ce7fca3f8aedadb7191fdd)의 visible alias guard 6줄을 적용했다. patch 702 bytes와 적용 후 node.c 25390 bytes, 당시의 무할당 정적 검토·mutant·26 route 비교는 [#105 통합 시점의 기록](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/f1412302637f9da72f4180977e8783a1abe4e7f4/docs/provenance/upstream-sources.md#native-runtime의-field-조회-patch-105)에 보존한다. 이 guard만으로 hidden field shadow·ERROR recovery·extra 경계의 API 차이 204건은 해결되지 않았다.

현재 [#118](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/118)의 local patch는 `ts_node_child_by_field_id`의 재귀 field-map 탐색을 기존 cursor 순회로 바꾼다. cursor가 직렬화하는 직접 child 중 요청 field를 가진 첫 child를 반환하며, 가장 가까운 hidden ancestor의 직접 field, visible alias의 소유권, extra 경계를 같은 구현으로 판정한다. `ts_node_child_by_field_name`과 negated-field query도 공유 함수를 호출한다. fork의 6줄 patch와 겹쳐 적용하지 않으며 새 local 함수 전체를 fork commit의 수정으로 귀속하지 않는다. 기반 runtime·MIT·ABI 13..15·나머지 82파일은 유지한다.

[literal patch 파일](../../src/drivers/native-c/runtime-field-lookup.patch.json)은 3326 bytes, SHA-256 `f489c7a7dc8c948274e673f2713fc0dedf54aa39bf10c194a57ee411e7cbaa1d`다. 원본 `lib/src/node.c`는 25151 bytes, SHA-256 `fb0b5eecacb6d7e324f60914893801c0d147f413dd0af73a19ef270d341a77b5`; 적용 후에는 23579 bytes, SHA-256 `4ffa3a64675b95316ae92e11cbfc9754f908bb151c5499c73fa6371a93358740`다. [runtime manifest](../../src/drivers/native-c/runtime-manifest.json)의 `origin: LOCAL`·tracking Issue·MIT·원본·subject·적용 후 83파일 closure가 출처와 build 입력을 결속한다.

조회는 visible child의 선형 순회와 임시 cursor stack 할당을 사용한다. 성공·미일치 경로 모두 cursor를 해제하며 반환 `TSNode`는 tree 수명에 속한다. 넓은 node에 반복 조회하면 비용이 커질 수 있으므로 기존 운영 상한에서 caller 회귀를 확인한다. 등록 large fixture의 API 관측은 기존 정책대로 꺼져 있으며, large의 전체 API 조회 성능을 보증하지 않는다. 새 I/O·외부 실행·dependency는 추가하지 않는다. 보안 검토는 이 local patch·호출자·소유 대조 범위이며 runtime 전체 감사가 아니다.

owned fixture는 named·anonymous alias와 일반 hidden 상속, 안쪽 field shadow, ERROR 내부 field, hidden token과 extra 경계를 검사한다. by-name locator의 실제 byte 범위와 unknown field null, negated-field query의 독립 기대 capture도 단언한다. 원래 runtime으로 되돌린 mutant와 후보의 실행 결과, 26 route·SVC·large 비교 및 실제 세 OS·main 근거는 #118에 기록한다.

고정 upstream release가 위 경계를 모두 보존하면 local patch 제거 후보를 만든다. 기반 pin·전체 closure hash를 갱신하고 patch 파일과 manifest의 `patches`를 제거하되 owned 시험은 유지한다. 같은 26 route·SVC·large·Oracle 비교와 세 OS qualification에서 기존 non-API 결과를 유지하고 API 차이가 없음을 확인한 뒤 PR로 제거한다. upstream branch의 이동이나 commit의 존재만으로 자동 제거하지 않는다.
