# Previous sibling identity와 inactive C# directive 보완 (#178·#179)

[#178](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/178)은 여러 hidden wrapper 안의 같은 값인 zero-width MISSING leaf를 이전 sibling 탐색이 다른 node로 오인하는 결함이다. 네 named leaf를 두 wrapper에 나눈 최소 tree에서 마지막 leaf의 previous sibling은 index2여야 하지만 index1이었다. Constructor로 만든 tree의 실제 public API 재현이며, 실제 C#/T-SQL source가 이 구조를 생성했다는 주장은 아니다.

Pinned runtime의 trailing-empty-descendant 탐색은 inline Subtree 값 대신 실제 child slot 주소와 `TSNode.id`를 비교한다. 기존 previous 탐색, next cursor 구현과 field/alias 경계를 유지한다. 세 번째 literal subject의 세 operation과 적용 후 83파일 closure를 manifest에 결속했다. Guard 시험은 정확한 operation 수, subject/input/output hash와 각 literal의 occurrence를 검사한다. Owned identity fixture는 flat·단일·복수·중첩 wrapper, named/anonymous, EOF와 positive tail에서 양방향 API 및 cross-tree identity를 단언한다. 기존 runtime은 새 단언에서 실패했고 후보는 이 시험과 field·named-navigation 시험을 통과했다.

[#179](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/179)은 inactive `#define 123`, unknown directive와 중복 nested `#else`가 오류 없이 opaque text에 포함되는 결함이다. Roslyn5.9의 CSharp7_3/CSharp14는 각각 CS1001, CS1024, CS1027을 관측했다. 기존 scanner는 중첩 depth만 세고 directive 인자와 순서를 검증하지 않았다.

Inactive ordinary text만 skipped token으로 남기고 directive는 기존 lexical production으로 검증한다. Nested group의 모든 본문은 계속 inactive이며 condition syntax와 alternative 순서는 grammar가 검증한다. `#endif`는 다른 directive처럼 physical newline/EOF를 검증하여 malformed tail 뒤 정상 declaration을 복구한다. Conditional CST는 종료 줄의 comment와 종료 문자까지 소유하며 기존 active anchor 세 개와 새 inactive nested/text anchor는 이 범위를 정확히 단언한다. 후속 declaration의 원본 범위는 보존한다. Symbol evaluation과 기존 branch 선택 정책은 변경하지 않았다.

C#에 invalid→valid→invalid history9개와 정상 대조4개를, SVC inline C#에 같은 결함의 history3개를 등록했다. Define/undef 인자, unknown directive, nested else/elif 순서·조건식, endif tail, line 인자와 trailing token을 포함한다. 기대 syntax, declaration 보존, fresh/incremental equality와 실제 API를 각각 검사한다. Compiler 대조62개 관측 중 inactive `#line abc`는 Roslyn이 진단을 생략하지만 사양의 lexical production은 이를 허용하지 않는다. 이 차이는 명세 기반 기대와 compiler 관측을 분리하며 새로운 compiler 동등성 claim으로 숨기지 않는다.

로컬 Windows에서 C# S05 305개, S06 334개가 모두 PASS이고 API findings와 requirement failures는 0이다. SVC S05/S06 각각50개 중49 PASS·의도된 VB 대조1 BLOCKED이며 동일하게 failures/API findings/requirement failures0이다. 고정 tree-sitter0.27.0·Node24.21.0의 두 workspace 재생성은 입력 불변성, determinism, 등록 출력6개 reference match와 js_reproduction을 모두 통과했다. Core CGO0 package 시험과 native 시험, foundation, module verify, vet, build를 실행했다. Foundation의 기존 single-operation fixture를 정확한 multi-operation fixture로 갱신해 runtime guard 회귀도 통과했다.

세 OS CI, 26-route qualification 및 전체 Windows large API는 최종 후보의 실제 artifact로 별도 검증해야 한다. 독립 context review와 실제 main integration/post-merge 결과는 로컬 검증으로 대체하지 않는다. 비공개 corpus 완료 판정은 종속 [#180](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/180)의 fresh snapshot·encoding·context·개별 처분과 로컬 receipt가 소유하며 이 공개 합성 결과만으로 완료 처리하지 않는다.

Private corpus 개발 helper도 canonical `SVC-SERVICEHOST-r2` profile을 사용한다. Batch의 directive 관측과 별도 등록 CodeBehind parse의 coverage는 구분하며, batch PASS만으로 참조 구현까지 검증했다고 판정하지 않는다.

임의 입력의 무결함, 실제 Framework4.6.1/SQL Server2012 host 실행, WCF/DB 실행 성공이나 일반 최악 탐색 성능은 주장하지 않는다. 비공개 원문·파일 이름·경로·개별 hash는 공개 자료에 포함하지 않는다.
