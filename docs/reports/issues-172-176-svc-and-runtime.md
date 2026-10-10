# #172–#176: SVC 구문·복구·참조와 runtime 탐색

대상은 `SVC-SERVICEHOST-r2`와 pinned Tree-sitter runtime의 sibling 탐색이다. C#·T-SQL grammar subject를 변경하지 않는다. `.svc` directive와 inline C#의 구문을 판정하고 caller가 명시한 CodeBehind C# 입력을 별도 파싱한다. 서비스 실행·타입 binding·IIS·compiler provider 설치는 검증 범위 밖이다.

## 변경과 원인

| Issue | 원인과 보완 |
|---|---|
| [#172](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/172) | `%>`를 문자열 안에서도 끝으로 취급하고 directive 이름·attribute 구문을 충분히 검증하지 않았다. Unicode 공백, 인용 값, 추가 directive, diagnostic 원본 span을 관측하고 directive와 inline tree의 구문을 합성한다. unquoted 값의 따옴표·등호·꺾쇠를 거부한다. |
| [#173](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/173) | 편집 중 한 step의 inline 경계가 사라지면 전체 이력의 native 관측을 잃었다. 파싱 가능한 연속 구간마다 native 실행을 보존하고 경계 복구 뒤 fresh tree로 재시작한다. gap을 가로지르는 reuse·equality를 주장하지 않는다. |
| [#174](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/174) | 기본 언어·CodeBehind 입력의 명시적 연결이 없었다. caller의 기본 언어와 provenance, 등록 case 연결을 사용한다. 참조 결과를 실제 별도 case와 대조하고 일반 C# 문자열의 directive 문구를 host 파일로 오인하지 않는다. |
| [#175](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/175) | SVC가 정상·비정상 구문을 검증하지 못한 관측 행에 머물렀다. 원본 bytes를 profile과 qualification inventory에 등록하고 47개 합성 case의 구문·복구·참조를 검증한다. 비C# inline 대조군은 BLOCKED를 유지한다. |
| [#176](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/176) | byte 위치로 형제를 찾으면 같은 위치의 zero-width node를 구별하지 못했다. cursor의 visible child에서 정확한 node identity를 찾아 다음 sibling을 반환한다. 실제 large API 후보에는 positional divergence 0을 요구한다. |

r2 replay는 등록 원본과 edit로 composite를 다시 관측한다. 비SVC profile의 composite, 원본 누락, step 순서 변경, 누락·중복·변조 identity와 참조 결과를 거부한다. composite inline tree는 같은 step의 full tree와 입력·CST·metadata 전체가 같아야 한다. full/summary/record form과 envelope의 일치를 요구하며, 두 full tree가 모두 null인 composite도 거부한다. API 활성 full tree에는 완전한 API 관측과 사례 claim을 요구하며, 누락과 첫 차이를 PASS로 숨기는 기록을 거부한다. r1 profile은 기존 context 없는 읽기 경로를 유지한다. native와 core의 분리, core의 CGO 비활성화, 기존 시험·resource 한도를 유지한다.

## 독립 대조와 검증

설치된 .NET Framework 4.8.9221.0의 activation assembly 4.0.0.0에서 `ServiceParser.ParseServiceDirective`를 reflection으로 호출했다. 실제 4.6.1 호스트 검증을 대신하지 않는다. [Microsoft reference source](https://github.com/microsoft/referencesource/blob/main/System.ServiceModel.Activation/System/ServiceModel/Activation/ServiceParser.cs)의 모든 directive 반복 탐색과 마지막 directive 뒤 source 추출을 확인했다. Assembly directive의 실제 assembly 로딩과 IIS 활성화는 실행하지 않았다.

공개 합성 directive 4개에서 token 삭제·삽입·치환 124건을 대조했다. 최초 5개 불일치 중 4개는 unquoted 값이 다음 attribute를 삼킨 lexical 과잉 수용으로 수정했다. 수정 뒤 123건이 일치한다. 나머지 `Language=?` 1건은 Framework의 compiler provider 조회 거부이며 lexer의 구문 오류로 만들지 않는다. unsupported inline 언어는 C# 성공으로 판정하지 않는다.

| 로컬 검사 | 관측 |
|---|---|
| core `src/kit`, CLI 및 module-proxy 소비 | PASS |
| foundation 전체, runtime 3-patch identity·준비 guard·문서/source 경계 | PASS |
| `go mod verify`, vet, CGO-free build | PASS |
| API 증거 필드·claim 누락/변조 및 SVC inline tree 전체 결합 음성 대조 | PASS |
| C# S05/S06 | 292/321 PASS |
| T-SQL S05/S06 | 993/1,000 PASS |
| SVC S05/S06 | 각각 47개: 46 PASS, 1 BLOCKED(등록 VB inline 대조군) |
| 등록 large fixture S05/S06 | 3개 PASS; summary 재현 일치, over-captures RESOURCE_LIMIT 대조 확인 |

full-node large API 및 세 OS의 실제 PR/main qualification은 [PR #177](https://github.com/wotjr1649/tree-sitter-grammar-kit/pull/177)의 실행 identity와 CI artifact를 최종 근거로 사용한다. 위 표는 로컬 검사 기록이다. 임의의 모든 C#·T-SQL·SVC 입력이 완전하게 처리된다는 판정은 하지 않는다. 등록 비정상 입력의 기대 ERROR가 PASS인 것은 시험 성공이며 유효 프로그램이라는 뜻이 아니다.

C# S06 321건의 모든 incremental/fresh API에서 mandatory mismatch와 positional divergence는 0이다. 이전 large API small 관측의 4개 divergence는 모두 `next_sibling` 방향이었다. `prev_sibling`에는 기존 node identity/trailing empty descendant 처리가 있으며 대응 결함은 현재 입력에서 재현되지 않았다. 현재 producer는 positional divergence를 API FAIL로 처리하고 qualification은 같은 위조 PASS가 세 host에 있어도 거부한다. CP949 U+3000 prefix는 기존 decoder를 공유하는 `HasDirectivePrefix` guard로 검사한다. Framework와 kit는 marker 내부 U+FEFF를 모두 REJECTED로 관측했다.

## PR 리뷰의 추가 보완

PR #177 자동 리뷰의 네 지적을 공개 합성 입력으로 재검증했다. 주석 prefix 자기 참조의 decoder 수용, 같은 step의 FAIL expectation이 뒤 PASS로 덮어쓰이는 현상, 실제 fault-control segment의 FAIL이 최종 PASS로 사라지는 현상은 수정 전 실패·수정 후 통과로 확인했다. replay도 gap 앞의 incremental 차이가 PASS로 승격되는 입력을 거부한다. parent 판정은 참조 결과를 연결한 뒤에도 유지하고 정상 expectation claim을 BLOCKED로 바꾸지 않는다.

sibling 탐색의 visible prefix 반복은 owned plain grammar의 27,001 node·54,002 sibling 호출에서 기존 2-patch 약 6ms, 최초 3-patch 약 397ms로 재현됐다. hidden chunk seek와 정확한 identity 역탐색 보완은 같은 조건에서 약 10ms, 49,501 node에서 약 19ms를 관측했다. 이는 GCC 16.2.0/O2 로컬 CPU 시간이며 일반적인 선형 시간 보장은 아니다. 별도 native fixture는 실제 runtime constructor로 같은 위치의 missing node 3개를 만들고 public sibling API에서 named/anonymous 혼합·hidden wrapper·EOF·서로 다른 tree의 identity를 검사한다. 실제 parser의 9,001-node flat source와 MISSING·extra 입력도 API 차이 0으로 검증한다.

추가 자동 리뷰의 두 연결 누락도 수정 전 시험 실패로 확인했다. CI route 집계는 최상위와 segment의 incremental claim을 같은 규칙으로 검사하고 오류 tree의 BLOCKED route는 별도 requirement로 기록한다. S06의 stdout 요약에 없는 segment는 상세 record에서 읽는다. observation-only r2 producer는 tree·process 없이 Oracle claim을 초기 NOT_CLAIMED로 명시하며 qualification은 API claim의 누락·PASS·BLOCKED 대체를 거부한다. Oracle의 후속 판정도 기존 composite FAIL/BLOCKED를 유지한다.

후속 리뷰에서 public qualification이 올바르게 FAIL/BLOCKED로 기록된 segment를 최상위 NOT_CLAIMED 때문에 놓치는 경로를 추가 확인했다. 세 구간 각각의 equality·route·query 실패, segment claim 누락·변조를 회귀시험으로 재현했다. replay의 기존 comparator 결과로 구간 claim을 재계산하고 qualification에 전달한다. 마지막 구간, 한 step 구간, 같은 step의 복수 기대값, gap 기대값과 directive diagnostic을 별도 검사하며 observation-only 기록의 unexpected segment도 거부한다.
