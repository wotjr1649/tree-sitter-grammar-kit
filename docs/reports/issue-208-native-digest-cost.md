# #208 대형 native tree digest 비용 조사

[Issue #208](https://github.com/wotjr1649/tree-sitter-grammar-kit/issues/208)은 PR #207의 Windows 32 MiB C# oracle 시간 제한 실패를 추적한다. [실패한 CI](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/38598435159)는 원래 90초 wall·60초 parse·8 GiB 제한을 유지했으며, 실패를 성공으로 재분류하거나 무변경 재실행하지 않았다.

입력은 `cs-large-32mib-errors`의 33,554,432 bytes, SHA-256 `6409672e6b17cdf655cd975cc908992aabe8ad4ba4537d95323956de4bc7fa3e`다. CI의 oracle은 90,398 ms에 WALL_LIMIT, peak memory 5,168,136,192 bytes였다. 같은 CI의 일반 parse는 parse 52,292 ms·process 88,334 ms로 완료했고, 17,478,169 nodes를 순회했다. 일반 26개 route와 oracle 26개 route도 통과했다. 이는 대형 사례의 안정성이나 Go 문법 수정과의 인과관계를 증명하지 않는다. 해당 driver는 이전 main과 PR #207에서 동일했다.

`digest_node`는 node마다 숫자 포맷팅에 `snprintf`를 9번 호출했다. 이 입력에서는 157,303,521번이다. 수정은 기존 `puti`/`putu`와 24-byte stack buffer를 재사용한다. 최대 segment는 NUL·20자리 숫자·colon의 22 bytes로, heap allocation 없이 들어간다. parent는 root의 -1 또는 node index이고 좌표는 uint32다. [digest 계약](../specs/tree-and-adapter-protocol.md)의 NUL 구분자, decimal 길이, flag, 문자열, 좌표 순서와 LF는 그대로 유지한다. 일반 int64 formatter의 모든 입력에 대한 안전성을 주장하지 않는다.

2026-10-15 Windows/amd64, Go 1.27.2, GCC 16.2.0 및 CI와 같은 pinned runtime·C# grammar·입력에서 다음 로컬 결과를 관측했다.

| 관측 | 기존 구현 | 수정 후보 |
|---|---:|---:|
| 동일 32 MiB oracle native process wall (ms) | 81,801 | 61,089 |
| 해당 실행 parse (ms) | 44,470 | 37,714 |
| peak job committed memory (bytes) | 5,201,711,104 | 5,201,707,008 |
| real runtime·owned plain grammar, 1,000,000 node digest kernel, 3회 (ms) | 750 / 550 / 532 | 341 / 290 / 268 |

두 대형 실행은 원래 제한 아래 COMPLETED/PASS, fact reproduction PASS였다. node 수와 digest `4fb32d84e29573dd76f84511ff0552173b3ab5f8a95b455e3ff601d6d8eed38e`가 일치했다. kernel은 실제 Tree-sitter node를 사용했고, 숫자 경계의 NUL 포함 byte 비교·root parent·빈/UTF-8 field·32개 flag 조합·세 쌍의 digest가 일치했다. 이 kernel 측정은 전체 parser 성능 측정과 구분한다.

전체 wall 차이에는 parse 시간 차이와 환경 변동도 포함된다. 한 쌍의 대형 실행으로 일반적인 속도 향상률이나 CI timeout의 영구 해결을 확정하지 않는다. compiler bytes도 CI host와 로컬이 다르다. 로컬 원인 조사 근거이며 세 플랫폼 지원 판정을 새로 만들지 않는다.

`TestSummaryGate`는 기존 full/summary digest 동치에 더해 full 응답을 독립 Go `TreeDigest`로 검증하고 실제 process exit code를 전달한다. 전체 Go suite·native owned suite·vet·build, 두 STATIC context 리뷰, 새 필수 CI 및 실제 main 검증은 최종 통합 전 확인 대상이다. 오른쪽 reviewer의 STATIC 리뷰는 독립 실행이나 formal approval이 아니다. 원래 제한·입력·query·coverage·실패 전파는 유지한다. release/tag 발행과 중단된 실제 소비자 개발 재개는 이 변경에 포함하지 않는다.
