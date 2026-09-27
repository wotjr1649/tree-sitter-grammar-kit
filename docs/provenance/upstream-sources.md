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
