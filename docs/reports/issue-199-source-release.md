# 다음 source-only release 준비 (#199)

잠정 version은 `v0.3.0`이다. 제품 비교 기준은 v0.2.0과 actual main 33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8이며, 이 안내는 #199 task branch에서 준비 중이다. 최종 release candidate가 아니다. 최종 tag 대상 commit·발행일은 미확정이고 tag/Release/package를 발행하지 않았다. source 소비 검증은 각 후보의 개별 근거로 확인하며, mandatory CI·actual main 및 최종 release commit의 검증이 모여야 최종 안내로 사용할 수 있다.

## 제품 범위

VERIFIED offline core는 inspect·identity·verify·schema check/diff와 해당 공개 Go API다. corpus·reproduce·incremental·oracle record·replay·evidence verify·qualify 및 고급 native API는 experimental이며 호환성 약속이 없다. 등록 26 route×3host qualification SUPPORTED는 고정 입력·의무에 대한 판정이고 임의 입력의 문법 적합성 인증이 아니다. 새 release 판정은 최종 후보의 근거로 다시 고정한다. [소유 계약](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/specs/platform-support.md)

## v0.2.0 이후 주요 변경

- grammar/runtime 보완: C#/T-SQL token·조건부 fragment·context 회귀, SVC source/edit/segment/실패 증거 결속, runtime field ownership·visible alias·sibling navigation 대조를 보강했다. 개별 범위는 #118·#120·#122·#123·#128·#131·#137~#188의 보고서·등록 회귀가 소유한다. upstream과 동일하다고 주장하거나 역사적 v0.2.0의 제약 목록을 덮어쓰지 않는다.
- CLI 경로 안전(#196): descendant directory identity와 확인 실패를 반영해 input 내부 출력·신뢰 문서 별칭을 거부한다. 위치를 검증할 수 없으면 INPUT_PATH_UNVERIFIED/OUTPUT_PATH_UNVERIFIED로 실패한다. no-clobber를 유지한다. 완전한 TOCTOU sandbox나 미실행 Windows subst 실험을 의미하지 않는다.
- 선택적 logger 관측(#197): incremental --reuse-observation은 bounded 수집·관측 검증과 public pure Go validator src/kit/reuseobs를 제공한다. opt-in result r2/native r3이며 기본 result r1/native r1과 기존 oracle r2 경로를 유지한다. event 수는 unique node 수·reuse율·속도 향상·실행의 암호학적 인증이 아니다. 원 입력 witness가 없는 edit-point 검증은 해당 gate를 UNRESOLVED로 남긴다. [보고서](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/reports/issue-197-reuse-observation.md)
- 측정 근거(#193·#198): Windows의 한정된 성능 측정과 JSON 독립 비교, Go/C#/C/C++/T-SQL 82 language-case pair·168 SDK context observation·102 native CST 수집 결과를 문서화했다. case·context·tree 수를 하나의 정확도 분모로 합치지 않는다. Go identifier의 당시 false acceptance는 역사적 관측으로 유지한다. #205의 PR #207은 범주와 Unicode 17 문자 집합을 수정했으며 로컬 검증은 PASS다. PR 사전 통합 CI와 내부 증거 검증은 아래 run에서 PASS다. 실제 main 검증은 별도 gate이며 PR 결과만으로 승계하지 않는다. T-SQL 한 recovery 사례의 incremental route BLOCKED도 보존한다. [성능·비교 범위](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/reports/issue-193-evaluation.md), [holdout](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/reports/issue-198-core-language-holdout.md)

- Go 식별자(#205, PR #207): XID 대신 Go의 `L/_`·`L/Nd/_` 규칙과 Unicode 17 보충 범위를 적용했다. 고정 generator의 두 workspace 출력6개 일치와 등록 기준 재현, 생성 lexer 전체 집합 차이0, Windows native와 Go 1.27.2 SDK 대조를 확인했다. 기존 Go32와 별도 식별자16의 새 후보 FP/FN은 모두0이다. 이 로컬 결과는 새 세 플랫폼 qualification 또는 언어 전체 정확도가 아니다. [후보 보고서](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/reports/issue-205-go-identifiers.md)

- Native digest 처리 비용(#208, PR #207): 기존 정수 formatter를 재사용해 node마다 snprintf 9번 호출을 제거했다. 같은 고정 32 MiB C# 입력의 로컬 native wall은 기존81,801 ms·수정61,089 ms였으며 node 수·digest·보존 의미가 같았다. parse 시간도 달라 전체 parser의 일반 향상률을 의미하지 않는다. 기존90초 wall·60초 parse·8 GiB를 유지한다. 최초 PR #207 CI의 WALL_LIMIT 실패는 보존한다. 새 후보 PR CI의 고정 대용량 사례 검증은 아래 run에서 PASS이며 actual main과 최종 후보 검증은 별도 근거가 필요하다. [조사 보고서](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/reports/issue-208-native-digest-cost.md)

## API·CLI·schema 소비 시 확인

현재 정적 대조에서 공개 선언 제거는 없고 추가21·변경9개다. 변경에는 experimental struct7개의 필드8개 추가와 SvcFormat/SvcCompositeSchema의 r1→r2가 포함된다. 필드 추가는 unkeyed struct literal의 source compatibility에 영향을 줄 수 있다. 기존 필드의 type·JSON tag 제거/변경은 해당7개 struct 대조에서 없었다. 이는 전체 behavior/wire 호환성 보증이 아니다.

DeriveDynamicSQLChecked는 잘못된 capture 범위에 typed error를 반환한다. 기존 DeriveDynamicSQL의 잘못된 범위는 유효한 빈 fact처럼 보이지 않도록 Mapping이 빈 zero value다. ValidateTree는 root의 field 또는 빈 field를 거부한다. SVC r2와 legacy r1 소비 경계는 명시적으로 고정한다. [공개 API 계약](https://github.com/wotjr1649/tree-sitter-grammar-kit/blob/33f84fc32e0b6c92a2e41a5bdf11597f88c43ad8/docs/specs/public-go-api.md)

literal CLI flag의 추가는 reuse-observation 하나이고 제거는 없었다. 종료 코드 mapping과 최상위 usage는 같고 node-types schema implementation은 byte 동일하다. 모든 native/result/SVC schema가 동일하다는 의미는 아니다. module floor와 hosted CI 기준은 Go1.27.1이며 로컬 Go1.27.2 compatibility 검증은 별도 근거다. 과거 PR 후보 `77e3414`는 Go1.27.2·CGO_ENABLED=0·GOPROXY=off에서 추적 439파일(36,464,701 bytes)의 source export와 commit blob 일치, parent Git 탐색 차단, source-only foundation, 외부 공개 API/CLI 및 schema 소비, task-local install, PATH 없는 설치 CLI, 파일 module proxy 소비를 실제 통과했다. export의 LICENSE/NOTICE 4개도 기존 고정 bytes와 같다. 해당 결과는 그 과거 후보에만 결속한다. #208을 포함한 로컬 문서 후보 `1e127a8`에서도 별도의 443파일(36,491,431 bytes) source export·같은 소비 검사와 LICENSE/NOTICE 4개 byte 일치를 실제 확인했다. 이 결과는 `1e127a8`에만 결속하며 다음 문서 후보나 최종 release에 승계하지 않는다. 이 파일 proxy 시험은 실제 공개 tag 소비가 아니며 최종 release commit은 아직 미선정이다.

## 남은 준비와 발행 범위

#198의 PR #206 및 당시 실제 main `14d6b59` 증거는 PASS다. 입력 상한 설명과 #205·#208은 PR #207로 통합했다. PR #207 run `38616955169`/attempt 1/pull_request의 source `ba11bfe`, checkout `5cf8d3d`에서 12 job과 내부 26 route·78 cell·2478 obligation, 고정 large fixture 3개·fault control 9개·resource control 1개 및 별도 logger/SVC·diagnostic 근거가 PASS다. 실제 merge main `33f84fc`는 제품 tree가 같지만, 이 PR 근거는 main CI 결과가 아니다. #198·#205·#208의 완료는 actual main 검증 기록으로 별도 판정한다. #199 자체 PR·필수 CI·actual main, 최종 commit 고정과 해당 commit의 source 소비·API/CLI/schema·notices 검증도 필요하다. 이전 source 후보나 PR 성공을 최종 release 검증 완료로 승계하지 않는다. 성능의 새 host/새 toolchain·실사용 소비자·26-language 정확도는 측정하지 않은 범위로 남긴다. query host subset과 macOS sampled/NOT_CONTAINED 제약도 유지한다. C# 조건부 fragment 정책은 compiler symbol evaluator가 아니며, #198에서 확인한 지정 symbols·#undef·중첩 분기 문맥 차이는 미해결 지원 경계로 남는다.

예정 발행 범위는 source tag와 source-only GitHub Release이며 uploaded binary나 package registry 발행은 포함하지 않는다. exact version/commit/body를 확정한 검토 결과와 발행 권한이 필요하다. 소비자 개발 재개·go-treesitter source/CI 변경과 VM·entitlement·공유 인프라 구성은 별도 대상·효과다. 기존 v0.2.0 기록은 보존한다. 이 초안은 발행 승인 요청이나 발행 완료 증거가 아니다.
