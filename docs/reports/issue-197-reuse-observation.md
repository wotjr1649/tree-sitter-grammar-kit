# #197: 선택적 incremental logger 관측

[English](issue-197-reuse-observation.en.md) · [Issue #197](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/197)

이 변경은 개발 중인 opt-in 관측 기능이다. 현재 main의 지원 판정이나 release 완료를 뜻하지 않는다. 구현 계약은 [native protocol](../specs/tree-and-adapter-protocol.md), [CLI](../specs/cli-and-profile.md), [replay/qualification](../specs/identity-and-evidence.md), [검증](../validation/validation.md)이 소유한다.

공유 validator는 pure Go `src/kit/reuseobs`에 있으며 [공개 API 계약](../specs/public-go-api.md)에 경계를 명시한다. 기존 offline closure의 process·network·internal 실행 package 금지는 유지한다. opt-in 배치는 검증한 frame의 원 응답을 case별로 바로 저장하며 응답 전체를 누적하지 않는다. 저장 실패는 실행 성공으로 남기지 않는다.

기존 public `TSNode.id` intersection은 재사용된 공개 node identity를 센다. public logger의 accept/reject event는 별도 관측이다. 고정 JSON grammar의 `repair-close`, `break-close`, `repair-colon`에서 공개 route는 여전히 BLOCKED이고, logger accept event는 Windows 로컬 검사에서 각각 1·1·5회였다. old-tree를 생략한 대조군에서는 각각 0회이며 별도 관측 claim은 FAIL이었다. full incremental/fresh CST의 12개 공개 필드 비교는 유지했다. event 수는 고유 node 수·재사용률·속도 개선 비율이 아니다.

`incremental --reuse-observation`만 native r3와 result r2를 선택한다. 기본 native r1, 기존 result r1, oracle r2를 자동 변경하지 않는다. runtime source patch 없이 public `TSLogger`를 호출마다 설치·복원한다. 관측은 고정 runtime manifest와 검증된 build, run ID, case ID, history 시작 번호, parse 역할, 입력·직전 입력 hash에 결속한다. 같은 입력으로 SVC segment가 재시작해도 local step 0을 다른 segment의 관측으로 바꿔 붙이지 않는다.

완료 수집의 0과 logger 누락·미실행·unknown format·부분 수집·cap 중단·합성 overflow를 구분한다. unknown key, 빈 symbol payload, 종료 뒤 event, null string, 누락된 counter나 역할, 불가능한 byte prefix는 성공 관측으로 처리하지 않는다. OVERFLOW control은 counter 상태를 주입한 방어 경로 시험이다. 정상 bounded 수집은 event cap 때문에 64-bit overflow에 도달하지 않는다.

offline `native-result-r3`는 관측과 raw/projection을 재검증한다. 원 입력 witness를 명시적으로 등록하면 입력 버전과 edit point도 재계산하며, 원 입력 없이 hash만 있는 경우 point는 UNRESOLVED로 남긴다. CLI는 원문을 자동 복사하지 않는다. 현재 query/API qualification은 이 관측을 소비하지 않으므로 관측 필드가 섞인 기록을 명시적으로 거부한다. logger 수집 성공으로 기존 route·언어 기대값·case assessment·78칸 지원 판정을 승격하지 않는다.

현재 로컬 targeted 검사에는 정상/old-tree 생략, logger/형식 누락, event 순서, 세 cap, 합성 overflow, host 역할 누락, batch isolation, SVC 경계, JSON 세 사례, CLI→replay, source witness 및 point 동시 변조, qualification의 미지원 trace 거부가 포함된다. right pane의 독립 정적 리뷰 finding을 수정하고 해당 대조군을 실행했다. 정적 리뷰는 독립 실행 검증이나 GitHub 승인과 다르다. 전체 suite·vet·build, 최종 diff 리뷰, 새 세 플랫폼 후보 CI와 실제 main post-merge 근거를 마치기 전까지 #197은 완료하지 않는다. 이 후보의 logger 비용과 실사용 성능은 아직 측정하지 않았으며 [기존 평가](issue-193-evaluation.md)를 새 성능 측정으로 승계하지 않는다.
