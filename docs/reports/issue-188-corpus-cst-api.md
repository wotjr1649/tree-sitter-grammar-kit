# #188 corpus 후속 검증

기존 snapshot의 record-only 무결성 완료와 full CST/API 재검증을 구분하기 위해 개발용 `src/dev/large-api`에 로컬 corpus page mode를 추가한다. 기존 등록 workload와 제품 native/replay/qualification 정책은 유지한다. 소유 계약은 [tree/adapter](../specs/tree-and-adapter-protocol.md)의 로컬 corpus CST 페이지 검증이다.

원본 encoding의 완전한 CST 페이지를 저장하고 Go에서 canonical digest를 재계산한다. API audit는 모든 preorder node의 관계·child field·language field lookup과 0/EOF point를 검사한다. 페이지·source·producer·policy의 identity와 expected 집합을 결속하고, root 밖 caller pins를 이용하는 별도 replay가 저장 증거를 다시 검증한다. Error tree의 구조/API 검사가 성공해도 원문의 언어 문법 정상 판정과는 별개다.

공개 pilot은 C#/T-SQL/XML의 정상·오류 입력을 UTF-8·UTF-16LE·UTF-16BE·CP949로 관측하고 empty C# 및 50,000노드 초과 입력, directive-only SVC를 포함한다. 전용 page schema·producer route/host identity·총저장 accounting·resource gate를 반영한 실행 30사례에서 CST/API 26사례·56,373노드·27페이지·3,723,154 checks가 완료됐고, SVC 4사례의 API는 NOT_CLAIMED였다. Native 실행은 63,785 ms였으며 caller-pinned 저장 page의 source-bound canonical digest 재계산 replay도 PASS였다. 과거 pilot receipt는 별도로 보존하며 새 executable 결과로 표시하지 않는다. 기존 registered helper의 작은 full oracle·9개 fault·allocation control도 SMALL_CONTROLS/PASS, large_complete=false로 확인했다.

데이터-only Go 검사에는 canonical full tree와 page stream digest 일치, CST type/field/flag/point 변조, 잘못된 parent/range/catalog/row shape, source drift·hardlink·traversal·exact/one-byte-over read/storage, encoding/missing/duplicate member, page/binding 혼용·누락·중복·producer/policy·cleanup·navigation 변조 거부를 포함한다. 이 synthetic replay fixture는 native 실행 관측이 아니다.

현재 전체 실제 corpus 실행·최종 helper native pilot·exact-head 독립 리뷰·필수 CI·main 통합은 미완료다. 원문/목적 10개, Oracle/client 39개 및 .NET/WCF·SQL2012·Oracle runtime 환경 검증은 후속 순차 범위로 유지한다. 비공개 원문·경로·파일별 hash는 로컬에만 보존한다.
