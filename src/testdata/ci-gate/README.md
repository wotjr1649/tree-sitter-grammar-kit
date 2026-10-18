# 과거 margin 호환성 fixture

`margin-main-39068412883-1.json`은 [Foundation actual main run 39068412883](https://github.com/wotjr1649/tree-sitter-grammar-kit/actions/runs/39068412883), attempt 1의 `ci-margin-39068412883-1` artifact에 있던 원본 `margin.json`이다. checkout은 `8db0db980a7a1d9d2933fb99f113b3795db34eda`다. 9개 configuration·17개 metric의 identity, source, key와 관측값을 재생성하거나 정규화하지 않았다.

2026-10-18에 공식 API artifact ID `12117268405`, ZIP 2,542 bytes와 SHA256 `c3aa532d31acdf2cbb6ae1a7f0fa346bd80da7723f51fece71a3263e93f262ce`를 보존 ZIP과 재대조했다. 단일 `margin.json` 12,995 bytes의 SHA256은 `24ccddb1f7df38cd7d31e1df672298b2be78ef99f17d67bd3d6aca9ec65e88f4`다.

이 고정 자료는 과거 Mac matrix job `native-routes`의 history 호환성 시험이다. 현재 후보의 `NEW_RUN`이나 새 구성의 표본 수를 대신하지 않는다. artifact 보존 기한 이후에도 이 fixture는 과거 관측 bytes의 회귀 입력으로만 사용한다.
