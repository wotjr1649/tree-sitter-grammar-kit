# 개발 campaign

전체 추적: [TSGK-C1 #1](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/1). Session 00의 완료와 PR13/15/19 후속 계약을 보존한다. [PREPARE #20](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/20)은 D1~D13 채택과 실제 준비 상태를 정합화하며 S01 앞에서 종료한다. Session 01~08은 PREPARATION_READY 및 별도 상위 실행 지시 후 순서대로 시작한다. 일반 개발의 진입 조건은 [문서 지도](README.md)를 따른다. campaign과 release는 별도 결정이다.

| Session | 범위 | 선행 | Issue | Milestone |
|---|---|---|---|---|
| 00 | Foundation / Architecture / Campaign Design | 없음 | [#2](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/2) | [1](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/1) |
| 01 | Inventory & Identity | PREPARE + 00 | [#3](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/3) | [2](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/2) |
| 02 | Strict Profile & Verification | 01 | [#4](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/4) | [3](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/3) |
| 03 | Schema Contract | 02 | [#5](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/5) | [4](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/4) |
| 04 | Reproducibility & Runner | 03 | [#6](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/6) | [5](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/5) |
| 05 | Incremental Verification | 04 | [#7](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/7) | [6](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/6) |
| 06 | Native Oracle & Tree Protocol | 05 | [#8](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/8) | [7](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/7) |
| 07 | Evidence & Replay | 06 | [#9](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/9) | [8](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/8) |
| 08 | Real-world & Cross-platform Qualification | 07 | [#10](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/10) | [9](https://github.com/wotjr1649/tree-sitter-grammar-kit/milestone/9) |

## Session 00

기반 설계와 실제 foundation 검사를 확정하고 후속 campaign을 준비한다.

완료 조건: src/와 root go.mod, CGO_ENABLED=0, 60개 비공백 줄 이내 AGENTS, PowerShell 7 전용, local 경로 비추적을 확인한다. stable/development reference를 분리하고 canonical 계약·negative foundation 검사·세 OS CI를 만든다. S00~S08 추적과 local prompt/hash manifest를 연결한다.

제외: 제품 명령·native 실행·후속 Session·tag/Release/package 구현 및 발행.

branch: `session/00-foundation-architecture`. 계약: [architecture.md](design/architecture.md).

## Session 01

inspect와 filesystem identity, CLI와 같은 코어를 쓰는 작은 공개 offline API를 구현한다.

완료 조건: zero-config discovery는 grammar.js를 데이터로만 읽는다. legacy/unknown layout과 누락 metadata는 관측으로 보고한다. 26개 route의 multi-grammar/shared source 선택을 조사하고 manifest r1의 path/role/mode/provenance/size/SHA-256와 E0를 출력한다. 최초 fingerprint를 출처 인증으로 부르지 않는다. 최소 path/link/size/cancellation guard와 직접 API 호출·checkout 밖 외부 module 시험을 포함한다.

제외: strict profile/archive extraction/schema/native 실행.

branch: `session/01-inventory-identity`. 계약: [identity-and-evidence.md](specs/identity-and-evidence.md).

## Session 02

strict profile과 trusted expected manifest 검증, bounded archive inspection을 구현한다.

완료 조건: duplicate key/type/version/unknown 필수 필드와 숫자 손실을 거부한다. exact-set verify에서 누락·추가·중복·변조를 구분한다. portable path/count/byte/depth 제한과 archive member 중복·탈출·link·폭탄을 거부한다. extraction은 새 caller-owned workspace가 필요한 경우만 별도 권한으로 구현한다.

제외: 임의 shell hook·자동 설치·native 실행·일반 sandbox.

branch: `session/02-profile-security`. 계약: [trust-and-execution.md](specs/trust-and-execution.md).

## Session 03

node-types 정적 구조와 diff를 구현한다.

완료 조건: node/field/type/required/multiple/supertype의 추가·제거·변경을 구분하고 최초 의미 예시를 고정한다. schema r0와 report를 연결하며 정적 변경을 breaking language claim으로 확대하지 않는다.

제외: CST runtime 동등성·언어 적합성·자동 SemVer 판정.

branch: `session/03-schema-contract`. 계약: [tree-and-adapter-protocol.md](specs/tree-and-adapter-protocol.md).

## Session 04

명시적 권한과 OS backend를 갖춘 pinned generation을 구현한다.

완료 조건: source snapshot에서 두 독립 workspace를 만들어 pinned CLI/JS dependency closure로 generation하고 기준 생성물·서로의 bytes를 비교한다. argv/cwd/env·timeout/output/process-tree cleanup과 capability preflight를 구현한다. native build/실행을 허용할 backend는 S05 전 adversarial 검증을 마친다.

제외: native oracle driver·Go adapter·자동 dependency install.

branch: `session/04-reproducibility`. 계약: [platform-support.md](specs/platform-support.md).

## Session 05

최소 native driver와 하나의 edit/comparison engine으로 각 edit 직후 incremental/fresh 결과를 비교한다.

완료 조건: S04의 native supervision을 재사용한다. 명시적 BUILD_NATIVE/EXEC_NATIVE 권한과 pinned runtime/header/compiler/parser/scanner closure를 결속한다. binary-safe request와 실제 ordered tree를 생성하고 이전 tree의 edit/전달 경로를 독립 검증한다. 모든 중간 상태·inverse repair·malformed·stateful-scanner·byte encoding을 비교한다. 26개 route의 등록 edit/feature case와 필수 세 OS 결과를 확인한다. Query는 S06 전까지 unsupported다.

제외: 임시 CLI-output parser 중복 구현·query 엔진·unsupported 필드 추정.

branch: `session/05-incremental`. 계약: [tree-and-adapter-protocol.md](specs/tree-and-adapter-protocol.md).

## Session 06

S05의 동일 driver/protocol을 query/API 관측과 native oracle 기록으로 확장한다.

완료 조건: runtime/parser/scanner/compiler/source closure/executable identity를 기록한다. ordered nodes·field/range/error/missing/query 순서와 duplicate node/capture를 보존한다. predicate/directive 지원 범위와 executed-empty/unsupported/not-run을 구분한다. 같은 S05 edit engine의 회귀와 26개 route의 세 OS 등록 case를 검증한다. 다른 runner/driver로 교체하지 않는다.

제외: CGO/FFI·go-treesitter 구현·임의 runtime bundle.

branch: `session/06-native-oracle`. 계약: [tree-and-adapter-protocol.md](specs/tree-and-adapter-protocol.md).

## Session 07

versioned evidence·raw 재판정·제한적 승계를 구현한다.

완료 조건: S01 E0의 execution_status/evidence_mode/assessment를 확장한다. 등록된 data-only reducer로 raw 소비 전수성·중복·누락·미사용·stale identity·승계 영향 범위를 검사한다. 선택한 BrightScript historical subset과 원 FAIL/raw를 보존하고 지원하지 않는 reducer를 replay PASS로 만들지 않는다. Python verifier 진단은 별도 EXEC_ADAPTER이며 offline product replay에 포함하지 않는다.

제외: 일반 임의 verifier 실행·전역 2 ULP/8875 상수·새 측정으로 relabel.

branch: `session/07-evidence-replay`. 계약: [identity-and-evidence.md](specs/identity-and-evidence.md).

## Session 08

26개 필수 구문 route 전체의 현재 후보 qualification을 세 OS에서 수행한다.

완료 조건: 승인된 legacy~stable feature/fixture/check를 가진 78개 route×OS 요약 칸과 maintained BrightScript/historical audit/owned fixture를 구분한다. exact source/input/profile/protocol/comparator/run attempt의 새 native 실행을 집계하고 semantic parity와 host 관측을 분리한다. historical/replay/다른 후보의 PASS는 대체 근거가 아니다. CLI grammar-update와 공개 API의 versioned external consumer 사용을 검증하고 실제 지원 한계를 문서화한다.

제외: 자동 tag/Release/package·Go adapter·추가 플랫폼 지원 추정.

branch: `session/08-real-world-qualification`. 계약: [platform-support.md](specs/platform-support.md).

S00 branch는 `session/00-foundation-architecture`의 역사적 기록이다. 이후 branch는 직전 merge/post-merge CI 확인 후 최신 main에서 하나씩 만든다. S01 E0 → S04 runner → S05 최소 native driver/edit → S06 동일 producer 확장 → S07 shared evidence 순서다. go-treesitter adapter와 tag/Release/package publication은 이 campaign 밖이다. [기계 정의](../src/contracts/campaign-01.json)가 branch/Issue/Milestone/acceptance ID를 연결한다.

공통 종료 절차는 [validation](validation/validation.md), 기능별 정상/negative/mutant acceptance는 [workload](validation/workload-matrix.md)가 소유한다. 로컬 prompt는 실행 자료이며 공개 acceptance의 유일한 근거가 아니다.
