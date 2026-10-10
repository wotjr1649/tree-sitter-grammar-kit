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

r2 replay는 등록 원본과 edit로 composite를 다시 관측한다. 비SVC profile의 composite, 원본 누락, step 순서 변경, 누락·중복·변조 identity와 참조 결과를 거부한다. r1 profile은 기존 context 없는 읽기 경로를 유지한다. native와 core의 분리, core의 CGO 비활성화, 기존 시험·resource 한도를 유지한다.

## 독립 대조와 검증

설치된 .NET Framework 4.8.9221.0의 activation assembly 4.0.0.0에서 `ServiceParser.ParseServiceDirective`를 reflection으로 호출했다. 실제 4.6.1 호스트 검증을 대신하지 않는다. [Microsoft reference source](https://github.com/microsoft/referencesource/blob/main/System.ServiceModel.Activation/System/ServiceModel/Activation/ServiceParser.cs)의 모든 directive 반복 탐색과 마지막 directive 뒤 source 추출을 확인했다. Assembly directive의 실제 assembly 로딩과 IIS 활성화는 실행하지 않았다.

공개 합성 directive 4개에서 token 삭제·삽입·치환 124건을 대조했다. 최초 5개 불일치 중 4개는 unquoted 값이 다음 attribute를 삼킨 lexical 과잉 수용으로 수정했다. 수정 뒤 123건이 일치한다. 나머지 `Language=?` 1건은 Framework의 compiler provider 조회 거부이며 lexer의 구문 오류로 만들지 않는다. unsupported inline 언어는 C# 성공으로 판정하지 않는다.

| 검사 | 현재 관측 |
|---|---|
| core `src/kit`, CLI 및 module-proxy 소비 | PASS |
| native package 전체 및 runtime field/navigation/API 음성 대조 | 초기 구현 PASS; 추가 claim/CP949 guard의 최종 native 재검증 대기 |
| foundation runtime 3-patch identity·준비 guard·문서/source 경계 | PASS; 보고서 등록 뒤 전체 최종 회귀는 후속 기록 |
| C#·T-SQL S05/S06, SVC 47개, 등록 large fixture | C# 292/321, T-SQL S05 993 PASS; 나머지 실행 중 |
| Windows full large API | 미실행 |
| 세 OS PR/main CI 및 qualification | 미실행 |

현재 보고서는 로컬 구현·검증 진행 상태다. 실제 route·large API·CI 완료 후 관측을 갱신한다. 임의의 모든 C#·T-SQL·SVC 입력이 완전하게 처리된다는 판정은 하지 않는다. 등록 비정상 입력의 기대 ERROR가 PASS인 것은 시험 성공이며 유효 프로그램이라는 뜻이 아니다.

C# S06 321건의 모든 incremental/fresh API에서 mandatory mismatch와 positional divergence는 0이다. 이전 large API small 관측의 4개 divergence는 모두 `next_sibling` 방향이었다. `prev_sibling`에는 기존 node identity/trailing empty descendant 처리가 있으며 대응 결함은 현재 입력에서 재현되지 않았다. 현재 producer는 positional divergence를 API FAIL로 처리하고 qualification은 같은 위조 PASS가 세 host에 있어도 거부한다. CP949 U+3000 prefix는 기존 decoder를 공유하는 `HasDirectivePrefix` guard로 검사한다. Framework와 kit는 marker 내부 U+FEFF를 모두 REJECTED로 관측했다.
