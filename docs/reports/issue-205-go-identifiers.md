# #205 Go 식별자 범주와 Unicode 17 정합성

이 보고서는 #205 수정 후보의 로컬 검증 결과다. 비교 대상은 Go 1.27.2 (`windows/amd64`, Unicode 17.0.0)의 syntax parser이며 type checking은 포함하지 않는다. 세 플랫폼 CI와 실제 main 통합은 아직 확인하지 않았다. [#198 보고서](issue-198-core-language-holdout.md)의 기존 관측은 당시 후보의 기록으로 유지한다.

## 원인과 수정

기존 Go grammar는 `XID_Start/XID_Continue`를 사용했다. Go의 식별자는 시작 `L/_`, 이후 `L/Nd/_`이므로 서로 다른 문자 집합이다. 또한 고정 tree-sitter 0.27.0 generator의 regex-syntax 0.8.11은 Unicode 16.0 표를 사용한다. 범주만 바꾸면 최신 Go 1.27의 Unicode 17 문자를 누락한다.

[공통 identifier C2 패치](../../src/dev/c2-patches/go.json)는 `L/Nd`와 Unicode 17 추가 범위를 결합한다. 전체 범위를 중복한 22,248-byte literal 대신 같은 집합을 표현하는 875-byte 규칙을 사용했다. type/field/package/label alias도 이 규칙을 사용한다. semicolon scanner, runtime, core API, Go module의 최소 버전 1.27.1은 변경하지 않았다. [Go B01 계약](../validation/language-feature-disposition.md#go)이 범위 소유자다.

Go [1.27.1](https://github.com/golang/go/blob/go1.27.1/src/unicode/tables.go)·[1.27.2](https://github.com/golang/go/blob/go1.27.2/src/unicode/tables.go)의 해당 tables와 설치된 SDK 표는 같은 Git blob으로 확인했다. 이전 모든 Go 버전의 Unicode 표가 같다는 뜻은 아니다. generator를 바꿀 때에는 보충 범위와 property 확장에 따른 과잉 수용을 포함해 전체 집합을 다시 대조해야 한다.

## 관측 결과

| 검사 | 로컬 결과 |
|---|---|
| Go Unicode 17 목표 predicate | numeric 0..10FFFF의 두 predicate 2,228,224회 대조 |
| 생성 lexer 시작 집합 | 145,673 code points / 685 intervals; 목표 대비 extra 0, missing 0 |
| 생성 lexer 연속 집합 | 146,443 code points / 737 intervals; 목표 대비 extra 0, missing 0 |
| tree-sitter 0.27.0 / Node 24.21.0 / ABI 15 | 두 작업 공간의 6개 출력 일치; 등록 기준 재현 PASS |
| Windows native | 기본 54개, query·API 포함 57개 사례 PASS; requirement/API finding 없음 |
| strict CST | 두 cohort 89개 기록, full tree 167개, fresh 비교 39쌍, 원문·CST 복원 11개, 음성대조 10개 PASS |
| SDK | 새 회귀 31개 source-step 관측과 기존 holdout 원본 32개: 63개 기대 판정 일치 |
| Go 1.27.2 / CGO_ENABLED=0 | 전체 `./src/...` suite, vet, build PASS |

최초 생성물 탐색은 output reference를 `ABSENT`로 명시했으므로 `BLOCKED`이며 generator·deterministic claim만 PASS다. 실제 6개 hash를 세 registry에 등록한 후 별도의 정상 재현이 reference-match·JS reproduction PASS를 확인했다. 생성된 C 파일을 직접 수정하지 않았다. qualification inventory는 기존 `TestQualificationInventory` writer로 재생성했으며 26 routes·78 cells를 유지한다.

| 고정 syntax 입력 집합 | 수정 전 TP/TN/FP/FN | 수정 후보 TP/TN/FP/FN |
|---|---|---|
| #198 원본 Go 32개 | 18 / 13 / 1 / 0 | 18 / 14 / 0 / 0 |
| 별도 #205 식별자 진단 16개 | 3 / 1 / 7 / 5 | 8 / 8 / 0 / 0 |

수용 판정은 완료된 full CST의 root `has_error=false`다. `ERROR`·missing node가 없다는 사실만으로 대체하지 않는다. 두 표의 입력을 독립적인 언어 전체 표본으로 취급하거나 전체 정확도 비율을 계산하지 않는다.

새 등록 회귀는 23개 원본이며 진단 16개 외에 U+200C, Greek·Hangul·CJK·Nd, Unicode 17 보충 범위의 시작·끝, type/field/package/label alias, BOM·CRLF·semicolon insertion, 네 번의 valid→invalid→restore를 포함한다. 새 23개 원본은 TP 14 / TN 9 / FP 0 / FN 0이다. native 수집 뒤 추가한 정상 tree 종류 기대 17개는 이미 수집한 해당 full CST와 별도로 대조했다. 로컬 native를 이 설명 보완만으로 재실행하지 않았으며 최종 CI는 등록된 기대값을 사용해야 한다.

## 한계와 종료 조건

SDK helper source 한도는 8,192 UTF-8 bytes, 직렬화 입력은 1 MiB, native operation의 per-case 입력 상한은 65,536 bytes다. 작은 고정 입력과 문자 집합 대조이며 상한 크기 입력, 모든 code point의 실제 program parse, 타 grammar, 처리량·메모리·대용량 성능을 측정한 결과가 아니다. Go SDK 1.27.1 실행이나 Linux·macOS native 실행을 로컬 결과에 합산하지 않았다.

처음 Go suite는 qualification inventory 미갱신에 따른 등록값 불일치와 격리 환경의 `git`·`pwsh` 경로 누락으로 실패했다. inventory와 PATH를 보완한 후 전체 suite·vet·build가 통과했다. 최초 native helper의 CLI build는 격리한 HOME에서 module cache를 찾지 못해 중단됐으며, 기존 고정 module cache를 지정한 다음 실행이 통과했다. 실패 기록을 보존했고 의존성을 새로 내려받거나 검사·한도를 완화하지 않았다.

종료에는 독립 context review의 finding 처리, 최종 후보의 세 플랫폼 mandatory CI, PR과 실제 merge SHA의 main CI·내부 증거 검증이 남아 있다. STATIC review는 별도 실행이나 GitHub의 formal approval을 대체하지 않는다. release 발행이나 소비자 개발 재개를 이 수정의 완료로 간주하지 않는다.
