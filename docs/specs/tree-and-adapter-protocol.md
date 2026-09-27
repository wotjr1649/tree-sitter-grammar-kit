# Tree와 adapter — draft/r0

S03의 정적 node-types 비교와 S06의 runtime CST는 다른 주장이다. schema 일치, ordered CST 일치, query 일치, 언어 사양 적합성은 서로 대체하지 않는다. 아래 형식은 experimental이며 실제 native/두 번째 grammar/consumer 검증 전에 안정 API로 고정하지 않는다.

## Ordered tree

envelope는 `schema: tsgk-tree/r0`, `input`(`bytes`, `sha256`, `encoding: bytes`), `status`, `capabilities`, `nodes`, `captures`와 producer/source/policy identity를 가진다. parse status는 COMPLETED/CANCELLED/RESOURCE_LIMIT/FAILED다. tree가 없으면 `nodes: null`이고 빈 성공 tree로 바꾸지 않는다. 완료된 tree는 최소 root 하나를 가진다.

nodes는 cursor preorder 배열이다. 각 항목은 `parent`(root -1, 나머지는 앞선 index), `type`(이름), `field`(부모 기준 이름 또는 null), `named`, `extra`, `is_error`, `has_error`, `is_missing`(bool), `start_byte`, `end_byte`, `start_point`, `end_point`를 가진다. point는 0-based `row`, byte 단위 `column`이다. 범위는 반열림 `[start,end)`이고 0≤start≤end≤input.bytes를 만족한다. 노드 배열과 sibling 순서, anonymous/extra, ERROR 내부 구조, zero-width·중복 범위를 보존한다. root start가 항상 0이라고 가정하지 않는다. numeric symbol/field ID를 runtime 간 의미 키로 쓰지 않는다.

captures는 query identity와 함께 원 producer의 ordered `match_index`, `pattern_index`, `capture_index`, `name`, `node_index`, byte/point range를 기록한다. duplicate capture와 같은 위치의 동률도 producer 순서를 보존한다. sorting/dedup/span 보정은 비교 전처리가 아니다. S06에서 Tree-sitter API ordering을 pinned version으로 검증해 comparator revision을 확정한다. runtime에 따라 순서를 보장할 수 없으면 해당 query comparison capability를 UNSUPPORTED로 둔다.

필수 비교 필드는 profile이 선택한다. adapter가 제공하지 못하는 필드는 null과 명시적 unsupported capability로 표시하고 false/0으로 채우지 않는다. strict profile 필드가 미지원이면 BLOCKED다. 부분 결과는 부분 관측만 주장한다. malformed 입력 bytes도 hash/길이를 원형대로 기록한다. edit offset/point 변환은 byte 기준이며 UTF-8 모드의 boundary 정책은 S05에서 fixture와 함께 고정한다.

## 실행 경계와 단계

native oracle은 `src/drivers/native-c/`의 generic C driver와 build-local language shim, 고정 runtime/parser/scanner source를 별도 executable로 build한다. ABI/header/compiler/options/source closure/executable hash를 모두 고정한다. [scanner 직렬화 계약](https://tree-sitter.github.io/tree-sitter/creating-parsers/4-external-scanners.html)은 stateful fixture의 필수 대조다.

미래 adapter transport는 versioned stdin/stdout JSON이다. process당 하나의 bounded request/response, stdout은 protocol만, stderr는 bounded 진단이다. handshake/request 필드·최대 frame·에러 code는 S06 Issue에서 native round-trip과 함께 확정하고 그 전 외부 adapter 호환성을 주장하지 않는다. Go adapter는 이번 campaign 밖이며 향후 `src/adapters/go-treesitter/` 독립 module/CI를 검토할 수 있다. 빈 module은 만들지 않는다.

S05는 pinned CLI adapter로 표현 가능한 incremental/fresh 결과부터 비교한다. S06 driver를 선행 의존성으로 삼지 않는다. S04에서 필요한 native supervision을 먼저 확보하고, S06에서 같은 edit engine을 common protocol에 연결해 회귀를 재실행한다.

참고한 go-treesitter driver는 ordered Parent/Named/Extra/Missing/Error/range를 제공하지만 node별 has_error 등 kit 필드를 그대로 충족한다고 가정하지 않는다. 원 record를 adapter에서 변환할 때 누락 정보와 consumer 특유 root 가정을 드러낸다. [고정 출처](../provenance/upstream-sources.md)
