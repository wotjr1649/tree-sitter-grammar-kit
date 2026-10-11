# #188 corpus 후속 검증

기존 snapshot의 record-only 무결성 완료와 full CST/API 재검증을 구분하기 위해 개발용 `src/dev/large-api`에 로컬 corpus page mode를 추가한다. 기존 등록 workload와 제품 native/replay/qualification 정책은 유지한다. 소유 계약은 [tree/adapter](../specs/tree-and-adapter-protocol.md)의 로컬 corpus CST 페이지 검증이다.

원본 encoding의 완전한 CST 페이지를 저장하고 Go에서 canonical digest를 재계산한다. API audit는 모든 preorder node의 관계·child field·language field lookup과 0/EOF point를 검사한다. 페이지·source·producer·policy의 identity와 expected 집합을 결속하고, root 밖 caller pins를 이용하는 별도 replay가 저장 증거를 다시 검증한다. Error tree의 구조/API 검사가 성공해도 원문의 언어 문법 정상 판정과는 별개다.

공개 pilot은 C#/T-SQL/XML의 정상·오류 입력을 UTF-8·UTF-16LE·UTF-16BE·CP949로 관측하고 empty C# 및 50,000노드 초과 입력, directive-only SVC를 포함한다. helper source 기준 `f7ecbe9d3f17dea39fe9672307ec3cdb82e31810`의 실행 30사례에서 CST/API 26사례·56,373노드·27페이지·3,723,154 checks가 완료됐고, SVC 4사례의 API는 NOT_CLAIMED였다. Native 실행은 91,810 ms였으며 caller-pinned 저장 page의 source-bound canonical digest 재계산 replay도 PASS였다. 과거 pilot receipt는 별도로 보존하며 새 executable 결과로 표시하지 않는다. 기존 registered helper의 작은 full oracle·9개 fault·allocation control도 SMALL_CONTROLS/PASS, large_complete=false로 확인했다.

[Issue #190](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/190)의 Windows runner 수정 뒤 source `a16b2a4670d3b6654d8e3aa75c7d7d9ca5736963`로 동일 공개 pilot을 다시 실행했다. 30사례·CST/API 26사례·SVC NOT_CLAIMED 4사례·56,373노드·27페이지·3,723,154 checks와 저장 page replay PASS를 관측했으며 native 실행은 44,598 ms였다. 두 실행의 시간 차이를 일반 성능 개선으로 해석하지 않는다.

실제 corpus 실행에서는 Windows native root가 exit 0으로 종료하고 cleanup Verified=true여도 residual=1인 두 관측 때문에 각 실행을 중단했다. 해당 실패와 부분 증거는 보존하며 완료 집합에 넣지 않는다. #190은 `cmd.Wait`의 확인된 root 종료를 backend에 전달하고, Windows Job Object의 TotalProcesses=1·ActiveProcesses=1인 경우에만 종료된 단독 root를 running tree에서 제외한다. 실제 descendant history, query error, hard memory·wall·output 한도와 corpus의 residual=0 요구는 유지한다. 기존 두 실패가 이 accounting 조건 때문이었는지는 확정하지 않았다. 정상 root·실제 orphan·cancel/wall·query-error 검사와 CGO_ENABLED=0 core tests/vet/build, owned native tests가 PASS였고, 4파일 delta의 독립 정적 리뷰는 STATIC_REVIEW_NO_FINDINGS였다. Kernel accounting 예외를 실제 OS에서 결정적으로 재현했다는 주장은 하지 않는다.

데이터-only Go 검사에는 canonical full tree와 page stream digest 일치, CST type/field/flag/point 변조, 잘못된 parent/range/catalog/row shape, source drift·hardlink·traversal·exact/one-byte-over read/storage, encoding/missing/duplicate member, page/binding 혼용·누락·중복·producer/policy·cleanup·navigation 변조 거부를 포함한다. 이 synthetic replay fixture는 native 실행 관측이 아니다.

helper source 기준 `f7ecbe9`의 공개 tracked 범위에 대한 독립 정적 리뷰는 STATIC_REVIEW_NO_FINDINGS였다. 이는 reviewer의 native 실행, 비공개 corpus 검증이나 통합 승인을 뜻하지 않는다. 같은 head의 [CI run 38157253749 attempt 1](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38157253749)은 12개 job 모두 성공했다. 저자는 공개 artifact 9종의 archive 크기·SHA-256·run/head/checkout 결속과 내부 결과를 검증했다. 실제 CI checkout은 `04e38557e8aea8530c57294447043169c992bfd8`이며, 세 OS foundation/native routes·prepare·Windows registered large API·qualification·두 OS race·macOS regeneration 근거가 PASS였다. Qualification은 78개 cell·2,478개 의무 PASS, finding 0건이다. 이 CI 완료는 그 exact head의 관측이며 이후 문서 변경의 CI나 실제 corpus 완료로 승계하지 않는다.

실제 corpus의 고정 선택 집합 17,957개에 대한 로컬 CST/API 검증과 저장 증거 replay, 최종 원본 재대조 및 전체 완료 감사는 PASS였다. C# 6,624개·T-SQL 6,150개·XML 계열 5,163개·directive-only SVC 20개이며 원본 encoding은 UTF-8 14,517개·CP949 651개·UTF-16LE 2,789개였다. 실제 선택 집합에는 UTF-16BE 입력이 없으며 그 encoding은 공개 pilot에서 검증했다.

CST/API 대상은 17,937개·162,114,126노드·19,838페이지·11,042,001,187 checks다. SVC 20개는 전체 directive와 원본 gap을 재관측해 완료하고 API NOT_CLAIMED로 보존했다. 원본 구문 관측은 NO_ERROR 17,888개·ERROR 49개·SVC 별도 관측 20개이며, 구조/API PASS가 ERROR 49개를 정상 원문으로 바꾸지는 않는다. 사용자 채택 처분에 따라 49개는 원문 문제 10개와 Oracle/client 방언 지원 경계 39개로 남긴다.

전체 완료 집합은 실패 부분을 제외한 45개 완료 단위의 정확한 합집합이다. 수정 전 host의 완료 7,922개와 #190 수정 host의 완료 10,035개를 각각의 등록 identity로 검증했고, 두 source head 사이의 grammar/runtime/native/CST/API subject가 동일한 Git 객체임을 대조했다. 전체를 단일 executable의 관측으로 표시하지 않는다. 각 cohort의 실행/replay와 caller 등록·source/encoding/route/producer/policy·연속 page·cleanup/HARD memory 근거, 누락·중복·추가·identity 변경 음성 대조를 검증했다. 최종 원본 17,957개·1,274,719,349 bytes 재대조에서 missing/extra/changed/link 위반은 모두0이었다.

Pilot부터 중단·수정·재개와 최종 원본 대조까지 최초 시작 시각을 유지한 경과는 20,312.55초(약5시간39분)였고, 실패·부분·pilot을 보존한 corpus 증거는 7,853,833,422 bytes였다. 채택한 최대6시간·32GiB 안에서 완료했으며 단일 실행/native/process 한도는 유지했다. 전체 집합 감사도 PASS였고 원본 변경은0이다. 비공개 원문·경로·파일별 hash와 receipt는 로컬에만 보존한다.

원문 문제 10개는 원본을 보존한 별도 snapshot 17개로 비교했다. C# conflict 5개의 양쪽 후보12개, SQL 설명/결과/heading을 주석으로 보존한 후보4개, XML 앞 텍스트를 별도 메모로 보존한 후보1개다. 후보17개 모두 native full tree에서 NO_ERROR 및 API PASS였고 manifest/input/source/encoding/process/cleanup과 저장 record 무결성을 대조했다. 최초 실행 script의 존재하지 않는 --work 디렉터리는 CLI가 native 실행 전에 거부했으며 해당 로그를 보존한 뒤 원본 root 밖의 기존 caller-owned 디렉터리로 수정했다. 이 비교는 실제 원문 선택·내용 의도·애플리케이션 빌드 성공의 판정이 아니다. 소유자의 원문 의도는 UNRESOLVED이고 원본 변경은0이다.

이 보고 시점의 로컬 grammar 검증과 채택 처분은 완료됐으며, 최종 문서 변경의 독립 리뷰/CI와 main 통합·post-merge 확인은 별도 단계다. 앞선 exact-head CI·정적 리뷰를 새 head의 통합 근거로 소급 승계하지 않는다.

grammar kit의 완료 기준은 지원 계약 안의 구문 해석과 CST·API·증분 처리, 검증 근거의 무결성이다. kit 자체의 빌드·시험과 native parser 실행은 구현 검증에 필요하지만, 대상 애플리케이션의 전체 빌드·타입/참조 해결·DB 실행·WCF activation 성공은 grammar 완료 조건이 아니다. .NET/WCF·SQL2012·Oracle 환경의 읽기 전용 관측은 참고 근거로만 남기며 실제 실행 호환성을 주장하지 않는다. 구문 정상 판정이나 error tree의 구조/API PASS를 애플리케이션 실행 성공으로 확대하지 않는다.

Oracle/client 지원 경계 39개의 정적 사용 문맥은 원본 hash와 대조한 C# 154개 및 project XML 1개에서 확인했다. Project item은 Content 31개·None 8개이며 Oracle provider 참조가 있었다. 문자열·quoted identifier·comment를 마스킹한 제한된 lexical 대조에서 39개 모두 colon bind가 남았고 합계는 426회였다. 이 detector는 Oracle parser가 아니며 해당 SQL의 Oracle 문법 정상 여부를 판정하지 않는다. Provider·CommandText·실행 API와 파일 읽기 신호는 존재하지만 39개 파일명의 직접 literal 참조는 확보하지 못했다. 동적 loader에서 각 원본이 CommandText에 도달하는 경로와 실제 실행은 미입증이며, 이를 미사용 파일이라고 단정하지 않는다. 이번 처분은 사용자 채택 범위의 T-SQL 방언 지원 경계이며 Oracle grammar 지원 추가나 DB 실행 검증을 뜻하지 않는다.

읽기 전용 환경 관측에서는 .NET Framework release 533509·CLR file version 4.8.9345.0과 4.6.1 reference assemblies를 구분했고, LocalDB executable file version은 17.0.1000.7이었다. 이 정보는 실제 .NET 4.6.1 실행이나 SQL Server 2012 engine 실행 근거가 아니다. 조회한 범위에서 Oracle 도구·서비스 실행 환경을 확보하지 못했다. DB 접속, 서비스·설정 변경, SDK 설치 및 대상 application build는 수행하지 않았으며 grammar 완료 판정의 미충족 gate로 세지 않는다.
