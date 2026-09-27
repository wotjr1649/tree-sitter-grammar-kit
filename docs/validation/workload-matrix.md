# 기능별 검증 workload

실제 test는 해당 Session 구현과 함께 추가한다. 아래는 계획이며 PASS 기록이 아니다. 모든 Session은 [공통 gate](validation.md)를 적용하고 미지원 capability를 BLOCKED/UNSUPPORTED로 구분한다.

| Session / Issue | 정상·negative·mutant·경계 검증 | 플랫폼/완료 범위 |
|---|---|---|
| 00 / [#2](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/2) | 잘못된 module/경계·누락 문서·broken link·긴 AGENTS·tracked local artifact를 검출한다. source-only가 상위 Git을 탐색하지 않는지 확인한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 01 / [#3](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/3) | 오프라인·target 불변·같은 bytes의 결정성, 파일 변조/잘못된 expected identity/unknown layout/drive·UNC·traversal·link·size 초과를 검사한다. 언어별 하드코딩 mutant를 거부한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 02 / [#4](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/4) | 중복 key·큰 정수·NaN·case/device/ADS/trailing-dot 충돌·symlink/junction·ZIP duplicate/overlap/zip-bomb·정상 macOS root alias를 시험한다. 경계 직전/직후와 검증 삭제 mutant가 실패해야 한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 03 / [#5](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/5) | required/multiple 변경, missing/duplicate node, named/anonymous type, supertype 참조, key-order만 달라진 정상 예시와 planted structural difference를 검출한다. | 세 OS CGO-free offline/foundation; target 불변·결정성 |
| 04 / [#6](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/6) | source 불변, stale/poisoned cache, 다른 generator/옵션, nonzero exit, output flood, timeout/cancel, child/grandchild cleanup을 시험한다. 미지원 backend cap은 BLOCKED로 종료한다. | 세 OS 적용 가능한 capability별 검증; native/support 근거 없는 범위는 미지원 |
| 05 / [#7](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/7) | CRLF/NUL/BOM·잘못된 UTF-8·EOF·zero-width·byte boundary·다중 edit·취소·중간 malformed와 planted incremental mismatch를 시험한다. fresh final 한 번만 비교하는 mutant를 검출한다. | 세 OS 적용 가능한 capability별 검증; native/support 근거 없는 범위는 미지원 |
| 06 / [#8](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/8) | scanner 누락, ABI 불일치, 잘못된 state serialization, incomplete/null tree, ERROR 내부 구조, duplicate range/capture, child cleanup과 source identity mutant를 검출한다. | 세 OS 적용 가능한 capability별 검증; native/support 근거 없는 범위는 미지원 |
| 07 / [#9](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/9) | 숫자 경계·signed zero·max winner·threshold 근처·누락/중복 record·바뀐 comparator/policy/source를 대조한다. archive verifier 없이 재판정할 수 없는 데이터는 RECORDED_NOT_RECOMPUTED로 남긴다. | 세 OS 적용 가능한 capability별 검증; native/support 근거 없는 범위는 미지원 |
| 08 / [#10](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/10) | planted cross-platform difference, missing OS artifact, 다른 attempt/identity 혼합, scannerless와 scanner 경로, OS별 resource metric, normalization으로 차이를 숨기는 mutant를 검출한다. | 세 OS 적용 가능한 capability별 검증; native/support 근거 없는 범위는 미지원 |

작은 자체 scannerless와 stateful-scanner fixture를 독립·오프라인 회귀 기준으로 둔다. 외부 대형 grammar 전체나 verification ZIP을 testdata에 vendor하지 않는다. BrightScript stable identity와 historical replay, Cooklang audit 목적은 [provenance](../provenance/upstream-sources.md)에서 분리한다. malformed byte/JSON/archive fixture는 의도적인 negative 입력이며 문서 링크 검사 대상이 아니다.
