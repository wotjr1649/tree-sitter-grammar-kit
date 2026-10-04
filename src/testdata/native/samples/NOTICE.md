# 실사용 sample 고지

이 파일은 qualification 요구 사례가 W 생산자로 등록한 공개 실사용 sample(`src/testdata/native/{routes,gaps,n461}/*.json` 사례의 `sample`)마다 그 저작권 고지와 license 고지를 싣는다. 비공개 corpus는 sample이 아니다.

항목 형식은 다음과 같고 `TestSampleNotices`가 정확히 대조한다.

- 제목 줄 `## <owner/name>@<commit>`(40자리 16진 commit)
- 그 뒤 비어 있지 않은 첫 줄 `Path: <저장소 안 경로>`
- 둘째 줄 `License: <SPDX id>`. SPDX id는 `MIT`, `Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `PostgreSQL` 중 하나다.
- 이어서 upstream 저작권·license 고지 원문

같은 commit의 sample이 여럿이면 경로마다 항목을 하나씩 둔다.
