# #182–#185 C# fragment와 corpus receipt 보완

이 변경은 `#if` group이 인자 목록의 일부, initializer와 선언 종료, if header, try/catch 경계를 감쌀 때 발생하는 grammar 오류를 수정한다. #178·#179에 이어지는 변경이며 #181의 추가 리뷰에서 발견한 #183 SVC source binding과 #184 scanner comment 검증과 #185 record envelope replay도 보완한다. Runtime는 추가 변경하지 않는다. 임의의 모든 전처리 조합이나 symbol evaluation을 구현한 것은 아니다. 소유 계약은 [언어 처분](../validation/language-feature-disposition.md), [tree/adapter](../specs/tree-and-adapter-protocol.md), [NET461 workload](../validation/net461-workload.md)다.

예를 들어 `F(\n#if A\n1,\n#endif\n2);`의 선택된 인자와 쉼표, `int x =\n#if A\n1;\n#else\n2;\n#endif`의 initializer와 종료자를 group 안에서 검증한다. `#if A\nif (F())\n#endif\nG();`는 조건부 header와 공통 body를 보존한다. `#if A\ntry {\n#endif\nG();\n#if A\n} catch (System.Exception e) {}\n#endif`는 두 경계와 공통 statement를 `preproc_try_statement`로 보존한다. Inactive ordinary text와 nested directive 검증은 #179 계약을 그대로 적용한다.

기존 `f3408dfb4cafbb966273b15a33354715e0e95ea5` grammar와 공개 합성 입력 29개를 대조했다. 정상 입력 거부 15개와 잘못된 입력 수용 3개를 확인했으며 나머지 11개는 이미 기대와 같았다. 새 grammar의 동일 29개는 모두 `PASS`였다. Missing comma·trailing comma·missing terminator·missing parenthesis·missing handler·잘못된 directive·missing endif를 정상으로 승격하지 않는다.

같은 공개 입력을 Roslyn 5.9에서 C# 7.3/14, empty symbols/defined `A`로 각각 파싱한 116개 관측을 보존했다. 정상 입력 19개는 모두 parse error가 없었다. 음성 대조군 10개는 `A` 정의 시 parse error가 있었으며 그중 3개는 empty symbols에서 오류 부분이 inactive여서 진단이 없었다. Native의 첫 non-literal-false section 선택 정책과 compiler의 실제 symbol 환경을 동일한 판정이라고 설명하지 않는다. Roslyn parsing은 type checking·빌드·실행 검증이 아니다.

공식 C# fixture에는 32개 control과 8개 두 단계 edit/restore history를 추가했다. SVC에는 5개 control과 4개 history를 추가해 directive 앞부분, inline source 좌표, 기존 declaration anchor, incremental/fresh 비교와 API 관측을 함께 검증한다. 기존 fixture와 기대값은 유지한다. 새 `if_header`와 `preproc_try_statement`, terminated declaration 범위를 anchor로 단언한다.

생성기는 고정 Tree-sitter 0.27.0, Node 24.21.0, ABI 15를 사용한다. 두 격리 작업 공간, 각각 wall 300초·hard memory 4 GiB·output 8 MiB·storage 512 MiB 한도를 유지한다. Bootstrap의 `REFERENCE_ABSENT`는 deterministic 출력 확보만을 의미하며 등록된 여섯 출력과 reference-bound 재생성을 별도로 확인한다. Literal patch, patched source와 여섯 생성물 identity, reproduction/native/qualification 등록부를 같은 변경에서 갱신한다. #181 minor M1의 설명도 ordinary inactive text와 nested group을 구분하도록 수정한다.

Reference-bound 재생성은 `COMPLETED`, `generator_ran`·`deterministic`·`reference_match`·`js_reproduction` 모두 `PASS`였고 finding은 없었다. 두 workspace에서 등록 출력 여섯 개가 모두 일치했다.

Windows/amd64 공식 `run-routes.ps1 -Routes csharp -Oracle`은 exit 0이었다. #182 첫 회귀는 C# S05 349개·S06 378개 모두 `PASS`, SVC 각각 60 `PASS`와 기존 VB 대조군 1 `BLOCKED`였다. #184를 포함한 최종 회귀는 C# S05 361개·S06 390개 모두 `PASS`, SVC 각각 69개 중 68개 `PASS`와 동일 `n461-svc-language-vb-n-r1`만 `SVC_INLINE_UNSUPPORTED`로 `BLOCKED`였다. 새 #182 C# 40개·SVC 9개와 #184 C# 12개·SVC 8개는 양쪽에서 모두 `PASS`였다. API finding·requirement result·kit failure는 모두 0이며 S06 record set은 둘 다 검증됐다. 추가 field/event/multiple-declarator 정상 대조군 3개도 Roslyn 두 version·두 symbol 환경의 12개 관측에서 parse error가 없었다.

#184의 active/inactive/nested/EOF 공개 반례 네 개는 수정 전 모두 `has_error=false`로 잘못 수용됐다. Scanner는 `#endif` 뒤 slash comment가 `//`인지 먼저 확인하고 delimited comment를 거부한다. 종료 newline/EOF 소유는 유지하며 다른 tail 오류의 기존 grammar recovery도 보존한다. 모든 tail을 scanner에서 거부한 중간 후보는 기존 garbage-tail history의 후속 declaration 기대를 깨뜨렸으므로 채택하지 않았다. Same-line/multiline delimited comment와 정상 comment 대조군 9개의 Roslyn 36개 관측도 기대와 일치했다. 근거는 [C# lexical specification 6.5.1](https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/language-specification/lexical-structure#651-general)의 directive comment 경계다.

#183은 native 완료와 r2 replay 완료를 분리한다. Helper는 source 길이·SHA-256을 대조해 해당 SVC case의 로컬 `svc_source`에 등록하고 link·입력·profile 크기 한도를 검사한다. Replay source guard와 r2 계약은 유지한다. 공개 합성 corpus의 C# 1개·SVC 2개는 helper에서 완료됐고 SVC source bytes·길이·hash가 실제 입력과 모두 일치했다. Source 누락은 `SVC_SOURCE_MISSING`, 치환은 `SVC_SOURCE_INVALID`로 거부됐다.

이 검증 과정에서 #185의 producer/replay 불일치를 추가 확인했다. Producer는 `form: record`에 `tsgk-tree-record/r1` envelope를 붙였지만 input identity가 비어 있었고 replay는 record+envelope를 거부했다. Producer에 실제 source/producer/policy를 결속하고 replay의 canonical record schema·source·digest/count 검사를 완성한다. Bare legacy record의 recorded-only 경계는 유지한다. R1/R2 reducer의 정상 및 schema/source/bytes/encoding/encoding-source/digest/count/producer 및 identity 누락·schema·중복의 24개 대조가 통과했다. Record의 CST digest를 재계산했다고 주장하지 않는다. 수정 뒤 실제 공개 helper output의 `native-result-r2` replay는 `PASS`였다. Source 누락은 `FAIL/SVC_SOURCE_MISSING`, 치환은 `INVALID_INPUT/SVC_SOURCE_INVALID`로 거부됐다. Helper의 source hash·short/long length·32 MiB input 상한 초과·base64 document 상한 초과 다섯 대조도 profile 쓰기 전에 중단됐다.

CGO_ENABLED=0에서 현재 후보의 모든 core 패키지와 owned native 검사를 수행했다. 최초 전체 검사에서는 scanner identity 갱신 뒤 qualification inventory가 이전 값인 문제만 실패했다. 등록부를 정상 생성 경로로 갱신하고 foundation 전체 재검사가 통과했다. 나머지 패키지, go vet·build·module verify·diff 검사는 통과했다. 한도를 완화하거나 실패를 삭제하지 않았다.

실제 source의 bounded 보완 관측도 로컬에서 수행했다. SVC 20개와 연결된 C# 20개는 source-bound profile에서 모두 PASS였고 모든 CodeBehind resolution은 PARSED였다. 첫 full-tree 결과가 S07 evidence-replay의 파일 16 MiB를 넘었으므로 묶음을 나누어 새 native 실행 후 네 묶음의 native-result-r2 replay가 모두 PASS였다. 큰 결과를 상한 밖에서 읽거나 기존 receipt를 나누어 새 실행으로 표시하지 않았다. 실제 source 13개·39개 상태의 finite edit/API 검사와 네 record set의 VerifyOracleSet도 PASS였다. SVC directive-only 소유 사례의 API는 NOT_CLAIMED이며 native tree/API가 있는 12개 사례와 구분한다. 실제 C# 세 결함 중 두 파일은 이 bounded edit 표본에 포함됐고, 큰 파일 하나는 기존 input/node 상한 밖이므로 원본 native parsing 관측으로만 확인했다. 전체 corpus의 API 동등성을 주장하지 않는다.

현재 후보 CI·독립 의미 리뷰·통합 후 검증 결과는 완료되는 대로 기록한다. 앞선 후보 CI를 이 변경의 CI로 승계하지 않는다. 비공개 corpus 원문·경로·파일별 identity는 이 문서에 싣지 않으며 source repair나 다른 SQL dialect 지원으로 이 변경의 범위를 넓히지 않는다.

현재 미실행: 새 후보 세 OS CI와 대용량 API, #182 독립 의미 리뷰, 통합과 post-merge 검증. 이 항목을 완료하기 전에는 integration 완료 또는 전체 corpus의 무조건적 지원을 선언하지 않는다.
