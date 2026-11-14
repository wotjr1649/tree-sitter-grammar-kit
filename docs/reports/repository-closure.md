# 저장소 종료와 go-treesitter 인계

결정: 2026-11-14, 사용자([#195 코멘트](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/195#issuecomment-6875080857)).

## 왜 종료하는가

이 저장소는 `go-treesitter`의 grammar 도입·업데이트를 검증하려고 만들었다. 당시 `go-treesitter`는 pure-Go runtime 위에 patch를 얹어 쓰고 있었다. 이 저장소는 26개 grammar를 native로 빌드하고, 결함이 있으면 remedy와 C2 patch로 고친 뒤, 그 결과가 언어별 구문 요구를 만족하는지 판정했다.

`go-treesitter`는 [ADR-0020](https://github.com/wotjr1649/go-treesitter/blob/main/docs/design/decisions/ADR-0020-cgo-free-library-on-unmodified-tree-sitter.md)(2026-11-13)에서 방향을 바꿨다.

- upstream grammar와 runtime의 C를 **수정하지 않고** wasm을 거쳐 Go로 변환한다.
- 정확성 기준은 **같은 소스의 native C 빌드**이며, `go-treesitter`가 자기 도구(`tools/native-ref`, `tools/conformance`)로 직접 만든다.
- grammar-kit는 grammar **공급망 점검**에만 쓰고, CI나 tools 모듈에서만 실행한다.

그 결과 이 저장소의 무거운 부분은 할 일이 없어졌다.

- **patch된 grammar 판정:** `go-treesitter`는 원본 grammar를 쓰므로 대상이 다르다.
- **native 실행·oracle 비교:** `go-treesitter` 자체 도구와 겹친다.
- **26 route qualification, ordinary query 의미 검증, 과금 원장, C# G1 승인 체계:** 그 판정을 근거로 증명하는 장치였으므로 함께 필요 없어졌다.

그래서 저장소를 따로 둘 이유가 없다고 판단했다. 필요한 것만 `go-treesitter`가 흡수하고, 개발은 그곳에서 이어 간다.

## 무엇을 넘기는가

| 대상 | 인계 문서 |
|---|---|
| `schema check`/`schema diff`(정적 node-types 비교). ADR-0020이 grammar 변경마다 요구하지만 `go-treesitter`에는 아직 구현이 없다 | [인계 A부 §A1](repository-closure-handoff-a.md) |
| C# G1(B01–B04)과 #447에서 찾은 grammar 결함과 불일치. upstream 원본의 결함, kit patch가 바꾼 것, 설계 차이로 나눴다 | [인계 A부 §A2](repository-closure-handoff-a.md) |
| 26 route의 patch와 결함 목록. upstream 기여 후보와 공개할 known gap을 고르는 자료이며, wave 1은 표시해 두었다 | [인계 B부 §B1](repository-closure-handoff-b.md) |
| 26 route의 언어 기능 범위, 등록 사례, 시험 코퍼스로 재사용할 때의 주의 | [인계 B부 §B2](repository-closure-handoff-b.md) |

넘기지 않는 것은 다음과 같다.

- native 실행, oracle, incremental 비교
- qualification
- ordinary query 의미 검증
- eligibility 과금 원장
- G1 기대값의 봉인·승인 체계
- `identity`/`verify`: `go-treesitter`의 `tools/gen/pins.json` hash 검사가 대부분 대신한다

## 종료 상태

- **release:** v0.3.0 tag와 release를 그대로 둔다. 그 내용과 주장 범위는 [v0.3.0 release 문서](issue-220-v030-release.md)에 있다.
- **열린 PR과 이슈:** 종료 사유를 남기고 닫았다. 병합하지 않은 PR의 브랜치는 증거 링크가 가리키므로 지우지 않는다. 특히 B03·B04 G1 asset은 `feat/233-g1-3b`(PR #443)에만 있다.
- **진행 중이던 작업:** 중단한 시점과 상태는 각 이슈의 마지막 코멘트에 있다.
  - G1-3b2, #447 후속
  - #427 P4a와 모델 재측정
  - #441, #449
- **GitHub archive:** `go-treesitter`가 흡수를 마친 뒤 따로 확인을 받고 한다.
